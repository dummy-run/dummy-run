package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/record"
)

const enginesBad = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Bad</title></head><body><main><h1>Bad</h1>
<img id="pic" src="data:image/gif;base64,R0lGODlhAQABAAAAACw=">
<a id="empty" href="/a"></a>
<input id="field" type="text">
<span id="spill" style="display:inline-block;width:40px;height:20px;text-indent:60px;white-space:nowrap;color:#bbb;background:#ccc">Spills out</span>
</main></body></html>`

const enginesGood = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Good</title></head><body><main><h1>Good</h1>
<img src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" alt="A red dot">
<a href="/a">Read the guide</a>
<label for="f">Name</label><input id="f" type="text">
<button id="go" style="width:40px;height:40px;text-indent:-5000px;overflow:hidden;color:#bbb;background:#ccc;border:2px solid #000">Search</button>
<div style="width:60px;height:24px;overflow:hidden"><span id="hid" style="display:inline-block;margin-left:200px;white-space:nowrap;color:#bbb;background:#ccc">Hidden away</span></div>
</main></body></html>`

func TestEnginesAgreeOnPlantedProblems(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	mux := http.NewServeMux()
	for path, body := range map[string]string{"/bad": enginesBad, "/good": enginesGood} {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, body)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/bad", srv.URL + "/good"}, Browser: browser,
				Viewports: []Viewport{Desktop}, Schemes: []string{Light}, Headless: true, Engines: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Engines) != 4 {
				t.Errorf("report engines: %v", rep.Engines)
			}
			for _, r := range rep.Results {
				if r.Error != "" || len(r.EngineErrors) > 0 {
					t.Fatalf("%s: error %q engine errors %v", r.URL, r.Error, r.EngineErrors)
				}
				if !slices.Equal(r.EnginesRan, []string{"ibm", "htmlcs", "alfa", "qualweb"}) {
					t.Errorf("%s: engines ran %v", r.URL, r.EnginesRan)
				}
				high := map[string][]string{}
				for _, f := range r.Violations {
					// Agreement among the other engines alone is its own finding, on the same element; axe's are checked here.
					if IsEnginesRule(f.Rule) {
						continue
					}
					for _, n := range f.Nodes {
						if n.Confidence == "" {
							t.Errorf("%s: %s %s has no confidence", r.URL, f.Rule, n.Target)
						}
						if len(n.Engines) >= 3 {
							high[n.Target] = n.Engines
						}
					}
				}
				if strings.HasSuffix(r.URL, "/good") {
					if len(r.Violations) != 0 {
						t.Errorf("the good page has violations: %+v", r.Violations)
					}
					// The off-screen label of #go and the text of #hid, which its parent clips away, have no contrast anyone sees.
					for _, c := range r.Candidates {
						for _, n := range c.Nodes {
							if c.Rule == RuleEnginesPrefix+"1.4.3" && (strings.Contains(n.Target, "#go") || strings.Contains(n.Target, "#hid")) {
								t.Errorf("contrast reported for text that is not painted: %+v", n)
							}
						}
					}
					continue
				}
				// #spill's text sits outside its box but is on the page and not clipped, so its contrast still counts.
				spilt := false
				for _, list := range [][]Finding{r.Violations, r.Candidates} {
					for _, f := range list {
						for _, n := range f.Nodes {
							spilt = spilt || (strings.Contains(n.Target, "#spill") && (f.Rule == "color-contrast" || f.Rule == RuleEnginesPrefix+"1.4.3"))
						}
					}
				}
				if !spilt {
					t.Errorf("no contrast failure for text that overflows its element but is seen")
				}
				seen := map[string]bool{}
				for _, sel := range []string{"#pic", "#empty", "#field"} {
					if e := high[sel]; len(e) < 3 || !slices.Contains(e, "axe") {
						t.Errorf("%s: want axe and at least two others to agree, got %v", sel, e)
					}
					for _, name := range high[sel] {
						seen[name] = true
					}
				}
				for _, name := range []string{"axe", "ibm", "htmlcs", "alfa", "qualweb"} {
					if !seen[name] {
						t.Errorf("%s never agreed on a planted problem", name)
					}
				}
			}
		})
	}
}

func TestEngineFindingsSplitAgreementFromCandidates(t *testing.T) {
	allow, err := ParseAllowList(strings.NewReader("dummyrun-engines-1.3.1 | / | #b | a known layout table\n"))
	if err != nil {
		t.Fatal(err)
	}
	violations := []Finding{{Rule: "image-alt", WCAG: []string{"1.1.1"}, Nodes: []Node{{Target: "#img"}, {Target: "#img2"}}}}
	r := &page.EngineResults{
		Axe: [][]string{{"ibm", "alfa"}, {}},
		Groups: []page.EngineGroup{
			{Selector: "#a", SC: []string{"4.1.2"}, Engines: []string{"htmlcs", "ibm"},
				Hits: []page.EngineHit{{Engine: "ibm", Rule: "a_text_purpose", Message: "no text"}, {Engine: "htmlcs", Rule: "H91"}}},
			{Selector: "#b", SC: []string{"1.3.1"}, Engines: []string{"alfa"}, Hits: []page.EngineHit{{Engine: "alfa", Rule: "sia-r2"}}},
			{Selector: "#c", SC: []string{"1.3.1"}, Engines: []string{"ibm"}, Hits: []page.EngineHit{{Engine: "ibm", Rule: "x"}}},
		},
	}
	agreed, cands := engineFindings(r, violations, "/", allow)
	// Three engines agree on the first node and axe is alone on the second. Both rest on axe's measured record, so both
	// rate high: the table has no separate data for three-way agreement yet and falls back to the axe subset.
	if n := violations[0].Nodes; n[0].Confidence != "high" || !slices.Equal(n[0].Engines, []string{"alfa", "axe", "ibm"}) || n[1].Confidence != "high" {
		t.Errorf("axe nodes: %+v", n)
	}
	if violations[0].Evidence != "" || agreed[0].Evidence != record.EvidenceEngineRule || cands[0].Evidence != record.EvidenceEngineRule {
		t.Errorf("evidence: axe %q agreed %q candidate %q", violations[0].Evidence, agreed[0].Evidence, cands[0].Evidence)
	}
	// Two other engines without axe have no measured record of their own, so the band is unknown and never high.
	if agreed[0].Nodes[0].Confidence != "unknown" || cands[0].Nodes[0].Confidence != "unknown" {
		t.Errorf("agreed %q candidate %q", agreed[0].Nodes[0].Confidence, cands[0].Nodes[0].Confidence)
	}
	if len(agreed) != 1 || agreed[0].Rule != "dummyrun-engines-4.1.2" || agreed[0].Impact != "serious" || agreed[0].Nodes[0].Detail != "IBM Equal Access (a_text_purpose): no text; HTML_CodeSniffer (H91)" {
		t.Errorf("agreed: %+v", agreed)
	}
	if cands[0].Impact != "" {
		t.Errorf("a candidate has an impact: %q", cands[0].Impact)
	}
	if len(cands) != 1 || len(cands[0].Nodes) != 2 || !cands[0].Nodes[0].Allowed || cands[0].Nodes[1].Allowed {
		t.Errorf("candidates: %+v", cands)
	}
	if !strings.Contains(cands[0].Help, "Info and Relationships") || !strings.HasSuffix(cands[0].HelpURL, "/info-and-relationships.html") {
		t.Errorf("candidate help: %q %q", cands[0].Help, cands[0].HelpURL)
	}
}

func TestCriteriaAAIsSortedAndComplete(t *testing.T) {
	c := CriteriaAA()
	if len(c) != 55 || c[0] != "1.1.1" || !slices.Contains(c, "2.5.8") || slices.Contains(c, "1.4.6") || slices.Contains(c, "4.1.1") {
		t.Errorf("criteria: %d %v", len(c), c)
	}
	if i, j := slices.Index(c, "1.4.4"), slices.Index(c, "1.4.10"); i > j {
		t.Error("1.4.10 sorts before 1.4.4")
	}
}

func TestSummaryCountsCandidatesAndEngineErrors(t *testing.T) {
	rep := &Report{Browser: "chrome", Results: []PageResult{
		{URL: "https://example.com/a", Browser: "chrome", Viewport: "desktop", Scheme: "light",
			Candidates:   []Finding{{Rule: "dummyrun-engines-1.3.1", Nodes: []Node{{Target: "#x"}, {Target: "#y", Allowed: true}}}},
			EngineErrors: map[string]string{"alfa": "load: boom", "ibm": "bad\u001b[2Kline"}},
		{URL: "https://example.com/b", Browser: "chrome", Viewport: "desktop", Scheme: "light"},
	}}
	rep.summarise(Config{Allow: &AllowList{}})
	if rep.Summary.Candidates != 1 {
		t.Errorf("candidates %d, want 1: an allowed candidate does not count", rep.Summary.Candidates)
	}
	var b strings.Builder
	rep.WriteSummary(&b)
	out := b.String()
	for _, want := range []string{"1 candidates only one engine reports (not failures)", "rule engines failed on 1 runs",
		"ENGINE Siteimprove Alfa https://example.com/a (chrome/desktop/light): load: boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "ENGINE IBM") < strings.Index(out, "ENGINE Siteimprove") {
		t.Error("engine errors are not in name order")
	}
}

func TestNoEngineRanFailsThePage(t *testing.T) {
	if msg := enginesFailed(&page.EngineResults{Ran: []string{"ibm"}, Errors: map[string]string{"alfa": "boom"}}); msg != "" {
		t.Errorf("one engine ran, got %q", msg)
	}
	msg := enginesFailed(&page.EngineResults{Errors: map[string]string{"ibm": "x", "alfa": "y"}})
	if msg != "none of the other accessibility engines ran (Siteimprove Alfa: y; IBM Equal Access: x)" {
		t.Errorf("got %q", msg)
	}
	rep := &Report{Results: []PageResult{{URL: "https://example.com/", Error: msg}}}
	rep.summarise(Config{Allow: &AllowList{}})
	if !rep.Failed() {
		t.Error("a sweep where no engine ran passed")
	}
}
