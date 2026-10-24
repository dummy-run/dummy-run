package runner

import (
	"strings"

	"github.com/dummy-run/dummy-run/internal/inject"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/record"
)

// inspect looks at what a step brought back. It records a refused request to another host as an event
// and scans the page for text that looks like an instruction to the model. It returns the hits, which
// the step keeps so the persona is warned and the record can cite the step.
func (r *run) inspect(s *Step) []inject.Hit {
	o := s.Obs
	for _, b := range o.Blocked {
		if strings.HasSuffix(b, "("+policy.BlockHost.String()+")") {
			r.addEvent("blocked:"+b, record.Event{Kind: record.EventOffOriginBlocked, Step: s.N, Detail: "request refused: " + b})
		}
	}
	p := o.Snapshot
	if p == nil {
		return nil
	}
	var hits []inject.Hit
	seen := map[string]bool{}
	scan := func(source, text string) {
		for _, h := range r.scanner.Scan(source, text) {
			key := h.Source + "|" + h.Pattern + "|" + h.Excerpt
			if seen[key] {
				continue
			}
			seen[key] = true
			detail := h.Excerpt
			if h.Low {
				// A weak match is an event for the reviewer. It does not warn the persona or file a finding.
				detail = "low confidence: " + detail
			} else {
				hits = append(hits, h)
			}
			r.addEvent("injection:"+key, record.Event{Kind: record.EventSuspectedInjection, Step: s.N,
				Source: h.Source, Pattern: h.Pattern, Detail: detail})
		}
	}
	visible := append([]string{p.Title, o.URL}, concat(p.Headings, p.Alerts)...)
	if p.ScanText != "" {
		// The browser cut out the visible code and quotes. Headings and alerts are in it already.
		visible = []string{p.Title, o.URL, p.ScanText}
	} else {
		visible = append(visible, p.Text)
	}
	scan(inject.SourceVisible, strings.Join(append(visible, o.ConsoleErrors...), "\n"))
	scan(inject.SourceLabel, strings.Join(append(controlLabels(p.Controls), focusName(p), p.Tree), "\n"))
	if p.Spoken != nil {
		// What the screen reader says is mostly names and labels, read out in the page's order.
		scan(inject.SourceLabel, strings.Join(p.Spoken.Phrases, "\n"))
	}
	for _, h := range p.Hidden {
		scan(inject.SourceHidden, h)
	}
	return hits
}

func concat(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

func controlLabels(cs []page.Control) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Label)
	}
	return out
}

func focusName(p *page.Snapshot) string {
	if p.Focus == nil {
		return ""
	}
	return p.Focus.Name
}
