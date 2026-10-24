package a11y

import "github.com/dummy-run/dummy-run/internal/page"

// Rule IDs of the markup checks. They sit beside axe's rule IDs in the report and the allow-list.
const (
	RuleLabelInName     = "dummyrun-label-in-name"
	RuleAutocompleteOff = "dummyrun-autocomplete-missing"
)

// MarkupRules are the markup checks in the order the report lists them.
var MarkupRules = []string{RuleLabelInName, RuleAutocompleteOff}

var markupRules = map[string]layoutRule{
	RuleLabelInName: {"serious", "The words a control shows are not part of its accessible name",
		"https://www.w3.org/WAI/WCAG22/Understanding/label-in-name.html", []string{"2.5.3"}},
	RuleAutocompleteOff: {"serious", "A field that asks for your own details has no autocomplete value",
		"https://www.w3.org/WAI/WCAG22/Understanding/identify-input-purpose.html", []string{"1.3.5"}},
}

// IsMarkupRule reports whether the rule ID is one of the markup checks.
func IsMarkupRule(id string) bool { _, ok := markupRules[id]; return ok }

func markupFindings(m *page.MarkupChecks, path string, allow *AllowList) []Finding {
	by := map[string][]page.CheckNode{RuleLabelInName: m.LabelInName, RuleAutocompleteOff: m.Autocomplete}
	var list []Finding
	for _, id := range MarkupRules {
		if len(by[id]) == 0 {
			continue
		}
		r := markupRules[id]
		f := Finding{Rule: id, Impact: r.impact, WCAG: r.wcag, Help: r.help, HelpURL: r.helpURL, Nodes: []Node{}}
		for _, c := range by[id] {
			n := Node{Target: c.Selector, Detail: c.Detail}
			if e := allow.Match(id, path, n.Target); e != nil {
				n.Allowed, n.Reason = true, e.Reason
			}
			f.Nodes = append(f.Nodes, n)
		}
		list = append(list, f)
	}
	return list
}
