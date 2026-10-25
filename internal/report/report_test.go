package report

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/identity"
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
		if format == "html" {
			got = shortenAssets(got)
		}
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

// shortenAssets replaces the inlined fonts and logos, which are large and tested on their own, so the
// golden file stays readable.
func shortenAssets(html string) string {
	return regexp.MustCompile(`data:(font/woff2|image/svg(?:\+|&#43;)xml);base64,(?:[A-Za-z0-9/=]|\+|&#43;)+`).ReplaceAllString(html, "data:$1;base64,...")
}

// The SVG files in assets/ are copies of the brand files and must not drift from them.
func TestAssetsMatchBrand(t *testing.T) {
	for name, src := range map[string]string{
		"mark.svg":                   "../../brand/logo/mark.svg",
		"lockup-horizontal.svg":      "../../brand/logo/lockup-horizontal.svg",
		"lockup-horizontal-dark.svg": "../../brand/logo/lockup-horizontal-dark.svg",
		"dummy-reading.svg":          "../../brand/mascot/dummy-reading.svg",
	} {
		want, err := os.ReadFile(src) //nolint:gosec // fixed paths
		if err != nil {
			t.Fatal(err)
		}
		got, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("internal/report/assets/%s differs from %s: copy the brand file again", name, src)
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
		if len(f.Reviewers) > 0 && f.Reviewers[0] == "example-reviewer" && f.ReviewedAt == "2026-10-18T09:30:00Z" && strings.Contains(f.Reason, "Reproduced") {
			withReview++
		} else {
			without++
		}
	}
	if withReview != 1 || without == 0 {
		t.Errorf("records with reviewer fields: %d, without: %d. A record without them must still load", withReview, without)
	}
	if !strings.Contains(render(t, rep, "html"), "example-reviewer") {
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
	for _, bad := range []string{"<script", "@import", "<iframe", "<video", "<audio"} {
		if strings.Contains(out, bad) {
			t.Errorf("the HTML contains %q, which could load something", bad)
		}
	}
	// Fonts, logos and the icon are inlined, so every URL a style, link or image source holds is a data URI.
	for _, re := range []string{`url\(["']?([^"')]*)`, `<link[^>]*href="([^"]*)"`, `srcset="([^"]*)"`, `<img[^>]*src="([^"]*)"`} {
		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(out, -1) {
			if !strings.HasPrefix(m[1], "data:") {
				t.Errorf("the HTML loads %q", m[1])
			}
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

func TestBaselineItemsAreNotFixedWhenTheNewReportDidNotLookForThem(t *testing.T) {
	opt := fixtureOptions(t)
	opt.IncludePending = true
	base, err := Build(loadFixture(t), opt)
	if err != nil {
		t.Fatal(err)
	}
	if base.Axe == nil || len(base.Findings) == 0 {
		t.Fatal("the fixture baseline needs persona findings and an axe sweep")
	}
	// A report with runs and no sweep must not call the baseline's axe rules fixed.
	noSweep := fixtureOptions(t)
	noSweep.A11y = nil
	noSweep.Baseline = base
	rep, err := Build(loadFixture(t), noSweep)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Fixed {
		if f.Source == "axe" {
			t.Errorf("axe rule %q is listed as fixed although this report ran no sweep", f.Title)
		}
	}
	// A report with a sweep and no runs must not call the baseline's persona findings fixed.
	sweepOnly := Options{A11y: fixtureOptions(t).A11y, Date: fixedDate, Baseline: base}
	rep, err = Build(nil, sweepOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Fixed {
		if f.Source == "persona" {
			t.Errorf("finding %q is listed as fixed although this report has no runs", f.Title)
		}
	}
}

func TestAxeRulesAreNotFixedWhenTheSweepHadPageErrors(t *testing.T) {
	opt := fixtureOptions(t)
	base, err := Build(loadFixture(t), opt)
	if err != nil {
		t.Fatal(err)
	}
	if base.Axe == nil || base.Axe.PageErrors == 0 {
		t.Skip("the fixture sweep has no page errors")
	}
	base.Axe.Rules = append(base.Axe.Rules, AxeRule{Fingerprint: "lost-rule", Rule: "lost-rule", Help: "A rule on a page that failed to load", Impact: "serious"})
	opt.Baseline = base
	rep, err := Build(loadFixture(t), opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Fixed {
		if f.Source == "axe" {
			t.Errorf("axe rule %q is listed as fixed although the sweep could not test every page", f.Title)
		}
	}
}

func TestWhatWorkedListsObservationsAndNeverCountsThem(t *testing.T) {
	runs := loadFixture(t)
	base, err := Build(runs, fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range runs {
		if i < 2 {
			r.Rec.Observations = append(r.Rec.Observations, record.Observation{Text: "The search box found the draft at once", Steps: []int{1}})
		}
	}
	rep, err := Build(runs, fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Worked) != 1 || rep.Worked[0].Runs != 2 {
		t.Fatalf("worked %+v, want one note seen in 2 runs", rep.Worked)
	}
	if rep.Summary.FindingsTotal != base.Summary.FindingsTotal {
		t.Errorf("observations changed the finding count from %d to %d", base.Summary.FindingsTotal, rep.Summary.FindingsTotal)
	}
	for _, format := range []string{"html", "md", "json"} {
		out := render(t, rep, format)
		if !strings.Contains(out, "The search box found the draft at once") {
			t.Errorf("%s output lacks the note", format)
		}
	}
	if strings.Contains(render(t, base, "md"), "What worked") {
		t.Error("a report with no observations has a What worked section")
	}
}

func TestEverySymptomHasPlainWords(t *testing.T) {
	for _, s := range identity.Symptoms {
		if _, ok := symptomWords[s]; !ok {
			t.Errorf("symptom %q has no plain wording", s)
		}
		if strings.Contains(symptomText(s), "-") {
			t.Errorf("symptom %q reads as %q", s, symptomText(s))
		}
	}
	if got := symptomText("missing-information:rates"); got != "Information that was needed is missing (rates)" {
		t.Errorf("a parameter is lost: %q", got)
	}
}

func TestFindingCardsAreInPlainWords(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"html", "md"} {
		out := render(t, rep, format)
		for _, bad := range []string{"No criterion recorded", "None recorded for this finding", "Run 2026-", "persona lin-contributor signed in"} {
			if strings.Contains(out, bad) {
				t.Errorf("%s still has %q", format, bad)
			}
		}
		for _, want := range []string{"Sign in as a", "Technical details"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s is missing %q", format, want)
			}
		}
	}
	if got := placeText("/payments", "link:rates"); got != "the Payments page, the Rates link" {
		t.Errorf("place reads %q", got)
	}
}

func TestMarkdownCellsHoldFreeText(t *testing.T) {
	if got := mdCell("a | b\nc"); got != `a \| b c` {
		t.Errorf("cell is %q", got)
	}
	if got := whenText(Scope{StartedAt: "2026-10-16T10:00:00Z"}); got != "on 16 October 2026" {
		t.Errorf("a missing end gives %q", got)
	}
	if got := howSure(CoverageSection{Unavailable: "The runs span two builds."}); !strings.Contains(got, "two builds") {
		t.Errorf("the reason is lost: %q", got)
	}
}

func TestMarkdownFindingWithoutALocation(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	rep.Findings[0].Route = ""
	rep.Findings[0].Locus = ""
	rep.Findings[0].Evidence = "line one\nline `two`"
	out := render(t, rep, "md")
	if !strings.Contains(out, ".\n\n| | |\n|---|---|") && !strings.Contains(out, "\n\n\n| | |\n|---|---|") {
		t.Error("a finding with no location should still start its table on its own line")
	}
	if strings.Contains(out, "evidence line one\nline") || strings.Contains(out, "`two`") {
		t.Error("newlines and backticks in free text should be flattened in the technical details")
	}
}

func TestAdviceIsShownApartFromFailures(t *testing.T) {
	opt := fixtureOptions(t)
	before, err := Build(loadFixture(t), opt)
	if err != nil {
		t.Fatal(err)
	}
	sweep := *opt.A11y[0]
	sweep.Results = append([]a11y.PageResult{}, sweep.Results...)
	sweep.Results[0].Advice = []a11y.Finding{{Rule: "region", Help: "All page content should be contained by landmarks",
		HelpURL: "https://dequeuniversity.com/rules/axe/4.13/region", Nodes: []a11y.Node{{Target: "p.intro"}}}}
	sweep.AdviceTags = []string{"best-practice"}
	opt.A11y = []*a11y.Report{&sweep}
	rep, err := Build(loadFixture(t), opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Axe.Advice) != 1 || rep.Axe.Advice[0].Elements != 1 {
		t.Fatalf("advice: %+v", rep.Axe.Advice)
	}
	if len(rep.Axe.AdviceTags) != 1 || slices.Contains(rep.Axe.Tags, "best-practice") {
		t.Errorf("advice tags should be listed apart from the WCAG tags: %v / %v", rep.Axe.Tags, rep.Axe.AdviceTags)
	}
	if out := render(t, rep, "md"); !strings.Contains(out, "It also checked `best-practice`") {
		t.Error("the check scope does not name the advice tags")
	}
	if rep.Axe.FailingNodes != before.Axe.FailingNodes || len(rep.Axe.Rules) != len(before.Axe.Rules) {
		t.Errorf("advice changed the failures: %d rules, %d elements", len(rep.Axe.Rules), rep.Axe.FailingNodes)
	}
	for _, format := range []string{"html", "md"} {
		if out := render(t, rep, format); !strings.Contains(out, "Advice beyond WCAG A and AA") || !strings.Contains(out, "All page content should be contained by landmarks") {
			t.Errorf("%s has no advice section", format)
		}
	}
	if out := render(t, before, "html"); strings.Contains(out, "Advice beyond WCAG A and AA") {
		t.Error("a report with no advice shows the section")
	}
}

func TestSeverityAndConfidenceComeFromTheInputs(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]AxeRule{}
	for _, r := range rep.Axe.Rules {
		rules[r.Rule] = r
	}
	// button-name is critical, so a blocker. color-contrast is serious on a page the personas visited: major, since
	// only a critical impact makes a blocker. Both rest on axe alone, which has a measured record, so both are high.
	if r := rules["button-name"]; r.Severity != "blocker" || r.Confidence != "high" || r.Inputs == nil || !r.Inputs.OnJourney || r.Inputs.EvidenceKind != record.EvidenceEngineRule {
		t.Errorf("button-name: severity %q confidence %q inputs %+v", r.Severity, r.Confidence, r.Inputs)
	}
	if r := rules["color-contrast"]; r.Severity != "major" || r.Confidence != "high" || r.ConfidenceReason != "axe-core reports it, and no other engine was asked." { //nolint:misspell // an axe rule ID
		t.Errorf("color-contrast: severity %q confidence %q reason %q", r.Severity, r.Confidence, r.ConfidenceReason) //nolint:misspell // an axe rule ID
	}
	if len(rep.Axe.NeedsReview) > 0 && rep.Axe.NeedsReview[0].Severity != "" {
		t.Error("a rule that needs review has no severity: nothing failed yet")
	}
	// The fixture records predate inputs, so every persona finding is the model's own report: low, with how many of
	// the runs that visited its route also found it.
	for _, f := range rep.Findings {
		if f.Confidence != "low" || f.Inputs == nil || f.Inputs.EvidenceKind != record.EvidenceModelOnly || f.Inputs.Reproduced == nil || f.Inputs.Reproduced.Of != 4 {
			t.Errorf("%s: confidence %q inputs %+v", f.Title, f.Confidence, f.Inputs)
		}
		if !strings.HasPrefix(f.ConfidenceReason, "The persona's own report") || !strings.Contains(f.ConfidenceReason, "of 4 runs") {
			t.Errorf("%s: reason %q", f.Title, f.ConfidenceReason)
		}
	}
	html := render(t, rep, "html")
	for _, want := range []string{"Low confidence</span>", "<dt>How sure</dt><dd>Low confidence. The persona&#39;s own report", `<th scope="col">How sure</th>`, "High<br>"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
	// A record that carries inputs keeps them, and a finding the sweep's engines back rates by them.
	runs := loadFixture(t)
	runs[0].Rec.Findings[0].Inputs = &record.FindingInputs{EvidenceKind: record.EvidenceEngineRule, Engines: []string{"axe"}, Check: "label", OnJourney: true}
	rep, err = Build(runs, fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	var backed *Finding
	for i := range rep.Findings {
		if rep.Findings[i].Fingerprint == runs[0].Rec.Findings[0].Fingerprint {
			backed = &rep.Findings[i]
		}
	}
	if backed == nil || backed.Confidence != "high" || backed.Inputs.Check != "label" {
		t.Errorf("a finding with recorded inputs: %+v", backed)
	}
}
