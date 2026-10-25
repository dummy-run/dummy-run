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

	"github.com/dummy-run/dummy-run/internal/validate"
)

func TestHTMLFindingsKeepEachErrorOnce(t *testing.T) {
	served := []validate.Message{
		{Type: "error", Message: "Duplicate ID “a”.", LastLine: 3, Extract: "<p id=\"a\">\n2</p>"},
		{Type: "error", Message: "End tag “b” violates nesting rules.", LastLine: 9},
		{Type: "error", Message: "The “center” element is obsolete. Use CSS instead."},
		{Type: "info", SubType: "warning", Message: "The first occurrence of ID “a” was here."},
	}
	built := []validate.Message{
		{Type: "error", Message: "Duplicate ID “a”."},
		{Type: "error", Message: "Duplicate ID “late”."},
	}
	got, ignored := htmlFindings(served, built, "/", nil)
	if ignored != 1 {
		t.Errorf("ignored %d, want the obsolete element only", ignored)
	}
	if len(got) != 2 || got[0].Rule != "dummyrun-html-duplicate-id" || got[1].Rule != "dummyrun-html-structure" {
		t.Fatalf("findings: %+v", got)
	}
	if len(got[0].Nodes) != 2 {
		t.Errorf("the duplicate in both copies counts once: %+v", got[0].Nodes)
	}
	if d := got[0].Nodes[0].Detail; d != `line 3 of the source: Duplicate ID “a”. In: <p id="a"> 2</p>` {
		t.Errorf("detail %q", d)
	}
}

const htmlPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>HTML</title></head><body><main><h1>HTML</h1>
<p><b><i>misnested</b></i></p>
<button type="button"><a href="/x">inside</a></button>
<center>old</center>
<div id="slot"></div>
<script>const p = document.createElement("p"); p.id = "slot"; document.querySelector("main").append(p);</script>
</main></body></html>`

func TestHTMLCheckReadsTheSourceAndThePage(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	if _, err := validate.Find(); err != nil {
		t.Skip("vnu is not installed: ", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, htmlPage)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop, Phone},
				Schemes: []string{Light}, Headless: true, HTML: true})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Vnu != validate.VnuVersion || len(rep.Results) != 2 {
				t.Fatalf("vnu %q, %d results", rep.Vnu, len(rep.Results))
			}
			r := rep.Results[0]
			if !r.HTMLChecked || !r.HTMLSourceChecked || r.HTMLError != "" {
				t.Fatalf("checked %v source %v error %q", r.HTMLChecked, r.HTMLSourceChecked, r.HTMLError)
			}
			if rep.Results[1].HTMLChecked {
				t.Error("the same URL was checked again at the second window size")
			}
			rules := map[string]string{}
			for _, f := range r.Advice {
				for _, n := range f.Nodes {
					rules[f.Rule] += n.Detail + "\n"
				}
			}
			// Misnesting is only in the source, the script's duplicate ID only in the built page.
			if !strings.Contains(rules["dummyrun-html-structure"], "of the source") {
				t.Errorf("no misnesting from the source: %v", rules)
			}
			if !strings.Contains(rules["dummyrun-html-duplicate-id"], "slot") {
				t.Errorf("no duplicate ID from the script: %v", rules)
			}
			if rules["dummyrun-html-nested-interactive"] == "" {
				t.Errorf("no link inside a button: %v", rules)
			}
			if r.HTMLIgnored == 0 {
				t.Error("the obsolete center element was not counted as ignored")
			}
			for _, f := range r.Violations {
				if strings.HasPrefix(f.Rule, "dummyrun-html-") {
					t.Errorf("an HTML error is a violation: %s", f.Rule)
				}
			}
		})
	}
}
