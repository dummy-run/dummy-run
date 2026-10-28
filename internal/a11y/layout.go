package a11y

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dummy-run/dummy-run/internal/record"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/page"
)

// Rule IDs of the layout checks. They sit beside axe's rule IDs in the report and the allow-list.
const (
	RuleReflow      = "dummyrun-reflow"
	RuleClippedText = "dummyrun-clipped-text"
	RuleOverlap     = "dummyrun-overlap"
	RuleTargetSize  = "dummyrun-target-size"
	// RuleFocusObscured is a control that, once it has focus, is entirely hidden by something opaque.
	RuleFocusObscured = "dummyrun-focus-obscured"
)

// LayoutRules are the layout checks in the order the report lists them.
var LayoutRules = []string{RuleReflow, RuleClippedText, RuleOverlap, RuleTargetSize, RuleFocusObscured}

type layoutRule struct {
	impact, help, helpURL string
	wcag                  []string
}

var layoutRules = map[string]layoutRule{
	RuleReflow: {"serious", "The page scrolls sideways at this window width",
		"https://www.w3.org/WAI/WCAG22/Understanding/reflow.html", []string{"1.4.10"}},
	RuleClippedText: {"serious", "Text is cut off by the box it sits in",
		"https://www.w3.org/WAI/WCAG22/Understanding/resize-text.html", []string{"1.4.4"}},
	RuleOverlap: {"serious", "A control is covered by another control",
		"https://www.w3.org/WAI/WCAG22/Understanding/reflow.html", []string{"1.4.10", "1.4.4"}},
	RuleTargetSize: {"moderate", "Interactive element is smaller than 24 by 24 CSS pixels",
		"https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html", []string{"2.5.8"}},
	RuleFocusObscured: {"serious", "A control with focus is entirely hidden by other content",
		"https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum.html", []string{"2.4.11"}},
}

// IsLayoutRule reports whether the rule ID is one of the layout checks.
func IsLayoutRule(id string) bool { _, ok := layoutRules[id]; return ok }

// layoutNode is a failing element with the box to crop for its screenshot.
type layoutNode struct {
	Node
	box *page.LayoutBox
}

// overlapDetail says what the browser reported on top of the covered control.
func overlapDetail(o page.LayoutOverlap) string {
	what := "its text"
	if !o.Text {
		what = "its box"
	}
	return fmt.Sprintf("%s is under %s at %d of %d points the browser checked", what, o.B.Selector, o.Covered, o.Sampled)
}

// layoutFindings turns what the geometry checks measured into findings, one per rule that has any.
func layoutFindings(l *page.Layout, path string, allow *AllowList) (list []Finding, boxes map[string][]*page.LayoutBox) {
	byRule := map[string][]layoutNode{}
	if o := l.Overflow; o != nil {
		if len(o.Culprits) == 0 {
			byRule[RuleReflow] = append(byRule[RuleReflow], layoutNode{Node: Node{Target: "html",
				Detail: fmt.Sprintf("the page is %.0f px wider than the %d px window", o.Excess, o.ClientWidth)}})
		}
		for _, c := range o.Culprits {
			byRule[RuleReflow] = append(byRule[RuleReflow], layoutNode{Node: Node{Target: c.Selector,
				Detail: fmt.Sprintf("the page is %.0f px wider than the %d px window; this element reaches %.0f px", o.Excess, o.ClientWidth, c.Right)}})
		}
	}
	for _, c := range l.Clipped {
		box := c.Box
		byRule[RuleClippedText] = append(byRule[RuleClippedText], layoutNode{Node: Node{Target: c.Selector, Detail: c.Detail}, box: &box})
	}
	for _, o := range l.Overlaps {
		box := o.Box
		byRule[RuleOverlap] = append(byRule[RuleOverlap], layoutNode{Node: Node{Target: o.A.Selector + " & " + o.B.Selector,
			Detail: overlapDetail(o)}, box: &box})
	}
	for _, o := range l.Obscured {
		box := o.Box
		by := o.By.Selector
		if o.ByPosition == "sticky" || o.ByPosition == "fixed" {
			by += " (" + o.ByPosition + ")"
		}
		byRule[RuleFocusObscured] = append(byRule[RuleFocusObscured], layoutNode{Node: Node{Target: o.Selector,
			Detail: "with focus, it is entirely hidden by " + by}, box: &box})
	}
	for _, s := range l.Small {
		box := s.Box
		byRule[RuleTargetSize] = append(byRule[RuleTargetSize], layoutNode{Node: Node{Target: s.Selector,
			Detail: fmt.Sprintf("%.0f x %.0f px", s.Width, s.Height)}, box: &box})
	}
	boxes = map[string][]*page.LayoutBox{}
	for _, id := range LayoutRules {
		nodes := byRule[id]
		if len(nodes) == 0 {
			continue
		}
		r := layoutRules[id]
		f := Finding{Rule: id, Impact: r.impact, WCAG: r.wcag, Help: r.help, HelpURL: r.helpURL, Evidence: record.EvidenceMeasurement, Nodes: []Node{}}
		for _, n := range nodes {
			if e := allow.Match(id, path, n.Target); e != nil {
				n.Allowed, n.Reason = true, e.Reason
			}
			n.Confidence = rate(record.EvidenceMeasurement, id, nil)
			f.Nodes = append(f.Nodes, n.Node)
			boxes[id] = append(boxes[id], n.box)
		}
		list = append(list, f)
	}
	return list, boxes
}

// maxShots is the most screenshot crops one page run writes.
const maxShots = 12

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9]+`)

// shoot writes a PNG crop for each failing element that has a box, up to maxShots for the page run,
// and sets the node's Screenshot to the file's path. A page that cannot be captured keeps its findings.
func shoot(ctx context.Context, conn *bidi.Conn, cx, dir string, res *PageResult, boxes map[string][]*page.LayoutBox) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return
	}
	taken := 0
	for fi := range res.Violations {
		f := &res.Violations[fi]
		bs := boxes[f.Rule]
		if !IsLayoutRule(f.Rule) {
			continue
		}
		for ni := range f.Nodes {
			if taken >= maxShots {
				return
			}
			var data string
			var err error
			if f.Rule == RuleReflow {
				if ni > 0 {
					continue // one view of the window shows the overflow for every culprit
				}
				data, err = conn.CaptureScreenshot(ctx, cx)
			} else {
				if ni >= len(bs) || bs[ni] == nil {
					continue
				}
				data, err = conn.CaptureScreenshotClip(ctx, cx, clipAround(*bs[ni]))
			}
			if err != nil {
				continue
			}
			png, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				continue
			}
			name := screenshotName(dir, res, f.Rule, ni)
			if err := os.WriteFile(name, png, 0o600); err != nil {
				continue
			}
			f.Nodes[ni].Screenshot = name
			if f.Rule == RuleReflow {
				for j := range f.Nodes {
					f.Nodes[j].Screenshot = name
				}
			}
			taken++
		}
	}
}

// screenshotName is the file for the ni-th node of a rule on one page and setting. It ends in a short hash
// of the address, because two addresses can share their first 60 characters or differ only in punctuation.
func screenshotName(dir string, res *PageResult, rule string, ni int) string {
	base := unsafeName.ReplaceAllString(strings.TrimPrefix(strings.TrimPrefix(res.URL, "https://"), "http://"), "-")
	if len(base) > 60 {
		base = base[:60]
	}
	sum := sha256.Sum256([]byte(res.URL))
	setting := strings.ReplaceAll(res.Setting(), "/", "-")
	return filepath.Join(dir, fmt.Sprintf("%s-%s-%s-%s-%d.png", base, hex.EncodeToString(sum[:4]), setting, strings.TrimPrefix(rule, "dummyrun-"), ni+1))
}

// clipAround pads a box so the crop shows what surrounds the element, and keeps it inside the page.
func clipAround(b page.LayoutBox) bidi.Clip {
	const pad = 12
	x, y := max(b.X-pad, 0), max(b.Y-pad, 0)
	return bidi.Clip{X: x, Y: y, Width: max(min(b.Width+2*pad, 2000), 1), Height: max(min(b.Height+2*pad, 2000), 1)}
}
