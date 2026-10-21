package runner

import (
	"fmt"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/record"
)

// slowStep is how long one step must take before a "slow" finding counts as measured.
const slowStep = 5 * time.Second

// minQuoteRunes is the shortest quote the engine will look for. Shorter text matches almost any page.
const minQuoteRunes = 3

// VerifyEvidence checks every finding against what the engine saw at the steps it cites. A finding is backed when
// a cited step that the browser really read shows its control (@ref), its quoted words, the failing axe rule, or
// the measured error, slowness or broken status its symptom names. A backed finding gets the element, the page
// address and the step where the evidence is. VerifyEvidence returns one message per finding that is not backed,
// saying what to fix, and marks those findings with an empty EvidenceStep.
func VerifyEvidence(f *Finish, steps []Step) []string {
	var problems []string
	for i := range f.Findings {
		ff := &f.Findings[i]
		ff.Element, ff.URL, ff.EvidenceStep = "", "", 0
		if msg := verifyOne(ff, steps); msg != "" {
			problems = append(problems, msg)
		}
	}
	return problems
}

// DemoteUnverified moves the findings VerifyEvidence could not back into the observations as unverified notes,
// so a claim without evidence never reaches a report as a finding. It returns how many it moved.
func DemoteUnverified(f *Finish) int {
	kept := f.Findings[:0:0]
	n := 0
	for _, ff := range f.Findings {
		if ff.EvidenceStep > 0 {
			kept = append(kept, ff)
			continue
		}
		f.Observations = append(f.Observations, FinishObservation{Text: ff.Title, Steps: ff.Steps, Kind: record.ObservationUnverified})
		n++
	}
	f.Findings = kept
	return n
}

func verifyOne(ff *FinishFinding, steps []Step) string {
	var read []*Step
	for _, n := range ff.Steps {
		if s := stepAt(steps, n); s != nil && (readable(s) || s.Obs.A11y != nil) {
			read = append(read, s)
		}
	}
	if len(read) == 0 {
		return fmt.Sprintf("finding %q: the steps it cites (%s) are steps where the browser could not read the page, so they show nothing about the site. "+
			"Cite a step where the page loaded, or leave the finding out", ff.Title, joinSteps(ff.Steps))
	}
	ref := strings.TrimPrefix(strings.TrimSpace(ff.Control), "@")
	if ref != "" {
		for _, s := range read {
			if s.Obs.Snapshot == nil {
				continue
			}
			if el, ok := controlAt(s.Obs.Snapshot, ref); ok {
				ff.Element, ff.URL, ff.EvidenceStep = el, s.Obs.URL, s.N
				if ff.Quote != "" && !quoteIn(s.Obs.Snapshot, ff.Quote) {
					ff.Quote = ""
				}
				return ""
			}
		}
	}
	if q := ff.Quote; runeCount(normalise(q)) >= minQuoteRunes {
		for _, s := range read {
			if s.Obs.Snapshot == nil {
				continue
			}
			if el, ok := quoteElement(s.Obs.Snapshot, q); ok {
				ff.Element, ff.URL, ff.EvidenceStep = el, s.Obs.URL, s.N
				return ""
			}
		}
	}
	ff.Quote = ""
	base, param, _ := strings.Cut(ff.Symptom, ":")
	if base == "a11y" {
		param = strings.ToLower(strings.TrimSpace(ff.Rule))
	}
	for _, s := range read {
		if el, ok := measured(s, base, param); ok {
			ff.Element, ff.URL, ff.EvidenceStep = el, s.Obs.URL, s.N
			return ""
		}
	}
	return fmt.Sprintf("finding %q: nothing at steps %s shows it. Give the control as @number from one of those steps, or copy into quote the exact words "+
		"on the page that show the problem (at most 15 words), or cite the step that does show it. A finding with no evidence becomes a note and is not reported",
		ff.Title, joinSteps(ff.Steps))
}

func stepAt(steps []Step, n int) *Step {
	if n < 1 || n > len(steps) {
		return nil
	}
	return &steps[n-1]
}

// readable is true when the browser read the page at this step. A harness failure, such as a lost connection to
// the browser, leaves no page to show and is never evidence about the site.
func readable(s *Step) bool {
	if s.Obs.Snapshot == nil {
		return false
	}
	e := s.Obs.Error
	return !strings.Contains(e, "bidi:") && !strings.Contains(e, "could not read the page")
}

// controlAt names the control with this @ref as role and accessible name, such as link "Read more".
func controlAt(snap *page.Snapshot, ref string) (string, bool) {
	if role, name, _, ok := fromTree(snap.Tree, ref); ok {
		return elementText(role, name), true
	}
	for _, c := range snap.Controls {
		if c.Ref == ref {
			return elementText(roleOf(c), cleanLabel(c.Label)), true
		}
	}
	return "", false
}

// ElementFor names the control an action used, from the controls the persona saw before it, or "" when the
// target was not an @ref the page listed.
func ElementFor(target string, before []page.Control, snap *page.Snapshot) string {
	t := strings.TrimSpace(target)
	if !strings.HasPrefix(t, "@") {
		return ""
	}
	ref := strings.TrimPrefix(t, "@")
	for _, c := range before {
		if c.Ref == ref {
			return elementText(roleOf(c), cleanLabel(c.Label))
		}
	}
	if snap != nil {
		if el, ok := controlAt(snap, ref); ok {
			return el
		}
	}
	return ""
}

func elementText(role, name string) string {
	if role == "" {
		role = "element"
	}
	if strings.TrimSpace(name) == "" {
		return role + " with no name"
	}
	return fmt.Sprintf("%s %q", role, shorten(strings.Join(strings.Fields(name), " "), 80))
}

// quoteElement finds the quoted words on the page and names what holds them: a control, a heading or the page text.
func quoteElement(snap *page.Snapshot, q string) (string, bool) {
	nq := normalise(q)
	for _, c := range snap.Controls {
		if label := normalise(cleanLabel(c.Label)); label != "" && containsQuote(label, nq) {
			return elementText(roleOf(c), cleanLabel(c.Label)), true
		}
	}
	for _, h := range snap.Headings {
		if containsQuote(normalise(h), nq) {
			return elementText("heading", h), true
		}
	}
	if quoteIn(snap, q) {
		return fmt.Sprintf("text %q", shorten(strings.Join(strings.Fields(q), " "), 120)), true
	}
	return "", false
}

func quoteIn(snap *page.Snapshot, q string) bool {
	nq := normalise(q)
	hay := []string{snap.Title, snap.Text, snap.Tree, strings.Join(snap.Headings, "\n"), strings.Join(snap.Alerts, "\n")}
	for _, c := range snap.Controls {
		hay = append(hay, c.Label)
	}
	for _, h := range hay {
		if containsQuote(normalise(h), nq) {
			return true
		}
	}
	return false
}

// containsQuote is true when every part of the quote, split at an ellipsis, is in the text in order.
func containsQuote(text, quote string) bool {
	rest := text
	any := false
	for _, part := range strings.Split(quote, "...") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		i := strings.Index(rest, part)
		if i < 0 {
			return false
		}
		rest = rest[i+len(part):]
		any = true
	}
	return any
}

var quoteFold = strings.NewReplacer("‘", "'", "’", "'", "“", "\"", "”", "\"", "…", "...", " ", " ", "–", "-", "—", "-")

// normalise lower-cases text, folds curly quotes and dashes, collapses white space and trims surrounding quotes
// and full stops, so a quote the model copied matches the page however it spaced or punctuated it.
func normalise(s string) string {
	s = strings.ToLower(quoteFold.Replace(s))
	s = strings.Join(strings.Fields(s), " ")
	return strings.Trim(s, " \"'.,;:")
}

func runeCount(s string) int { return len([]rune(s)) }

// measured is true when the engine itself measured the symptom at this step: the failing axe rule, an error status
// or console error, a failed request, or a step slower than a person would wait.
func measured(s *Step, symptom, rule string) (string, bool) {
	o := s.Obs
	switch symptom {
	case "a11y":
		for _, v := range o.A11y {
			if rule != "" && v.ID == rule {
				return fmt.Sprintf("axe rule %s on %d element(s)", v.ID, v.Nodes), true
			}
		}
	case "error-shown", "dead-link":
		switch {
		case o.Status >= 400:
			return fmt.Sprintf("page status %d", o.Status), true
		case symptom == "error-shown" && len(o.ConsoleErrors) > 0:
			return "console error: " + shorten(o.ConsoleErrors[0], 120), true
		case symptom == "error-shown" && len(o.FailedRequests) > 0:
			return "failed request: " + shorten(o.FailedRequests[0], 120), true
		}
	case "slow":
		if o.Duration >= slowStep {
			return fmt.Sprintf("step took %.1f seconds", o.Duration.Seconds()), true
		}
	}
	return "", false
}

func joinSteps(n []int) string {
	parts := make([]string, len(n))
	for i, v := range n {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, ", ")
}
