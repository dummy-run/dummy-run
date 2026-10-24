package a11y

import (
	"fmt"

	"github.com/dummy-run/dummy-run/internal/page"
)

// Rule IDs of the keyboard checks. They sit beside axe's rule IDs in the report and the allow-list.
const (
	RuleFocusNotVisible = "dummyrun-focus-not-visible"
	RuleFocusHidden     = "dummyrun-focus-hidden"
	RuleKeyboardTrap    = "dummyrun-keyboard-trap"
	RuleSkipLinkBroken  = "dummyrun-skip-link-broken"
)

// KeyboardRules are the keyboard checks in the order the report lists them.
var KeyboardRules = []string{RuleKeyboardTrap, RuleFocusNotVisible, RuleFocusHidden, RuleSkipLinkBroken}

var keyboardRules = map[string]layoutRule{
	RuleKeyboardTrap: {"critical", "Keyboard focus gets stuck and Tab and Escape do not get out",
		"https://www.w3.org/WAI/WCAG22/Understanding/no-keyboard-trap.html", []string{"2.1.2"}},
	RuleFocusNotVisible: {"serious", "Nothing on screen changes when the element has keyboard focus",
		"https://www.w3.org/WAI/WCAG22/Understanding/focus-visible.html", []string{"2.4.7"}},
	RuleFocusHidden: {"serious", "Keyboard focus lands on an element you cannot see",
		"https://www.w3.org/WAI/WCAG22/Understanding/focus-visible.html", []string{"2.4.7"}},
	RuleSkipLinkBroken: {"serious", "The skip link points to a place that does not exist or is not shown",
		"https://www.w3.org/WAI/WCAG22/Understanding/bypass-blocks.html", []string{"2.4.1"}},
}

// IsKeyboardRule reports whether the rule ID is one of the keyboard checks.
func IsKeyboardRule(id string) bool { _, ok := keyboardRules[id]; return ok }

// keyboardFindings turns a Tab walk into findings, one per rule that has any. Each node is something the walk
// measured: a place focus went, with the selector of the element.
func keyboardFindings(w *page.TabWalk, path string, allow *AllowList) []Finding {
	byRule := map[string][]Node{}
	for _, s := range w.Trap {
		byRule[RuleKeyboardTrap] = append(byRule[RuleKeyboardTrap], Node{Target: s.Selector,
			Detail: fmt.Sprintf("focus cycles through %d elements and Escape does not leave", len(w.Trap))})
	}
	for _, s := range w.Stops {
		switch {
		case s.Hidden || s.Offscreen:
			why := "it has no size or is hidden"
			if s.Offscreen {
				why = "it sits outside the window"
			}
			byRule[RuleFocusHidden] = append(byRule[RuleFocusHidden], Node{Target: s.Selector, Detail: why})
		case s.NoIndicator:
			byRule[RuleFocusNotVisible] = append(byRule[RuleFocusNotVisible], Node{Target: s.Selector,
				Detail: "the pixels around it are the same with and without focus"})
		}
	}
	if sk := w.Skip; sk != nil && (!sk.Found || !sk.Rendered) {
		d := fmt.Sprintf("%q points to %s, which is not on the page", sk.Text, sk.Href)
		if sk.Found {
			d = fmt.Sprintf("%q points to %s, which is not shown", sk.Text, sk.Href)
		}
		byRule[RuleSkipLinkBroken] = append(byRule[RuleSkipLinkBroken], Node{Target: sk.Selector, Detail: d})
	}
	var list []Finding
	for _, id := range KeyboardRules {
		nodes := byRule[id]
		if len(nodes) == 0 {
			continue
		}
		r := keyboardRules[id]
		f := Finding{Rule: id, Impact: r.impact, WCAG: r.wcag, Help: r.help, HelpURL: r.helpURL, Nodes: []Node{}}
		for _, n := range nodes {
			if e := allow.Match(id, path, n.Target); e != nil {
				n.Allowed, n.Reason = true, e.Reason
			}
			f.Nodes = append(f.Nodes, n)
		}
		list = append(list, f)
	}
	return list
}
