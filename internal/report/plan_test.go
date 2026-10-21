package report

import (
	"math"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestPlanSection(t *testing.T) {
	ps, usage, err := LoadPlan("testdata/plan.json")
	if err != nil {
		t.Fatal(err)
	}
	want := "Your site looks like a personal site for people who want to know who Sam is and read her posts, so we tried the things its visitors come to do."
	if ps.Intro != want {
		t.Errorf("intro = %q", ps.Intro)
	}
	if got := ps.Personas[0].Text; got != "We sent Alex, a first-time visitor, to find out who Sam is, read a post and get in touch. A new reader shows whether the site makes sense at first sight." {
		t.Errorf("persona text = %q", got)
	}
	without, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	opt := fixtureOptions(t)
	opt.Plan, opt.PlanUsage = ps, usage
	rep, err := Build(loadFixture(t), opt)
	if err != nil {
		t.Fatal(err)
	}
	if d := rep.Scope.Cost.USD - without.Scope.Cost.USD; math.Abs(d-0.00585) > 1e-9 || rep.Scope.Cost.ModelCalls != without.Scope.Cost.ModelCalls+1 {
		t.Errorf("planning cost was not added: %+v vs %+v", rep.Scope.Cost, without.Scope.Cost)
	}
	html := render(t, rep, "html")
	for _, s := range []string{`<h2 id="h-tested">What we tested and why</h2>`, `href="#tested"`, "<strong>Read a post.</strong>", "We sent Alex"} {
		if !strings.Contains(html, s) {
			t.Errorf("HTML lacks %q", s)
		}
	}
	md := render(t, rep, "md")
	if !strings.Contains(md, "## What we tested and why\n\nYour site looks like a personal site") || !strings.Contains(md, "- **Get in touch.** The site invites messages.") {
		t.Errorf("Markdown section is missing or malformed")
	}
	if strings.Index(md, "## What we tested and why") > strings.Index(md, "## Findings") {
		t.Errorf("the section comes after the findings")
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(render(t, rep, "json")))
	if err != nil {
		t.Fatal(err)
	}
	if err := compileSchema(t).Validate(doc); err != nil {
		t.Errorf("report JSON with a plan does not match the schema: %v", err)
	}
	if strings.Contains(render(t, without, "html"), "What we tested and why") {
		t.Errorf("a report without a plan shows the section")
	}
}

func TestLoadPlanRejectsOtherJSON(t *testing.T) {
	if _, _, err := LoadPlan("testdata/a11y.json"); err == nil {
		t.Error("an a11y file was read as a plan")
	}
}
