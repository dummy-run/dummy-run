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

// kbLong has more links than one walk presses Tab for, so the walk stops before it is done.
var kbLong = "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>Long</title><style>a{outline:2px solid #005fcc}</style></head><body><main><h1>Long</h1>" +
	strings.Repeat("<a href=\"/x\">Link</a> ", 100) + "</main></body></html>"

// kbEmpty has nothing that takes focus, so its walk is complete with no stops.
const kbEmpty = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Empty</title></head><body><main><h1>Empty</h1><p>Only text.</p></main></body></html>`

func keyboardServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range map[string]string{"/good": kbGood, "/bad": kbBad, "/trap": kbTrap, "/long": kbLong, "/empty": kbEmpty} {
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
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/good", srv.URL + "/bad", srv.URL + "/bad#plain", srv.URL + "/trap", srv.URL + "/long", srv.URL + "/empty"}, Browser: browser,
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
				if partial := strings.HasSuffix(r.URL, "/long") || strings.HasSuffix(r.URL, "/trap"); r.KeyboardPartial != partial {
					t.Errorf("%s: keyboard_partial is %v, want %v", r.URL, r.KeyboardPartial, partial)
				}
				m := map[string]string{}
				for _, f := range r.Violations {
					if IsKeyboardRule(f.Rule) {
						m[f.Rule] = f.Nodes[0].Target
					}
				}
				got[strings.TrimPrefix(r.URL, srv.URL)] = m
			}
			if len(got["/good"]) != 0 || len(got["/empty"]) != 0 {
				t.Errorf("the good or empty page has keyboard findings: %v %v", got["/good"], got["/empty"])
			}
			// A page opened at a #fragment starts Tab from the top, so the skip link is still the first stop.
			if got["/bad#plain"][RuleSkipLinkBroken] == "" {
				t.Errorf("the bad page opened at a fragment lacks %s: %v", RuleSkipLinkBroken, got["/bad#plain"])
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

func TestKeyboardFindingsFromHandBuiltWalks(t *testing.T) {
	walk := &page.TabWalk{
		Trap: []page.TabStop{{Selector: "#one"}, {Selector: "#two"}},
		Stops: []page.TabStop{
			{Selector: "#plain", NoIndicator: true, Checked: true},
			{Selector: "#ghost", Hidden: true},
			{Selector: "#far", Offscreen: true},
		},
		Skip: &page.SkipLink{Selector: "#skip", Text: "Skip to content", Href: "#main", Found: true, Rendered: false},
	}
	allow := &AllowList{Entries: []*AllowEntry{{Rule: RuleFocusNotVisible, Page: "/p", Selector: "#plain", Reason: "drawn by the theme"}}}
	got := keyboardFindings(walk, "/p", allow)

	var order []string
	byRule := map[string]Finding{}
	for _, f := range got {
		order = append(order, f.Rule)
		byRule[f.Rule] = f
	}
	want := []string{RuleKeyboardTrap, RuleFocusNotVisible, RuleFocusHidden, RuleSkipLinkBroken}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("rules in order %v, want %v", order, want)
	}
	if n := byRule[RuleKeyboardTrap].Nodes; len(n) != 2 || !strings.Contains(n[0].Detail, "2 elements") {
		t.Errorf("trap nodes: %+v", n)
	}
	if n := byRule[RuleFocusNotVisible].Nodes; len(n) != 1 || !n[0].Allowed || n[0].Reason != "drawn by the theme" {
		t.Errorf("the allow-listed node is not marked: %+v", n)
	}
	if n := byRule[RuleFocusHidden].Nodes; len(n) != 2 || !strings.Contains(n[0].Detail, "hidden") || !strings.Contains(n[1].Detail, "outside the window") {
		t.Errorf("hidden nodes: %+v", n)
	}
	if n := byRule[RuleSkipLinkBroken].Nodes; len(n) != 1 || !strings.Contains(n[0].Detail, "not shown") {
		t.Errorf("a skip link that exists but is not rendered should say so: %+v", n)
	}

	// A skip link whose target is missing says that instead.
	missing := keyboardFindings(&page.TabWalk{Skip: &page.SkipLink{Selector: "#s", Text: "Skip", Href: "#gone"}}, "/p", nil)
	if len(missing) != 1 || !strings.Contains(missing[0].Nodes[0].Detail, "not on the page") {
		t.Errorf("missing target: %+v", missing)
	}
	// A working skip link is not a finding.
	if got := keyboardFindings(&page.TabWalk{Skip: &page.SkipLink{Found: true, Rendered: true}}, "/p", nil); len(got) != 0 {
		t.Errorf("a clean walk gave findings: %+v", got)
	}
}
