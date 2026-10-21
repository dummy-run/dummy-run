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

const kbGood = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Good</title>
<style>a:focus-visible,button:focus-visible{outline:3px solid #005fcc}</style></head><body>
<a href="#main" id="skip">Skip to main content</a>
<nav><a href="/a">First</a> <a href="/b">Second</a></nav>
<main id="main"><h1>Good</h1><button>Press</button></main></body></html>`

const kbBad = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Bad</title>
<style>a,button{outline:none}</style></head><body>
<a href="#nowhere">Skip to content</a>
<nav><a href="/a">First</a> <a href="/b">Second</a></nav>
<main><h1>Bad</h1><button id="plain">Plain</button>
<a id="ghost" href="/ghost" style="position:absolute;left:-9999px">Ghost</a></main></body></html>`

const kbTrap = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Trap</title>
<style>a,button,input{outline:2px solid #005fcc}</style></head><body>
<main><h1>Trap</h1><a href="/before">Before</a>
<div id="box"><input id="one" aria-label="one"><input id="two" aria-label="two"></div></main>
<script>
document.getElementById("box").addEventListener("keydown", function (e) {
  if (e.key !== "Tab") return;
  e.preventDefault();
  var a = document.getElementById("one"), b = document.getElementById("two");
  (document.activeElement === a ? b : a).focus();
});
</script></body></html>`

func keyboardServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range map[string]string{"/good": kbGood, "/bad": kbBad, "/trap": kbTrap} {
		body := body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, body)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestKeyboardFindingsFromAWalk(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := keyboardServer(t)
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/good", srv.URL + "/bad", srv.URL + "/trap"}, Browser: browser,
				Viewports: []Viewport{Desktop}, Schemes: []string{Light}, Headless: true, Keyboard: true})
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]map[string]string{}
			for _, r := range rep.Results {
				if r.Error != "" {
					t.Fatalf("%s: %s", r.URL, r.Error)
				}
				if !r.KeyboardChecked {
					t.Fatalf("%s: the Tab walk did not run", r.URL)
				}
				m := map[string]string{}
				for _, f := range r.Violations {
					if IsKeyboardRule(f.Rule) {
						m[f.Rule] = f.Nodes[0].Target
					}
				}
				got[strings.TrimPrefix(r.URL, srv.URL)] = m
			}
			if len(got["/good"]) != 0 {
				t.Errorf("the good page has keyboard findings: %v", got["/good"])
			}
			for _, rule := range []string{RuleFocusNotVisible, RuleFocusHidden, RuleSkipLinkBroken} {
				if got["/bad"][rule] == "" {
					t.Errorf("the bad page lacks %s: %v", rule, got["/bad"])
				}
			}
			if got["/trap"][RuleKeyboardTrap] == "" {
				t.Errorf("the trap page lacks %s: %v", RuleKeyboardTrap, got["/trap"])
			}
		})
	}
}
