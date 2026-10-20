package runner

import (
	"fmt"
	"strings"

	"github.com/dummy-run/dummy-run/internal/identity"
	"github.com/dummy-run/dummy-run/internal/inject"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/record"
)

// injectionTitle is the title of the finding the harness files when a page tries to instruct the persona.
const injectionTitle = "Page content tries to give instructions to an AI persona"

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
			key := h.Pattern + "|" + h.Excerpt
			if seen[key] {
				continue
			}
			seen[key] = true
			hits = append(hits, h)
			r.addEvent("injection:"+key, record.Event{Kind: record.EventSuspectedInjection, Step: s.N,
				Source: h.Source, Pattern: h.Pattern, Detail: h.Excerpt})
		}
	}
	scan(inject.SourceVisible, strings.Join(append([]string{p.Title, o.URL, p.Text}, concat(p.Headings, p.Alerts, o.ConsoleErrors)...), "\n"))
	scan(inject.SourceLabel, strings.Join(append(controlLabels(p.Controls), focusName(p), p.Tree), "\n"))
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

// addInjectionFinding adds the finding for suspected injection to the run's findings. It cites every
// step where something matched. The symptom is misleading-copy, the closest of the closed list: the
// text is there to mislead a reader that is a model.
func (r *run) addInjectionFinding(findings []record.Finding, loc *Locator) []record.Finding {
	var steps []int
	severity := "minor"
	for _, s := range r.steps {
		if len(s.Hits) == 0 {
			continue
		}
		steps = append(steps, s.N)
		for _, h := range s.Hits {
			if h.Source == inject.SourceHidden || h.Pattern == "invisible-characters" {
				severity = "major"
			}
		}
	}
	if len(steps) == 0 {
		return findings
	}
	id := record.FindingID("security", injectionTitle)
	for _, f := range findings {
		if f.ID == id {
			return findings
		}
	}
	const symptom = "misleading-copy"
	route, locus := loc.Route(steps), loc.Locus(steps, "")
	fp, err := identity.Fingerprint(route, locus, symptom)
	if err != nil {
		fmt.Fprintf(r.log, "injection: finding not filed: %v\n", err)
		return findings
	}
	return append(findings, record.Finding{ID: id, Kind: "security", Severity: severity, Title: injectionTitle, Steps: steps,
		Evidence: "measured", Verdict: record.VerdictPending, Class: record.ClassProblem, Route: route, Locus: locus,
		Symptom: symptom, TaxonomyVersion: identity.TaxonomyVersion, Fingerprint: fp})
}
