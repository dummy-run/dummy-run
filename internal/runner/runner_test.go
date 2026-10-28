package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/identity"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/provider"
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
	args := json.RawMessage(`{"verdict":"meh","goal_met":"sort of","report":"r","findings":[
	  {"kind":"oops","severity":"huge","symptom":"error-shown","title":" The thing broke ","steps":[0,2,2,9],"evidence":"guess"},
	  {"kind":"bug","severity":"major","title":"","steps":[1],"evidence":"measured"},
	  {"kind":"bug","severity":"major","title":"No steps","steps":[],"evidence":"measured"}]}`)
	f, err := ParseFinish(args, 5)
	if err != nil {
		t.Fatal(err)
	}
	if f.Verdict != "blocked" || f.GoalMet != "no" {
		t.Errorf("verdict %q, goal_met %q", f.Verdict, f.GoalMet)
	}
	if len(f.Findings) != 1 {
		t.Fatalf("findings %+v", f.Findings)
	}
	ff := f.Findings[0]
	if ff.Kind != "question" || ff.Evidence != "inferred" || ff.Title != "The thing broke" || len(ff.Steps) != 1 || ff.Steps[0] != 2 {
		t.Errorf("finding %+v", ff)
	}
	if _, err := ParseFinish(json.RawMessage(`not json`), 5); err == nil {
		t.Error("accepted bad JSON")
	}
	recs := RecordFindings(Finish{Findings: []FinishFinding{
		{Kind: "bug", Severity: "major", Symptom: "error-shown", Title: "Same thing", Steps: []int{1}, Evidence: "measured"},
		{Kind: "bug", Severity: "minor", Symptom: "error-shown", Title: "Same thing!", Steps: []int{2}, Evidence: "measured"},
	}}, NewLocator([]Step{sampleStep(1, "look", ""), sampleStep(2, "look", "")}))
	if len(recs) != 1 || recs[0].ID != "bug-same-thing" || recs[0].Verdict != record.VerdictPending {
		t.Errorf("record findings %+v", recs)
	}
}

func TestParseFinishSplitsObservationsFromFindings(t *testing.T) {
	args := json.RawMessage(`{"verdict":"pass","goal_met":"yes","report":"r",
	  "findings":[
	    {"kind":"accessibility","severity":"minor","symptom":"a11y","rule":"image-alt","title":"The logo has no alt text","steps":[2],"evidence":"measured"},
	    {"class":"observation","kind":"question","severity":"info","title":" The home page check found no violations ","steps":[1],"evidence":"measured"}],
	  "observations":[
	    {"text":" Checkout took three clicks. ","steps":[3,3,0,9]},
	    {"text":"","steps":[1]},
	    {"text":"No steps cited","steps":[]}]}`)
	f, err := ParseFinish(args, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Findings) != 1 || f.Findings[0].Class != record.ClassProblem {
		t.Fatalf("findings %+v, want only the problem", f.Findings)
	}
	want := []FinishObservation{{Text: "Checkout took three clicks.", Steps: []int{3}}, {Text: "The home page check found no violations", Steps: []int{1}}}
	if len(f.Observations) != 2 || f.Observations[0].Text != want[0].Text || f.Observations[0].Steps[0] != 3 || f.Observations[1].Text != want[1].Text {
		t.Fatalf("observations %+v, want %+v", f.Observations, want)
	}
	recs := RecordObservations(f)
	if len(recs) != 2 || recs[1].Text != want[1].Text || len(recs[1].Steps) != 1 {
		t.Errorf("record observations %+v", recs)
	}
	if got := RecordFindings(f, NewLocator([]Step{sampleStep(1, "look", ""), sampleStep(2, "look", "")})); len(got) != 1 {
		t.Errorf("%d record findings, want 1: observations are not findings", len(got))
	}
}

func TestFinishToolSeparatesProblemsFromObservations(t *testing.T) {
	var finish provider.Tool
	for _, tl := range Tools {
		if tl.Name == "finish" {
			finish = tl
		}
	}
	props := finish.Parameters["properties"].(map[string]any)
	if _, ok := props["observations"]; !ok {
		t.Fatal("finish has no observations")
	}
	items := props["findings"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if _, ok := items["class"]; ok {
		t.Error("a finding still offers class: findings are problems only")
	}
	if d := props["findings"].(map[string]any)["description"].(string); !strings.Contains(d, "Problems only") {
		t.Errorf("findings description %q", d)
	}
	for _, r := range finish.Parameters["required"].([]string) {
		if r == "observations" {
			t.Error("observations must stay optional")
		}
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
	if !strings.HasPrefix(sum, `step 2: visit @1 · now at https://t.test/ · title "Home" · error: timeout`) {
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

func TestPressToolNamesTheModifiers(t *testing.T) {
	for _, tool := range Tools {
		if tool.Name != "press" {
			continue
		}
		params, err := json.Marshal(tool.Parameters)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Shift", "Control", "Alt", "Meta", "+", "Shift+Tab"} {
			if !strings.Contains(tool.Description, want) {
				t.Errorf("the press description doesn't mention %q: %s", want, tool.Description)
			}
		}
		if !strings.Contains(string(params), "Shift+Tab") {
			t.Errorf("the key parameter doesn't show a modifier: %s", params)
		}
		return
	}
	t.Fatal("no press tool")
}

func TestGoalMetFallsBackToTheVerdict(t *testing.T) {
	for verdict, want := range map[string]string{"pass": "yes", "friction": "partly", "fail": "no", "blocked": "no"} {
		f, err := ParseFinish(json.RawMessage(`{"verdict":"`+verdict+`","report":"r","findings":[]}`), 3)
		if err != nil || f.GoalMet != want {
			t.Errorf("%s: goal_met %q (%v), want %q", verdict, f.GoalMet, err, want)
		}
	}
	f, _ := ParseFinish(json.RawMessage(`{"verdict":"fail","goal_met":"partly","report":"r","findings":[]}`), 3)
	if f.GoalMet != "partly" {
		t.Errorf("goal_met the persona gave was replaced: %q", f.GoalMet)
	}
}

func TestSymptomsAreBuiltAndValidated(t *testing.T) {
	ok := map[string]FinishFinding{
		"off-screen":                       {Symptom: "off-screen"},
		"missing-information:uk-data-home": {Symptom: "missing-information", Topic: "UK data home!"},
		"a11y:image-alt":                   {Symptom: "a11y", Rule: " Image-Alt "},
	}
	for want, ff := range ok {
		ff.Title = "t"
		got, err := ff.FullSymptom()
		if err != nil || got != want {
			t.Errorf("FullSymptom(%+v) = %q, %v; want %q", ff, got, err, want)
		}
	}
	for _, ff := range []FinishFinding{{Symptom: "weird"}, {Symptom: ""}, {Symptom: "missing-information"}, {Symptom: "a11y"}, {Symptom: "a11y", Rule: "has space"}} {
		if _, err := ff.FullSymptom(); err == nil {
			t.Errorf("accepted %+v", ff)
		}
	}
	f := Finish{Findings: []FinishFinding{{Symptom: "off-screen", Title: "a"}, {Symptom: "weird", Title: "b"}}}
	if len(f.Problems()) != 1 {
		t.Errorf("problems %v", f.Problems())
	}
	if n := f.DropInvalid(); n != 1 || len(f.Findings) != 1 || f.Findings[0].Title != "a" {
		t.Errorf("DropInvalid dropped %d, left %+v", n, f.Findings)
	}
}

func TestFinishToolOffersTheClosedSymptomList(t *testing.T) {
	var finish map[string]any
	for _, tool := range Tools {
		if tool.Name == "finish" {
			finish = tool.Parameters
		}
	}
	props := finish["properties"].(map[string]any)
	if _, ok := props["goal_met"]; !ok {
		t.Error("finish has no goal_met")
	}
	item := props["findings"].(map[string]any)["items"].(map[string]any)
	enum := item["properties"].(map[string]any)["symptom"].(map[string]any)["enum"].([]string)
	if len(enum) != 17 {
		t.Errorf("%d symptoms in the tool", len(enum))
	}
	required := item["required"].([]string)
	if !contains(required, "symptom") {
		t.Errorf("symptom is not required: %v", required)
	}
}

func locatorSteps() []Step {
	tree := "  banner\n  main\n    heading 1 \"Docs\"\n    navigation \"Docs menu\"\n      link \"Assessment taking too long\" @3\n      button (no name) @4\n    heading 2 \"Setup\"\n    button \"Save changes\" @5 [disabled]"
	s1 := Step{N: 1, Obs: page.Observation{Action: "visit", URL: "https://x.test/users/12345/orders/?b=2&a=1",
		Snapshot: &page.Snapshot{Headings: []string{"Docs", "Setup"}, Tree: tree, Controls: []page.Control{{Ref: "9", Tag: "input", Type: "checkbox", Label: "Remember me"}}}}}
	s2 := Step{N: 2, Obs: page.Observation{Action: "click"}}
	s3 := Step{N: 3, Obs: page.Observation{Action: "look", URL: "https://x.test/docs", Snapshot: &page.Snapshot{}}}
	return []Step{s1, s2, s3}
}

func TestLocatorRoute(t *testing.T) {
	l := NewLocator(locatorSteps())
	for cited, want := range map[string]string{"1": "/users/:id/orders?a&b", "2": "/users/:id/orders?a&b", "3": "/docs", "2,3": "/docs", "3,1": "/users/:id/orders?a&b"} {
		var steps []int
		for _, p := range strings.Split(cited, ",") {
			n := int(p[0] - '0')
			steps = append(steps, n)
		}
		if got := l.Route(steps); got != want {
			t.Errorf("Route(%v) = %q, want %q", steps, got, want)
		}
	}
	if got := NewLocator(nil).Route([]int{4}); got != "/" {
		t.Errorf("no steps: %q", got)
	}
}

func TestLocatorLocus(t *testing.T) {
	l := NewLocator(locatorSteps())
	for _, c := range []struct {
		control string
		steps   []int
		want    string
	}{
		{"@3", []int{1}, "link:assessment taking too long"},
		{"3", []int{1}, "link:assessment taking too long"},
		{"@4", []int{1}, "region:docs menu"}, // no name: the nearest landmark above
		{"@5", []int{1}, "button:save changes"},
		{"@9", []int{1}, "checkbox:remember me"}, // not in the outline: from the control list
		{"", []int{1}, "region:docs"},            // no control: the first heading
		{"@77", []int{1}, "region:docs"},         // a control the page does not have
		{"@3", []int{3}, "page"},                 // a page with nothing to name
		{"", nil, "page"},
	} {
		if got := l.Locus(c.steps, c.control); got != c.want {
			t.Errorf("Locus(%v, %q) = %q, want %q", c.steps, c.control, got, c.want)
		}
	}
}

func TestRecordedFindingGetsEngineIdentity(t *testing.T) {
	l := NewLocator(locatorSteps())
	recs := RecordFindings(Finish{Findings: []FinishFinding{
		{Kind: "friction", Severity: "major", Symptom: "off-screen", Control: "@3", Title: "Link off screen", Steps: []int{1}, Evidence: "measured", Class: record.ClassProblem},
		{Kind: "bug", Severity: "minor", Symptom: "not-a-symptom", Title: "Dropped", Steps: []int{1}, Evidence: "measured"},
	}}, l)
	if len(recs) != 1 {
		t.Fatalf("%+v", recs)
	}
	f := recs[0]
	want, _ := identity.Fingerprint("/users/:id/orders?a&b", "link:assessment taking too long", "off-screen")
	if f.Fingerprint != want || f.Route != "/users/:id/orders?a&b" || f.Symptom != "off-screen" || f.TaxonomyVersion != identity.TaxonomyVersion || f.Class != "problem" {
		t.Errorf("%+v, want fingerprint %s", f, want)
	}
}

func TestBuildHash(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("<html>home</html>"))
	}))
	defer srv.Close()
	a, err := BuildHash(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := BuildHash(context.Background(), nil, srv.URL)
	if len(a) != 12 || a != b || calls != 2 {
		t.Errorf("hashes %q %q, %d calls", a, b, calls)
	}
	bad := httptest.NewServer(http.NotFoundHandler())
	defer bad.Close()
	if _, err := BuildHash(context.Background(), nil, bad.URL); err == nil {
		t.Error("hashed a 404")
	}
}

func TestCountRepeats(t *testing.T) {
	dir := t.TempDir()
	mk := func(id, batch, model string) {
		r := &record.Record{SchemaVersion: record.SchemaVersion, ID: id, Batch: batch,
			StartedAt: time.Now(), EndedAt: time.Now(),
			Persona:  record.Persona{ID: "p", Role: "r", Device: "desktop", Dimensions: &record.Dimensions{}},
			Scenario: record.Scenario{ID: "s", Title: "S", GoalMet: "yes"},
			Target:   record.Target{Adapter: "url", Environment: "e", BaseURL: "https://t.test"},
			Provider: record.Provider{Kind: "k", Model: model, Region: "g"}, Browser: record.Browser{Name: "chrome", Version: "1"},
			Outcome: record.OutcomeCompleted, Steps: []record.Step{}, Findings: []record.Finding{}, Usage: record.Usage{}}
		if _, err := record.Write(dir, r); err != nil {
			t.Fatal(err)
		}
	}
	mk("2026-10-19-p-s-a", "b", "m")
	mk("2026-10-19-p-s-b", "b", "m")
	mk("2026-10-19-p-s-c", "b", "other")
	mk("2026-10-19-p-s-d", "other", "m")
	if got := CountRepeats(dir, "b", "p", "s", "m", "2026-10-19-p-s-new"); got != 2 {
		t.Errorf("repeats %d, want 2", got)
	}
	if got := CountRepeats(dir, "b", "p", "s", "m", "2026-10-19-p-s-a"); got != 1 {
		t.Errorf("repeats leaving out itself %d, want 1", got)
	}
}

func TestMinStepsClampsToTheBudget(t *testing.T) {
	for _, c := range []struct{ requested, budget, want int }{{0, 25, 0}, {8, 25, 8}, {30, 25, 22}, {5, 2, 0}} {
		if got := MinSteps(c.requested, c.budget); got != c.want {
			t.Errorf("MinSteps(%d, %d) = %d, want %d", c.requested, c.budget, got, c.want)
		}
	}
}

func TestEarlyFinishIsSentBackUntilTheMinimum(t *testing.T) {
	pass := json.RawMessage(`{"verdict":"pass"}`)
	blocked := json.RawMessage(`{"verdict":"blocked"}`)
	if earlyFinish(0, 0, 0, false, pass) != "" {
		t.Error("no minimum should accept finish")
	}
	if msg := earlyFinish(8, 2, 0, false, pass); !strings.Contains(msg, "2 of at least 8") {
		t.Errorf("finish at step 2 of 8 was not sent back: %q", msg)
	}
	if earlyFinish(8, 8, 0, false, pass) != "" {
		t.Error("finish at the minimum should be accepted")
	}
	if earlyFinish(8, 1, 0, false, blocked) != "" {
		t.Error("a blocked verdict should be accepted")
	}
	if earlyFinish(8, 1, 0, true, pass) != "" {
		t.Error("a finish the engine forced should be accepted")
	}
	if earlyFinish(8, 1, maxEarlyFinishes, false, pass) != "" {
		t.Error("finish should be accepted after the allowed refusals")
	}
	if earlyFinish(8, 1, 0, false, json.RawMessage(`not json`)) == "" {
		t.Error("unreadable arguments should still be sent back")
	}
}

func TestRecordFindingsSetsSeverityAndInputsFromTheSteps(t *testing.T) {
	a11yStep := sampleStep(2, "a11y", "")
	a11yStep.Obs.A11y = []page.Violation{
		{ID: "image-alt", Impact: "critical", Help: "Images must have alternate text", Nodes: 1},
		{ID: "button-name", Impact: "critical", Help: "Buttons must have discernible text", Nodes: 1, Elements: []page.ViolationElement{{Selector: "#menu", Name: "Menu"}}},
	}
	loc := NewLocator([]Step{sampleStep(1, "look", ""), a11yStep, sampleStep(3, "click", "")})
	recs := RecordFindings(Finish{Findings: []FinishFinding{
		// The model's severity is ignored: the rule sets it from the inputs.
		{Kind: "accessibility", Severity: "info", Symptom: "a11y", Rule: "image-alt", Title: "The logo has no alt text", Steps: []int{2}, Evidence: "measured"},
		// Naming a rule axe reported is not enough when the finding is about a control axe did not flag.
		{Kind: "accessibility", Symptom: "a11y", Rule: "button-name", Control: "@2", Title: "Go has no name", Steps: []int{2}, Evidence: "measured"},
		{Kind: "accessibility", Severity: "blocker", Symptom: "a11y", Rule: "color-contrast", Title: "Grey text on grey", Steps: []int{1}, Evidence: "measured"}, //nolint:misspell // an axe rule ID
		{Kind: "friction", Severity: "blocker", Symptom: "missing-information", Topic: "prices", Title: "No prices", Steps: []int{3}, Evidence: "inferred"},
		{Kind: "bug", Severity: "info", Symptom: "not-operable", Control: "@2", Title: "Go does nothing", Steps: []int{3}, Evidence: "measured"},
	}}, loc)
	if len(recs) != 5 {
		t.Fatalf("%d findings", len(recs))
	}
	if other := recs[1]; other.Inputs.EvidenceKind != record.EvidenceModelOnly || other.Severity != "major" {
		t.Errorf("a rule axe reported on another element: %s %+v", other.Severity, other.Inputs)
	}
	recs = append(recs[:1], recs[2:]...)
	backed := recs[0]
	if backed.Severity != "blocker" || backed.Inputs == nil || backed.Inputs.EvidenceKind != record.EvidenceEngineRule ||
		backed.Inputs.Check != "image-alt" || len(backed.Inputs.Engines) != 1 || backed.Inputs.Engines[0] != "axe" || !backed.Inputs.OnJourney {
		t.Errorf("a finding the a11y step backs: severity %s inputs %+v", backed.Severity, backed.Inputs)
	}
	claimed := recs[1]
	if claimed.Severity != "major" || claimed.Inputs.EvidenceKind != record.EvidenceModelOnly || claimed.Inputs.Check != "color-contrast" || len(claimed.Inputs.Engines) != 0 { //nolint:misspell // an axe rule ID
		t.Errorf("a rule the a11y step did not report: severity %s inputs %+v", claimed.Severity, claimed.Inputs)
	}
	if recs[2].Severity != "minor" || recs[2].Inputs.Check != "missing-information" || recs[2].Inputs.EvidenceKind != record.EvidenceModelOnly {
		t.Errorf("missing information: severity %s inputs %+v", recs[2].Severity, recs[2].Inputs)
	}
	if recs[3].Severity != "major" || recs[3].Inputs.Check != "not-operable" {
		t.Errorf("not operable on a journey: severity %s inputs %+v", recs[3].Severity, recs[3].Inputs)
	}
	var finish provider.Tool
	for _, tl := range Tools {
		if tl.Name == "finish" {
			finish = tl
		}
	}
	items := finish.Parameters["properties"].(map[string]any)["findings"].(map[string]any)["items"].(map[string]any)
	if _, ok := items["properties"].(map[string]any)["severity"]; ok {
		t.Error("the finish tool still asks the model for a severity")
	}
	for _, r := range items["required"].([]string) {
		if r == "severity" {
			t.Error("severity is still required")
		}
	}
}
