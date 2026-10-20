package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/runner"
)

//nolint:misspell // the axe rule ID
const ruleContrast = "color-contrast"

const (
	libraryDir = "../../casts/library"
	exampleRun = "../../examples/runs/2026-10-16-ada-editor-003.json"
)

// fakeProvider stands in for a model. The fake runner never calls it.
type fakeProvider struct{}

func (fakeProvider) Kind() string   { return "openai-compatible" }
func (fakeProvider) Model() string  { return "fake" }
func (fakeProvider) Region() string { return "local" }
func (fakeProvider) Chat(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{}, errors.New("not used")
}

// harness connects an in-process client to a server built from cfg.
type harness struct {
	t        *testing.T
	cs       *mcp.ClientSession
	srv      *Server
	runs     string
	progress chan string
}

func newHarness(t *testing.T, cfg Config) *harness {
	t.Helper()
	h := &harness{t: t, progress: make(chan string, 64)}
	h.runs = t.TempDir()
	cfg.RunsDir = h.runs
	cfg.LibraryDir = libraryDir
	if cfg.Origins == nil {
		cfg.Origins, _ = ParseOrigins(nil)
	}
	srv, state := New(cfg)
	h.srv = state
	t.Cleanup(state.Close)
	ct, st := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, r *mcp.ProgressNotificationClientRequest) {
			h.progress <- r.Params.Message
		}})
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	h.cs = cs
	return h
}

// call runs a tool and decodes its structured result into out when the call succeeds.
func (h *harness) call(name string, args, out any) *mcp.CallToolResult {
	h.t.Helper()
	params := &mcp.CallToolParams{Name: name, Arguments: args}
	params.SetProgressToken("p1")
	res, err := h.cs.CallTool(context.Background(), params)
	if err != nil {
		// A handler error that the SDK reports as a protocol error still reads as a failed call.
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}
	}
	if out != nil && !res.IsError {
		data, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(data, out); err != nil {
			h.t.Errorf("decode %s result: %v", name, err)
		}
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestToolsAreListedWithTypedSchemas(t *testing.T) {
	h := newHarness(t, Config{})
	res, err := h.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"list_personas": nil,
		"a11y_sweep":    {"url"},
		"run_persona":   {"url", "persona"},
		"get_run":       {"path"},
	}
	if len(res.Tools) != len(want) {
		t.Fatalf("got %d tools, want %d", len(res.Tools), len(want))
	}
	for _, tool := range res.Tools {
		required, ok := want[tool.Name]
		if !ok {
			t.Errorf("unexpected tool %s", tool.Name)
			continue
		}
		in := schemaMap(t, tool.InputSchema)
		if in["type"] != "object" {
			t.Errorf("%s input schema type = %v", tool.Name, in["type"])
		}
		got := map[string]bool{}
		if r, ok := in["required"].([]any); ok {
			for _, v := range r {
				got[v.(string)] = true
			}
		}
		if len(got) != len(required) {
			t.Errorf("%s required = %v, want %v", tool.Name, got, required)
		}
		for _, name := range required {
			if !got[name] {
				t.Errorf("%s does not require %s", tool.Name, name)
			}
		}
		if tool.OutputSchema == nil {
			t.Errorf("%s has no output schema", tool.Name)
		}
		if tool.Description == "" || tool.Annotations == nil {
			t.Errorf("%s needs a description and annotations", tool.Name)
		}
	}
	for _, tool := range res.Tools {
		ro := tool.Annotations.ReadOnlyHint
		if (tool.Name == "run_persona") == ro {
			t.Errorf("%s ReadOnlyHint = %v", tool.Name, ro)
		}
	}
}

func schemaMap(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestListPersonasLibraryAndLocalCast(t *testing.T) {
	cast := t.TempDir()
	dir := filepath.Join(cast, "tom-developer")
	if err := os.MkdirAll(filepath.Join(dir, "scenarios"), 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("profile.md", "---\nid: tom-developer\nrole: developer\ndevice: desktop\n---\n# Tom\n\nTom reads the quickstart first. He skims the rest.\n")
	write("prompt.md", "You are Tom.\n")
	write("scenarios/01-find-the-quickstart.md", "# Find the quickstart\n\nGoal.\n")
	h := newHarness(t, Config{CastDirs: []string{cast, filepath.Join(t.TempDir(), "missing")}})
	var out ListPersonasOutput
	if res := h.call("list_personas", map[string]any{}, &out); res.IsError {
		t.Fatal(text(res))
	}
	byID := map[string]PersonaInfo{}
	for _, p := range out.Personas {
		byID[p.ID] = p
	}
	if len(out.Personas) != 12 {
		t.Errorf("got %d personas, want the eleven in the library and one local", len(out.Personas))
	}
	tom := byID["tom-developer"]
	if tom.Source != filepath.Base(cast) || tom.Summary != "Tom reads the quickstart first." || len(tom.Scenarios) != 1 || tom.Scenarios[0].ID != "find-the-quickstart" {
		t.Errorf("local persona = %+v", tom)
	}
	alex := byID["first-time-visitor"]
	if alex.Source != "library" || alex.Summary == "" || alex.Device != "desktop" || len(alex.Scenarios) != 4 {
		t.Errorf("library persona = %+v", alex)
	}
	if len(out.Warnings) != 0 {
		t.Errorf("warnings: %v", out.Warnings)
	}
}

func TestExtractLibraryIsReusedAndComplete(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)
	first, err := ExtractLibrary()
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExtractLibrary()
	if err != nil || second != first {
		t.Fatalf("second extraction = %q, %v; want %q", second, err, first)
	}
	if !strings.HasPrefix(first, cache) {
		t.Errorf("extracted to %s, outside the cache %s", first, cache)
	}
	s := &Server{cfg: Config{LibraryDir: first}}
	p, err := s.findPersona("screen-reader-user")
	if err != nil || len(p.Scenarios) == 0 {
		t.Fatalf("findPersona: %v", err)
	}
}

func TestA11ySweepRefusesOtherSitesBeforeAnythingStarts(t *testing.T) {
	called := false
	h := newHarness(t, Config{Sweep: func(context.Context, a11y.Config) (*a11y.Report, error) {
		called = true
		return &a11y.Report{}, nil
	}})
	for _, u := range []string{"https://example.com/", "http://169.254.169.254/", "file:///etc/hosts"} {
		res := h.call("a11y_sweep", map[string]any{"url": u}, nil)
		if !res.IsError {
			t.Errorf("%s was accepted", u)
		}
	}
	if called {
		t.Error("the sweep ran for a refused URL")
	}
}

func TestA11ySweepReturnsViolationsFromAFakeBrowser(t *testing.T) {
	var got a11y.Config
	h := newHarness(t, Config{Sweep: func(_ context.Context, cfg a11y.Config) (*a11y.Report, error) {
		got = cfg
		f := func(rule, impact, target string) a11y.Finding {
			return a11y.Finding{Rule: rule, Impact: impact, WCAG: []string{"1.4.3"}, Help: "help " + rule,
				HelpURL: "https://dequeuniversity.com/rules/axe/4.13/" + rule, Nodes: []a11y.Node{{Target: target}}}
		}
		return &a11y.Report{Browser: "chrome", AxeVersion: "4.13.0", Results: []a11y.PageResult{
			{URL: cfg.URLs[0], Browser: "chrome", Viewport: "desktop", Scheme: "light", Violations: []a11y.Finding{
				f(ruleContrast, "serious", "#faint"), f("button-name", "critical", "#nameless")}},
			{URL: cfg.URLs[0], Browser: "chrome", Viewport: "phone", Scheme: "light", Violations: []a11y.Finding{
				f(ruleContrast, "serious", "#faint")}, NeedsReview: []a11y.Finding{{Rule: "x"}}},
		}}, nil
	}})
	var out A11ySweepOutput
	res := h.call("a11y_sweep", map[string]any{"url": "http://localhost:3000/pricing", "viewport": "both"}, &out)
	if res.IsError {
		t.Fatal(text(res))
	}
	if len(got.Viewports) != 2 || len(got.Schemes) != 1 || got.Schemes[0] != a11y.Light || !got.Headless || got.URLs[0] != "http://localhost:3000/pricing" {
		t.Errorf("sweep config = %+v", got)
	}
	if out.Total != 2 || len(out.Violations) != 2 || out.NeedsReview != 1 {
		t.Fatalf("out = %+v", out)
	}
	first := out.Violations[0]
	if first.Rule != "button-name" || first.Selector != "#nameless" || first.HelpURL == "" || first.WCAG[0] != "1.4.3" {
		t.Errorf("most severe first, got %+v", first)
	}
	if v := out.Violations[1]; len(v.Where) != 2 {
		t.Errorf("the repeated failure should list both viewports: %+v", v)
	}
}

func TestA11ySweepRejectsBadOptionsAndCountsCalls(t *testing.T) {
	cfg := Config{Limits: Limits{DefaultSteps: 1, MaxSteps: 1, DefaultTokens: 1, MaxTokens: 1, SessionTokens: 1, MaxRuns: 1, MaxSweeps: 1},
		Sweep: func(context.Context, a11y.Config) (*a11y.Report, error) { return &a11y.Report{}, nil }}
	h := newHarness(t, cfg)
	if res := h.call("a11y_sweep", map[string]any{"url": "http://localhost/", "viewport": "huge"}, nil); !res.IsError {
		t.Error("a bad viewport was accepted")
	}
	if res := h.call("a11y_sweep", map[string]any{"url": "http://localhost/"}, nil); res.IsError {
		t.Fatal(text(res))
	}
	res := h.call("a11y_sweep", map[string]any{"url": "http://localhost/"}, nil)
	if !res.IsError || !strings.Contains(text(res), "limit") {
		t.Errorf("the second sweep should hit the session limit: %v", text(res))
	}
}

// fakeRun is a browserless runner. It writes a real record, so get_run and the schema are exercised.
type fakeRun struct {
	mu   sync.Mutex
	cfgs []runner.Config
	err  error
	used int
}

func (f *fakeRun) run(_ context.Context, cfg runner.Config) (*runner.Result, error) {
	f.mu.Lock()
	f.cfgs = append(f.cfgs, cfg)
	f.mu.Unlock()
	rec, err := record.Read(exampleRun)
	if err != nil {
		return nil, err
	}
	rec.ID = "2026-10-19-fake-001"
	if f.used > 0 {
		rec.Usage.InputTokens, rec.Usage.OutputTokens = f.used, 0
	}
	path, err := record.Write(cfg.OutDir, rec)
	if err != nil {
		return nil, err
	}
	_, _ = cfg.Log.Write([]byte("run start\n  model call 1: 1.0s, 10 in / 2 out tokens\n"))
	return &runner.Result{Record: rec, Path: path}, f.err
}

func providerFor(string) func() (provider.Provider, error) {
	return func() (provider.Provider, error) { return fakeProvider{}, nil }
}

func TestRunPersonaWithAFakeBrowserAndModel(t *testing.T) {
	f := &fakeRun{}
	h := newHarness(t, Config{Run: f.run, NewProvider: providerFor("")})
	var out RunOutput
	res := h.call("run_persona", map[string]any{"url": "http://localhost:3000/", "persona": "first-time-visitor",
		"task": "find the price of the team plan", "max_steps": 99, "max_tokens": 9_000_000}, &out)
	if res.IsError {
		t.Fatal(text(res))
	}
	if len(f.cfgs) != 1 {
		t.Fatalf("ran %d times", len(f.cfgs))
	}
	cfg := f.cfgs[0]
	if !cfg.Target.ReadOnly || len(cfg.Target.AllowedHosts) != 1 || cfg.Target.AllowedHosts[0] != "localhost" {
		t.Errorf("target = %+v", cfg.Target)
	}
	if cfg.StepBudget != 25 || cfg.TokenBudget != 400_000 || !cfg.Headless || cfg.Scenario.ID != "inline" ||
		!strings.Contains(cfg.Scenario.Body, "team plan") || cfg.Persona.ID != "first-time-visitor" {
		t.Errorf("config = steps %d, tokens %d, scenario %q", cfg.StepBudget, cfg.TokenBudget, cfg.Scenario.ID)
	}
	if out.RunID != "2026-10-19-fake-001" || out.Outcome == "" || out.RecordPath == "" || len(out.Findings) == 0 || out.Usage.ModelCalls == 0 {
		t.Fatalf("out = %+v", out)
	}
	cited := 0
	for _, fd := range out.Findings {
		cited += len(fd.CitedSteps)
		if fd.Verdict != record.VerdictPending && fd.Verdict == "" {
			t.Errorf("finding %s has no verdict", fd.ID)
		}
	}
	if cited == 0 {
		t.Error("no finding cites a step")
	}
	if out.Steps != nil || out.SessionTokens == nil || out.SessionTokens.TokensUsed == 0 {
		t.Errorf("steps and session = %v, %+v", out.Steps, out.SessionTokens)
	}
	select {
	case msg := <-h.progress:
		if msg == "" {
			t.Error("empty progress message")
		}
	case <-time.After(2 * time.Second):
		t.Error("no progress notification arrived")
	}
	// get_run reads the same record back, with every step.
	var again RunOutput
	if res := h.call("get_run", map[string]any{"path": out.RecordPath}, &again); res.IsError {
		t.Fatal(text(res))
	}
	if again.RunID != out.RunID || len(again.Steps) == 0 || again.StepCount != out.StepCount {
		t.Errorf("get_run = %+v", again)
	}
	if res := h.call("get_run", map[string]any{"path": out.RunID}, &again); res.IsError {
		t.Errorf("get_run by ID: %s", text(res))
	}
}

func TestRunPersonaChecksInputsFirst(t *testing.T) {
	f := &fakeRun{}
	h := newHarness(t, Config{Run: f.run, NewProvider: providerFor("")})
	for name, args := range map[string]map[string]any{
		"public site":      {"url": "https://example.com/", "persona": "first-time-visitor", "task": "x"},
		"unknown persona":  {"url": "http://localhost/", "persona": "nobody", "task": "x"},
		"no task":          {"url": "http://localhost/", "persona": "first-time-visitor"},
		"both":             {"url": "http://localhost/", "persona": "first-time-visitor", "task": "x", "scenario_id": "what-is-this"},
		"unknown scenario": {"url": "http://localhost/", "persona": "first-time-visitor", "scenario_id": "nope"},
		"long task":        {"url": "http://localhost/", "persona": "first-time-visitor", "task": strings.Repeat("a", 2001)},
		"private http":     {"url": "http://192.168.1.2:3000/", "persona": "first-time-visitor", "task": "x"},
	} {
		if res := h.call("run_persona", args, nil); !res.IsError {
			t.Errorf("%s: accepted", name)
		}
	}
	if len(f.cfgs) != 0 {
		t.Error("a refused call still started a run")
	}
	// A real scenario ID works.
	if res := h.call("run_persona", map[string]any{"url": "http://localhost/", "persona": "first-time-visitor", "scenario_id": "what-is-this"}, nil); res.IsError {
		t.Error(text(res))
	}
	if len(f.cfgs) != 1 || f.cfgs[0].Scenario.ID != "what-is-this" || f.cfgs[0].StepBudget != 10 || f.cfgs[0].TokenBudget != 150_000 {
		t.Errorf("config = %+v", f.cfgs)
	}
}

func TestRunPersonaNeedsAProvider(t *testing.T) {
	f := &fakeRun{}
	h := newHarness(t, Config{Run: f.run, NewProvider: func() (provider.Provider, error) { return nil, errors.New("no model is set") }})
	res := h.call("run_persona", map[string]any{"url": "http://localhost/", "persona": "first-time-visitor", "task": "x"}, nil)
	if !res.IsError || !strings.Contains(text(res), "no model provider") || len(f.cfgs) != 0 {
		t.Errorf("got %v", text(res))
	}
	// The other tools do not need one.
	if res := h.call("list_personas", map[string]any{}, nil); res.IsError {
		t.Error(text(res))
	}
}

func TestSessionLimitsStopRuns(t *testing.T) {
	f := &fakeRun{used: 600}
	h := newHarness(t, Config{Run: f.run, NewProvider: providerFor(""),
		Limits: Limits{DefaultSteps: 5, MaxSteps: 5, DefaultTokens: 1000, MaxTokens: 1000, SessionTokens: 1000, MaxRuns: 5, MaxSweeps: 1}})
	args := map[string]any{"url": "http://localhost/", "persona": "first-time-visitor", "task": "x"}
	if res := h.call("run_persona", args, nil); res.IsError {
		t.Fatal(text(res))
	}
	// 600 of 1000 are spent, so the next run may spend at most 400.
	if res := h.call("run_persona", args, nil); res.IsError {
		t.Fatal(text(res))
	}
	if got := f.cfgs[1].TokenBudget; got != 400 {
		t.Errorf("second run budget = %d, want 400", got)
	}
	res := h.call("run_persona", args, nil)
	if !res.IsError || !strings.Contains(text(res), "token budget") {
		t.Errorf("third run: %v", text(res))
	}
	if len(f.cfgs) != 2 {
		t.Errorf("ran %d times", len(f.cfgs))
	}
}

func TestRunErrorIsReportedAndRedacted(t *testing.T) {
	t.Setenv("DUMMYRUN_TEST_KEY", "supersecretvalue123")
	f := &fakeRun{err: errors.New("model call 2: 401 for key supersecretvalue123 and sk-abcdefghijklmnopqrstuv")}
	h := newHarness(t, Config{Run: f.run, NewProvider: providerFor(""), SecretEnv: []string{"DUMMYRUN_TEST_KEY"}})
	res := h.call("run_persona", map[string]any{"url": "http://localhost/", "persona": "first-time-visitor", "task": "x"}, nil)
	if !res.IsError {
		t.Fatal("a failed run should be an error result")
	}
	msg := text(res)
	if strings.Contains(msg, "supersecret") || strings.Contains(msg, "sk-abc") || !strings.Contains(msg, "[redacted]") {
		t.Errorf("secret in %q", msg)
	}
}

func TestGetRunReadsOnlyTheRunsFolder(t *testing.T) {
	h := newHarness(t, Config{})
	inside := filepath.Join(h.runs, "a.json")
	data, err := os.ReadFile(exampleRun)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, data, 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "b.json")
	if err := os.WriteFile(outside, data, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(h.runs, "link.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	notJSON := filepath.Join(h.runs, "secret.txt")
	if err := os.WriteFile(notJSON, []byte("password=hunter2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := h.call("get_run", map[string]any{"path": inside}, nil); res.IsError {
		t.Errorf("inside: %s", text(res))
	}
	if res := h.call("get_run", map[string]any{"path": "a"}, nil); res.IsError {
		t.Errorf("by ID: %s", text(res))
	}
	for name, p := range map[string]string{"outside": outside, "symlink": link, "text file": notJSON,
		"dot dot": filepath.Join(h.runs, "..", filepath.Base(filepath.Dir(outside)), "b.json"), "passwd": "/etc/passwd", "empty": ""} {
		res := h.call("get_run", map[string]any{"path": p}, nil)
		if !res.IsError {
			t.Errorf("%s: read", name)
		} else if strings.Contains(text(res), "hunter2") {
			t.Errorf("%s: leaked contents", name)
		}
	}
}

func TestCloseStopsRunningCalls(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	h := newHarness(t, Config{NewProvider: providerFor(""),
		Run: func(ctx context.Context, _ runner.Config) (*runner.Result, error) {
			close(started)
			<-ctx.Done()
			close(stopped)
			return nil, ctx.Err()
		}})
	go func() {
		_, _ = h.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "run_persona",
			Arguments: map[string]any{"url": "http://localhost/", "persona": "first-time-visitor", "task": "x"}})
	}()
	<-started
	h.srv.Close()
	select {
	case <-stopped:
	default:
		t.Error("Close returned while the run was still going")
	}
}

func TestRedactIgnoresShortValues(t *testing.T) {
	t.Setenv("DUMMYRUN_TEST_SHORT", "abc")
	s := &Server{cfg: Config{SecretEnv: []string{"DUMMYRUN_TEST_SHORT"}}}
	if got := s.redact("abc is fine"); got != "abc is fine" {
		t.Errorf("got %q", got)
	}
}
