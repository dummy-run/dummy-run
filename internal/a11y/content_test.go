package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// Each planted problem has an id starting "bad" and the rule that should find it. Elements starting "ok" are close to
// a problem but fine.
const contentPage = `<!doctype html><html lang="en-GB"><head><meta charset="utf-8"><title>Content</title></head><body><main><h1>Content</h1>
<p>News <a id="bad-vague" href="/n1">Read more</a></p>
<p><a id="bad-vague2" href="/n2">Click here &raquo;</a></p>
<p><a id="ok-vague" href="/n3">Read more about our opening hours</a></p>
<p><a id="ok-vague2" href="/n4" aria-label="Read more about the library">Read more</a></p>
<p><a id="bad-url" href="https://example.com/a/very/long/path/to/a/page">https://example.com/a/very/long/path/to/a/page</a></p>
<p><a id="ok-url" href="https://example.com/">example.com</a></p>
<p><a id="bad-new" href="/x" target="_blank">Our partners</a></p>
<p><a id="ok-new" href="/y" target="_blank">Our partners (opens in a new tab)</a></p>
<p><a id="bad-doc" href="/files/report.pdf">Annual report</a></p>
<p><a id="ok-doc" href="/files/report2.pdf">Annual report (PDF, 2 MB)</a></p>
<p><a id="ok-doc2" href="/files/pdf-guide">How to read a PDF</a></p>
<p><a id="bad-doc2" href="/files/plan.docx">Download the plan</a></p>
<p><a id="bad-doc3" href="/files/plan2.pdf" title="PDF">The plan</a></p>
<p><a id="bad-new2" href="/x2" target="_blank">Partners (external)</a></p>
<p><a id="bad-new3" href="/x3" target="_blank" title="Opens in a new tab">Suppliers</a></p>
<p><a id="ok-new2" href="/y2" target="_blank">Suppliers <svg aria-label="opens in a new window" width="8" height="8"></svg></a></p>
<p><a id="bad-new4" href="/x4" target="_blank">Makers <svg aria-hidden="true"><g aria-label="opens in a new window"></g></svg><svg role="img" aria-label="opens in a new tab" aria-hidden="true" width="8" height="8"></svg></a></p>
<p><a id="bad-doc4" href="/files/plan3.pdf">The plan (Word)</a></p>
<p><a id="ok-doc3" href="/files/plan4.xlsx">The figures (spreadsheet)</a></p>
<img id="bad-file" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="IMG_20240101.jpg">
<img id="bad-file2" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="hero-banner.png">
<img id="ok-file" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="The pier at sunset">
<img id="ok-file2" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="Photo 2024 of the team">
<img id="bad-file3" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="DSC_0042">
<img id="bad-redundant" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="Image of a dog on a beach">
<img id="ok-redundant" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="Imagery from the 1920s archive">
<img id="bad-long" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="` + "A long description of a chart that goes on and on about every bar and every line and every label so that it runs past the limit of one hundred and fifty characters easily" + `">
<img id="ok-empty" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="">
<p id="bad-fake"><strong>Opening times</strong></p>
<p>We are open every day.</p>
<p id="ok-fake"><strong>Note:</strong> the shop is closed on bank holidays.</p>
<p id="ok-fake2"><b>Thanks for reading.</b></p>
<p>Last paragraph.</p>
<p id="bad-caps">PLEASE READ THESE TERMS CAREFULLY BEFORE YOU ORDER</p>
<p id="bad-caps2" style="text-transform:uppercase">please read these terms carefully before you order</p>
<p id="ok-caps">The NHS and the BBC and the DVLA are short capitals in normal text.</p>
<p>Some <span id="bad-underline" style="text-decoration:underline">important words</span> here.</p>
<p>Some <u id="bad-underline2">more words</u> here.</p>
<p>Some <abbr id="ok-underline" title="for example">e.g.</abbr> and <a id="ok-underline2" href="/z">a real link</a>.</p>
<p hidden><a id="ok-hidden" href="/h">Read more</a></p>
</main></body></html>`

// A page not in English gets none of the phrase checks.
const welshPage = `<!doctype html><html lang="cy"><head><meta charset="utf-8"><title>Cynnwys</title></head><body><main><h1>Cynnwys</h1>
<p><a id="cy-vague" href="/n1">Read more</a></p>
<img id="cy-file" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="20" height="20" alt="IMG_20240101.jpg">
</main></body></html>`

func TestContentChecksFindOnlyPlantedProblems(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/cy" {
			_, _ = fmt.Fprint(w, welshPage)
			return
		}
		_, _ = fmt.Fprint(w, contentPage)
	}))
	defer srv.Close()
	want := map[string][]string{
		RuleVagueLink:    {"#bad-vague", "#bad-vague2"},
		RuleURLLink:      {"#bad-url"},
		RuleNewWindow:    {"#bad-new", "#bad-new2", "#bad-new3", "#bad-new4"},
		RuleDocumentLink: {"#bad-doc", "#bad-doc2", "#bad-doc3", "#bad-doc4"},
		RuleAltFileName:  {"#bad-file", "#bad-file2", "#bad-file3"},
		RuleAltRedundant: {"#bad-redundant"},
		RuleAltLong:      {"#bad-long"},
		RuleFakeHeading:  {"#bad-fake"},
		RuleAllCaps:      {"#bad-caps", "#bad-caps2"},
		RuleUnderline:    {"#bad-underline", "#bad-underline2"},
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/", srv.URL + "/cy"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Content: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rep.Results {
				if r.Error != "" || !r.ContentChecked {
					t.Fatalf("%s: error %q ran %v", r.URL, r.Error, r.ContentChecked)
				}
				for _, f := range r.Violations {
					if IsContentRule(f.Rule) {
						t.Errorf("%s fails the sweep; content checks are advice", f.Rule)
					}
				}
				got := map[string][]string{}
				for _, f := range r.Advice {
					if !IsContentRule(f.Rule) {
						continue
					}
					for _, n := range f.Nodes {
						if n.Detail == "" {
							t.Errorf("%s %s has no detail", f.Rule, n.Target)
						}
						got[f.Rule] = append(got[f.Rule], n.Target)
					}
				}
				for _, ts := range got {
					slices.Sort(ts)
				}
				if strings.HasSuffix(r.URL, "/cy") {
					// The file name check isn't about words, so it still runs on a page in Welsh.
					if !reflect.DeepEqual(got, map[string][]string{RuleAltFileName: {"#cy-file"}}) {
						t.Errorf("Welsh page: got %v", got)
					}
					continue
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("got  %v\nwant %v", got, want)
				}
			}
		})
	}
}
