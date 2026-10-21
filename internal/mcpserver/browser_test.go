package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dummy-run/dummy-run/internal/provider"
)

//nolint:misspell // CSS property names
const badPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Fixture</title></head><body><main>
<h1>Known violations</h1><button id="nameless"></button>
<form method="post" action="/subscribe"><label for="e">Email</label><input id="e" name="e"><button type="submit">Subscribe</button></form>
</main></body></html>`

// scriptedModel clicks the form's button once and then finishes with two findings. "Subscribe does nothing" cites
// only the step where the read-only policy stopped the POST, so the engine drops it as the test's own limit.
type scriptedModel struct{ n atomic.Int32 }

func (*scriptedModel) Kind() string   { return "openai-compatible" }
func (*scriptedModel) Model() string  { return "scripted" }
func (*scriptedModel) Region() string { return "local" }
func (m *scriptedModel) Chat(_ context.Context, req provider.Request) (provider.Response, error) {
	n := m.n.Add(1)
	resp := provider.Response{Usage: provider.Usage{InputTokens: 500, OutputTokens: 20}, FinishReason: "tool_calls"}
	call := provider.ToolCall{ID: fmt.Sprintf("c%d", n), Name: "click", Args: json.RawMessage(`{"target":"Subscribe"}`)}
	if n > 1 || req.ForceTool == "finish" {
		call.Name = "finish"
		call.Args = json.RawMessage(`{"verdict":"friction","goal_met":"no","report":"The form did not submit.","findings":[` +
			`{"kind":"bug","severity":"minor","symptom":"not-operable","title":"Subscribe does nothing","steps":[2],"evidence":"measured"},` +
			`{"kind":"accessibility","severity":"minor","symptom":"unclear-label","title":"The first button has no name","steps":[1],"evidence":"measured"}]}`)
	}
	resp.ToolCalls = []provider.ToolCall{call}
	return resp, nil
}

// TestServerWithRealBrowsers drives the server through an in-process MCP client against a local page, in each
// browser named in DUMMYRUN_BROWSERS, such as "chrome,firefox". Without it the test is skipped.
func TestServerWithRealBrowsers(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser test")
	}
	var mu sync.Mutex
	posts := 0
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mu.Lock()
			posts++
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, badPage)
	}))
	t.Cleanup(site.Close)
	cast := t.TempDir()
	dir := filepath.Join(cast, "reader", "scenarios")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"reader/profile.md":                "---\nid: reader\nrole: visitor\ndevice: desktop\n---\n# Reader\n\nReads pages.\n",
		"reader/prompt.md":                 "You are a reader.\n",
		"reader/scenarios/01-subscribe.md": "# Subscribe\n\nSubscribe with an email.\n",
	} {
		if err := os.WriteFile(filepath.Join(cast, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			h := newHarness(t, Config{Browser: browser, CastDirs: []string{cast},
				NewProvider: func() (provider.Provider, error) { return &scriptedModel{}, nil }})
			var sweep A11ySweepOutput
			res := h.call("a11y_sweep", map[string]any{"url": site.URL + "/"}, &sweep)
			if res.IsError {
				t.Fatal(text(res))
			}
			found := false
			for _, vi := range sweep.Violations {
				if vi.Rule == "button-name" && strings.Contains(vi.Selector, "nameless") && len(vi.WCAG) > 0 && vi.HelpURL != "" {
					found = true
				}
			}
			if !found || len(sweep.Errors) != 0 {
				t.Errorf("sweep = %+v", sweep)
			}
			var run RunOutput
			res = h.call("run_persona", map[string]any{"url": site.URL + "/", "persona": "reader", "scenario_id": "subscribe", "max_steps": 6}, &run)
			if res.IsError {
				t.Fatal(text(res))
			}
			if run.Outcome == "" || run.RecordPath == "" || len(run.Findings) != 1 || len(run.Findings[0].CitedSteps) != 1 ||
				run.Findings[0].Title != "The first button has no name" {
				t.Errorf("run = %+v", run)
			}
			mu.Lock()
			defer mu.Unlock()
			if posts != 0 {
				t.Errorf("%d POST requests reached the read-only site", posts)
			}
		})
	}
}
