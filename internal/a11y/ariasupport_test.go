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

func TestARIASupportFiguresLoad(t *testing.T) {
	if len(ariaSupportFor("button")) == 0 || len(ariaSupportFor("tablist")) == 0 {
		t.Fatal("the ARIA-AT figures didn't load")
	}
	if ariaSupportFor("not-a-feature") != nil {
		t.Error("a feature ARIA-AT doesn't test has figures")
	}
}

func TestARIASupportAdvice(t *testing.T) {
	// In the figures fetched on 2026-10-26, grid is below 60 percent for two pairs and button is at 90 or more for
	// all three. Update the expected figures when the data is fetched again.
	got := ariaSupportAdvice([]page.ARIAFeature{
		{Name: "grid", Selector: "#g", Count: 2}, {Name: "button", Selector: "#b", Count: 9}, {Name: "made-up", Selector: "#m", Count: 1},
	}, "/", nil)
	if len(got) != 1 || len(got[0].Nodes) != 1 || got[0].Rule != RuleARIASupport {
		t.Fatalf("got %+v", got)
	}
	n := got[0].Nodes[0]
	if n.Target != "#g" || !strings.Contains(n.Detail, "role grid: 2 elements use it") || !strings.Contains(n.Detail, "NVDA with Chrome 18% (2 of 11)") {
		t.Errorf("node %+v", n)
	}
	if ariaSupportAdvice([]page.ARIAFeature{{Name: "button", Selector: "#b", Count: 1}}, "/", nil) != nil {
		t.Error("a well supported feature was reported")
	}
}

func TestARIASupportInTheBrowser(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	const body = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>ARIA</title></head><body><main><h1>ARIA</h1>
<div id="g" role="future-role grid" aria-label="Prices"><div role="row"><span role="gridcell">1</span></div></div>
<div role="grid" aria-label="More prices"><div role="row"><span role="gridcell">2</span></div></div>
<div hidden><div id="t" role="tablist"><button role="tab">One</button></div></div>
<input id="o" aria-orientation="vertical" type="range"><button id="b">Plain</button><span role="button" tabindex="0">Fake</span>
</main></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Markup: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" {
				t.Fatal(r.Error)
			}
			var nodes []Node
			for _, f := range r.Advice {
				if f.Rule == RuleARIASupport {
					nodes = f.Nodes
				}
			}
			for _, f := range r.Violations {
				if f.Rule == RuleARIASupport {
					t.Error("ARIA support is advice, not a failure")
				}
			}
			var grid bool
			for _, n := range nodes {
				if strings.Contains(n.Detail, "role grid:") {
					grid = n.Target == "#g" && strings.Contains(n.Detail, "2 elements use it")
				}
				if strings.Contains(n.Detail, "tablist") || strings.Contains(n.Detail, "role button") || strings.Contains(n.Detail, "role gridcell:") && n.Target == "#g" {
					t.Errorf("reported %s: %s", n.Target, n.Detail)
				}
			}
			var orientation bool
			for _, n := range nodes {
				orientation = orientation || (n.Target == "#o" && strings.Contains(n.Detail, "aria-orientation:"))
			}
			if !orientation {
				t.Errorf("an element with only aria-orientation isn't reported: %+v", nodes)
			}
			if !grid {
				t.Errorf("no grid advice on #g counting 2: %+v", nodes)
			}
		})
	}
}
