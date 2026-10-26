package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

// Two disclosure buttons: one whose handler holds the page for 700 ms, and one that answers at once.
const inpPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>INP</title></head><body><main><h1>INP</h1>
<button id="slow" aria-expanded="false" aria-controls="p1">Slow</button><div id="p1" hidden>One</div>
<button id="fast" aria-expanded="false" aria-controls="p2">Fast</button><div id="p2" hidden>Two</div>
<script>
for (const id of ["slow", "fast"]) {
  const b = document.getElementById(id);
  b.addEventListener("click", () => {
    if (id === "slow") { const end = performance.now() + 700; while (performance.now() < end) {} }
    b.setAttribute("aria-expanded", "true");
    document.getElementById(b.getAttribute("aria-controls")).hidden = false;
  });
}
</script></main></body></html>`

func TestSlowResponsesAreMeasured(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, inpPage)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Keyboard: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" || r.Tasks == nil {
				t.Fatalf("error %q, tasks %+v", r.Error, r.Tasks)
			}
			for _, resp := range r.Tasks.Responses {
				t.Logf("%s %s: %.0f ms %s (%.0f + %.0f + %.0f) on %s", resp.Action, resp.Selector, resp.Duration, resp.Rating, resp.InputDelay, resp.Processing, resp.Presentation, resp.Target)
			}
			var slow, fast bool
			for _, resp := range r.Tasks.Responses {
				if resp.Selector == "#slow" && resp.Action == "click" && resp.Duration >= 650 && resp.Processing >= 600 && resp.Rating == "poor" {
					slow = true
				}
				if resp.Selector == "#fast" && resp.Action == "click" && resp.Duration < slowResponse {
					fast = true
				}
			}
			if !slow || !fast {
				t.Errorf("slow click measured %v, fast click measured %v", slow, fast)
			}
			var advice []Node
			for _, f := range r.Advice {
				if f.Rule == RuleSlowResponse {
					advice = f.Nodes
				}
			}
			for _, f := range r.Violations {
				if f.Rule == RuleSlowResponse {
					t.Error("a slow response is advice, not a failure")
				}
			}
			if len(advice) != 1 || advice[0].Target != "#slow" || !strings.Contains(advice[0].Detail, "in its handlers") {
				t.Errorf("advice %+v", advice)
			}
		})
	}
}

// A disclosure that a click or Space opens, slowly from Space, and Enter leaves alone, so the sweep presses both.
const inpKeysPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Keys</title></head><body><main><h1>Keys</h1>
<div id="toggle" role="button" tabindex="0" aria-expanded="false" aria-controls="p1">More</div><div id="p1" hidden>One</div>
<script>
const b = document.getElementById("toggle");
const open = () => { b.setAttribute("aria-expanded", "true"); document.getElementById("p1").hidden = false; };
b.addEventListener("click", open);
b.addEventListener("keydown", (e) => {
  if (e.key !== " ") return;
  e.preventDefault();
  const end = performance.now() + 700; while (performance.now() < end) {}
  open();
});
</script></main></body></html>`

func TestKeyboardResponsesAreMeasured(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, inpKeysPage)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Keyboard: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" || r.Tasks == nil {
				t.Fatalf("error %q, tasks %+v", r.Error, r.Tasks)
			}
			var enter, space *page.Response
			for i, resp := range r.Tasks.Responses {
				t.Logf("%s %s: %.0f ms, earlier %v, on %s", resp.Action, resp.Selector, resp.Duration, resp.Earlier, resp.Target)
				switch {
				case resp.Selector == "#toggle" && resp.Action == "enter":
					enter = &r.Tasks.Responses[i]
				case resp.Selector == "#toggle" && resp.Action == "space":
					space = &r.Tasks.Responses[i]
				}
			}
			if enter == nil || space == nil {
				t.Fatalf("enter %+v, space %+v, want a response to each", enter, space)
			}
			// Enter did nothing slow, so whatever INP describes then is quick, its own or a Tab press's.
			if enter.Duration >= slowResponse {
				t.Errorf("enter: %+v", enter)
			}
			if space.Earlier || space.Duration < 650 || space.Processing < 600 {
				t.Errorf("space: %+v, want its own slow response", space)
			}
			var advice []Node
			for _, f := range r.Advice {
				if f.Rule == RuleSlowResponse {
					advice = f.Nodes
				}
			}
			if len(advice) != 1 || advice[0].Target != "#toggle" || !strings.Contains(advice[0].Detail, "press of Space") {
				t.Errorf("advice %+v", advice)
			}
		})
	}
}
