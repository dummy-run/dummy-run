package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/identity"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/runner"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

// scripted is a provider that replays tool calls and records what it was shown.
type scripted struct {
	calls    []provider.ToolCall
	requests []provider.Request
	mu       sync.Mutex
}

func (s *scripted) Kind() string   { return "openai-compatible" }
func (s *scripted) Model() string  { return "scripted" }
func (s *scripted) Region() string { return "local" }
func (s *scripted) Chat(_ context.Context, req provider.Request) (provider.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	n := len(s.requests) - 1
	resp := provider.Response{Usage: provider.Usage{InputTokens: 1000, OutputTokens: 50}, FinishReason: "tool_calls"}
	if req.ForceTool == "finish" || n >= len(s.calls) {
		resp.ToolCalls = []provider.ToolCall{{ID: fmt.Sprintf("call_%d", n), Name: "finish", Args: json.RawMessage(
			`{"verdict":"friction","goal_met":"partly","report":"Done. See step 2.","findings":[{"kind":"friction","severity":"minor","symptom":"not-operable","control":"@3","title":"The subscribe form cannot be used","steps":[4,1],"evidence":"measured"}],"observations":[{"text":"The home page loaded with its heading.","steps":[1]}]}`)}}
		return resp, nil
	}
	c := s.calls[n]
	c.ID = fmt.Sprintf("call_%d", n)
	resp.ToolCalls = []provider.ToolCall{c}
	return resp, nil
}

const sitePage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Site</title></head><body><main>
<h1>Welcome</h1><p>Read me.</p><a href="/docs">Documentation</a>
<form method="post" action="/subscribe"><label for="e">Email</label><input id="e" name="e"><button type="submit">Subscribe</button></form>
</main></body></html>`

func writeCast(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "reader")
	if err := os.MkdirAll(filepath.Join(dir, "scenarios"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"profile.md":                "---\nid: reader\nrole: visitor\ndevice: desktop\n---\n# Reader\n",
		"prompt.md":                 "You are a reader.\n",
		"scenarios/01-read-docs.md": "# Read the documentation\n\nFind the docs and read them.\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestRunEndToEnd plays a scripted persona against a local site on each
// browser in DUMMYRUN_BROWSERS and checks the run record, the read-only
// policy and the headers the site received.
func TestRunEndToEnd(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the end-to-end test")
	}
	// A copy of webappanalyzer in the environment could add technologies and journeys.
	t.Setenv("DUMMYRUN_WEBAPPANALYZER", "")
	for _, browser := range strings.Split(browsers, ",") {
		t.Run(browser, func(t *testing.T) {
			var mu sync.Mutex
			seen := map[string]http.Header{}
			var posts int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen[r.URL.Path] = r.Header.Clone()
				if r.Method == http.MethodPost {
					posts++
				}
				mu.Unlock()
				w.Header().Set("Content-Type", "text/html")
				if r.URL.Path == "/docs" {
					_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>Docs</title><main><h1>Docs</h1><p>All the docs.</p></main></html>`)
					return
				}
				// The home page carries one technology marker, a meta tag, which the run records with the journeys it
				// suggests.
				_, _ = io.WriteString(w, strings.Replace(sitePage, "</head>", `<meta name="generator" content="PrestaShop"></head>`, 1))
			}))
			t.Cleanup(srv.Close)

			castDir := writeCast(t)
			persona, err := cast.Load(castDir, "reader")
			if err != nil {
				t.Fatal(err)
			}
			prov := &scripted{calls: []provider.ToolCall{
				{Name: "click", Args: json.RawMessage(`{"target":"Documentation"}`)},
				{Name: "back", Args: json.RawMessage(`{}`)},
				{Name: "click", Args: json.RawMessage(`{"target":"Subscribe"}`)},
				{Name: "a11y", Args: json.RawMessage(`{}`)},
				{Name: "visit", Args: json.RawMessage(`{"url":"https://elsewhere.test/"}`)},
			}}
			out := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			res, err := runner.Run(ctx, runner.Config{
				Persona: persona, Scenario: persona.Scenarios[0], Provider: prov, Browser: browser, Headless: true, OutDir: out,
				Target: policy.Target{Adapter: "url", Environment: "local", BaseURL: srv.URL + "/", AllowedHosts: []string{"127.0.0.1"}, ReadOnly: true},
				Seed:   func() *int64 { n := int64(5); return &n }(),
			})
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			rec := res.Record
			// The version 2 fields the coverage estimators need.
			f0 := rec.Findings[0]
			wantFP, _ := identity.Fingerprint(f0.Route, f0.Locus, f0.Symptom)
			if rec.SchemaVersion != 2 || rec.Batch == "" || rec.RepeatIndex != 0 || rec.Seed != nil || rec.Scenario.GoalMet != "partly" {
				t.Errorf("version 2 header %+v", rec)
			}
			if f0.Fingerprint != wantFP || f0.Symptom != "not-operable" || f0.Class != "problem" || f0.Route == "" || !strings.Contains(f0.Locus, "subscribe") {
				t.Errorf("finding identity %+v", f0)
			}
			for _, st := range rec.Steps {
				if st.URL != "" && !strings.HasPrefix(st.Route, "/") {
					t.Errorf("step %d has no route: %+v", st.N, st)
				}
			}
			if rec.Target.TechError != "" {
				t.Errorf("tech detection failed: %s", rec.Target.TechError)
			}
			if len(rec.Target.Tech) != 1 || rec.Target.Tech[0].Name != "PrestaShop" || !slices.Equal(rec.Target.Journeys, []string{"basket", "checkout"}) {
				t.Errorf("target tech %+v journeys %v: want PrestaShop, with basket and checkout", rec.Target.Tech, rec.Target.Journeys)
			}
			if len(rec.Target.Commit) != 12 || rec.Target.Commit != rec.Target.BuildHash {
				t.Errorf("target %+v", rec.Target)
			}
			if rec.Usage.CostUSD != nil || rec.Persona.Dimensions == nil || rec.Persona.Dimensions.Viewport.Width != 1400 || *rec.Persona.Dimensions.AT != "none" {
				t.Errorf("usage %+v dimensions %+v", rec.Usage, rec.Persona.Dimensions)
			}
			if rec.Outcome != record.OutcomeCompleted || res.Finish == nil || res.Finish.Verdict != "friction" {
				t.Errorf("outcome %s finish %+v", rec.Outcome, res.Finish)
			}
			if len(rec.Observations) != 1 || rec.Observations[0].Steps[0] != 1 || len(rec.Findings) != 1 {
				t.Errorf("observations %+v, findings %d: want one of each, kept apart", rec.Observations, len(rec.Findings))
			}
			if len(rec.Steps) != 7 {
				t.Fatalf("%d steps: %+v", len(rec.Steps), rec.Steps)
			}
			actions := []string{}
			for _, s := range rec.Steps {
				actions = append(actions, s.Action)
			}
			if got := strings.Join(actions, ","); got != "visit,click,back,click,a11y,visit,finish" {
				t.Errorf("actions %s", got)
			}
			if !strings.HasSuffix(rec.Steps[1].URL, "/docs") || *rec.Steps[1].Status != 200 {
				t.Errorf("step 2 %+v", rec.Steps[1])
			}
			if !strings.Contains(rec.Steps[3].Note, "blocked: POST") {
				t.Errorf("step 4 should note the blocked POST: %+v", rec.Steps[3])
			}
			if rec.Steps[4].A11yViolations == nil {
				t.Error("a11y step has no violation count")
			}
			if !strings.Contains(rec.Steps[5].Note, "outside this run's target") {
				t.Errorf("step 6 should refuse the other host: %+v", rec.Steps[5])
			}
			if len(rec.Findings) != 1 || rec.Findings[0].Verdict != "pending" || rec.Findings[0].Steps[0] != 4 {
				t.Errorf("findings %+v", rec.Findings)
			}
			if rec.Usage.ModelCalls != 6 || rec.Usage.InputTokens != 6000 {
				t.Errorf("usage %+v", rec.Usage)
			}
			if rec.Browser.Version == "unknown" || rec.Browser.Name != browser {
				t.Errorf("browser %+v", rec.Browser)
			}
			// The record on disk validates, and the screenshots it names exist.
			back, err := record.Read(res.Path)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range back.Steps {
				if s.Screenshot == "" {
					t.Errorf("step %d has no screenshot", s.N)
					continue
				}
				if _, err := os.Stat(filepath.Join(out, s.Screenshot)); err != nil {
					t.Errorf("screenshot %s missing", s.Screenshot)
				}
			}
			for _, f := range []string{"report.md", "steps.jsonl"} {
				if _, err := os.Stat(filepath.Join(out, rec.ID, f)); err != nil {
					t.Errorf("%s missing", f)
				}
			}
			// The site saw the baggage and the derived traceparent on every page, and no POST.
			mu.Lock()
			defer mu.Unlock()
			tr := telemetry.NewTrace(rec.ID)
			// "/" was loaded by step 1 (the opening visit) and "/docs" by step 2 (the click).
			for path, step := range map[string]int{"/": 1, "/docs": 2} {
				h := seen[path]
				if h == nil {
					t.Fatalf("site never saw %s", path)
				}
				if h.Get("baggage") != "dummyrun.run.id="+rec.ID+",dummyrun.persona.id=reader" {
					t.Errorf("%s baggage %q", path, h.Get("baggage"))
				}
				want := "00-" + tr.TraceID + "-" + tr.StepSpanID(step) + "-01"
				if h.Get("traceparent") != want {
					t.Errorf("%s traceparent %q, want %q (step %d)", path, h.Get("traceparent"), want, step)
				}
				if !strings.HasSuffix(h.Get("User-Agent"), "DummyRun/"+telemetry.Version) {
					t.Errorf("%s user agent %q", path, h.Get("User-Agent"))
				}
			}
			if posts != 0 {
				t.Errorf("the site received %d POST requests", posts)
			}
			// The model saw the first page with a screenshot and the budget line.
			last := prov.requests[len(prov.requests)-1]
			if len(last.Turns) == 0 || !strings.Contains(last.Turns[0].Text, "Begin.") || len(prov.requests[0].Turns[0].Images) != 1 {
				t.Errorf("opening turn %+v", prov.requests[0].Turns)
			}
			var sawBudget bool
			for _, turn := range last.Turns {
				if strings.Contains(turn.Text, "Actions left:") {
					sawBudget = true
				}
			}
			if !sawBudget {
				t.Error("the model was never told its remaining budget")
			}
		})
	}
}

// writePersonaCast writes a cast with one persona whose front matter and
// scenario text the test chooses.
func writePersonaCast(t *testing.T, extraFrontMatter, scenario string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "kit")
	if err := os.MkdirAll(filepath.Join(dir, "scenarios"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"profile.md":                "---\nid: kit\nrole: visitor\ndevice: desktop\n" + extraFrontMatter + "---\n# Kit\n",
		"prompt.md":                 "You are Kit.\n",
		"scenarios/01-read-docs.md": "# Read the documentation\n\n" + scenario + "\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func plainServer(t *testing.T) (*httptest.Server, func(path string) int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/docs" {
			_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>Docs</title><main><h1>Docs</h1></main></html>`)
			return
		}
		_, _ = io.WriteString(w, sitePage)
	}))
	t.Cleanup(srv.Close)
	return srv, func(path string) int { mu.Lock(); defer mu.Unlock(); return hits[path] }
}

func runScripted(t *testing.T, browser, castDir string, calls []provider.ToolCall, target string) (*runner.Result, *scripted) {
	t.Helper()
	persona, err := cast.Load(castDir, "kit")
	if err != nil {
		t.Fatal(err)
	}
	prov := &scripted{calls: calls}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	res, err := runner.Run(ctx, runner.Config{
		Persona: persona, Scenario: persona.Scenarios[0], Provider: prov, Browser: browser, Headless: true, OutDir: t.TempDir(),
		Target: policy.Target{Adapter: "url", Environment: "local", BaseURL: target + "/", AllowedHosts: []string{"127.0.0.1"}, ReadOnly: true},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return res, prov
}

// TestKeyboardPersonaCannotClick plays a persona with click denied. The click
// is refused and recorded as a blocked step, the browser never follows the
// link, and Tab then Enter still works with focus reported at each step.
func TestKeyboardPersonaCannotClick(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the end-to-end test")
	}
	for _, browser := range strings.Split(browsers, ",") {
		t.Run(browser, func(t *testing.T) {
			srv, hits := plainServer(t)
			castDir := writePersonaCast(t, "tools-denied: click\n", "Find the documentation.")
			res, prov := runScripted(t, browser, castDir, []provider.ToolCall{
				{Name: "click", Args: json.RawMessage(`{"target":"Documentation"}`)},
				{Name: "press", Args: json.RawMessage(`{"key":"Tab"}`)},
				{Name: "press", Args: json.RawMessage(`{"key":"Enter"}`)},
			}, srv.URL)
			rec := res.Record
			if got := rec.Steps[1]; got.Action != "click" || !strings.HasPrefix(got.Note, "blocked: click is not one of the tools kit may use") {
				t.Errorf("step 2 should be a blocked click: %+v", got)
			}
			if !strings.HasSuffix(rec.Steps[1].URL, "/") {
				t.Errorf("the refused click changed the page to %s", rec.Steps[1].URL)
			}
			if !strings.HasSuffix(rec.Steps[3].URL, "/docs") || hits("/docs") != 1 {
				t.Errorf("Tab then Enter should reach /docs once: step 4 %+v, %d hits", rec.Steps[3], hits("/docs"))
			}
			if rec.Persona.Zoom != 0 || rec.Persona.Device != "desktop" {
				t.Errorf("persona block %+v", rec.Persona)
			}
			// The model was told, and saw focus after Tab.
			if sys := prov.requests[0].System; !strings.Contains(sys, "You may not use these tools: click.") {
				t.Errorf("system prompt does not list the refused tool")
			}
			var sawFocus bool
			for _, turn := range prov.requests[len(prov.requests)-1].Turns {
				sawFocus = sawFocus || strings.Contains(turn.Text, `focus: link "Documentation" @`)
			}
			if !sawFocus {
				t.Error("the model never saw focus on the Documentation link after Tab")
			}
			var sawRefusal bool
			for _, turn := range prov.requests[1].Turns {
				sawRefusal = sawRefusal || strings.Contains(turn.Text, "REFUSED: click is not one of the tools kit may use")
			}
			if !sawRefusal {
				t.Error("the model was not told the click was refused")
			}
			steps, err := os.ReadFile(filepath.Join(filepath.Dir(res.Path), rec.ID, "steps.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(steps), `"refusal":"click is not one of the tools kit may use`) {
				t.Error("steps.jsonl does not record the refusal")
			}
			if !strings.Contains(string(steps), `"focus":{"role":"link","name":"Documentation"`) {
				t.Error("steps.jsonl does not record focus")
			}
		})
	}
}

// TestScenarioDeviceAndZoom plays a persona with a zoom of 200 on a scenario
// that sets its own device. The scenario's device wins, the page sees the
// zoomed viewport, the screenshot has the window's pixel size and the record
// says what was used.
func TestScenarioDeviceAndZoom(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the end-to-end test")
	}
	for _, browser := range strings.Split(browsers, ",") {
		t.Run(browser, func(t *testing.T) {
			srv, _ := plainServer(t)
			castDir := writePersonaCast(t, "zoom: 200\n", "Device: phone. Start at the home page.")
			res, prov := runScripted(t, browser, castDir, []provider.ToolCall{{Name: "look", Args: json.RawMessage(`{}`)}}, srv.URL)
			rec := res.Record
			if rec.Persona.Device != "phone" || rec.Persona.Zoom != 200 {
				t.Errorf("persona block %+v, want phone at 200", rec.Persona)
			}
			first := prov.requests[0].Turns[0].Text
			if !strings.Contains(first, "zoom: 200% (the page lays out in 195 x 422 CSS px, device pixel ratio 2)") {
				t.Errorf("the first observation does not show the zoom:\n%s", first)
			}
			shot, err := os.Open(filepath.Join(filepath.Dir(res.Path), rec.Steps[0].Screenshot))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = shot.Close() }()
			cfg, err := png.DecodeConfig(shot)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Width != 390 || cfg.Height != 844 {
				t.Errorf("screenshot is %d by %d pixels, want the phone window 390 by 844", cfg.Width, cfg.Height)
			}
		})
	}
}

const overlapSite = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title>
<style>body{margin:0;font:1rem sans-serif} button{position:absolute;top:20px;height:48px;font:inherit}</style></head>
<body><main><h1>Shop</h1><button style="left:10px;width:150px">Add to basket</button><button style="left:100px;width:150px">Buy now</button></main></body></html>`

// TestTextScaleRun plays a persona with a text scale of 200 on a phone. The record says so, the model is
// told, the first observation shows the text size and names the two overlapping buttons, and the page is
// not zoomed: the window and the device pixel ratio stay as the phone's.
func TestTextScaleRun(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the end-to-end test")
	}
	for _, browser := range strings.Split(browsers, ",") {
		t.Run(browser, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = io.WriteString(w, overlapSite)
			}))
			t.Cleanup(srv.Close)
			castDir := writePersonaCast(t, "text_scale: 200\n", "Device: phone. Start at the home page.")
			res, prov := runScripted(t, browser, castDir, []provider.ToolCall{{Name: "look", Args: json.RawMessage(`{}`)}}, srv.URL)
			rec := res.Record
			if rec.Persona.TextScale != 200 || rec.Persona.Zoom != 0 || rec.Persona.Device != "phone" {
				t.Errorf("persona block %+v, want phone at text scale 200 and no zoom", rec.Persona)
			}
			if z := rec.Persona.Dimensions.Zoom; z == nil || *z != 1 {
				t.Errorf("dimensions zoom %v, want 1: text scale is not zoom", z)
			}
			if _, err := record.Read(res.Path); err != nil {
				t.Errorf("the record does not validate: %v", err)
			}
			if sys := prov.requests[0].System; !strings.Contains(sys, "system text size raised to 200 percent") {
				t.Errorf("the model was not told about the text size")
			}
			first := prov.requests[0].Turns[0].Text
			for _, want := range []string{"text size: 200% of normal", "layout: 1 pair of controls overlap (@1 @2)"} {
				if !strings.Contains(first, want) {
					t.Errorf("the first observation lacks %q:\n%s", want, first)
				}
			}
			if strings.Contains(first, "zoom:") {
				t.Errorf("a text scale run shows a zoom line:\n%s", first)
			}
		})
	}
}

func TestRobotsDisallowStopsBeforeTheBrowser(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.URL.Path+" "+r.Header.Get("User-Agent"))
		mu.Unlock()
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: DummyRun\nDisallow: /\n"))
		}
	}))
	defer srv.Close()
	persona, err := cast.Load(writeCast(t), "reader")
	if err != nil {
		t.Fatal(err)
	}
	res, err := runner.Run(context.Background(), runner.Config{Persona: persona, Scenario: persona.Scenarios[0], Provider: &scripted{},
		Browser: "chrome", Headless: true, OutDir: t.TempDir(),
		Target: policy.Target{Adapter: "url", Environment: "local", BaseURL: srv.URL + "/", AllowedHosts: []string{"127.0.0.1"}, ReadOnly: true,
			RespectRobots: true, Identify: true}})
	if !errors.Is(err, runner.ErrRobotsDisallow) || res == nil {
		t.Fatalf("res %v err %v", res, err)
	}
	rec := res.Record
	if rec.Outcome != record.OutcomeBlocked || !strings.Contains(rec.OutcomeReason, "robots.txt") || len(rec.Steps) != 0 || !rec.Target.RespectRobots || !rec.Target.Identify {
		t.Errorf("record %+v", rec)
	}
	if _, rerr := record.Read(res.Path); rerr != nil {
		t.Errorf("record does not validate: %v", rerr)
	}
	mu.Lock()
	defer mu.Unlock()
	// With no commit the engine would hash the home page. It must not do that for a disallowed site.
	if len(hits) != 1 || !strings.HasPrefix(hits[0], "/robots.txt ") || !strings.HasSuffix(hits[0], "(+https://dummyrun.dev/scanner/)") {
		t.Errorf("requests %v", hits)
	}
}

// TestIdentifyAndRobotsInBrowser runs a scripted persona on each browser in DUMMYRUN_BROWSERS against a site
// whose robots.txt disallows /private for DummyRun. The site must see the identified user agent, the
// disallowed page must be blocked in the browser, and the allowed page must load.
func TestIdentifyAndRobotsInBrowser(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser test")
	}
	for _, browser := range strings.Split(browsers, ",") {
		t.Run(browser, func(t *testing.T) {
			var mu sync.Mutex
			agents := map[string]string{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				agents[r.URL.Path] = r.Header.Get("User-Agent")
				mu.Unlock()
				switch r.URL.Path {
				case "/robots.txt":
					w.Header().Set("Content-Type", "text/plain")
					_, _ = io.WriteString(w, "User-agent: *\nDisallow: /\n\nUser-agent: DummyRun\nDisallow: /private\n")
				case "/docs", "/private":
					w.Header().Set("Content-Type", "text/html")
					_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>Page</title><main><h1>Page</h1></main></html>`)
				default:
					w.Header().Set("Content-Type", "text/html")
					_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>Home</title><main><h1>Home</h1><a href="/docs">Docs</a> <a href="/private">Private</a></main></html>`)
				}
			}))
			t.Cleanup(srv.Close)
			persona, err := cast.Load(writeCast(t), "reader")
			if err != nil {
				t.Fatal(err)
			}
			prov := &scripted{calls: []provider.ToolCall{
				{Name: "visit", Args: json.RawMessage(`{"url":"/docs"}`)},
				{Name: "visit", Args: json.RawMessage(`{"url":"/private"}`)},
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			res, err := runner.Run(ctx, runner.Config{Persona: persona, Scenario: persona.Scenarios[0], Provider: prov, Browser: browser,
				Headless: true, OutDir: t.TempDir(),
				Target: policy.Target{Adapter: "url", Environment: "local", BaseURL: srv.URL + "/", AllowedHosts: []string{"127.0.0.1"}, ReadOnly: true,
					RespectRobots: true, Identify: true, Commit: "abcdef1"}})
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !res.Record.Target.Identify || !res.Record.Target.RespectRobots {
				t.Errorf("target %+v", res.Record.Target)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, path := range []string{"/", "/docs"} {
				if ua := agents[path]; !strings.HasSuffix(ua, "DummyRun/"+telemetry.Version+" (+https://dummyrun.dev/scanner/)") || !strings.Contains(ua, "Mozilla/5.0") {
					t.Errorf("%s user agent %q", path, ua)
				}
			}
			if _, seen := agents["/private"]; seen {
				t.Error("the site received a request for the disallowed /private")
			}
			var noted bool
			for _, st := range res.Record.Steps {
				if strings.Contains(st.Note, "robots.txt disallows DummyRun") || strings.Contains(st.Note, "blocked") && strings.Contains(st.Note, "/private") {
					noted = true
				}
			}
			if !noted {
				t.Errorf("no step noted the robots.txt block: %+v", res.Record.Steps)
			}
		})
	}
}

// A webappanalyzer copy that can't be read is recorded in target.tech_error, and the run goes on with the engine's
// own fingerprints.
func TestTechErrorDoesNotStopTheRun(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the end-to-end test")
	}
	t.Setenv("DUMMYRUN_WEBAPPANALYZER", t.TempDir())
	browser := strings.Split(browsers, ",")[0]
	srv, _ := plainServer(t)
	castDir := writePersonaCast(t, "", "Find the documentation.")
	res, _ := runScripted(t, browser, castDir, []provider.ToolCall{
		{Name: "click", Args: json.RawMessage(`{"target":"Documentation"}`)},
	}, srv.URL)
	rec := res.Record
	if !strings.Contains(rec.Target.TechError, "webappanalyzer") || rec.Outcome != record.OutcomeCompleted {
		t.Errorf("tech error %q, outcome %s: want the error recorded and the run completed", rec.Target.TechError, rec.Outcome)
	}
}
