package runner

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/record"
)

func harnessSteps() []Step {
	home := &page.Snapshot{URL: "https://example.test/", Title: "Home", ContentType: "text/html"}
	feed := &page.Snapshot{URL: "https://example.test/feed.xml", ContentType: "application/rss+xml"}
	return []Step{
		{N: 1, Obs: page.Observation{Action: "visit", URL: home.URL, Snapshot: home}},
		{N: 2, Obs: page.Observation{Action: "click", Target: "@4", Error: "bidi: input.performActions: context deadline exceeded"}},
		{N: 3, Obs: page.Observation{Action: "a11y", URL: home.URL, Snapshot: home, Error: "axe.run: context deadline exceeded"}},
		{N: 4, Obs: page.Observation{Action: "click", URL: home.URL, Snapshot: home, Blocked: []string{"POST https://example.test/api/query (read-only target)"}}},
		{N: 5, Obs: page.Observation{Action: "click", URL: feed.URL, Snapshot: feed}},
		{N: 6, Refusal: "click is not one of the tools kit may use"},
		{N: 7, Obs: page.Observation{Action: "look", URL: home.URL, Snapshot: home}},
		// A beacon the page sent by itself while the persona scrolled, and one on a click that did change the page.
		{N: 8, PageChanged: true, Obs: page.Observation{Action: "scroll", URL: home.URL, Snapshot: home, Blocked: []string{"POST https://example.test/beacon (read-only target)"}}},
		{N: 9, PageChanged: true, Obs: page.Observation{Action: "click", URL: "https://example.test/next", Snapshot: home, Blocked: []string{"POST https://example.test/beacon (read-only target)"}}},
		// Chrome shows its own error page when the test refuses a form's POST.
		{N: 10, PageChanged: true, Obs: page.Observation{Action: "click", URL: "chrome-error://chromewebdata/", Snapshot: home, Blocked: []string{"POST https://example.test/subscribe (read-only target)"}}},
		// Firefox's own pages for a failed load and a bad certificate.
		{N: 11, PageChanged: true, Obs: page.Observation{Action: "click", URL: "about:neterror?e=connectionFailure", Snapshot: home, Blocked: []string{"GET https://other.test/ (host not allowed)"}}},
		{N: 12, PageChanged: true, Obs: page.Observation{Action: "visit", URL: "about:certerror?e=nssBadCert", Snapshot: home, Blocked: []string{"GET https://other.test/ (host not allowed)"}}},
	}
}

func TestHarnessOnlyDropsFindingsAboutTheTest(t *testing.T) {
	steps := harnessSteps()
	for _, c := range []struct {
		name    string
		ff      FinishFinding
		harness bool
		reason  string
	}{
		{"lost browser connection", FinishFinding{Symptom: "wrong-outcome", Steps: []int{2}}, true, "could not read"},
		{"axe timed out", FinishFinding{Symptom: "error-shown", Steps: []int{3}}, true, "tool failed"},
		{"a feed", FinishFinding{Symptom: "unclear-label", Steps: []int{5}}, true, "not a web page"},
		{"a read-only refusal broke the demo", FinishFinding{Symptom: "not-operable", Steps: []int{4}}, true, "blocked a request"},
		{"a refused tool", FinishFinding{Symptom: "not-reachable", Steps: []int{6}}, true, "refused"},
		{"all of them together", FinishFinding{Symptom: "wrong-outcome", Steps: []int{2, 3, 6}}, true, "step 2"},
		{"chrome's error page after a refused post", FinishFinding{Symptom: "not-operable", Steps: []int{10}}, true, "own error page"},
		{"firefox's error page after a blocked load", FinishFinding{Symptom: "unclear-label", Steps: []int{11}}, true, "own error page"},
		{"firefox's certificate page after a blocked load", FinishFinding{Symptom: "dead-link", Steps: []int{12}}, true, "own error page"},
		{"one real step keeps it", FinishFinding{Symptom: "wrong-outcome", Steps: []int{2, 7}}, false, ""},
		{"a blocked request does not excuse a label problem", FinishFinding{Symptom: "unclear-label", Steps: []int{4}}, false, ""},
		{"a beacon while scrolling does not excuse a failure", FinishFinding{Symptom: "not-operable", Steps: []int{8}}, false, ""},
		{"a click that went to a new page is not excused by a beacon there", FinishFinding{Symptom: "wrong-outcome", Steps: []int{9}}, false, ""},
		{"an ordinary page", FinishFinding{Symptom: "missing-information:contact", Steps: []int{1}}, false, ""},
		{"no step in the run", FinishFinding{Symptom: "wrong-outcome", Steps: []int{99}}, false, ""},
	} {
		why, ok := HarnessOnly(c.ff, steps)
		if ok != c.harness || !strings.Contains(why, c.reason) {
			t.Errorf("%s: HarnessOnly = %q, %v; want %v with %q", c.name, why, ok, c.harness, c.reason)
		}
	}
}

func TestDropHarnessOnlyRecordsAnEvent(t *testing.T) {
	r := &run{steps: harnessSteps(), log: io.Discard, eventKeys: map[string]bool{}}
	f := &Finish{Findings: []FinishFinding{
		{Title: "The live demo could not run", Symptom: "not-operable", Steps: []int{4}},
		{Title: "The contact page has no email", Symptom: "missing-information:contact", Steps: []int{1}},
		{Title: "The live demo could not run", Symptom: "wrong-outcome", Steps: []int{2}},
	}}
	r.dropHarnessOnly(f)
	if len(f.Findings) != 1 || f.Findings[0].Title != "The contact page has no email" {
		t.Fatalf("findings %+v", f.Findings)
	}
	if len(r.events) != 2 || r.events[0].Kind != record.EventHarnessLimitFinding || !strings.Contains(r.events[0].Detail, "The live demo could not run") {
		t.Errorf("events %+v", r.events)
	}
}

func TestANonHTMLStepIsDescribedAsAFile(t *testing.T) {
	s := harnessSteps()[4]
	out := Render(s)
	if !strings.Contains(out, "This is not a web page: application/rss+xml") || strings.Contains(out, "controls:") {
		t.Errorf("render:\n%s", out)
	}
}

func TestCleanLabel(t *testing.T) {
	for in, want := range map[string]string{"Read more": "Read more", "(no name)": "", "(no name) /cart": "", "(hidden) Menu": "Menu", "(no label) x": ""} {
		if got := cleanLabel(in); got != want {
			t.Errorf("cleanLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProblemsNameAxeElementsAndCountOffSiteBlocksOnce(t *testing.T) {
	s := Step{N: 3, Obs: page.Observation{Action: "a11y", A11y: []page.Violation{{ID: "link-name", Impact: "serious", Help: "Links must have discernible text", Nodes: 5,
		Elements: []page.ViolationElement{{Selector: "header a.account"}, {Selector: "#cart", Name: "Basket"}}}},
		Blocked: []string{"GET https://other.test/ (host not allowed)", "POST https://example.test/subscribe (read-only target)"}}}
	out := problems(s)
	for _, want := range []string{`link-name: Links must have discernible text (5) at header a.account, #cart "Basket" and 3 more`,
		"POST https://example.test/subscribe (read-only target)"} {
		if !strings.Contains(out, want) {
			t.Errorf("problems lacks %q: %s", want, out)
		}
	}
	if strings.Contains(out, "other.test") {
		t.Errorf("an off-site block is shown in full: %s", out)
	}
	r := &run{log: io.Discard, dir: t.TempDir(), eventKeys: map[string]bool{}}
	var notes int
	for i := 1; i <= 3; i++ {
		st := r.finishExecute(page.Observation{Action: "look", Blocked: []string{"GET https://other.test/x (host not allowed)"},
			Snapshot: &page.Snapshot{Title: "t"}}, nil, time.Now(), "")
		if st.OffSiteNote != "" {
			notes++
		}
	}
	if notes != 1 {
		t.Errorf("the off-site note appeared on %d steps, want 1", notes)
	}
}
