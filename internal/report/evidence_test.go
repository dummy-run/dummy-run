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
	if got == nil || got.Body == "" || got.FixText == "" || got.Element == "" || got.URL == "" || len(got.WCAG) != 1 {
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

// TestEvidenceShownIsTheReproductions checks that when sightings share a fingerprint, the element, the quoted
// words, the page and the body shown come from the sighting whose steps and screenshot are shown, not from the
// sighting that gave the title. Another sighting's words could describe a different page state.
func TestEvidenceShownIsTheReproductions(t *testing.T) {
	runs := loadFixture(t)
	var lead, rep *record.Finding
	for _, run := range runs {
		for i := range run.Rec.Findings {
			f := &run.Rec.Findings[i]
			if f.Fingerprint != "1718b75300702f7b" {
				continue
			}
			switch run.Rec.ID {
			case "2026-10-16-lin-contributor-002":
				lead = f
			case "2026-10-17-lin-contributor-008":
				rep = f
			}
		}
	}
	if lead == nil || rep == nil {
		t.Fatal("the fixture lost the shared finding")
	}
	// The earliest sighting has the worst severity, so it gives the title, and is no longer confirmed, so every
	// sighting counts. The latest is the only one with a screenshot, so it gives the reproduction.
	lead.Severity, lead.Verdict, lead.Title = "major", record.VerdictPending, "Lead's title"
	lead.Body, lead.Fix, lead.Element, lead.Quote, lead.URL, lead.EvidenceStep = "lead body", "lead fix", `text "Paid"`, "Paid", "https://example.test/lead/", lead.Steps[0]
	rep.Severity = "info"
	rep.Body, rep.Fix, rep.Element, rep.Quote, rep.URL, rep.EvidenceStep = "rep body", "rep fix", `text "Payment pending"`, "Payment pending", "https://example.test/rep/", rep.Steps[0]
	report, err := Build(runs, fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	var got *Finding
	for i := range report.Findings {
		if report.Findings[i].Fingerprint == "1718b75300702f7b" {
			got = &report.Findings[i]
		}
	}
	if got == nil || got.Reproduction.Run != "2026-10-17-lin-contributor-008" || got.Title != "Lead's title" {
		t.Fatalf("lead and reproduction are not as the test expects: %+v", got)
	}
	if got.Element != rep.Element || got.Quote != rep.Quote || got.URL != rep.URL || got.Body != rep.Body || got.FixText != rep.Fix {
		t.Errorf("the evidence shown is not the reproduction's: element %q quote %q url %q body %q fix %q",
			got.Element, got.Quote, got.URL, got.Body, got.FixText)
	}
	if len(got.Reproduction.Steps) == 0 || !got.Reproduction.Steps[0].Evidence {
		t.Errorf("the reproduction's evidence step is not marked: %+v", got.Reproduction.Steps)
	}
}
