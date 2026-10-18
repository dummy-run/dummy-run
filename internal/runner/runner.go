// Package runner plays one persona through one scenario: it drives the page
// with the model's tool calls, keeps the step and token budgets, applies the
// patience rule and writes the run record.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

const (
	// DefaultTokenBudget is the total of input and output tokens a run may spend.
	DefaultTokenBudget = 1_500_000
	// keepFull is how many of the newest observations the model sees in full.
	keepFull = 3
	// keepShots is how many of the newest observations carry a screenshot.
	keepShots       = 2
	maxOutputTokens = 8192
)

// Config is everything one run needs.
type Config struct {
	Persona     *cast.Persona
	Scenario    cast.Scenario
	Target      policy.Target
	Provider    provider.Provider
	Browser     string
	Headless    bool
	OutDir      string
	StepBudget  int
	TokenBudget int
	Batch       string
	Log         io.Writer
	Launch      bidi.LaunchOptions
}

// Result is what a run produced.
type Result struct {
	Record *record.Record
	Path   string
	Finish *Finish
	GaveUp *GaveUp
}

type entry struct {
	role      provider.Role
	text      string
	calls     []provider.ToolCall
	callID    string
	step      *Step
	stepsLeft int
	gaveUp    *GaveUp
}

type run struct {
	cfg      Config
	id       string
	dir      string
	trace    *telemetry.Trace
	page     *page.Page
	steps    []Step
	entries  []entry
	patience *Patience
	usage    record.Usage
	lastSig  uint64
	controls []page.Control
	budget   int
	tokens   int
	forced   string
	started  time.Time
	log      io.Writer
}

// Run plays the scenario and writes the run record. The record is written
// even when the run errors, with outcome errored.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	if err := cfg.Target.Validate(); err != nil {
		return nil, err
	}
	if cfg.Log == nil {
		cfg.Log = io.Discard
	}
	if cfg.TokenBudget <= 0 {
		cfg.TokenBudget = DefaultTokenBudget
	}
	budget := StepBudget(cfg.Scenario.Budget)
	if cfg.StepBudget > 0 {
		budget = min(cfg.StepBudget, MaxStepBudget)
	}
	now := time.Now().UTC()
	id := telemetry.NewRunID(now, cfg.Persona.ID, cfg.Scenario.ID)
	r := &run{cfg: cfg, id: id, trace: telemetry.NewTrace(id),
		patience: NewPatience(), budget: budget, tokens: cfg.TokenBudget, started: now, log: cfg.Log}
	r.dir = filepath.Join(cfg.OutDir, r.id)
	if err := os.MkdirAll(filepath.Join(r.dir, "screens"), 0o750); err != nil {
		return nil, err
	}
	fmt.Fprintf(r.log, "run %s: %s / %s on %s, %d steps, %s %s (%s)\n", r.id, cfg.Persona.ID, cfg.Scenario.ID,
		cfg.Browser, budget, cfg.Provider.Kind(), cfg.Provider.Model(), cfg.Provider.Region())

	finish, gaveUp, runErr := r.play(ctx)
	rec := r.record(finish, runErr)
	path, err := record.Write(cfg.OutDir, rec)
	if err != nil {
		return nil, fmt.Errorf("write run record: %w", err)
	}
	r.writeReport(finish, runErr)
	fmt.Fprintf(r.log, "run %s: %s, %d steps, %d calls, %d in / %d out tokens, %s\n", r.id, rec.Outcome, len(rec.Steps),
		rec.Usage.ModelCalls, rec.Usage.InputTokens, rec.Usage.OutputTokens, path)
	return &Result{Record: rec, Path: path, Finish: finish, GaveUp: gaveUp}, runErr
}

func (r *run) play(ctx context.Context) (finish *Finish, gaveUp *GaveUp, err error) {
	cfg := r.cfg
	size := cast.Devices[cfg.Persona.Device]
	hook := func(req page.Request) page.Decision {
		switch v := cfg.Target.Decide(req.Method, req.URL); v {
		case policy.Allow:
			return page.Decision{Set: telemetry.Headers(req.Headers, r.id, cfg.Persona.ID, r.trace.Traceparent())}
		default:
			return page.Decision{Block: true, Reason: v.String()}
		}
	}
	p, err := page.Open(ctx, page.Options{Browser: cfg.Browser, Headless: cfg.Headless, Width: size[0], Height: size[1],
		RequestHook: hook, Launch: cfg.Launch})
	if err != nil {
		return nil, nil, fmt.Errorf("open browser: %w", err)
	}
	defer p.Close()
	r.page = p
	if err := p.SetUserAgent(ctx, telemetry.UserAgent(p.Browser().UserAgent)); err != nil {
		return nil, nil, fmt.Errorf("set user agent: %w", err)
	}

	first := r.execute(ctx, provider.ToolCall{Name: "visit", Args: json.RawMessage(`{"url":""}`)})
	r.entries = append(r.entries, entry{role: provider.RoleUser, step: first,
		text: "Begin. This is what you see now.\n\n"})

	system := r.systemPrompt()
	maxCalls := r.budget + 10
	for calls := 0; calls < maxCalls; calls++ {
		if err := ctx.Err(); err != nil {
			return nil, r.patience.GaveUp(), err
		}
		force := ""
		switch {
		case len(r.steps) >= r.budget:
			force, r.forced = "finish", "the step budget"
		case r.patience.Spent():
			force, r.forced = "finish", "patience"
		case r.usage.InputTokens+r.usage.OutputTokens >= r.tokens:
			force, r.forced = "finish", "the token budget"
		}
		callStart := time.Now()
		resp, err := cfg.Provider.Chat(ctx, provider.Request{System: system, Turns: r.turns(), Tools: Tools,
			ForceTool: force, MaxOutputTokens: maxOutputTokens})
		r.usage.ModelCalls++
		fmt.Fprintf(r.log, "  model call %d: %.1fs, %d in / %d out tokens%s\n", r.usage.ModelCalls, time.Since(callStart).Seconds(),
			resp.Usage.InputTokens, resp.Usage.OutputTokens, errNote(err))
		r.usage.InputTokens += resp.Usage.InputTokens
		r.usage.OutputTokens += resp.Usage.OutputTokens
		if err != nil {
			return nil, r.patience.GaveUp(), fmt.Errorf("model call %d: %w", r.usage.ModelCalls, err)
		}
		if resp.Note != "" {
			fmt.Fprintf(r.log, "provider: %s\n", resp.Note)
		}
		r.entries = append(r.entries, entry{role: provider.RoleAssistant, text: resp.Text, calls: resp.ToolCalls})
		if len(resp.ToolCalls) == 0 {
			r.entries = append(r.entries, entry{role: provider.RoleUser, text: "Use one of the tools, or call finish."})
			continue
		}
		for i, call := range resp.ToolCalls {
			if i > 0 {
				r.entries = append(r.entries, entry{role: provider.RoleTool, callID: call.ID,
					text: "Not run: one action per turn. Call it again if you still need it."})
				continue
			}
			if call.Name == "finish" {
				f, perr := ParseFinish(call.Args, len(r.steps))
				if perr != nil {
					r.entries = append(r.entries, entry{role: provider.RoleTool, callID: call.ID,
						text: "finish was not understood: " + perr.Error() + ". Call finish again with verdict, report and findings."})
					continue
				}
				r.finishStep(ctx, f)
				return &f, r.patience.GaveUp(), nil
			}
			step := r.execute(ctx, call)
			r.entries = append(r.entries, entry{role: provider.RoleTool, callID: call.ID, step: step,
				stepsLeft: r.budget - len(r.steps), gaveUp: r.patience.GaveUp()})
		}
	}
	return nil, r.patience.GaveUp(), errors.New("the model made too many calls without finishing")
}

// execute runs one tool and records the step.
func (r *run) execute(ctx context.Context, call provider.ToolCall) *Step {
	args := map[string]any{}
	if len(call.Args) > 0 {
		_ = json.Unmarshal(call.Args, &args)
	}
	get := func(k string) string {
		v, _ := args[k].(string)
		return strings.TrimSpace(v)
	}
	controlsBefore := r.controls
	r.trace.SetStep(len(r.steps) + 1)
	startedAt := time.Now().UTC()
	var obs page.Observation
	switch call.Name {
	case "look":
		obs = r.page.Look(ctx)
	case "screenshot":
		obs = r.page.Screenshot(ctx)
	case "visit":
		target, verr := r.resolveURL(get("url"))
		if verr != nil {
			obs = r.page.Look(ctx)
			obs.Action, obs.Target, obs.Error = "visit", get("url"), verr.Error()
		} else {
			obs = r.page.Visit(ctx, target)
		}
	case "click":
		obs = r.page.Click(ctx, get("target"))
	case "fill":
		obs = r.page.Fill(ctx, get("target"), get("value"))
	case "type":
		obs = r.page.Type(ctx, get("target"), get("text"))
	case "select":
		obs = r.page.Select(ctx, get("target"), get("option"))
	case "press":
		obs = r.page.Press(ctx, get("key"))
	case "back":
		obs = r.page.Back(ctx)
	case "scroll":
		dir := get("direction")
		if dir != "up" {
			dir = "down"
		}
		obs = r.page.Scroll(ctx, dir)
	case "wait":
		secs, _ := args["seconds"].(float64)
		if secs <= 0 {
			secs = 2
		}
		obs = r.page.Wait(ctx, int(secs))
	case "a11y":
		obs = r.page.A11y(ctx)
	default:
		obs = r.page.Look(ctx)
		obs.Action, obs.Error = "look", fmt.Sprintf("unknown tool %q", call.Name)
	}
	sig := Signature(obs)
	step := &Step{N: len(r.steps) + 1, At: startedAt, Obs: obs, PageChanged: sig != r.lastSig}
	r.lastSig = sig
	r.patience.Note(step.N, obs.URL, obs.Action, obs.Target, controlsBefore, obs.Error != "" || !step.PageChanged)
	if obs.Snapshot != nil {
		r.controls = obs.Snapshot.Controls
	}
	r.steps = append(r.steps, *step)
	r.saveStep(step)
	fmt.Fprintf(r.log, "  step %d: %s %s -> %s%s\n", step.N, obs.Action, obs.Target, shorten(obs.URL, 80), errSuffix(obs))
	return step
}

func errNote(err error) string {
	if err == nil {
		return ""
	}
	return " (error: " + err.Error() + ")"
}

func errSuffix(o page.Observation) string {
	var parts []string
	if o.Error != "" {
		parts = append(parts, "error: "+o.Error)
	}
	if len(o.Blocked) > 0 {
		parts = append(parts, fmt.Sprintf("%d blocked", len(o.Blocked)))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// resolveURL turns the persona's address into an absolute URL on the target
// and refuses hosts outside the allow list before the browser is involved.
func (r *run) resolveURL(raw string) (string, error) {
	base, _ := url.Parse(r.cfg.Target.BaseURL)
	if raw == "" {
		return base.String(), nil
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("bad address %q", raw)
	}
	u := base.ResolveReference(ref)
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("address %q is not a web address", raw)
	}
	if !r.cfg.Target.HostAllowed(u.Hostname()) {
		return "", fmt.Errorf("%s is outside this run's target (%s); stay on the target site", u.Hostname(), strings.Join(r.cfg.Target.AllowedHosts, ", "))
	}
	return u.String(), nil
}

func (r *run) finishStep(ctx context.Context, f Finish) {
	r.trace.SetStep(len(r.steps) + 1)
	startedAt := time.Now().UTC()
	obs := r.page.Look(ctx)
	obs.Action, obs.Target = "finish", f.Verdict
	r.steps = append(r.steps, Step{N: len(r.steps) + 1, At: startedAt, Obs: obs, PageChanged: false})
	r.saveStep(&r.steps[len(r.steps)-1])
	fmt.Fprintf(r.log, "  step %d: finish %s, %d findings\n", len(r.steps), f.Verdict, len(f.Findings))
}

// saveStep writes the screenshot and the full observation line for the reviewer.
func (r *run) saveStep(s *Step) {
	if len(s.Obs.Screenshot) > 0 {
		name := fmt.Sprintf("%03d.png", s.N)
		_ = os.WriteFile(filepath.Join(r.dir, "screens", name), s.Obs.Screenshot, 0o600)
	}
	line := map[string]any{
		"step": s.N, "at": s.At.Format(time.RFC3339Nano), "action": s.Obs.Action, "target": s.Obs.Target,
		"ms": s.Obs.Duration.Milliseconds(), "error": s.Obs.Error, "url": s.Obs.URL, "status": s.Obs.Status,
		"page_changed": s.PageChanged, "page": s.Obs.Snapshot,
		"problems": map[string]any{"console_errors": s.Obs.ConsoleErrors, "failed_requests": s.Obs.FailedRequests,
			"blocked_requests": s.Obs.Blocked, "accessibility": s.Obs.A11y},
		"screenshot": fmt.Sprintf("screens/%03d.png", s.N),
	}
	data, err := json.Marshal(line)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(r.dir, "steps.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
	_ = f.Close()
}

// turns builds the conversation for the next model call: the newest
// observations in full with screenshots, older ones as one line each.
func (r *run) turns() []provider.Turn {
	var obsIdx []int
	for i, e := range r.entries {
		if e.step != nil {
			obsIdx = append(obsIdx, i)
		}
	}
	full := map[int]bool{}
	shot := map[int]bool{}
	for k, i := range obsIdx {
		if k >= len(obsIdx)-keepFull {
			full[i] = true
		}
		if k >= len(obsIdx)-keepShots {
			shot[i] = true
		}
	}
	var turns []provider.Turn
	for i, e := range r.entries {
		switch e.role {
		case provider.RoleUser:
			t := provider.Turn{Role: provider.RoleUser, Text: e.text}
			if e.step != nil {
				t.Text += r.observationText(e, full[i])
				if shot[i] && len(e.step.Obs.Screenshot) > 0 {
					t.Images = []provider.Image{{PNG: e.step.Obs.Screenshot}}
				}
			}
			turns = append(turns, t)
		case provider.RoleAssistant:
			turns = append(turns, provider.Turn{Role: provider.RoleAssistant, Text: e.text, ToolCalls: e.calls})
		case provider.RoleTool:
			text := e.text
			if e.step != nil {
				text = r.observationText(e, full[i])
			}
			turns = append(turns, provider.Turn{Role: provider.RoleTool, ToolCallID: e.callID, Text: text})
			if e.step != nil && shot[i] && len(e.step.Obs.Screenshot) > 0 {
				turns = append(turns, provider.Turn{Role: provider.RoleUser,
					Text:   fmt.Sprintf("Screenshot after step %d.", e.step.N),
					Images: []provider.Image{{PNG: e.step.Obs.Screenshot}}})
			}
		}
	}
	return turns
}

func (r *run) observationText(e entry, full bool) string {
	if !full {
		return Summary(*e.step)
	}
	text := Render(*e.step)
	if e.role == provider.RoleUser {
		return text
	}
	if e.gaveUp != nil {
		return text + fmt.Sprintf("\n\nYou have tried %s %d times and it has not worked. A real user gives up here, so the run ends: "+
			"your next call must be finish. Say where you got stuck, citing the steps. The verdict is fail if the site stopped you "+
			"and blocked if the set-up did, never pass unless you had already reached the goal.", e.gaveUp.Action, e.gaveUp.Tries)
	}
	text += fmt.Sprintf("\n\nActions left: %d.", e.stepsLeft)
	switch {
	case e.stepsLeft <= 1:
		text += " Your next call must be finish. If you have not reached the goal, say the budget ran out; the verdict is then blocked (or fail if the site stopped you), never pass."
	case e.stepsLeft <= 3:
		text += " Finish now if you have what you need: call finish with your full report."
	}
	return text
}

func (r *run) systemPrompt() string {
	t := r.cfg.Target
	var b strings.Builder
	b.WriteString(strings.TrimSpace(r.cfg.Persona.Prompt))
	b.WriteString("\n\n# Your scenario for this run\n\n")
	b.WriteString(strings.TrimSpace(r.cfg.Scenario.Body))
	b.WriteString("\n\n# Rules for every run\n\n")
	fmt.Fprintf(&b, "- You are visiting %s. The browser only reaches %s; requests anywhere else fail, so do not try.\n", t.BaseURL, strings.Join(t.AllowedHosts, " and "))
	if t.ReadOnly {
		b.WriteString("- This target is read-only. The browser refuses any request that could change it (POST, PUT, PATCH, DELETE), so a form, sign-up, log-in, search box that posts or chat widget will not work. Do not fill in or submit forms and do not type into chat widgets. If your scenario would need that, note it in the report and move on.\n")
	}
	fmt.Fprintf(&b, "- Patience: you have %d actions. After three failed tries at the same thing, give up and report where you got stuck, as a real user would.\n", r.budget)
	b.WriteString("- Look, then act. Use the numbered controls (@12) from the latest result. Type an address only when the scenario says to.\n")
	b.WriteString("- Evidence or it did not happen: every finding names the step numbers that show it.\n")
	b.WriteString("- Likely, not certain: write \"possible bug\" and \"I think\". A reviewer and a person decide.\n")
	b.WriteString("- Never copy a person's name, email address or other personal data from the site into your report. Describe what you saw.\n")
	b.WriteString("- Stop on harm: anything that could reach a real person (email, payment, someone else's account) ends the run with verdict blocked.\n")
	b.WriteString("\n# How this run works\n\n")
	b.WriteString("You drive the browser through tools, one action per call: look, visit, click, fill, type, select, press, back, scroll, wait, a11y, screenshot, finish. " +
		"After each call you get the page as text with numbered controls, and a screenshot for the newest steps; older steps are shortened to one line. " +
		"\"Actions left\" is your budget. If the page did not change after an action, the result says so: do not repeat it, try something else or give up. " +
		"Step numbers in your report are the harness's own (\"step N\" in each result). When you are done, or have given up, call finish with your verdict, " +
		"your report and your findings. Each finding is one problem or question with its kind, severity, title, the steps that show it and whether you measured it or inferred it.\n")
	return b.String()
}

func (r *run) record(finish *Finish, runErr error) *record.Record {
	cfg := r.cfg
	rec := &record.Record{
		SchemaVersion: 1, ID: r.id, Batch: cfg.Batch, StartedAt: r.started, EndedAt: time.Now().UTC(),
		Persona:  record.Persona{ID: cfg.Persona.ID, Role: cfg.Persona.Role, Device: cfg.Persona.Device},
		Scenario: record.Scenario{ID: cfg.Scenario.ID, Title: cfg.Scenario.Title},
		Target:   record.Target{Adapter: "url", Environment: cfg.Target.Environment, BaseURL: cfg.Target.BaseURL, Commit: cfg.Target.Commit},
		Provider: record.Provider{Kind: cfg.Provider.Kind(), Model: cfg.Provider.Model(), Region: cfg.Provider.Region()},
		Browser:  record.Browser{Name: cfg.Browser, Version: "unknown"},
		Steps:    []record.Step{}, Findings: []record.Finding{}, Usage: r.usage,
	}
	if r.page != nil && r.page.Browser().Version != "" {
		rec.Browser.Version = r.page.Browser().Version
	}
	for _, s := range r.steps {
		st := record.Step{N: s.N, At: s.At, Action: s.Obs.Action, Target: s.Obs.Target, URL: s.Obs.URL,
			DurationMs: int(s.Obs.Duration.Milliseconds()), ConsoleErrors: record.Int(len(s.Obs.ConsoleErrors))}
		if len(s.Obs.Screenshot) > 0 {
			st.Screenshot = fmt.Sprintf("%s/screens/%03d.png", r.id, s.N)
		}
		if s.Obs.Status > 0 {
			st.Status = record.Int(s.Obs.Status)
		}
		if s.Obs.A11y != nil {
			st.A11yViolations = record.Int(len(s.Obs.A11y))
		}
		var notes []string
		if s.Obs.Error != "" {
			notes = append(notes, "error: "+s.Obs.Error)
		}
		for _, bl := range s.Obs.Blocked {
			notes = append(notes, "blocked: "+bl)
		}
		if s.Obs.Action == "finish" {
			notes = append(notes, "verdict: "+s.Obs.Target)
			st.Target = ""
		}
		st.Note = strings.Join(notes, "; ")
		rec.Steps = append(rec.Steps, st)
	}
	rec.Outcome = outcome(finish, runErr, r.forced, r.patience.GaveUp())
	if finish != nil {
		rec.Findings = RecordFindings(*finish)
	}
	return rec
}

// outcome maps how the run ended onto the schema's four outcomes.
func outcome(finish *Finish, runErr error, forced string, gaveUp *GaveUp) string {
	switch {
	case runErr != nil:
		return record.OutcomeErrored
	case finish == nil:
		return record.OutcomeBudgetExhausted
	case finish.Verdict == "pass":
		return record.OutcomeCompleted
	case forced == "the step budget" || forced == "the token budget":
		return record.OutcomeBudgetExhausted
	case finish.Verdict == "blocked" || gaveUp != nil:
		return record.OutcomeBlocked
	default:
		return record.OutcomeCompleted
	}
}

func (r *run) writeReport(finish *Finish, runErr error) {
	var b strings.Builder
	switch {
	case finish != nil:
		fmt.Fprintf(&b, "## Verdict\n\n%s\n\n%s\n", finish.Verdict, strings.TrimSpace(finish.Report))
	case runErr != nil:
		fmt.Fprintf(&b, "## Verdict\n\nerrored\n\nThe harness failed: %s\n", runErr.Error())
	default:
		b.WriteString("## Verdict\n\nbudget_exhausted\n\nThe run ended without a report.\n")
	}
	b.WriteString("\n## Problems the harness logged\n\n")
	n := 0
	for _, s := range r.steps {
		if len(s.Obs.ConsoleErrors) > 0 {
			fmt.Fprintf(&b, "- step %d: console errors: %s\n", s.N, strings.Join(s.Obs.ConsoleErrors, "; "))
			n++
		}
		if len(s.Obs.FailedRequests) > 0 {
			fmt.Fprintf(&b, "- step %d: failed requests: %s\n", s.N, strings.Join(s.Obs.FailedRequests, "; "))
			n++
		}
		if len(s.Obs.Blocked) > 0 {
			fmt.Fprintf(&b, "- step %d: blocked by policy: %s\n", s.N, strings.Join(s.Obs.Blocked, "; "))
			n++
		}
	}
	if n == 0 {
		b.WriteString("- None\n")
	}
	_ = os.WriteFile(filepath.Join(r.dir, "report.md"), []byte(b.String()), 0o600)
}
