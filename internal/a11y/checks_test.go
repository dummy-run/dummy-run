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

const markupPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Markup</title></head><body><main><h1>Markup</h1>
<a id="bad1" href="/a" aria-label="Open the account page">Sign in</a>
<button id="bad2" aria-label="Go">Search the shop</button>
<a id="ok1" href="/b" aria-label="Sign in to your account">Sign in</a>
<button id="ok2" aria-label="Close">&times;</button>
<a id="ok3" href="/c">Plain link</a>
<button id="ok4" aria-label="Search, the whole site">Search the whole site</button>
<form action="/join">
<label for="e1">Your email</label><input id="e1" type="email" name="email">
<label for="e2">Email</label><input id="e2" type="email" name="email2" autocomplete="email">
<label for="t1">Phone</label><input id="t1" type="tel" autocomplete="off">
<label for="f1">First name</label><input id="f1" type="text" name="first_name">
<label for="e3">Friend's email</label><input id="e3" type="email" name="friend">
<label for="s1">Search</label><input id="s1" type="text" name="q">
</form>
<form role="search" action="/s"><label for="e4">Email</label><input id="e4" type="email"></form>
</main></body></html>`

func TestMarkupChecksFindOnlyPlantedProblems(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, markupPage)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Checks: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" || !r.ChecksRan {
				t.Fatalf("error %q ran %v", r.Error, r.ChecksRan)
			}
			got := map[string][]string{}
			for _, f := range r.Violations {
				if IsMarkupRule(f.Rule) {
					for _, n := range f.Nodes {
						got[f.Rule] = append(got[f.Rule], n.Target)
					}
				}
			}
			want := map[string]string{
				RuleLabelInName:     "#bad1,#bad2",
				RuleAutocompleteOff: "#e1,#t1,#f1",
			}
			for rule, w := range want {
				if g := strings.Join(got[rule], ","); g != w {
					t.Errorf("%s: got %q want %q", rule, g, w)
				}
			}
		})
	}
}
