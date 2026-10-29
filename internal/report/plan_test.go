package report

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

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

// writePlan writes a plan.json with the given usage and journeys, for the cases the fixture does not cover.
func writePlan(t *testing.T, usage, journeys, personaJourneys string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.json")
	body := fmt.Sprintf(`{"plan_version":1,"url":"https://x.example/","site_type":"blog","sells":false,"journeys":%s,
	 "personas":[{"id":"first-time-visitor","name":"Alex","label":"First-time visitor","why":"","journeys":%s,"scenario":{"id":"s"}}],
	 "source":"model","usage":%s}`, journeys, personaJourneys, usage)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPlanCostCases(t *testing.T) {
	for name, tc := range map[string]struct {
		usage         string
		calls, floor  int
		wantUSDChange bool
	}{
		"priced":   {`{"model_calls":1,"input_tokens":3000,"output_tokens":800,"cost_usd":0.00585}`, 1, 0, true},
		"unpriced": {`{"model_calls":1,"input_tokens":3000,"output_tokens":800,"cost_usd":null}`, 1, 1, false},
		"fallback": {`{"model_calls":0,"input_tokens":0,"output_tokens":0,"cost_usd":null}`, 0, 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			ps, usage, err := LoadPlan(writePlan(t, tc.usage, `[{"id":"read","name":"Read a post","why":"w"}]`, `["read"]`))
			if err != nil {
				t.Fatal(err)
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
			c, w := rep.Scope.Cost, without.Scope.Cost
			if c.ModelCalls != w.ModelCalls+tc.calls || c.UnpricedRuns != w.UnpricedRuns+tc.floor || (c.USD != w.USD) != tc.wantUSDChange {
				t.Errorf("cost with the plan = %+v, without = %+v", c, w)
			}
			if tc.floor > 0 && !strings.Contains(render(t, rep, "md"), "so the cost is a floor") {
				t.Errorf("an unpriced planning call should make the cost a floor")
			}
		})
	}
}

func TestPlanSectionWithoutRuns(t *testing.T) {
	ps, usage, err := LoadPlan("testdata/plan.json")
	if err != nil {
		t.Fatal(err)
	}
	opt := Options{A11y: fixtureOptions(t).A11y, Date: fixedDate, Plan: ps, PlanUsage: usage}
	rep, err := Build(nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scope.Cost.ModelCalls != 1 {
		t.Fatalf("cost = %+v", rep.Scope.Cost)
	}
	for _, format := range []string{"html", "md"} {
		out := render(t, rep, format)
		if !strings.Contains(out, "Model cost") || !strings.Contains(out, "over 1 model calls") {
			t.Errorf("%s report with a plan and no runs drops the planning cost", format)
		}
		if !strings.Contains(out, "What we tested and why") {
			t.Errorf("%s report with a plan and no runs drops the plan section", format)
		}
	}
	plain, err := Build(nil, Options{A11y: fixtureOptions(t).A11y, Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(render(t, plain, "html"), "Model cost") {
		t.Errorf("a sweep-only report with no model calls shows a cost")
	}
}

func TestPlanTextKeepsRunesAndArrays(t *testing.T) {
	ps, _, err := LoadPlan(writePlan(t, `{"model_calls":0,"cost_usd":null}`,
		`[{"id":"elan","name":"Élan vital lesen","why":"w"},{"id":"jp","name":"日本語の記事を読む","why":"w"},{"id":"nhs","name":"NHS services","why":"w"}]`, `["elan","jp","nhs"]`))
	if err != nil {
		t.Fatal(err)
	}
	if want := "We sent Alex, a first-time visitor, to élan vital lesen, 日本語の記事を読む and NHS services."; ps.Personas[0].Text != want {
		t.Errorf("text = %q\nwant   %q", ps.Personas[0].Text, want)
	}
	empty, _, err := LoadPlan(writePlan(t, `{"model_calls":0,"cost_usd":null}`, `null`, `null`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("a plan with no journeys serialises null somewhere: %s", data)
	}
	for _, s := range []string{"Élan", "élan", "ÉLAN", "É", "", "iPhone"} {
		if got := lowerFirst(s); utf8.RuneCountInString(got) != utf8.RuneCountInString(s) {
			t.Errorf("lowerFirst(%q) = %q changed the rune count", s, got)
		}
	}
}

func TestLoadPlanRejectsOtherJSON(t *testing.T) {
	if _, _, err := LoadPlan("testdata/a11y.json"); err == nil {
		t.Error("an a11y file was read as a plan")
	}
}
