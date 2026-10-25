package runner

import (
	"regexp"
	"sort"
	"strings"

	"github.com/dummy-run/dummy-run/internal/identity"
	"github.com/dummy-run/dummy-run/internal/page"
)

// Locator works out where a finding is from the steps it cites: the route, and the locus (the control
// the persona named, or the nearest heading). It reads only what the engine saw, never what the model wrote.
type Locator struct {
	steps []Step
}

// NewLocator builds a locator over the steps of a run.
func NewLocator(steps []Step) *Locator { return &Locator{steps: steps} }

// StepRoute is the normalised route of step n, or "" when the step has no URL.
func (l *Locator) StepRoute(n int) string {
	if n < 1 || n > len(l.steps) || l.steps[n-1].Obs.URL == "" {
		return ""
	}
	return identity.NormaliseRoute(l.steps[n-1].Obs.URL)
}

// Route is the route of the first cited step that has one, else of the nearest earlier step, else "/".
func (l *Locator) Route(cited []int) string {
	cited = sortedCopy(cited)
	for _, n := range cited {
		if r := l.StepRoute(n); r != "" {
			return r
		}
	}
	if len(cited) > 0 {
		for n := cited[0] - 1; n >= 1; n-- {
			if r := l.StepRoute(n); r != "" {
				return r
			}
		}
	}
	return "/"
}

func sortedCopy(in []int) []int {
	out := append([]int(nil), in...)
	sort.Ints(out)
	return out
}

// A11yRule reports whether a cited accessibility check reported the axe rule, and that rule's impact.
func (l *Locator) A11yRule(cited []int, rule string) (impact string, ok bool) {
	for _, n := range cited {
		if n < 1 || n > len(l.steps) {
			continue
		}
		s := l.steps[n-1]
		if s.Obs.Action != "a11y" || s.Obs.Error != "" {
			continue
		}
		for _, v := range s.Obs.A11y {
			if v.ID == rule {
				return v.Impact, true
			}
		}
	}
	return "", false
}

// Locus is "role:accessible name" of the control the persona named by @ref, found in the outline of the
// first cited step that lists it. Without a control, or when the control has no name, it is
// "region:<nearest heading or landmark>", and "page" when the page has neither.
func (l *Locator) Locus(cited []int, control string) string {
	ref := strings.TrimPrefix(strings.TrimSpace(control), "@")
	cited = sortedCopy(cited)
	if ref != "" {
		for _, n := range cited {
			if n < 1 || n > len(l.steps) || l.steps[n-1].Obs.Snapshot == nil {
				continue
			}
			snap := l.steps[n-1].Obs.Snapshot
			if role, name, region, ok := fromTree(snap.Tree, ref); ok {
				if name != "" {
					return identity.NormaliseLocus(role, name, "")
				}
				if region != "" {
					return identity.NormaliseLocus("", "", region)
				}
				continue
			}
			for _, c := range snap.Controls {
				if c.Ref == ref {
					if name := cleanLabel(c.Label); name != "" {
						return identity.NormaliseLocus(roleOf(c), name, "")
					}
				}
			}
		}
	}
	for _, n := range cited {
		if n >= 1 && n <= len(l.steps) && l.steps[n-1].Obs.Snapshot != nil {
			if r := firstHeading(l.steps[n-1].Obs.Snapshot); r != "" {
				return identity.NormaliseLocus("", "", r)
			}
		}
	}
	return "page"
}

var (
	refRE     = regexp.MustCompile(` @([0-9]+)(?: |$|\[)`)
	headingRE = regexp.MustCompile(`^(?:heading) [1-6]? ?"(.*)"`)
)

// fromTree reads one outline line: `  role "name" @12 [states]`. It also returns the nearest
// heading or landmark above it.
func fromTree(tree, ref string) (role, name, region string, ok bool) {
	var near string
	for _, line := range strings.Split(tree, "\n") {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		m := refRE.FindStringSubmatch(line)
		if m != nil && m[1] == ref {
			left, _, _ := strings.Cut(line, " @"+ref)
			role = fields[0]
			if i, j := strings.Index(left, `"`), strings.LastIndex(left, `"`); i >= 0 && j > i {
				name = left[i+1 : j]
			}
			return role, name, near, true
		}
		switch fields[0] {
		case "heading":
			if mm := headingRE.FindStringSubmatch(trimmed); mm != nil {
				near = mm[1]
			}
		case "banner", "navigation", "main", "complementary", "contentinfo", "search", "form", "region":
			if i, j := strings.Index(trimmed, `"`), strings.LastIndex(trimmed, `"`); i >= 0 && j > i {
				near = trimmed[i+1 : j]
			} else {
				near = fields[0]
			}
		}
	}
	return "", "", "", false
}

func firstHeading(s *page.Snapshot) string {
	if len(s.Headings) > 0 {
		return s.Headings[0]
	}
	return ""
}

// cleanLabel is the control's accessible name from its label in the control list: the "(hidden)" marker is
// removed, and a control shown as "(no name)", with or without a hint such as its link path, has no name.
func cleanLabel(label string) string {
	label = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(label), "(hidden)"))
	if strings.HasPrefix(label, "(no name)") || strings.HasPrefix(label, "(no label)") {
		return ""
	}
	return label
}

// roleOf gives the implicit ARIA role of a control from its tag and type, for pages whose outline was cut.
func roleOf(c page.Control) string {
	switch c.Tag {
	case "a":
		return "link"
	case "button", "summary":
		return "button"
	case "select":
		return "combobox"
	case "textarea":
		return "textbox"
	case "input":
		switch c.Type {
		case "checkbox", "radio":
			return c.Type
		case "button", "submit", "reset", "image":
			return "button"
		case "range":
			return "slider"
		}
		return "textbox"
	}
	if c.Type != "" {
		return c.Type
	}
	return c.Tag
}
