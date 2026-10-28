package report

import (
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/a11y"
)

// A colour contrast failure lists each pair of colours once, with the colours that meet the ratio, in every format.
func TestContrastColoursReachTheReport(t *testing.T) {
	opts := fixtureOptions(t)
	p := &opts.A11y[0].Results[0]
	grey := &a11y.Colours{Text: "#777777", Background: "#ffffff", Ratio: 4.48, Need: 4.5, TextFix: "#767676", BackgroundFix: "#000000"}
	p.Violations = append(p.Violations, a11y.Finding{Rule: "color-contrast", Impact: "serious", WCAG: []string{"1.4.3"},
		Help:  "Elements must meet minimum color contrast ratio thresholds",
		Nodes: []a11y.Node{{Target: "#a", Colours: grey}, {Target: "#b", Colours: grey}, {Target: "#c"}}})
	rep, err := Build(loadFixture(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	var found *AxeRule
	for i, r := range rep.Axe.Rules {
		if r.Rule == "color-contrast" {
			found = &rep.Axe.Rules[i]
		}
	}
	if found == nil || len(found.Colours) != 1 {
		t.Fatalf("want one colour pair on color-contrast: %+v", found)
	}
	if c := found.Colours[0]; c.Elements != 2 || c.TextFix != "#767676" || c.BackgroundFix != "#000000" {
		t.Errorf("colour pair = %+v", c)
	}
	for _, format := range []string{"md", "html"} {
		got := render(t, rep, format)
		if !strings.Contains(got, "Colours that meet the ratio") || !strings.Contains(got, "#767676") || !strings.Contains(got, "4.48:1") {
			t.Errorf("%s: no colours table with the suggestion", format)
		}
	}
	if !strings.Contains(render(t, rep, "json"), `"text_fix": "#767676"`) {
		t.Error("json: no text_fix")
	}
}

// A report with no colour pairs has no colours table.
func TestNoColoursTableWithoutPairs(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"md", "html"} {
		if strings.Contains(render(t, rep, format), "Colours that meet the ratio") {
			t.Errorf("%s: colours table with no pairs", format)
		}
	}
}
