package report

import (
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
	if got := render(t, rep, "json"); !strings.Contains(got, `"owner": "development"`) {
		t.Errorf("json: no fix on dummyrun-reflow")
	}
}
