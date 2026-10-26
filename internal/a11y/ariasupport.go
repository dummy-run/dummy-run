package a11y

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/dummy-run/dummy-run/internal/page"
)

// RuleARIASupport is a role or aria-* attribute that screen readers support only in part, by the ARIA-AT project's
// tests. It is advice, since the page may be right and the screen readers wrong.
const RuleARIASupport = "dummyrun-aria-support"

// ARIASupportFetched is the date internal/a11y/ariaat/features.json was fetched.
const ARIASupportFetched = "2026-10-26"

// ariaSupportBelow is the share of ARIA-AT assertions, in percent, below which a screen reader and browser pair
// counts as supporting a feature only in part. ARIA-AT tests whole patterns, and most features have a pair below 90
// percent, so the line is drawn where support is clearly weak.
const ariaSupportBelow = 60

//go:embed ariaat/features.json
var ariaatJSON []byte

// ariaSupport is how one screen reader and browser pair did on one feature in the ARIA-AT tests.
type ariaSupport struct {
	Feature string `json:"refId"`
	AT      string `json:"atName"`
	Browser string `json:"browserName"`
	Total   int    `json:"total"`
	Passed  int    `json:"passed"`
	Percent int    `json:"passedPercentage"`
}

var (
	ariaSupportOnce sync.Once
	ariaSupportBy   map[string][]ariaSupport
)

// ariaSupportFor returns the ARIA-AT figures for a feature, in screen reader order, or nil when it wasn't tested.
func ariaSupportFor(feature string) []ariaSupport {
	ariaSupportOnce.Do(func() {
		var doc struct {
			Data struct {
				Metrics struct {
					ByAT []ariaSupport `json:"ariaFeaturesByAtBrowser"`
				} `json:"ariaHtmlFeaturesMetrics"`
			} `json:"data"`
		}
		ariaSupportBy = map[string][]ariaSupport{}
		if json.Unmarshal(ariaatJSON, &doc) != nil {
			return
		}
		for _, s := range doc.Data.Metrics.ByAT {
			if s.Total > 0 {
				ariaSupportBy[s.Feature] = append(ariaSupportBy[s.Feature], s)
			}
		}
		for _, list := range ariaSupportBy {
			slices.SortFunc(list, func(a, b ariaSupport) int { return strings.Compare(a.AT+a.Browser, b.AT+b.Browser) })
		}
	})
	return ariaSupportBy[feature]
}

// ariaSupportAdvice reports each role or aria-* attribute on the page that a screen reader and browser pair passed
// fewer than 60 percent of ARIA-AT's assertions for. Each node is the first element that uses it.
func ariaSupportAdvice(features []page.ARIAFeature, path string, allow *AllowList) []Finding {
	f := Finding{Rule: RuleARIASupport, Help: "A role or ARIA attribute that some screen readers support only in part, by the ARIA-AT tests",
		HelpURL: "https://aria-at.w3.org/reports", Nodes: []Node{}}
	for _, ft := range features {
		list := ariaSupportFor(ft.Name)
		if !slices.ContainsFunc(list, func(s ariaSupport) bool { return s.Percent < ariaSupportBelow }) {
			continue
		}
		var parts []string
		for _, s := range list {
			parts = append(parts, fmt.Sprintf("%s with %s %d%% (%d of %d)", s.AT, s.Browser, s.Percent, s.Passed, s.Total))
		}
		what := "role " + ft.Name
		if strings.HasPrefix(ft.Name, "aria-") {
			what = ft.Name
		}
		uses := "1 element uses it"
		if ft.Count != 1 {
			uses = fmt.Sprintf("%d elements use it", ft.Count)
		}
		n := Node{Target: ft.Selector, Detail: fmt.Sprintf("%s: %s. In ARIA-AT's tests of patterns that use it, the share of assertions passed is %s", what, uses, strings.Join(parts, ", "))}
		if e := allow.Match(RuleARIASupport, path, n.Target); e != nil {
			n.Allowed, n.Reason = true, e.Reason
		}
		f.Nodes = append(f.Nodes, n)
	}
	if len(f.Nodes) == 0 {
		return nil
	}
	return []Finding{f}
}
