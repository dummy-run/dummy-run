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
