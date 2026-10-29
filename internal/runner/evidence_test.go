package runner

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/record"
)

// evidenceSteps is a short run on a blog: the list page with three "Read more" links, a post, a step where the
// browser connection failed, an accessibility check and a slow error page.
func evidenceSteps() []Step {
	blog := &page.Snapshot{URL: "https://example.test/blog/", Title: "Blog", Headings: []string{"Latest posts"},
		Text: "Latest posts. Building a thing. Read more. Another thing. Read more. Thing three. Read more.",
		Tree: "main\n  heading 1 \"Latest posts\"\n  link \"Read more\" @4\n  link \"Read more\" @5\n  link \"Read more\" @6\n",
		Controls: []page.Control{{Ref: "4", Tag: "a", Label: "Read more"}, {Ref: "5", Tag: "a", Label: "Read more"},
			{Ref: "9", Tag: "input", Type: "search", Label: "(no label)"}}}
	post := &page.Snapshot{URL: "https://example.test/blog/a-thing/", Title: "Building a thing",
		Headings: []string{"Building a thing"}, Text: "Building a thing. Posted on 3 May. It was “hard” — really hard."}
	return []Step{
		{N: 1, Obs: page.Observation{Action: "visit", URL: blog.URL, Snapshot: blog, Status: 200}},
		{N: 2, Obs: page.Observation{Action: "click", Target: "@4", URL: post.URL, Snapshot: post, Status: 200}},
		{N: 3, Obs: page.Observation{Action: "click", Target: "@10", Error: "bidi: input.performActions: context deadline exceeded"}},
		{N: 4, Obs: page.Observation{Action: "a11y", URL: post.URL, A11y: []page.Violation{{ID: "image-alt", Nodes: 2}}}},
		{N: 5, Obs: page.Observation{Action: "visit", URL: "https://example.test/old/", Status: 404, Duration: 6 * time.Second,
			Snapshot: &page.Snapshot{URL: "https://example.test/old/", Title: "Not found"}}},
	}
}

func TestVerifyEvidenceBacksAFindingWithItsControlOrQuote(t *testing.T) {
	f := Finish{Findings: []FinishFinding{
		{Title: "control", Symptom: "unclear-label", Control: "@5", Steps: []int{1}},
		{Title: "quote", Symptom: "unclear-label", Quote: "  Read MORE. ", Steps: []int{1}},
		{Title: "curly quote on the second cited step", Symptom: "misleading-copy", Quote: `It was "hard" - really hard`, Steps: []int{1, 2}},
		{Title: "ellipsis", Symptom: "misleading-copy", Quote: "Building a thing ... really hard", Steps: []int{2}},
		{Title: "heading", Symptom: "unclear-label", Quote: "latest posts", Steps: []int{1}},
		{Title: "unnamed search box", Symptom: "unclear-label", Control: "@9", Steps: []int{1}},
	}}
	if probs := VerifyEvidence(&f, evidenceSteps()); len(probs) != 0 {
		t.Fatalf("want every finding backed, got %v", probs)
	}
	want := []struct {
		element string
		step    int
	}{
		{`link "Read more"`, 1}, {`link "Read more"`, 1}, {`text "It was \"hard\" - really hard"`, 2},
		{`text "Building a thing ... really hard"`, 2}, {`heading "Latest posts"`, 1}, {"textbox with no name", 1},
	}
	for i, w := range want {
		ff := f.Findings[i]
		if ff.Element != w.element || ff.EvidenceStep != w.step || ff.URL == "" {
			t.Errorf("%s: element %q step %d url %q, want %q at step %d", ff.Title, ff.Element, ff.EvidenceStep, ff.URL, w.element, w.step)
		}
	}
}

func TestVerifyEvidenceRefusesClaimsTheStepsDoNotShow(t *testing.T) {
	f := Finish{Findings: []FinishFinding{
		{Title: "words not on the page", Symptom: "misleading-copy", Quote: "Free trial for 30 days", Steps: []int{1, 2}},
		{Title: "no control, no quote", Symptom: "missing-information:pricing", Topic: "pricing", Steps: []int{1}},
		{Title: "a control the page does not have", Symptom: "unclear-label", Control: "@77", Steps: []int{1}},
		{Title: "only a harness failure", Symptom: "wrong-outcome", Quote: "Read more", Steps: []int{3}},
		{Title: "a quote too short to mean anything", Symptom: "unclear-label", Quote: "a", Steps: []int{1}},
		{Title: "fragments joined by an ellipsis", Symptom: "unclear-label", Quote: "a ... e", Steps: []int{1}},
		{Title: "the wrong axe rule", Symptom: "a11y", Rule: "color-contrast", Steps: []int{4}},
	}}
	probs := VerifyEvidence(&f, evidenceSteps())
	if len(probs) != len(f.Findings) {
		t.Fatalf("want %d problems, got %d: %v", len(f.Findings), len(probs), probs)
	}
	if !strings.Contains(probs[3], "could not read the page") {
		t.Errorf("a harness failure should be named as such: %s", probs[3])
	}
	for _, ff := range f.Findings {
		if ff.EvidenceStep != 0 || ff.Element != "" {
			t.Errorf("%s was backed: %+v", ff.Title, ff)
		}
	}
	if n := DemoteUnverified(&f); n != 7 || len(f.Findings) != 0 || len(f.Observations) != 7 {
		t.Fatalf("demoted %d, %d findings and %d notes left", n, len(f.Findings), len(f.Observations))
	}
	for _, o := range f.Observations {
		if o.Kind != record.ObservationUnverified {
			t.Errorf("note %q has kind %q", o.Text, o.Kind)
		}
	}
	if obs := RecordObservations(f); obs[0].Kind != record.ObservationUnverified {
		t.Errorf("the record loses the kind: %+v", obs[0])
	}
}

func TestVerifyEvidenceAcceptsWhatTheEngineMeasured(t *testing.T) {
	f := Finish{Findings: []FinishFinding{
		{Title: "axe", Symptom: "a11y", Rule: "image-alt", Steps: []int{4}},
		{Title: "dead link", Symptom: "dead-link", Steps: []int{5}},
		{Title: "slow", Symptom: "slow", Steps: []int{5}},
	}}
	if probs := VerifyEvidence(&f, evidenceSteps()); len(probs) != 0 {
		t.Fatalf("%v", probs)
	}
	for i, want := range []string{"axe rule image-alt on 2 element(s)", "page status 404", "step took 6.0 seconds"} {
		if f.Findings[i].Element != want {
			t.Errorf("%s: element %q, want %q", f.Findings[i].Title, f.Findings[i].Element, want)
		}
	}
}

func TestElementForNamesTheControlFromTheLastLook(t *testing.T) {
	before := evidenceSteps()[0].Obs.Snapshot.Controls
	if got := ElementFor("@4", before, nil); got != `link "Read more"` {
		t.Errorf("ElementFor(@4) = %q", got)
	}
	if got := ElementFor("Read more", before, nil); got != "" {
		t.Errorf("a target given in words is already readable, got %q", got)
	}
	if got := ElementFor("@6", nil, evidenceSteps()[0].Obs.Snapshot); got != `link "Read more"` {
		t.Errorf("ElementFor from the outline = %q", got)
	}
}

func TestRecordFindingsCarriesTheEvidence(t *testing.T) {
	steps := evidenceSteps()
	f := Finish{Findings: []FinishFinding{{Kind: "friction", Severity: "minor", Symptom: "misleading-copy", Title: "t",
		Body: "Readers are confused.", Fix: "Say it plainly.", WCAG: []string{"3.1.5"}, Quote: "really hard", Steps: []int{1, 2}, Evidence: "measured"}}}
	if probs := VerifyEvidence(&f, steps); len(probs) != 0 {
		t.Fatal(probs)
	}
	recs := RecordFindings(f, NewLocator(steps))
	if len(recs) != 1 {
		t.Fatalf("%+v", recs)
	}
	r := recs[0]
	if r.Route != "/blog/a-thing" || r.EvidenceStep != 2 || r.URL != "https://example.test/blog/a-thing/" || r.Body == "" || r.Fix == "" ||
		r.Element != `text "really hard"` || r.Quote != "really hard" || len(r.WCAG) != 1 {
		t.Errorf("%+v", r)
	}
}

// TestElementTextKeepsWholeRunes checks a long control name in a three-byte script is cut between characters,
// so the element never carries a broken tail such as \xe3 into the record and the report.
func TestElementTextKeepsWholeRunes(t *testing.T) {
	name := strings.Repeat("あいうえお", 20)
	got := elementText("link", name)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, `..."`) || !strings.Contains(got, "あいうえお") {
		t.Errorf("elementText cut inside a rune or lost the name: %s", got)
	}
	if got := elementText("link", "Read more"); got != `link "Read more"` {
		t.Errorf("a short name is left as it is, got %s", got)
	}
}

func TestEvidenceRetryStopsWhenTheBudgetIsSpent(t *testing.T) {
	if !evidenceRetry(0, "") {
		t.Error("the first unbacked finish should go back to the model")
	}
	if evidenceRetry(maxEvidenceRetries, "") {
		t.Error("a second retry would be one model call too many")
	}
	// The finish that crossed the token budget sets forced before the evidence check runs, so no call follows it.
	for _, forced := range []string{"the token budget", "the step budget", "patience"} {
		if evidenceRetry(0, forced) {
			t.Errorf("a finish forced by %s should not go back to the model", forced)
		}
	}
}
