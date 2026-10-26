package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/journey"
)

func TestJourneysInTheSweep(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	// A copy of webappanalyzer in the environment could suggest more journeys.
	t.Setenv("DUMMYRUN_WEBAPPANALYZER", "")
	var basket atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /basket\n")
			return
		case "/basket":
			basket.Add(1)
		case "/join":
			// Sign-up redirects to the basket, which robots.txt disallows, so the tab mustn't follow it.
			http.Redirect(w, r, "/basket", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><title>J</title><header>
<button id="menu" aria-expanded="false" aria-controls="nav" onmousedown="this.setAttribute('aria-expanded','true');document.getElementById('nav').hidden=false">Menu</button>
<ul id="nav" hidden><li><a href="/a">A</a></li></ul></header><main><h1>J</h1>
<label for="q">Search</label><input id="q" type="search" name="q"> <a href="/basket">Basket</a> <a href="/join">Sign up</a></main></html>`)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			basket.Store(0)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop, Phone},
				Schemes: []string{Light}, Headless: true, Journeys: true, RespectRobots: true})
			if err != nil {
				t.Fatal(err)
			}
			first, second := rep.Results[0], rep.Results[1]
			if first.JourneysError != "" || len(first.Journeys) != 4 || len(second.Journeys) != 0 {
				t.Fatalf("journeys %+v (error %q), then %d at the second size", first.Journeys, first.JourneysError, len(second.Journeys))
			}
			// robots.txt disallows the basket, so its journey is skipped, and the redirect to it from /join is stopped, so the
			// basket is never loaded.
			for _, j := range first.Journeys {
				if j.Kind == "basket" && (j.Mouse.Outcome != journey.Skipped || !strings.Contains(j.Mouse.Detail, "robots.txt")) {
					t.Errorf("basket journey %+v, want skipped for robots.txt", *j.Mouse)
				}
			}
			if n := basket.Load(); n != 0 {
				t.Errorf("the basket was loaded %d times", n)
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
