package runner

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/record"
)

func TestStepBudget(t *testing.T) {
	for in, want := range map[int]int{0: 25, 10: 25, 25: 25, 30: 30, 40: 40, 99: 40} {
		if got := StepBudget(in); got != want {
			t.Errorf("StepBudget(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestPatienceGivesUpAfterThreeFailedTries(t *testing.T) {
	controls := []page.Control{{Ref: "28", Tag: "button", Label: "Save"}}
	p := NewPatience()
	for i := 1; i <= 2; i++ {
		p.Note(i, "/timesheets", "click", "@28", controls, true)
		if p.Spent() {
			t.Fatalf("gave up after %d tries", i)
		}
	}
	// A look between tries does not count, and neither does a try that worked.
	p.Note(3, "/timesheets", "look", "", controls, true)
	p.Note(4, "/timesheets", "click", "@28", controls, false)
	if p.Spent() {
		t.Fatal("a successful try or a look counted as a failure")
	}
	// The control was renumbered; it is still the same thing.
	p.Note(5, "/timesheets", "click", "@3", []page.Control{{Ref: "3", Tag: "button", Label: "Save"}}, true)
	if !p.Spent() {
		t.Fatal("did not give up after three failed tries")
	}
	g := p.GaveUp()
	if g.Step != 5 || g.Tries != 3 || g.Action != `click button "Save"` {
		t.Errorf("gave up %+v", g)
	}
	// A different page resets the count.
	q := NewPatience()
	q.Note(1, "/a", "click", "Save", nil, true)
	q.Note(2, "/b", "click", "Save", nil, true)
	q.Note(3, "/c", "click", "Save", nil, true)
	if q.Spent() {
		t.Error("the same control on different pages counted as one thing")
	}
}

func TestParseFinishRepairsBadValues(t *testing.T) {
	args := json.RawMessage(`{"verdict":"meh","report":"r","findings":[
	  {"kind":"oops","severity":"huge","title":" The thing broke ","steps":[0,2,2,9],"evidence":"guess"},
	  {"kind":"bug","severity":"major","title":"","steps":[1],"evidence":"measured"},
	  {"kind":"bug","severity":"major","title":"No steps","steps":[],"evidence":"measured"}]}`)
	f, err := ParseFinish(args, 5)
	if err != nil {
		t.Fatal(err)
	}
	if f.Verdict != "blocked" {
		t.Errorf("verdict %q", f.Verdict)
	}
	if len(f.Findings) != 1 {
		t.Fatalf("findings %+v", f.Findings)
	}
	ff := f.Findings[0]
	if ff.Kind != "question" || ff.Severity != "info" || ff.Evidence != "inferred" || ff.Title != "The thing broke" || len(ff.Steps) != 1 || ff.Steps[0] != 2 {
		t.Errorf("finding %+v", ff)
	}
	if _, err := ParseFinish(json.RawMessage(`not json`), 5); err == nil {
		t.Error("accepted bad JSON")
	}
	recs := RecordFindings(Finish{Findings: []FinishFinding{
		{Kind: "bug", Severity: "major", Title: "Same thing", Steps: []int{1}, Evidence: "measured"},
		{Kind: "bug", Severity: "minor", Title: "Same thing!", Steps: []int{2}, Evidence: "measured"},
	}})
	if len(recs) != 1 || recs[0].ID != "bug-same-thing" || recs[0].Verdict != record.VerdictPending {
		t.Errorf("record findings %+v", recs)
	}
}

func sampleStep(n int, action string, err string) Step {
	return Step{N: n, PageChanged: true, Obs: page.Observation{Action: action, Target: "@1", URL: "https://t.test/", Error: err,
		Duration: 300 * time.Millisecond, Status: 200, ConsoleErrors: []string{"boom"},
		Snapshot: &page.Snapshot{Title: "Home", Headings: []string{"Welcome"}, Viewport: []int{1400, 1000}, Scroll: []int{0, 3000},
			Text: "line one\nline two", LogText: "line one\nline two",
			Controls: []page.Control{{Ref: "1", Tag: "a", Label: "Docs", Type: ""}, {Ref: "2", Tag: "button", Type: "submit", Label: "Go", Disabled: true}}}}}
}

func TestRenderAndSummary(t *testing.T) {
	s := sampleStep(3, "click", "")
	out := Render(s)
	for _, want := range []string{"step 3 · click @1 · 300 ms · https://t.test/", "title: Home", "status: 200",
		"headings: Welcome", "console errors: boom", `@1 a "Docs"`, `@2 button/submit "Go" [disabled]`, "  line one"} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}
	s.PageChanged = false
	if !strings.Contains(Render(s), "nothing on the page changed") {
		t.Error("unchanged page not flagged")
	}
	s.Obs.Error = "no control @1"
	if !strings.Contains(Render(s), "ACTION FAILED: no control @1") {
		t.Error("error not shown")
	}
	s.Obs.A11y = []page.Violation{{ID: "label", Impact: "critical", Help: "Form elements must have labels", Nodes: 2}}
	if !strings.Contains(Render(s), "accessibility: critical label: Form elements must have labels (2)") {
		t.Error("a11y not shown")
	}
	s.Obs.Snapshot = nil
	if !strings.Contains(Render(s), "could not be read") {
		t.Error("missing snapshot not explained")
	}
	sum := Summary(sampleStep(2, "visit", "timeout"))
	if !strings.HasPrefix(sum, "step 2: visit @1 · now at https://t.test/ · Home · error: timeout") {
		t.Errorf("summary %q", sum)
	}
}

func TestSignature(t *testing.T) {
	a := sampleStep(1, "look", "")
	b := sampleStep(2, "click", "")
	if Signature(a.Obs) != Signature(b.Obs) {
		t.Error("the step number or action changed the signature")
	}
	b.Obs.Snapshot.LogText = "different"
	if Signature(a.Obs) == Signature(b.Obs) {
		t.Error("changed text kept the signature")
	}
}

func TestOutcome(t *testing.T) {
	pass := &Finish{Verdict: "pass"}
	fail := &Finish{Verdict: "fail"}
	blocked := &Finish{Verdict: "blocked"}
	cases := []struct {
		name   string
		finish *Finish
		err    error
		forced string
		gaveUp *GaveUp
		want   string
	}{
		{"harness error", pass, errors.New("x"), "", nil, record.OutcomeErrored},
		{"never finished", nil, nil, "", nil, record.OutcomeBudgetExhausted},
		{"pass", pass, nil, "", nil, record.OutcomeCompleted},
		{"pass at the last step", pass, nil, "the step budget", nil, record.OutcomeCompleted},
		{"fail", fail, nil, "", nil, record.OutcomeCompleted},
		{"blocked", blocked, nil, "", nil, record.OutcomeBlocked},
		{"budget ran out", fail, nil, "the step budget", nil, record.OutcomeBudgetExhausted},
		{"tokens ran out", blocked, nil, "the token budget", nil, record.OutcomeBudgetExhausted},
		{"gave up", fail, nil, "patience", &GaveUp{}, record.OutcomeBlocked},
	}
	for _, tc := range cases {
		if got := outcome(tc.finish, tc.err, tc.forced, tc.gaveUp); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestToolsCoverTheSchemaActions(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range Tools {
		names[tool.Name] = true
	}
	for _, a := range record.Actions {
		if !names[a] {
			t.Errorf("no tool for action %q", a)
		}
	}
	if len(Tools) != 13 {
		t.Errorf("%d tools, want 13", len(Tools))
	}
}
