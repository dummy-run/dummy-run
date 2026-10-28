package a11y

import (
	"fmt"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/record"
)

// Rule IDs of the keyboard checks. They sit beside axe's rule IDs in the report and the allow-list.
const (
	RuleFocusNotVisible = "dummyrun-focus-not-visible"
	RuleFocusHidden     = "dummyrun-focus-hidden"
	RuleKeyboardTrap    = "dummyrun-keyboard-trap"
	RuleSkipLinkBroken  = "dummyrun-skip-link-broken"
	// RuleMouseOnly is a control that works when clicked and not from the keyboard.
	RuleMouseOnly = "dummyrun-mouse-only"
	// RuleSkipLinkNoFocus is a skip link whose target is there, but focus does not move to it.
	RuleSkipLinkNoFocus = "dummyrun-skip-link-no-focus"
	// RuleFocusLowContrast is a focus indicator none of whose pixels changes by 3:1 when focus arrives.
	RuleFocusLowContrast = "dummyrun-focus-low-contrast"
	// RuleFocusAppearance is a focus indicator smaller than WCAG 2.4.13 asks for. It is a AAA criterion, so advice.
	RuleFocusAppearance = "dummyrun-focus-appearance"
)

// KeyboardRules are the keyboard checks in the order the report lists them.
var KeyboardRules = []string{RuleKeyboardTrap, RuleMouseOnly, RuleFocusNotVisible, RuleFocusLowContrast, RuleFocusHidden, RuleSkipLinkBroken, RuleSkipLinkNoFocus}

var keyboardRules = map[string]layoutRule{
	RuleKeyboardTrap: {"critical", "Keyboard focus gets stuck and Tab and Escape do not get out",
		"https://www.w3.org/WAI/WCAG22/Understanding/no-keyboard-trap.html", []string{"2.1.2"}},
	RuleFocusNotVisible: {"serious", "Nothing on screen changes when the element has keyboard focus",
		"https://www.w3.org/WAI/WCAG22/Understanding/focus-visible.html", []string{"2.4.7"}},
	RuleFocusLowContrast: {"serious", "The focus indicator changes the colours around the element by less than 3:1",
		"https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html", []string{"1.4.11"}},
	RuleFocusAppearance: {"", "The focus indicator is smaller than a 2 pixel line round the element",
		"https://www.w3.org/WAI/WCAG22/Understanding/focus-appearance.html", []string{"2.4.13"}},
	RuleFocusHidden: {"serious", "Keyboard focus lands on an element you cannot see",
		"https://www.w3.org/WAI/WCAG22/Understanding/focus-visible.html", []string{"2.4.7"}},
	RuleSkipLinkBroken: {"serious", "The skip link points to a place that does not exist or is not shown",
		"https://www.w3.org/WAI/WCAG22/Understanding/bypass-blocks.html", []string{"2.4.1"}},
	RuleMouseOnly: {"serious", "It works when clicked, but not from the keyboard",
		"https://www.w3.org/WAI/WCAG22/Understanding/keyboard.html", []string{"2.1.1"}},
	RuleSkipLinkNoFocus: {"serious", "The skip link doesn't move keyboard focus to the content it names",
		"https://www.w3.org/WAI/WCAG22/Understanding/bypass-blocks.html", []string{"2.4.1"}},
}

// IsKeyboardRule reports whether the rule ID is one of the keyboard checks.
func IsKeyboardRule(id string) bool { _, ok := keyboardRules[id]; return ok }

// taskTests says in words how each kind of task's success is tested.
var taskTests = map[string]string{
	"expanded": "its aria-expanded became true",
	"controls": "the element its aria-controls names was shown",
	"details":  "its details opened",
	"selected": "the tab became selected",
	"checked":  "it became checked",
	"change":   "the page changed",
}

// taskFindings turns the keyboard and mouse comparison into findings. A task counts only when its success test was
// false on a fresh load, a click made it true, and on another fresh load the keyboard did not.
func taskFindings(t *page.Tasks, path string, allow *AllowList) []Finding {
	byRule := map[string][]Node{}
	for _, task := range t.Tasks {
		if !task.MouseOnly() {
			continue
		}
		how := "Tab never reached it"
		if task.Reached {
			how = "Tab reached it, but Enter and Space did nothing"
		}
		byRule[RuleMouseOnly] = append(byRule[RuleMouseOnly], Node{Target: task.Selector,
			Detail: "a click worked (" + taskTests[task.Kind] + "); " + how})
	}
	if sk := t.Skip; sk != nil && !sk.Moved {
		byRule[RuleSkipLinkNoFocus] = append(byRule[RuleSkipLinkNoFocus], Node{Target: sk.Link,
			Detail: fmt.Sprintf("after Enter on %q, the next Tab went to an element before %s", sk.Label, sk.Target)})
	}
	return ruleFindings(byRule, path, allow)
}

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
		case lowContrast(s.Appearance):
			byRule[RuleFocusLowContrast] = append(byRule[RuleFocusLowContrast], Node{Target: s.Selector,
				Detail: fmt.Sprintf("%.0f pixels change when it gets focus, and the biggest change in contrast is %.2f:1", s.Appearance.Changed, s.Appearance.Contrast)})
		}
	}
	if sk := w.Skip; sk != nil && (!sk.Found || !sk.Rendered) {
		d := fmt.Sprintf("%q points to %s, which is not on the page", sk.Text, sk.Href)
		if sk.Found {
			d = fmt.Sprintf("%q points to %s, which is not shown", sk.Text, sk.Href)
		}
		byRule[RuleSkipLinkBroken] = append(byRule[RuleSkipLinkBroken], Node{Target: sk.Selector, Detail: d})
	}
	return ruleFindings(byRule, path, allow)
}

// ruleFindings makes one finding for each keyboard rule that has nodes, in report order.
func ruleFindings(byRule map[string][]Node, path string, allow *AllowList) []Finding {
	var list []Finding
	for _, id := range KeyboardRules {
		nodes := byRule[id]
		if len(nodes) == 0 {
			continue
		}
		r := keyboardRules[id]
		// A mouse-only control is a task the sweep tried by keyboard and could not complete.
		ev := record.EvidenceMeasurement
		if id == RuleMouseOnly {
			ev = record.EvidenceTaskVerdict
		}
		f := Finding{Rule: id, Impact: r.impact, WCAG: r.wcag, Help: r.help, HelpURL: r.helpURL, Evidence: ev, Nodes: []Node{}}
		for _, n := range nodes {
			if e := allow.Match(id, path, n.Target); e != nil {
				n.Allowed, n.Reason = true, e.Reason
			}
			n.Confidence = rate(ev, id, nil)
			f.Nodes = append(f.Nodes, n)
		}
		list = append(list, f)
	}
	return list
}

// minIndicatorArea is how many CSS pixels must change before a focus indicator is measured. Fewer can be a blinking
// caret or a pixel of anti-aliasing.
const minIndicatorArea = 6

// lowContrast says a focus indicator was seen, but none of its pixels changes by 3:1.
func lowContrast(a *page.FocusAppearance) bool {
	return a != nil && a.Changed >= minIndicatorArea && a.Contrast < 3
}

// focusAppearanceAdvice reports focus indicators with enough contrast but less area than WCAG 2.4.13 asks for: a
// 2 CSS pixel line round the element. WCAG 2.4.13 is level AAA, so this is advice.
func focusAppearanceAdvice(w *page.TabWalk, path string, allow *AllowList) []Finding {
	r := keyboardRules[RuleFocusAppearance]
	f := Finding{Rule: RuleFocusAppearance, WCAG: r.wcag, Help: r.help, HelpURL: r.helpURL, Evidence: record.EvidenceMeasurement, Nodes: []Node{}}
	for _, s := range w.Stops {
		a := s.Appearance
		if s.NoIndicator || s.Hidden || s.Offscreen || a == nil || a.Changed < minIndicatorArea || lowContrast(a) || a.Area >= a.Need {
			continue
		}
		n := Node{Target: s.Selector, Detail: fmt.Sprintf("%.0f square pixels change by 3:1 or more when it gets focus, and a 2 pixel line round it is %.0f", a.Area, a.Need)}
		if e := allow.Match(RuleFocusAppearance, path, n.Target); e != nil {
			n.Allowed, n.Reason = true, e.Reason
		}
		f.Nodes = append(f.Nodes, n)
	}
	if len(f.Nodes) == 0 {
		return nil
	}
	return []Finding{f}
}
