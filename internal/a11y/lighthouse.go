package a11y

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/lighthouse"
	"github.com/dummy-run/dummy-run/internal/record"
)

// RuleLighthousePrefix starts the rule ID of a Lighthouse audit that failed where axe found nothing, as in
// dummyrun-lighthouse-paste-preventing-inputs.
const RuleLighthousePrefix = "dummyrun-lighthouse-"

// lighthouseTimeout bounds one Lighthouse run. A run usually takes about 20 seconds.
const lighthouseTimeout = 2 * time.Minute

// lighthouseMatchJS finds, for each of Lighthouse's selectors, every axe target that names the same element in the
// sweep's tab. An element that fails two axe rules has two targets. Lighthouse and axe write selectors differently,
// so they are compared by the element they find. Called with (lighthouseJSON, axeJSON), two JSON lists of selectors.
const lighthouseMatchJS = `(lhJSON, axeJSON) => {
  const find = (s) => {
    if (!s || s.includes(" >> ")) return null;
    try {
      return document.querySelector(s);
    } catch (e) {
      return null;
    }
  };
  const axe = JSON.parse(axeJSON).map(find);
  return JSON.parse(lhJSON).map((s) => {
    const el = find(s);
    const out = [];
    if (el) axe.forEach((a, i) => { if (a === el) out.push(i); });
    return out;
  });
}`

// lighthouseRef is one axe node on the page, by its list, finding and node.
type lighthouseRef struct {
	list *[]Finding
	f, n int
}

// lighthouseCheck runs Lighthouse on the page in the same Chrome and folds its failures into the result. It reports
// whether Lighthouse ran.
func lighthouseCheck(ctx context.Context, conn *bidi.Conn, cx, debugger, u string, res *PageResult, path string, allow *AllowList) bool {
	if debugger == "" {
		res.LighthouseError = "lighthouse: it runs only in Chrome"
		return false
	}
	bin, err := lighthouse.Find()
	if err != nil {
		res.LighthouseError = err.Error()
		return false
	}
	lctx, cancel := context.WithTimeout(ctx, lighthouseTimeout)
	defer cancel()
	r, err := lighthouse.Run(lctx, bin, u, debugger)
	if err != nil {
		res.LighthouseError = firstLine(err.Error())
		return false
	}
	res.LighthouseChecked, res.LighthouseScores = true, r.Scores

	refs, targets := lighthouseRefs(res)
	var sels []string
	for _, f := range r.Failures {
		if f.Category == "accessibility" {
			for _, it := range f.Items {
				sels = append(sels, it.Selector)
			}
		}
	}
	match := make([][]int, len(sels))
	if len(sels) > 0 && len(targets) > 0 {
		lhArg, _ := json.Marshal(sels)
		axeArg, _ := json.Marshal(targets)
		var got [][]int
		err := conn.CallFunctionJSON(lctx, cx, lighthouseMatchJS, &got, string(lhArg), string(axeArg))
		switch {
		case err != nil:
			res.LighthouseMatchError = firstLine(err.Error())
		case len(got) != len(sels):
			res.LighthouseMatchError = "lighthouse: matching its elements to axe's gave the wrong count"
		default:
			match = got
		}
	}
	foldLighthouse(res, r, refs, match, path, allow)
	return true
}

// lighthouseRefs lists every axe node the page has, with its target, so Lighthouse's elements can be matched to
// them.
func lighthouseRefs(res *PageResult) (refs []lighthouseRef, targets []string) {
	for _, list := range []*[]Finding{&res.Violations, &res.Advice} {
		for fi, f := range *list {
			if IsEnginesRule(f.Rule) || strings.HasPrefix(f.Rule, "dummyrun-") {
				continue
			}
			for ni, n := range f.Nodes {
				refs = append(refs, lighthouseRef{list, fi, ni})
				targets = append(targets, n.Target)
			}
		}
	}
	return refs, targets
}

// foldLighthouse folds Lighthouse's failures into the result. match holds, for each item of each accessibility
// audit in order, the indexes in refs of the axe nodes on the same element.
//   - an accessibility audit is an axe rule run by Lighthouse's own copy of axe-core. Where it fails an element our
//     axe failed for the same rule, the node gets lighthouse true. It confirms the result but is not a second
//     engine.
//   - an accessibility audit that fails where our axe found nothing goes to candidates, which never fail the
//     sweep. When matching failed, so it can't be told which these are, they are left out and
//     LighthouseMatchError says why.
//   - a best-practice audit that fails goes to advice.
func foldLighthouse(res *PageResult, r *lighthouse.Result, refs []lighthouseRef, match [][]int, path string, allow *AllowList) {
	k := 0
	for _, f := range r.Failures {
		access := f.Category == "accessibility"
		finding := Finding{Rule: RuleLighthousePrefix + f.ID, Help: "Lighthouse: " + f.Title,
			HelpURL: "https://developer.chrome.com/docs/lighthouse/", Evidence: record.EvidenceEngineRule, Nodes: []Node{}}
		if access {
			finding.HelpURL = "https://dequeuniversity.com/rules/axe/4.14/" + f.ID
		}
		items := f.Items
		if len(items) == 0 {
			items = []lighthouse.Item{{Selector: "html"}}
		}
		for _, it := range items {
			if access && len(f.Items) > 0 {
				var same []int
				if k < len(match) {
					same = match[k]
				}
				k++
				if res.LighthouseMatchError != "" {
					continue
				}
				agreed := false
				for _, i := range same {
					if i < 0 || i >= len(refs) {
						continue
					}
					rf := refs[i]
					if (*rf.list)[rf.f].Rule == f.ID {
						(*rf.list)[rf.f].Nodes[rf.n].Lighthouse = true
						res.LighthouseAgreed++
						agreed = true
						break
					}
				}
				if agreed {
					continue
				}
			}
			// The node names Lighthouse as the engine, so the report doesn't take it for one of axe's.
			n := Node{Target: it.Selector, Detail: strings.TrimSpace(strings.Trim(it.Label+": "+it.Detail, ": ")), Engines: []string{"lighthouse"}}
			if e := allow.Match(finding.Rule, path, n.Target); e != nil {
				n.Allowed, n.Reason = true, e.Reason
			}
			finding.Nodes = append(finding.Nodes, n)
		}
		if len(finding.Nodes) == 0 {
			continue
		}
		if access {
			res.Candidates = append(res.Candidates, finding)
		} else {
			res.Advice = append(res.Advice, finding)
		}
	}
}
