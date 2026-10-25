package a11y

import (
	"context"
	"fmt"
	"strings"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/plain"
)

// The plain language rules, by the Vale check that finds them. None of them is a WCAG A or AA failure, so they go to
// advice and never fail the sweep.
var plainRules = map[string]struct{ id, help string }{
	"DummyRun.WordsToAvoid":         {"dummyrun-plain-words-to-avoid", "A word the GOV.UK style guide asks you to avoid, because it is vague"},
	"DummyRun.LongSentence":         {"dummyrun-plain-long-sentence", "A sentence of more than 25 words, which is harder to hold in mind"},
	"DummyRun.Latin":                {"dummyrun-plain-latin", "A Latin abbreviation, which some readers and screen readers stumble on"},
	"Readability.FleschReadingEase": {"dummyrun-plain-reading-ease", "The main content scores below 70 for Flesch reading ease"},
	"Readability.FleschKincaid":     {"dummyrun-plain-reading-grade", "The main content needs a Flesch-Kincaid grade above 8"},
}

// PlainRules are the plain language rule IDs in the order the report lists them.
var PlainRules = []string{"dummyrun-plain-reading-ease", "dummyrun-plain-reading-grade", "dummyrun-plain-long-sentence",
	"dummyrun-plain-words-to-avoid", "dummyrun-plain-latin"}

// plainFindings turns Vale's alerts on the main content into advice, one finding per rule.
func plainFindings(alerts []plain.Alert, selector, path string, allow *AllowList) []Finding {
	by := map[string]*Finding{}
	for _, a := range alerts {
		r, ok := plainRules[a.Check]
		if !ok {
			continue
		}
		f, ok := by[r.id]
		if !ok {
			f = &Finding{Rule: r.id, Help: r.help, HelpURL: a.Link, Nodes: []Node{}}
			by[r.id] = f
		}
		// Vale's line numbers count lines of the extracted copy, not the page's source, so they are left out.
		detail := a.Message
		switch {
		case r.id == "dummyrun-plain-long-sentence" && a.Match != "":
			detail = fmt.Sprintf("a sentence that starts with %q has more than 25 words", a.Match)
		case a.Match != "" && !strings.Contains(a.Message, a.Match):
			detail = fmt.Sprintf("%q: %s", a.Match, a.Message)
		}
		n := Node{Target: selector, Detail: detail}
		if e := allow.Match(r.id, path, n.Target); e != nil {
			n.Allowed, n.Reason = true, e.Reason
		}
		f.Nodes = append(f.Nodes, n)
	}
	var out []Finding
	for _, id := range PlainRules {
		if f, ok := by[id]; ok {
			out = append(out, *f)
		}
	}
	return out
}

// plainCheck runs Vale on the page's main content. A missing Vale or a failed check is recorded on the result and
// does not stop the page's other checks, so a sweep with every check on still runs where Vale is not installed.
func plainCheck(ctx context.Context, conn *bidi.Conn, cx string, res *PageResult, path string, allow *AllowList) {
	vale, err := plain.Find()
	if err != nil {
		res.PlainLanguageError = err.Error()
		return
	}
	m, err := page.MainHTML(ctx, conn, cx)
	if err != nil {
		res.PlainLanguageError = firstLine(err.Error())
		return
	}
	// A main landmark the page hides has no words a visitor reads, so there is nothing to check.
	if m.HTML == "" {
		res.PlainLanguageChecked = true
		return
	}
	alerts, err := plain.Check(ctx, vale, m.HTML)
	if err != nil {
		res.PlainLanguageError = firstLine(err.Error())
		return
	}
	res.PlainLanguageChecked, res.PlainLanguagePartial = true, m.Truncated
	res.Advice = append(res.Advice, plainFindings(alerts, m.Selector, path, allow)...)
}
