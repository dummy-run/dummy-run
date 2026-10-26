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

	"github.com/dummy-run/dummy-run/internal/lighthouse"
)

const lighthousePage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Lighthouse</title></head><body><main><h1>Lighthouse</h1>
<img id="pic" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=">
<label for="pw">Password</label><input id="pw" type="password" onpaste="return false">
</main></body></html>`

func TestLighthouseConfirmsAxeAndAddsAdvice(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	if _, err := lighthouse.Find(); err != nil {
		t.Skip("lighthouse is not installed: ", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, lighthousePage)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop, Phone},
				Schemes: []string{Light}, Headless: true, Lighthouse: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if browser != "chrome" {
				if r.LighthouseChecked || r.LighthouseError == "" {
					t.Errorf("%s: checked %v, error %q; Lighthouse runs only in Chrome", browser, r.LighthouseChecked, r.LighthouseError)
				}
				return
			}
			if !r.LighthouseChecked || r.LighthouseError != "" {
				t.Fatalf("checked %v error %q", r.LighthouseChecked, r.LighthouseError)
			}
			if rep.Results[1].LighthouseChecked {
				t.Error("the same URL was audited again at the second window size")
			}
			if r.LighthouseScores["accessibility"] <= 0 || r.LighthouseScores["accessibility"] >= 1 {
				t.Errorf("accessibility score %v", r.LighthouseScores["accessibility"])
			}
			confirmed := false
			for _, f := range r.Violations {
				for _, n := range f.Nodes {
					if f.Rule == "image-alt" && n.Lighthouse {
						confirmed = true
					}
				}
			}
			if !confirmed || r.LighthouseAgreed == 0 {
				t.Errorf("Lighthouse did not confirm axe's image-alt: agreed %d %+v", r.LighthouseAgreed, r.Violations)
			}
			paste := false
			for _, f := range r.Advice {
				if f.Rule == RuleLighthousePrefix+"paste-preventing-inputs" && len(f.Nodes) > 0 {
					paste = true
				}
			}
			if !paste {
				t.Errorf("no advice for the field that blocks paste: %+v", r.Advice)
			}
			for _, f := range r.Candidates {
				if f.Rule == RuleLighthousePrefix+"image-alt" {
					t.Errorf("an element axe already failed is a candidate too: %+v", f)
				}
			}
		})
	}
}

// TestFoldLighthouseMatchesEachRule folds a Lighthouse result into a page without Chrome. The image fails two axe
// rules, and Lighthouse's image-alt failure must confirm the image-alt node, not the first one on the element.
func TestFoldLighthouseMatchesEachRule(t *testing.T) {
	page := func() *PageResult {
		return &PageResult{Violations: []Finding{
			{Rule: "color-contrast", Nodes: []Node{{Target: "#pic"}}},
			{Rule: "image-alt", Nodes: []Node{{Target: "#pic"}}},
		}}
	}
	r := &lighthouse.Result{Failures: []lighthouse.Failure{
		{ID: "image-alt", Category: "accessibility", Items: []lighthouse.Item{{Selector: "img.pic"}}},
		{ID: "label", Category: "accessibility", Items: []lighthouse.Item{{Selector: "input"}}},
		{ID: "document-title", Category: "accessibility"},
		{ID: "deprecations", Category: "best-practices", Items: []lighthouse.Item{{Selector: "html", Label: "API"}}},
	}}
	res := page()
	refs, targets := lighthouseRefs(res)
	if len(refs) != 2 || targets[0] != "#pic" || targets[1] != "#pic" {
		t.Fatalf("refs %v targets %v", refs, targets)
	}
	// The browser finds both axe nodes on the image, and nothing for the input.
	foldLighthouse(res, r, refs, [][]int{{0, 1}, {}}, "/", nil)
	if res.Violations[0].Nodes[0].Lighthouse || !res.Violations[1].Nodes[0].Lighthouse || res.LighthouseAgreed != 1 {
		t.Errorf("image-alt should confirm only the image-alt node: %+v, agreed %d", res.Violations, res.LighthouseAgreed)
	}
	var cands []string
	for _, c := range res.Candidates {
		cands = append(cands, c.Rule)
	}
	if strings.Join(cands, ",") != "dummyrun-lighthouse-label,dummyrun-lighthouse-document-title" {
		t.Errorf("candidates %v: want the unmatched input and the page-wide audit", cands)
	}
	if len(res.Advice) != 1 || res.Advice[0].Rule != "dummyrun-lighthouse-deprecations" {
		t.Errorf("advice %+v", res.Advice)
	}

	// When matching failed, element-level accessibility failures are left out rather than reported twice.
	res = page()
	res.LighthouseMatchError = "lighthouse: matching failed"
	foldLighthouse(res, r, refs, [][]int{nil, nil}, "/", nil)
	if res.LighthouseAgreed != 0 || len(res.Candidates) != 1 || res.Candidates[0].Rule != "dummyrun-lighthouse-document-title" {
		t.Errorf("with matching failed: agreed %d, candidates %+v", res.LighthouseAgreed, res.Candidates)
	}
}
