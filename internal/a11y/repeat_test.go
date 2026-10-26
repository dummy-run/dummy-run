package a11y

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func repeatResult(err string, rules ...string) PageResult {
	p := PageResult{URL: "https://example.com/", Browser: "chrome", Viewport: "desktop", Scheme: Light, Error: err,
		Violations: []Finding{}, NeedsReview: []Finding{}}
	for _, r := range rules {
		rule, target, _ := strings.Cut(r, "@")
		p.Violations = append(p.Violations, Finding{Rule: rule, Help: rule, Nodes: []Node{{Target: target}}})
	}
	return p
}

// TestMergeRepeatsCountsRuns checks each element counts the repeats it failed in, including ones that turn up only
// in a later repeat, and each page counts the repeats that could not test it.
func TestMergeRepeatsCountsRuns(t *testing.T) {
	other := repeatResult("")
	other.Viewport = "phone"
	got := mergeRepeats([][]PageResult{
		{repeatResult("", "image-alt@img", "label@#q"), other},
		{repeatResult("", "image-alt@img"), other},
		{repeatResult("timed out", "image-alt@img", "image-alt@#logo", "color-contrast@p"), other},
	})
	if len(got) != 2 {
		t.Fatalf("%d results, want one for each setting", len(got))
	}
	p := got[0]
	runs := map[string]int{}
	for _, f := range p.Violations {
		for _, n := range f.Nodes {
			runs[f.Rule+"@"+n.Target] = n.Runs
		}
	}
	want := map[string]int{"image-alt@img": 3, "label@#q": 1, "image-alt@#logo": 1, "color-contrast@p": 1}
	for k, n := range want {
		if runs[k] != n {
			t.Errorf("%s: %d runs, want %d", k, runs[k], n)
		}
	}
	if len(runs) != len(want) || len(p.Violations) != 3 {
		t.Errorf("violations %+v", p.Violations)
	}
	if p.Repeats != 3 || *p.ErrorRuns != 1 || p.Error != "timed out" {
		t.Errorf("repeats %d, error runs %d, error %q", p.Repeats, *p.ErrorRuns, p.Error)
	}
	if got[1].Repeats != 3 || *got[1].ErrorRuns != 0 || got[1].Violations == nil {
		t.Errorf("other setting: %+v", got[1])
	}
	// A repeated result that never failed still says so in the JSON, and a single sweep's result leaves it out.
	if b, _ := json.Marshal(got[1]); !strings.Contains(string(b), `"error_runs":0`) {
		t.Errorf("repeated result JSON has no error_runs: %s", b)
	}
	if b, _ := json.Marshal(PageResult{}); strings.Contains(string(b), "error_runs") {
		t.Errorf("single result JSON has error_runs: %s", b)
	}
	if mergeRepeats(nil) != nil {
		t.Error("no runs should merge to nothing")
	}
}

// TestMergeRepeatsKeepsDuplicateURLsApart checks a URL given twice in one sweep, such as from the sitemap and the
// command line, keeps two results that each count once per repeat.
func TestMergeRepeatsKeepsDuplicateURLsApart(t *testing.T) {
	run := []PageResult{repeatResult("", "image-alt@img"), repeatResult("", "image-alt@img")}
	got := mergeRepeats([][]PageResult{run, run})
	if len(got) != 2 {
		t.Fatalf("%d results, want 2", len(got))
	}
	for _, p := range got {
		if n := p.Violations[0].Nodes[0].Runs; n != 2 || p.Repeats != 2 {
			t.Errorf("runs %d of %d, want 2 of 2", n, p.Repeats)
		}
	}
}

// TestMergeRepeatsCountsAnElementOncePerRepeat checks an element a check reports twice in one run counts each
// report once per repeat, never twice.
func TestMergeRepeatsCountsAnElementOncePerRepeat(t *testing.T) {
	twice := repeatResult("", "dummyrun-clipped-text@#buy", "dummyrun-clipped-text@#buy")
	once := repeatResult("", "dummyrun-clipped-text@#buy")
	got := mergeRepeats([][]PageResult{{twice}, {once}, {twice}})[0]
	var runs []int
	for _, f := range got.Violations {
		for _, n := range f.Nodes {
			runs = append(runs, n.Runs)
		}
	}
	if len(runs) != 2 || runs[0] != 3 || runs[1] != 2 {
		t.Errorf("runs %v, want [3 2]", runs)
	}
}

// TestMergeRepeatsKeepsTheResultThatRan checks a page that failed to load in the first repeat and loaded in the
// second keeps the second's checks, with the first's error beside them.
func TestMergeRepeatsKeepsTheResultThatRan(t *testing.T) {
	failed := repeatResult("load: timed out")
	ok := repeatResult("", "dummyrun-label-in-name@#q")
	ok.MarkupChecked = true
	got := mergeRepeats([][]PageResult{{failed}, {ok}})[0]
	if !got.MarkupChecked || got.Error != "load: timed out" || *got.ErrorRuns != 1 || len(got.Violations) != 1 || got.Violations[0].Nodes[0].Runs != 1 {
		t.Errorf("%+v", got)
	}
	// Only the first repeat takes the --visual picture, so the merged result has it even when a later result is kept.
	failed.Visual = &Visual{First: true}
	if got := mergeRepeats([][]PageResult{{failed}, {ok}})[0]; got.Visual == nil || !got.Visual.First {
		t.Errorf("visual %+v", got.Visual)
	}
	// Only the first repeat takes the ARIA snapshot, so the merged result has it even when a later result is kept.
	failed.ARIASnapshot, failed.ARIAChanges = "shop.yaml", &ARIAChanges{AddedCount: 1}
	if got := mergeRepeats([][]PageResult{{failed}, {ok}})[0]; got.ARIASnapshot != "shop.yaml" || got.ARIAChanges == nil || got.ARIAChanges.AddedCount != 1 {
		t.Errorf("snapshot %q, changes %+v", got.ARIASnapshot, got.ARIAChanges)
	}
}

func TestSummaryShowsRepeatCounts(t *testing.T) {
	r := &Report{Repeats: 2, Results: mergeRepeats([][]PageResult{
		{repeatResult("", "image-alt@img")},
		{repeatResult("timed out")},
	})}
	r.summarise(Config{})
	var b bytes.Buffer
	r.WriteSummary(&b)
	for _, want := range []string{"in chrome/desktop/light (1 of 2 runs)", "ERROR https://example.com/ (chrome/desktop/light, 1 of 2 runs): timed out", ", each run 2 times"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("summary lacks %q:\n%s", want, b.String())
		}
	}
	if r.Summary.Failing != 1 {
		t.Errorf("failing %d, want the element once", r.Summary.Failing)
	}
}

// TestRepeatCountsAFlakyElement sweeps a page twice that leaves one image's alt text out only on its first load.
func TestRepeatCountsAFlakyElement(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	var loads atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		alt := ` alt="Logo"`
		if loads.Add(1)%2 == 1 {
			alt = ""
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Repeat</title></head>
<body><main><h1>Repeat</h1><img id="always" src="data:image/gif;base64,R0lGODlhAQABAAAAACw="><img id="flaky"%s src="data:image/gif;base64,R0lGODlhAQABAAAAACw="></main></body></html>`, alt)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			loads.Store(0)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Repeat: 2})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Repeats != 2 || len(rep.Results) != 1 || rep.Results[0].Repeats != 2 {
				t.Fatalf("repeats %d, %d results", rep.Repeats, len(rep.Results))
			}
			runs := map[string]int{}
			for _, f := range rep.Results[0].Violations {
				if f.Rule == "image-alt" {
					for _, n := range f.Nodes {
						runs[n.Target] = n.Runs
					}
				}
			}
			if runs["#always"] != 2 || runs["#flaky"] != 1 {
				t.Errorf("runs %v, want #always in 2 and #flaky in 1", runs)
			}
		})
	}
}

// TestMergeRepeatsKeepsTheChecksThatRanOnce checks the HTML check and Lighthouse, which run in the first repeat only,
// stay in the merged result when the first repeat's page failed after them and a later result is kept.
func TestMergeRepeatsKeepsTheChecksThatRanOnce(t *testing.T) {
	failed := repeatResult("layout checks: no boxes")
	failed.HTMLChecked, failed.HTMLIgnored = true, 2
	failed.LighthouseChecked, failed.LighthouseScores = true, map[string]float64{"accessibility": 0.9}
	ok := repeatResult("")
	got := mergeRepeats([][]PageResult{{failed}, {ok}})[0]
	if !got.HTMLChecked || got.HTMLIgnored != 2 || !got.LighthouseChecked || got.LighthouseScores["accessibility"] != 0.9 {
		t.Errorf("html %v %d, lighthouse %v %v", got.HTMLChecked, got.HTMLIgnored, got.LighthouseChecked, got.LighthouseScores)
	}
	// A later repeat's own results stay when the first repeat didn't run the checks.
	ok.HTMLChecked = true
	if got := mergeRepeats([][]PageResult{{repeatResult("load: timed out")}, {ok}})[0]; !got.HTMLChecked {
		t.Error("the kept result's HTML check was dropped")
	}
}

// TestMergeRepeatsKeepsTheFailureProbe checks the probe, which runs once for each URL, stays in the merged result
// when the first repeat's page failed after it, and that a probe from a later repeat stays when the first never ran it.
func TestMergeRepeatsKeepsTheFailureProbe(t *testing.T) {
	failed := repeatResult("layout checks: no boxes")
	failed.FailureProbe = &FailureProbe{Failed: 1}
	ok := repeatResult("")
	if got := mergeRepeats([][]PageResult{{failed}, {ok}})[0]; got.FailureProbe == nil || got.FailureProbe.Failed != 1 {
		t.Errorf("probe %+v", got.FailureProbe)
	}
	ok.FailureProbe = &FailureProbe{Failed: 2}
	if got := mergeRepeats([][]PageResult{{repeatResult("axe.run: no")}, {ok}})[0]; got.FailureProbe == nil || got.FailureProbe.Failed != 2 {
		t.Errorf("a later repeat's probe: %+v", got.FailureProbe)
	}
}

// TestMergeRepeatsKeepsErrorsFromTheFirstRepeat checks a check that failed in the first repeat keeps its error in the
// merged result, and so does a failure probe that couldn't run or wasn't tried.
func TestMergeRepeatsKeepsErrorsFromTheFirstRepeat(t *testing.T) {
	failed := repeatResult("layout checks: no boxes")
	failed.HTMLError, failed.LighthouseError, failed.FailureProbeError = "vnu: no Java", "lighthouse: timed out", "probe: no tab"
	got := mergeRepeats([][]PageResult{{failed}, {repeatResult("")}})[0]
	if got.HTMLError != "vnu: no Java" || got.LighthouseError != "lighthouse: timed out" || got.FailureProbeError != "probe: no tab" {
		t.Errorf("html %q, lighthouse %q, probe %q", got.HTMLError, got.LighthouseError, got.FailureProbeError)
	}
	skipped := repeatResult("layout checks: no boxes")
	skipped.FailureProbeSkipped = "not run with preview access"
	if got := mergeRepeats([][]PageResult{{skipped}, {repeatResult("")}})[0]; got.FailureProbeSkipped != "not run with preview access" {
		t.Errorf("probe skipped %q", got.FailureProbeSkipped)
	}
}
