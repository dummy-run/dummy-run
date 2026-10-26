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

func TestJourneysInTheSweep(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><title>J</title><header>
<button id="menu" aria-expanded="false" aria-controls="nav" onmousedown="this.setAttribute('aria-expanded','true');document.getElementById('nav').hidden=false">Menu</button>
<ul id="nav" hidden><li><a href="/a">A</a></li></ul></header><main><h1>J</h1>
<label for="q">Search</label><input id="q" type="search" name="q"></main></html>`)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop, Phone},
				Schemes: []string{Light}, Headless: true, Journeys: true})
			if err != nil {
				t.Fatal(err)
			}
			first, second := rep.Results[0], rep.Results[1]
			if first.JourneysError != "" || len(first.Journeys) != 2 || len(second.Journeys) != 0 {
				t.Fatalf("journeys %+v (error %q), then %d at the second size", first.Journeys, first.JourneysError, len(second.Journeys))
			}
			var nodes []Node
			for _, f := range first.Violations {
				if f.Rule == RuleJourneyKeyboard {
					nodes = f.Nodes
				}
			}
			if len(nodes) != 1 || nodes[0].Target != "#menu" || !strings.Contains(nodes[0].Detail, "menu") {
				t.Errorf("violation nodes %+v, want the menu only", nodes)
			}
		})
	}
}

// A page whose journeys couldn't be tried says so in the terminal summary, without the control characters a page
// could put in the message.
func TestSummaryListsJourneyErrors(t *testing.T) {
	rep := &Report{Browser: "chrome", Results: []PageResult{
		{URL: "https://example.com/a", Browser: "chrome", Viewport: "desktop", Scheme: "light", JourneysError: "load\u001b[2K failed"},
		{URL: "https://example.com/b", Browser: "chrome", Viewport: "desktop", Scheme: "light"},
	}}
	rep.summarise(Config{Allow: &AllowList{}})
	var b strings.Builder
	rep.WriteSummary(&b)
	out := b.String()
	for _, want := range []string{"JOURNEYS https://example.com/a (chrome/desktop/light): load[2K failed", "journeys not tried on 1 pages"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
	if strings.ContainsRune(out, '\u001b') {
		t.Errorf("summary keeps a control character:\n%q", out)
	}
}
