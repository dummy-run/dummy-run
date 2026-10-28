package runner

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/selfcheck"
)

var goalSection = regexp.MustCompile(`(?ms)^##\s+Goal\s*\n+(.+?)(?:\n\n|\n#|\z)`)

// selfCheck runs the self-check on the persona's findings and applies its decisions: a rewrite replaces the
// title, body and fix, an observation becomes a downgraded note, a drop removes the finding. Every decision goes
// into the record. Its model usage counts towards the run's cost.
func (r *run) selfCheck(ctx context.Context, f *Finish) []record.SelfCheck {
	if f == nil || len(f.Findings) == 0 {
		return nil
	}
	in := selfcheck.Input{Site: r.cfg.Scenario.Site}
	if m := goalSection.FindStringSubmatch(r.cfg.Scenario.Body); m != nil {
		in.Goal = strings.Trim(strings.TrimSpace(m[1]), `"`)
	}
	for _, ff := range f.Findings {
		it := selfcheck.Item{Title: ff.Title, Body: ff.Body, Fix: ff.Fix, Kind: ff.Kind,
			Symptom: ff.Symptom, WCAG: ff.WCAG, Page: ff.URL, Element: ff.Element, Quote: ff.Quote}
		if s := stepAt(r.steps, ff.EvidenceStep); s != nil {
			it.Context = evidenceContext(s, ff.Quote)
			it.Screenshot = s.Obs.Screenshot
		}
		in.Items = append(in.Items, it)
	}
	decisions, usage, err := selfcheck.Review(ctx, r.cfg.Provider, in)
	r.usage.ModelCalls++
	r.usage.InputTokens += usage.InputTokens
	r.usage.OutputTokens += usage.OutputTokens
	r.usage.CachedInputTokens += usage.CachedInputTokens
	if err != nil {
		fmt.Fprintf(r.log, "self-check: %v; the findings are kept as they were\n", err)
	}
	kept := f.Findings[:0:0]
	counts := map[string]int{}
	for i, ff := range f.Findings {
		d := decisions[i]
		counts[d.Decision]++
		switch d.Decision {
		case record.CheckDrop:
			continue
		case record.CheckObservation:
			f.Observations = append(f.Observations, FinishObservation{Text: ff.Title, Steps: ff.Steps, Kind: record.ObservationDowngraded})
			continue
		case record.CheckRewrite:
			ff.Title = d.Title
			if d.Body != "" {
				ff.Body = d.Body
			}
			if d.Fix != "" {
				ff.Fix = d.Fix
			}
		}
		kept = append(kept, ff)
	}
	f.Findings = kept
	fmt.Fprintf(r.log, "self-check: %d in / %d out tokens; kept %d, rewrote %d, notes %d, dropped %d, unchecked %d\n",
		usage.InputTokens, usage.OutputTokens, counts[record.CheckKeep], counts[record.CheckRewrite], counts[record.CheckObservation],
		counts[record.CheckDrop], counts[record.CheckUnchecked])
	return decisions
}

// evidenceContext is the page at the evidence step as the self-check reads it: the title, the headings and the
// text around the quote, or the start of the text when there is no quote.
func evidenceContext(s *Step, quote string) string {
	snap := s.Obs.Snapshot
	if snap == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", snap.Title)
	if len(snap.Headings) > 0 {
		h := snap.Headings
		if len(h) > 8 {
			h = h[:8]
		}
		fmt.Fprintf(&b, "Headings: %s\n", strings.Join(h, " | "))
	}
	text := strings.Join(strings.Fields(snap.Text), " ")
	start := 0
	if q := strings.Fields(quote); len(q) > 0 {
		if i := strings.Index(strings.ToLower(text), strings.ToLower(q[0])); i >= 0 {
			start = max(0, i-300)
		}
	}
	end := min(len(text), start+700)
	for start > 0 && start < len(text) && !isRuneStart(text[start]) {
		start++
	}
	for end < len(text) && !isRuneStart(text[end]) {
		end++
	}
	fmt.Fprintf(&b, "Text: %s", text[start:end])
	return b.String()
}

func isRuneStart(c byte) bool { return c&0xC0 != 0x80 }
