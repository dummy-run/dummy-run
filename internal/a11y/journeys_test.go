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
