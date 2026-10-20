package report

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/record"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var fixedDate = time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)

func loadFixture(t *testing.T) []*Run {
	t.Helper()
	runs, err := LoadRuns("testdata/runs")
	if err != nil {
		t.Fatal(err)
	}
	return runs
}

func fixtureOptions(t *testing.T) Options {
	t.Helper()
	sweep, err := LoadA11y("testdata/a11y.json")
	if err != nil {
		t.Fatal(err)
	}
	return Options{Title: "Inkwell audit", Client: "Inkwell Press", IncludePending: true, Date: fixedDate, ToolVersion: "0.0.0-test", A11y: sweepList(sweep)}
}

func render(t *testing.T, rep *Report, format string) string {
	t.Helper()
	var b bytes.Buffer
	if err := Render(&b, rep, format); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestGolden(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	for format, file := range map[string]string{"html": "report.html", "md": "report.md", "json": "report.json", "sarif": "report.sarif", "junit": "report.junit.xml"} {
		got := render(t, rep, format)
		path := filepath.Join("testdata", "golden", file)
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // test data
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path) //nolint:gosec // test data
		if err != nil {
			t.Fatalf("%v (run go test ./internal/report -update)", err)
		}
		if got != string(want) {
			t.Errorf("%s differs from %s. Run go test ./internal/report -update and read the diff", format, path)
		}
	}
}

func TestExamplesRender(t *testing.T) {
	runs, err := LoadRuns("../../examples/runs")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Build(runs, Options{Build: "a865ffd", IncludePending: true, Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.FindingsTotal != 3 {
		t.Errorf("examples build a865ffd: %d findings, want 3", rep.Summary.FindingsTotal)
	}
	for _, f := range Formats {
		render(t, rep, f)
	}
	// Without a build the three builds are all reported, and coverage says why it was not computed.
	rep, err = Build(runs, Options{Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Coverage.Available || !strings.Contains(rep.Coverage.Unavailable, "builds") {
		t.Errorf("coverage across builds: available %v, reason %q", rep.Coverage.Available, rep.Coverage.Unavailable)
	}
}

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(SchemaJSON()))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("report.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("report.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestJSONMatchesSchema(t *testing.T) {
	schema := compileSchema(t)
	base, err := Build(loadFixture(t), Options{Title: "Earlier", Date: fixedDate.AddDate(0, -1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]Options{
		"confirmed only": {Date: fixedDate},
		"with pending":   {IncludePending: true, Date: fixedDate},
		"full":           fixtureOptions(t),
	}
	full := fixtureOptions(t)
	full.Baseline = base
	cases["with baseline"] = full
	for name, opt := range cases {
		rep, err := Build(loadFixture(t), opt)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(render(t, rep, "json")))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(doc); err != nil {
			t.Errorf("%s: report JSON does not match schema/report.schema.json: %v", name, err)
		}
	}
	// An a11y-only report is valid too.
	rep, err := Build(nil, Options{A11y: fixtureOptions(t).A11y, Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := jsonschema.UnmarshalJSON(strings.NewReader(render(t, rep, "json")))
	if err := schema.Validate(doc); err != nil {
		t.Errorf("sweep-only report: %v", err)
	}
}

func TestConfirmedOnlyByDefault(t *testing.T) {
	rep, err := Build(loadFixture(t), Options{Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		if f.Status != StatusConfirmed {
			t.Errorf("finding %q has status %s in a confirmed-only report", f.Title, f.Status)
		}
	}
	if rep.Summary.PendingOmitted == 0 {
		t.Error("the fixture has a pending-only finding, and the report should count it as left out")
	}
	out := render(t, rep, "md")
	if strings.Contains(out, "too faint to read") {
		t.Error("a pending finding appeared without --include pending")
	}
	if !strings.Contains(out, "left out of this report") {
		t.Error("the summary does not say that findings were left out")
	}
}

func TestPendingIsMarked(t *testing.T) {
	rep, err := Build(loadFixture(t), Options{IncludePending: true, Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range rep.Findings {
		if strings.Contains(f.Title, "too faint to read") {
			found = true
			if f.Status != StatusNotReviewed {
				t.Errorf("status %s, want not-reviewed", f.Status)
			}
		}
	}
	if !found {
		t.Fatal("the pending finding is missing")
	}
	for _, format := range []string{"html", "md"} {
		if !strings.Contains(render(t, rep, format), "Not reviewed") {
			t.Errorf("%s does not mark the pending finding as not reviewed", format)
		}
	}
}

func TestRejectedNeverAppears(t *testing.T) {
	for _, pending := range []bool{false, true} {
		opt := fixtureOptions(t)
		opt.IncludePending = pending
		rep, err := Build(loadFixture(t), opt)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range Formats {
			if strings.Contains(render(t, rep, f), "hides pennies") {
				t.Errorf("a rejected finding appears in the %s output (include pending %v)", f, pending)
			}
		}
	}
}

func TestDeduplicatesByFingerprint(t *testing.T) {
	rep, err := Build(loadFixture(t), Options{IncludePending: true, Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range rep.Findings {
		if seen[f.Fingerprint] {
			t.Errorf("fingerprint %s appears twice", f.Fingerprint)
		}
		seen[f.Fingerprint] = true
	}
	for _, f := range rep.Findings {
		if strings.Contains(f.Title, "settled") && f.Runs != 3 {
			t.Errorf("the unclear-label problem was found in %d runs, want 3", f.Runs)
		}
	}
}

func TestSortedBySeverity(t *testing.T) {
	rep, err := Build(loadFixture(t), Options{IncludePending: true, Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(rep.Findings); i++ {
		if severityRank[rep.Findings[i-1].Severity] > severityRank[rep.Findings[i].Severity] {
			t.Errorf("finding %d (%s) comes before %d (%s)", i-1, rep.Findings[i-1].Severity, i, rep.Findings[i].Severity)
		}
	}
}

func TestReviewFieldsAreReadWhenPresent(t *testing.T) {
	rep, err := Build(loadFixture(t), Options{Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	var withReview, without int
	for _, f := range rep.Findings {
		if len(f.Reviewers) > 0 && f.Reviewers[0] == "Priya Shah" && f.ReviewedAt == "2026-10-18T09:30:00Z" && strings.Contains(f.Reason, "Reproduced") {
			withReview++
		} else {
			without++
		}
	}
	if withReview != 1 || without == 0 {
		t.Errorf("records with reviewer fields: %d, without: %d. A record without them must still load", withReview, without)
	}
	if !strings.Contains(render(t, rep, "html"), "Priya Shah") {
		t.Error("the reviewer is not in the HTML")
	}
}

func TestAxeJoinsPersonaFinding(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	var f *Finding
	for i := range rep.Findings {
		if rep.Findings[i].Symptom == "a11y:color-contrast" { //nolint:misspell // an axe rule ID
			f = &rep.Findings[i]
		}
	}
	if f == nil {
		t.Fatal("no contrast finding")
	}
	if len(f.WCAG) != 1 || f.WCAG[0] != "1.4.3" {
		t.Errorf("WCAG %v, want [1.4.3] from the sweep", f.WCAG)
	}
	if !strings.Contains(f.HelpURL, "contrast") {
		t.Errorf("help URL %q", f.HelpURL)
	}
	ax := rep.Axe
	if ax == nil || len(ax.Rules) != 2 || ax.Rules[0].Rule != "button-name" {
		t.Fatalf("axe rules should be 2 with critical first: %+v", ax)
	}
	if ax.FailingNodes != 2 || ax.AllowedNodes != 1 || ax.PageErrors != 1 || ax.Pages != 2 || len(ax.NeedsReview) != 1 {
		t.Errorf("axe totals: failing %d, allowed %d, errors %d, pages %d, review %d", ax.FailingNodes, ax.AllowedNodes, ax.PageErrors, ax.Pages, len(ax.NeedsReview))
	}
}

func TestBaseline(t *testing.T) {
	runs := loadFixture(t)
	// The baseline holds every confirmed problem. The new report is built from fewer runs, so some
	// problems are not seen again, and the sweep adds rules that the baseline did not have.
	base, err := Build(runs, Options{Title: "First audit", Date: fixedDate.AddDate(0, -1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(render(t, base, "json"))
	path := filepath.Join(t.TempDir(), "base.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	var some []*Run
	for _, r := range runs {
		if strings.Contains(r.Rec.ID, "lin-contributor-008") || strings.Contains(r.Rec.ID, "lin-contributor-002") {
			some = append(some, r)
		}
	}
	opt := fixtureOptions(t)
	opt.IncludePending = false
	opt.Baseline = loaded
	rep, err := Build(some, opt)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, f := range rep.Findings {
		got[f.Baseline]++
	}
	if got[BaselineStillPresent] != 2 || got[BaselineNew] != 0 {
		t.Errorf("persona findings: %v, want 2 still present", got)
	}
	if len(rep.Fixed) != 1 || !strings.Contains(rep.Fixed[0].Title, "server error") {
		t.Errorf("fixed: %+v, want the scheduling error", rep.Fixed)
	}
	for _, r := range rep.Axe.Rules {
		if r.Baseline != BaselineNew {
			t.Errorf("axe rule %s is %q, want new", r.Rule, r.Baseline)
		}
	}
	if rep.Summary.New != 2 || rep.Summary.StillPresent != 2 || rep.Summary.Fixed != 1 {
		t.Errorf("summary %+v", rep.Summary)
	}
	for _, f := range []string{"html", "md"} {
		out := render(t, rep, f)
		if !strings.Contains(out, "Since the baseline") || !strings.Contains(out, "Still present") {
			t.Errorf("%s does not show the baseline marks", f)
		}
	}
	if _, err := LoadBaseline("testdata/a11y.json"); err == nil {
		t.Error("an a11y file was accepted as a baseline")
	}
}

func TestScreenshotsEmbeddedWhenTheFileExists(t *testing.T) {
	rep, err := Build(loadFixture(t), Options{IncludePending: true, Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	out := render(t, rep, "html")
	if got := strings.Count(out, `src="data:image/png;base64,`); got != 1 {
		t.Errorf("%d embedded screenshots, want 1 (the others are missing from disk)", got)
	}
	if !strings.Contains(out, "was not found when this report was made") {
		t.Error("a missing screenshot is not named")
	}
	if strings.Contains(render(t, rep, "json"), "base64") {
		t.Error("the JSON holds screenshot data")
	}
}

func TestHTMLIsSelfContained(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	out := render(t, rep, "html")
	for _, bad := range []string{"<link", "<script", "@import", "url(", "<iframe", "<video", "<audio", "srcset"} {
		if strings.Contains(out, bad) {
			t.Errorf("the HTML contains %q, which could load something", bad)
		}
	}
	for _, m := range regexp.MustCompile(`(?:src|action|poster|data)=["'](https?:)?//[^"']*`).FindAllString(out, -1) {
		t.Errorf("the HTML loads %s", m)
	}
	for _, want := range []string{`<html lang="en-GB">`, "<title>Inkwell audit</title>", "<main", "<nav", "<footer", "<h1>", `@media print`} {
		if !strings.Contains(out, want) {
			t.Errorf("the HTML is missing %q", want)
		}
	}
}

func TestHTMLEscapesUserText(t *testing.T) {
	opt := fixtureOptions(t)
	opt.Title = `<script>alert(1)</script>`
	opt.Client = `A & B "Ltd"`
	rep, err := Build(loadFixture(t), opt)
	if err != nil {
		t.Fatal(err)
	}
	out := render(t, rep, "html")
	if strings.Contains(out, "<script>") || !strings.Contains(out, "A &amp; B") {
		t.Error("user text is not escaped")
	}
}

// The report never says the target passed or is compliant. The words are checked in the template
// sources and in every format rendered from the fixtures and from the examples.
var forbidden = regexp.MustCompile(`(?i)\b(pass|passes|passed|passing|compliant)\b`)

func TestNoPassOrCompliantWording(t *testing.T) {
	for _, name := range []string{"templates/report.html.tmpl", "templates/report.md.tmpl"} {
		data, err := templateFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if m := forbidden.FindString(string(data)); m != "" {
			t.Errorf("%s says %q", name, m)
		}
	}
	for _, l := range Limits {
		if m := forbidden.FindString(l); m != "" {
			t.Errorf("the limits say %q", m)
		}
	}
	exRuns, err := LoadRuns("../../examples/runs")
	if err != nil {
		t.Fatal(err)
	}
	sets := map[string]struct {
		runs []*Run
		opt  Options
	}{
		"fixture":         {loadFixture(t), fixtureOptions(t)},
		"fixture empty":   {loadFixture(t), Options{Date: fixedDate, Build: "zzzzzzzz"}},
		"examples":        {exRuns, Options{Build: "a865ffd", IncludePending: true, Date: fixedDate}},
		"examples clean":  {exRuns, Options{Build: "1953351", Date: fixedDate}},
		"sweep only":      {nil, Options{A11y: fixtureOptions(t).A11y, Date: fixedDate}},
		"no problems yet": {exRuns, Options{Build: "3660793", Date: fixedDate}},
	}
	for name, s := range sets {
		rep, err := Build(s.runs, s.opt)
		if err != nil {
			if name != "fixture empty" {
				t.Fatalf("%s: %v", name, err)
			}
			continue // the empty case: no records match
		}
		for _, f := range Formats {
			if m := forbidden.FindString(render(t, rep, f)); m != "" {
				t.Errorf("%s as %s says %q", name, f, m)
			}
		}
	}
}

func TestSARIF(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID              string            `json:"ruleId"`
				Level               string            `json:"level"`
				PartialFingerprints map[string]string `json:"partialFingerprints"`
				Locations           []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(render(t, rep, "sarif")), &log); err != nil {
		t.Fatal(err)
	}
	if log.Version != "2.1.0" || len(log.Runs) != 1 || log.Runs[0].Tool.Driver.Name != "dummyrun" {
		t.Fatalf("bad SARIF header: %+v", log)
	}
	rules := map[string]bool{}
	for _, r := range log.Runs[0].Tool.Driver.Rules {
		rules[r.ID] = true
	}
	// Persona problems and failing axe elements.
	if got := len(log.Runs[0].Results); got != rep.Summary.FindingsTotal+rep.Axe.FailingNodes {
		t.Errorf("%d results, want %d", got, rep.Summary.FindingsTotal+rep.Axe.FailingNodes)
	}
	for _, r := range log.Runs[0].Results {
		if !rules[r.RuleID] {
			t.Errorf("result rule %s is not declared", r.RuleID)
		}
		if r.Level != "error" && r.Level != "warning" && r.Level != "note" {
			t.Errorf("level %q", r.Level)
		}
		if len(r.Locations) != 1 || !strings.HasPrefix(r.Locations[0].PhysicalLocation.ArtifactLocation.URI, "https://") {
			t.Errorf("location %+v", r.Locations)
		}
		if r.PartialFingerprints["dummyrun/v1"] == "" {
			t.Errorf("result %s has no fingerprint", r.RuleID)
		}
	}
}

func TestJUnit(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Tests    int `xml:"tests,attr"`
		Failures int `xml:"failures,attr"`
		Skipped  int `xml:"skipped,attr"`
		Suites   []struct {
			Name  string `xml:"name,attr"`
			Cases []struct {
				Name string `xml:"name,attr"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}
	if err := xml.Unmarshal([]byte(render(t, rep, "junit")), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Suites) != 2 || s.Tests != rep.Summary.FindingsTotal+len(rep.Axe.Rules) {
		t.Errorf("suites %d, tests %d", len(s.Suites), s.Tests)
	}
	if s.Failures+s.Skipped != s.Tests || s.Skipped != rep.Summary.BySeverity["info"] {
		t.Errorf("failures %d, skipped %d, tests %d", s.Failures, s.Skipped, s.Tests)
	}
}

func TestBuildRefusals(t *testing.T) {
	if _, err := Build(nil, Options{}); err == nil {
		t.Error("an empty report was built")
	}
	if _, err := Build(loadFixture(t), Options{Build: "abc"}); err == nil {
		t.Error("a short --build was accepted")
	}
	if _, err := Build(loadFixture(t), Options{Build: "0000000"}); err == nil {
		t.Error("a --build that matches no record was accepted")
	}
	if err := Render(&bytes.Buffer{}, &Report{}, "pdf"); err == nil {
		t.Error("an unknown format was accepted")
	}
}

func TestLoadRunsRefusals(t *testing.T) {
	if _, err := LoadRuns(t.TempDir()); err == nil {
		t.Error("an empty directory was accepted")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"schema_version": 2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuns(bad); err == nil {
		t.Error("a record that fails the schema was accepted")
	}
	if _, err := LoadRuns("testdata/runs", "testdata/runs"); err == nil {
		t.Error("the same runs twice were accepted")
	}
}

func TestEmbeddedSchemaMatchesRepository(t *testing.T) {
	repo, err := os.ReadFile("../../schema/report.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(repo, SchemaJSON()) {
		t.Error("internal/report/report.schema.json differs from schema/report.schema.json. Run go generate ./internal/report")
	}
}

func sweepList(r ...*a11y.Report) []*a11y.Report { return r }

func TestBaselineDoesNotCallAFindingFixedWhenTheRunsSawItAgainUnreviewed(t *testing.T) {
	opt := fixtureOptions(t)
	opt.IncludePending = true
	base, err := Build(loadFixture(t), opt)
	if err != nil {
		t.Fatal(err)
	}
	var pendingFP string
	for _, f := range base.Findings {
		if f.Status == StatusNotReviewed && f.Fingerprint != "" {
			pendingFP = f.Fingerprint
		}
	}
	if pendingFP == "" {
		t.Fatal("the fixture has no unreviewed finding to test with")
	}
	now := fixtureOptions(t)
	now.IncludePending = false
	now.Baseline = base
	rep, err := Build(loadFixture(t), now)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Fixed {
		if f.Fingerprint == pendingFP {
			t.Errorf("%q is listed as fixed although the runs saw it again and only left it out as not reviewed", f.Title)
		}
	}
}

func TestScreenshotPathsStayInTheRunFolder(t *testing.T) {
	root := t.TempDir()
	run := filepath.Join(root, "run")
	if err := os.Mkdir(run, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "inside.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := hit{
		run: &Run{Dir: run, Rec: &record.Record{ID: "r1", Steps: []record.Step{
			{N: 1, Screenshot: "../outside.png"},
			{N: 2, Screenshot: "inside.png"},
		}}},
		f: record.Finding{Steps: []int{1, 2}},
	}
	shots := screenshotsFor(h)
	if len(shots) != 2 {
		t.Fatalf("%d screenshots, want 2", len(shots))
	}
	if shots[0].File != "" {
		t.Errorf("a path that climbs out of the run folder was resolved to %q", shots[0].File)
	}
	if shots[1].File == "" {
		t.Error("a screenshot inside the run folder was not found")
	}
}

func TestSplitReviewsKeepsLargeIntegersExact(t *testing.T) {
	in := `{"seed":9223372036854775807,"findings":[{"id":"f1","reviewer":"sam"}]}`
	reviews, out, err := splitReviews([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if reviews["f1"].Reviewer != "sam" {
		t.Errorf("reviews %+v", reviews)
	}
	if !strings.Contains(string(out), "9223372036854775807") {
		t.Errorf("the seed changed: %s", out)
	}
}
