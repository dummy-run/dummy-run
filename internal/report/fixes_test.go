package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/a11y"
)

// A failed rule from Dummy Run's own checks carries its fix into every format.
func TestOwnRuleCarriesItsFix(t *testing.T) {
	opts := fixtureOptions(t)
	p := &opts.A11y[0].Results[0]
	p.Violations = append(p.Violations, a11y.Finding{Rule: "dummyrun-reflow", Impact: "serious", WCAG: []string{"1.4.10"},
		Help: "The page scrolls sideways", Nodes: []a11y.Node{{Target: "#wide"}}})
	rep, err := Build(loadFixture(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	var found *AxeRule
	for i, r := range rep.Axe.Rules {
		if r.Rule == "dummyrun-reflow" {
			found = &rep.Axe.Rules[i]
		}
		if !strings.HasPrefix(r.Rule, "dummyrun-") && r.Fix != nil {
			t.Errorf("axe rule %s got a fix from our library", r.Rule)
		}
	}
	if found == nil || found.Fix == nil || found.Fix.Owner != "development" {
		t.Fatalf("dummyrun-reflow has no fix: %+v", found)
	}
	step := found.Fix.Steps[0]
	for _, format := range []string{"md", "html"} {
		got := render(t, rep, format)
		if !strings.Contains(got, "How to fix what failed") || !strings.Contains(got, "Usually done by development") {
			t.Errorf("%s: no fix section for dummyrun-reflow", format)
		}
		if format == "md" && !strings.Contains(got, step) {
			t.Errorf("md: the first step is missing: %q", step)
		}
	}
	// The fix belongs on the rule itself, at axe.rules[].fix.
	var doc struct {
		Axe struct {
			Rules []struct {
				Rule string          `json:"rule"`
				Fix  json.RawMessage `json:"fix"`
			} `json:"rules"`
		} `json:"axe"`
	}
	if err := json.Unmarshal([]byte(render(t, rep, "json")), &doc); err != nil {
		t.Fatal(err)
	}
	for _, r := range doc.Axe.Rules {
		if r.Rule != "dummyrun-reflow" {
			continue
		}
		var fix struct {
			Owner string   `json:"owner"`
			Steps []string `json:"steps"`
		}
		if err := json.Unmarshal(r.Fix, &fix); err != nil || fix.Owner != "development" || len(fix.Steps) == 0 {
			t.Errorf("json: dummyrun-reflow has no fix object: %s", r.Fix)
		}
		return
	}
	t.Error("json: dummyrun-reflow is missing from axe.rules")
}

// A persona finding on one of Dummy Run's own rules gets its fix, and no Deque link, which only covers axe rules.
func TestOwnRuleFindingCarriesItsFix(t *testing.T) {
	runs := loadFixture(t)
	moved := false
	for _, r := range runs {
		for i := range r.Rec.Findings {
			if r.Rec.Findings[i].Symptom == "a11y:color-contrast" { //nolint:misspell // an axe rule ID
				r.Rec.Findings[i].Symptom = "a11y:dummyrun-reflow"
				moved = true
			}
		}
	}
	if !moved {
		t.Fatal("the fixture has no contrast finding to move")
	}
	rep, err := Build(runs, fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	var f *Finding
	for i := range rep.Findings {
		if rep.Findings[i].Symptom == "a11y:dummyrun-reflow" {
			f = &rep.Findings[i]
		}
	}
	if f == nil {
		t.Fatal("no dummyrun-reflow finding")
	}
	if f.HelpURL != "" || f.Fix == nil || f.Fix.Owner != "development" {
		t.Fatalf("help %q fix %+v", f.HelpURL, f.Fix)
	}
	if got := render(t, rep, "md"); !strings.Contains(got, "How to fix:\n\n1. "+f.Fix.Steps[0]) {
		t.Errorf("md: the finding's steps are missing")
	}
}

// An axe rule the run did not describe still gets its Deque link.
func TestAxeRuleFindingKeepsItsDequeLink(t *testing.T) {
	runs := loadFixture(t)
	moved := false
	for _, r := range runs {
		for i := range r.Rec.Findings {
			if r.Rec.Findings[i].Symptom == "a11y:color-contrast" { //nolint:misspell // an axe rule ID
				r.Rec.Findings[i].Symptom = "a11y:frame-title"
				moved = true
			}
		}
	}
	if !moved {
		t.Fatal("the fixture has no contrast finding to move")
	}
	rep, err := Build(runs, fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		if f.Symptom == "a11y:frame-title" {
			if !strings.HasPrefix(f.HelpURL, "https://dequeuniversity.com/rules/axe/") || !strings.HasSuffix(f.HelpURL, "/frame-title") {
				t.Errorf("help %q", f.HelpURL)
			}
			return
		}
	}
	t.Fatal("no frame-title finding")
}

// A fix with no help link is linked from the rule table, and its example and sources are shown. Advice from Dummy
// Run's own checks gets its fix too.
func TestFixesForRulesAndAdvice(t *testing.T) {
	opts := fixtureOptions(t)
	p := &opts.A11y[0].Results[0]
	p.Violations = append(p.Violations, a11y.Finding{Rule: "dummyrun-reflow", Impact: "serious", WCAG: []string{"1.4.10"},
		Help: "The page scrolls sideways", Nodes: []a11y.Node{{Target: "#wide"}}})
	p.Advice = append(p.Advice, a11y.Finding{Rule: "dummyrun-plain-latin", Help: "Latin abbreviations", Nodes: []a11y.Node{{Target: "main"}}})
	rep, err := Build(loadFixture(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	html := render(t, rep, "html")
	for _, want := range []string{
		`<a href="#fix-dummyrun-reflow">How to fix dummyrun-reflow</a>, below`,
		`<h4 id="fix-dummyrun-reflow">`,
		"<pre><code>.panel { width: 960px; }</code></pre>",
		`<a href="https://www.w3.org/WAI/WCAG22/Understanding/reflow.html">`,
		"How to act on the advice",
		`<h4 id="fix-dummyrun-plain-latin">`,
		`(<a href="#fix-dummyrun-plain-latin">how to fix</a>, below)`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html lacks %q", want)
		}
	}
	md := render(t, rep, "md")
	for _, want := range []string{"Before: `.panel { width: 960px; }`", "### How to act on the advice", "#### `dummyrun-plain-latin`"} {
		if !strings.Contains(md, want) {
			t.Errorf("md lacks %q", want)
		}
	}
}

// A persona finding's fix shows in the HTML report too.
func TestOwnRuleFindingFixInHTML(t *testing.T) {
	runs := loadFixture(t)
	for _, r := range runs {
		for i := range r.Rec.Findings {
			if r.Rec.Findings[i].Symptom == "a11y:color-contrast" { //nolint:misspell // an axe rule ID
				r.Rec.Findings[i].Symptom = "a11y:dummyrun-reflow"
			}
		}
	}
	rep, err := Build(runs, fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := render(t, rep, "html"); !strings.Contains(got, "<dt>How to fix</dt><dd>Let the page fit a narrow window without scrolling sideways. Usually done by development.<ol><li>") {
		t.Error("html: the finding's fix is missing")
	}
}
