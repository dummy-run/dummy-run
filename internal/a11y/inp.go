package a11y

import (
	"fmt"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/record"
)

// RuleSlowResponse is a control the page took more than half a second to respond to, by web-vitals' INP measure.
// It is advice: no WCAG criterion sets a time, but a slow response leaves people unsure whether anything happened.
const RuleSlowResponse = "dummyrun-slow-response"

// slowResponse is the INP above which web-vitals rates a response poor, in milliseconds.
const slowResponse = 500

// slowResponseAdvice reports each control whose slowest measured response was poor, once, with where the time went.
// A response that describes an earlier interaction, such as a Tab press on the way, isn't the control's.
func slowResponseAdvice(t *page.Tasks, path string, allow *AllowList) []Finding {
	if t == nil {
		return nil
	}
	worst := map[string]page.Response{}
	var order []string
	for _, r := range t.Responses {
		if r.Duration <= slowResponse || r.Earlier {
			continue
		}
		w, seen := worst[r.Selector]
		if !seen {
			order = append(order, r.Selector)
		}
		if !seen || r.Duration > w.Duration {
			worst[r.Selector] = r
		}
	}
	if len(order) == 0 {
		return nil
	}
	f := Finding{Rule: RuleSlowResponse, Help: "The page takes more than half a second to respond to a control",
		HelpURL: "https://web.dev/articles/inp", Evidence: record.EvidenceMeasurement, Nodes: []Node{}}
	for _, sel := range order {
		r := worst[sel]
		n := Node{Target: sel, Detail: fmt.Sprintf("after a %s, the page took %.0f ms to show a response: %.0f ms waiting, %.0f ms in its handlers and %.0f ms drawing",
			actionName(r.Action), r.Duration, r.InputDelay, r.Processing, r.Presentation)}
		if e := allow.Match(RuleSlowResponse, path, n.Target); e != nil {
			n.Allowed, n.Reason = true, e.Reason
		}
		f.Nodes = append(f.Nodes, n)
	}
	return []Finding{f}
}

func actionName(a string) string {
	switch a {
	case "enter":
		return "press of Enter"
	case "space":
		return "press of Space"
	}
	return a
}
