package a11y

import (
	"slices"
	"strings"

	"github.com/dummy-run/dummy-run/internal/page"
)

// RuleEnginesPrefix starts the rule ID of a failure that the other engines report and axe does not. The criterion
// follows, as in dummyrun-engines-1.1.1.
const RuleEnginesPrefix = "dummyrun-engines-"

// Confidence of a failing element, from how many engines report it for the same criterion.
const (
	ConfidenceHigh   = "high"   // two or more engines agree
	ConfidenceSingle = "single" // one engine alone
)

// engineNames are the names a report shows for each engine.
var engineNames = map[string]string{
	"axe":    "axe-core",
	"ibm":    "IBM Equal Access",
	"htmlcs": "HTML_CodeSniffer",
	"alfa":   "Siteimprove Alfa",
}

// IsEnginesRule reports whether the rule ID is one the other engines produced.
func IsEnginesRule(id string) bool { return strings.HasPrefix(id, RuleEnginesPrefix) }

// axeNodes lists the elements of the axe violations, in order, with the criteria of their rule.
func axeNodes(violations []Finding) []page.AxeNode {
	var out []page.AxeNode
	for _, f := range violations {
		for _, n := range f.Nodes {
			out = append(out, page.AxeNode{SC: f.WCAG, Selector: n.Target})
		}
	}
	return out
}

// engineFindings marks each axe violation node with the engines that agree with it, and turns the failures axe did
// not report into findings. A group two or more engines agree on is a violation. A group one engine reports alone is
// a candidate: it is kept for review and never fails the sweep.
func engineFindings(r *page.EngineResults, violations []Finding, path string, allow *AllowList) (agreed, candidates []Finding) {
	i := 0
	for _, f := range violations {
		// f is a copy, but its Nodes share the backing array, so the marks land in violations.
		for ni, n := range f.Nodes {
			engines := []string{"axe"}
			if i < len(r.Axe) {
				engines = append(engines, r.Axe[i]...)
			}
			slices.Sort(engines)
			n.Engines = engines
			n.Confidence = confidence(engines)
			f.Nodes[ni] = n
			i++
		}
	}
	agreedAt, candAt := map[string]int{}, map[string]int{} // rule ID -> index in its list
	for _, g := range r.Groups {
		if len(g.SC) == 0 {
			continue
		}
		sc := slices.Clone(g.SC)
		slices.SortFunc(sc, compareCriteria)
		id := RuleEnginesPrefix + sc[0]
		// Agreed failures are serious, as the sweep's own measured rules are. Candidates never fail the sweep, so they
		// carry no impact.
		at, list, help, impact := candAt, &candidates, "One engine reports a possible failure of ", ""
		if len(g.Engines) >= 2 {
			at, list, help, impact = agreedAt, &agreed, "Two or more engines agree this fails ", "serious"
		}
		k, ok := at[id]
		if !ok {
			*list = append(*list, Finding{Rule: id, Impact: impact, WCAG: []string{sc[0]}, Help: help + "WCAG " + sc[0] + " " + wcag22[sc[0]].title,
				HelpURL: understandingURL(sc[0]), Nodes: []Node{}})
			k = len(*list) - 1
			at[id] = k
		}
		n := Node{Target: g.Selector, Detail: hitsDetail(g.Hits), Engines: g.Engines, Confidence: confidence(g.Engines)}
		if e := allow.Match(id, path, n.Target); e != nil {
			n.Allowed, n.Reason = true, e.Reason
		}
		(*list)[k].Nodes = append((*list)[k].Nodes, n)
	}
	sortFindings(agreed)
	sortFindings(candidates)
	return agreed, candidates
}

func confidence(engines []string) string {
	if len(engines) >= 2 {
		return ConfidenceHigh
	}
	return ConfidenceSingle
}

// hitsDetail says in words which engine reported what, one rule per engine.
func hitsDetail(hits []page.EngineHit) string {
	seen := map[string]bool{}
	var parts []string
	for _, h := range hits {
		key := h.Engine + "/" + h.Rule
		if seen[key] {
			continue
		}
		seen[key] = true
		s := engineNames[h.Engine] + " (" + h.Rule + ")"
		if h.Message != "" {
			s += ": " + h.Message
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

func sortFindings(list []Finding) {
	slices.SortStableFunc(list, func(a, b Finding) int { return compareCriteria(a.WCAG[0], b.WCAG[0]) })
}
