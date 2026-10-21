package report

import (
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/record"
)

// TestAFindingShowsItsEvidenceInWords checks that a finding carries the element, the page, the body, the fix and
// the WCAG names from the record, that its steps name controls instead of @numbers, that a signed-out visitor is
// not told to sign in, and that notes which did not become findings are not shown as things that worked.
func TestAFindingShowsItsEvidenceInWords(t *testing.T) {
	runs := loadFixture(t)
	rec := runs[0].Rec
	f := &rec.Findings[0]
	f.Body = "Screen reader users hear three links called Read more and cannot tell them apart."
	f.Fix = "Add the post title to each link."
	f.WCAG = []string{"2.4.4"}
	f.Element = `link "Read more"`
	f.Quote = "Read more"
	f.URL = "https://example.test/blog/"
	f.EvidenceStep = f.Steps[0]
	f.Symptom = "unclear-label"
	f.Fingerprint = "feedfacecafebeef"
	for i := range rec.Steps {
		if rec.Steps[i].N == f.Steps[0] {
			rec.Steps[i].Action, rec.Steps[i].Target, rec.Steps[i].Element = "click", "@17", `link "Read more"`
			rec.Steps[i].Note = "blocked: GET https://analytics.example/beacon.js (host not allowed)"
		}
	}
	rec.Persona.Role, rec.Persona.Name, rec.Signin = "visitor", "Alex", nil
	rec.Observations = append(rec.Observations,
		record.Observation{Text: "I could not find any pricing.", Steps: []int{1}, Kind: record.ObservationUnverified},
		record.Observation{Text: "No newsletter sign-up.", Steps: []int{1}, Kind: record.ObservationDowngraded})
	rep, err := Build(runs, fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	var got *Finding
	for i := range rep.Findings {
		if rep.Findings[i].Fingerprint == "feedfacecafebeef" {
			got = &rep.Findings[i]
		}
	}
	if got == nil || got.Body == "" || got.Fix == "" || got.Element == "" || got.URL == "" || len(got.WCAG) != 1 {
		t.Fatalf("the report lost the evidence: %+v", got)
	}
	for _, format := range []string{"html", "md"} {
		out := render(t, rep, format)
		for _, want := range []string{"Screen reader users hear three links", "Add the post title to each link.",
			"2.4.4 Link Purpose (In Context)", `Click the "Read more" link.`, `The "Read more" link on https://example.test/blog/`, "Alex, a visitor,"} {
			if !strings.Contains(out, want) && !strings.Contains(out, strings.ReplaceAll(want, `"`, "&#34;")) {
				t.Errorf("%s is missing %q", format, want)
			}
		}
		for _, bad := range []string{"Click @17", "Sign in as a visitor", "The persona said", "host not allowed", "any pricing", "newsletter"} {
			if strings.Contains(out, bad) {
				t.Errorf("%s still has %q", format, bad)
			}
		}
	}
}
