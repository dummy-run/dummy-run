package report

import (
	"embed"
	"encoding/base64"
	"fmt"
	htmltemplate "html/template"
	"strings"
	"time"
)

// assetFS holds the brand files the HTML report inlines. The SVG files are copies of the ones in
// brand/, and a test fails when they drift. The fonts are the site's, cut down to the Latin letters,
// digits and punctuation a report uses and to the weights it sets.
//
//go:embed assets/*.woff2 assets/*.svg
var assetFS embed.FS

var assetTypes = map[string]string{".woff2": "font/woff2", ".svg": "image/svg+xml"}

// asset returns a data URI for one of the embedded brand files, so the report loads nothing from the network.
func asset(name string) (htmltemplate.URL, error) {
	data, err := assetFS.ReadFile("assets/" + name)
	if err != nil {
		return "", err
	}
	typ := assetTypes[name[strings.LastIndex(name, "."):]]
	// The type comes from a fixed list and the rest is base64, so nothing in the URL is markup.
	// nosemgrep: go.lang.security.audit.xss.template-html-does-not-escape.unsafe-template-type, go.lang.security.audit.net.unescaped-data-in-url.unescaped-data-in-url
	return htmltemplate.URL("data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(data)), nil //nolint:gosec // see above
}

// SevItem is a finding with its position in the report, which gives it a stable anchor.
type SevItem struct {
	Index   int
	Finding *Finding
}

// SevGroup is the findings of one severity, in report order.
type SevGroup struct {
	Severity string
	Items    []SevItem
}

// sevGroups splits the findings, which are already sorted worst first, into one group per severity
// that has any.
func sevGroups(findings []Finding) []SevGroup {
	var out []SevGroup
	for i := range findings {
		f := &findings[i]
		if n := len(out); n == 0 || out[n-1].Severity != f.Severity {
			out = append(out, SevGroup{Severity: f.Severity})
		}
		g := &out[len(out)-1]
		g.Items = append(g.Items, SevItem{Index: i, Finding: f})
	}
	return out
}

// Segment is one severity's share of the overview bar, in percent of its width.
type Segment struct {
	Severity string
	Count    int
	X, W     string
}

// segments lays the severity counts out as consecutive bars, worst first. Severities with no
// findings have no segment.
func segments(s Summary) []Segment {
	total := 0
	for _, sev := range Severities {
		total += s.BySeverity[sev]
	}
	var out []Segment
	x := 0.0
	for _, sev := range Severities {
		n := s.BySeverity[sev]
		if n == 0 {
			continue
		}
		w := 100 * float64(n) / float64(total)
		out = append(out, Segment{Severity: sev, Count: n, X: fmt.Sprintf("%.2f", x), W: fmt.Sprintf("%.2f", w)})
		x += w
	}
	return out
}

// longDate turns 2026-10-19, or an RFC 3339 time, into 19 October 2026. Anything else comes back unchanged.
func longDate(s string) string {
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return fmt.Sprintf("%d %s %d", t.Day(), t.Month(), t.Year())
		}
	}
	return s
}

// period says when the runs happened: one date, or two.
func period(sc Scope) string {
	if sc.StartedAt == "" {
		return ""
	}
	a, b := longDate(sc.StartedAt), longDate(sc.EndedAt)
	if a == b || sc.EndedAt == "" {
		return a
	}
	return a + " to " + b
}

// howSure says in plain words what the coverage numbers allow you to conclude.
func howSure(c CoverageSection) string {
	switch {
	case !c.Available:
		if c.Unavailable != "" {
			return "Coverage was not computed. " + c.Unavailable
		}
		return "Coverage was not computed, so there is no estimate of how much is left to find."
	case c.UnfoundAtLeast != nil && c.CompletedRuns > 0:
		s := fmt.Sprintf("From the pattern of repeats, at least %.1f more problems are still unfound. That is a floor, not a count, so there may be more.", *c.UnfoundAtLeast)
		if c.Basis != BasisConfirmed {
			s += " It counts findings that no person has confirmed, so it shows the method and is not an estimate."
		}
		return s
	default:
		return "Too few findings repeated to estimate how many problems are still unfound."
	}
}

// impactSeverity maps an axe impact to the colour of the persona severity of the same weight.
func impactSeverity(impact string) string {
	switch impact {
	case "critical":
		return "blocker"
	case "serious":
		return "major"
	case "moderate":
		return "minor"
	}
	return "info"
}

// pagesTested is the number of pages the axe sweep tested, or the number of routes the personas visited when there was no sweep.
func pagesTested(rep *Report) int {
	if rep.Axe != nil {
		return rep.Axe.Pages
	}
	return len(rep.Scope.Routes)
}

// lead is the cover paragraph: the opening sentences of the summary, which say what was run and what it found.
func lead(text []string) string {
	if len(text) > 2 {
		text = text[:2]
	}
	return strings.Join(text, " ")
}
