package a11y

import (
	"fmt"

	"github.com/dummy-run/dummy-run/internal/page"
)

// RuleImageContrast is the rule ID of text whose background, measured pixel by pixel, has too little contrast.
const RuleImageContrast = "dummyrun-contrast-over-image"

// imageContrastTargets are the elements axe could not judge for colour contrast, usually because the text sits on an
// image or a gradient.
func imageContrastTargets(needsReview []Finding) []string {
	var out []string
	for _, f := range needsReview {
		if f.Rule != "color-contrast" {
			continue
		}
		for _, n := range f.Nodes {
			out = append(out, n.Target)
		}
	}
	return out
}

// imageContrastFindings reports each element whose text sits, for most of its box, on pixels with too little
// contrast. An element that passes at the median pixel contrast and fails only in its worst tenth is not reported.
func imageContrastFindings(ms []page.ImageContrast, path string, allow *AllowList) []Finding {
	f := Finding{Rule: RuleImageContrast, Impact: "serious", WCAG: []string{"1.4.3"},
		Help:    "Text on an image or gradient has too little contrast with what is behind it",
		HelpURL: understandingURL("1.4.3"), Nodes: []Node{}}
	for _, m := range ms {
		if !m.Fails() {
			continue
		}
		n := Node{Target: m.Selector, Detail: fmt.Sprintf("text %s measures %.2f:1 at the median of its background pixels and %.2f:1 at the worst tenth, and needs %.1f:1",
			m.Text, m.Median, m.Worst, m.Need)}
		if e := allow.Match(RuleImageContrast, path, n.Target); e != nil {
			n.Allowed, n.Reason = true, e.Reason
		}
		f.Nodes = append(f.Nodes, n)
	}
	if len(f.Nodes) == 0 {
		return nil
	}
	return []Finding{f}
}
