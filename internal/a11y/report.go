package a11y

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// WriteJSON writes the report as indented JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// maxSummaryLines bounds the failure lines in the human summary. The JSON has them all.
const maxSummaryLines = 200

// WriteSummary writes a short human summary. Each failure is listed once with the browser,
// viewport and colour scheme combinations it appeared in.
func (r *Report) WriteSummary(w io.Writer) {
	type key struct{ url, rule, target string }
	type row struct {
		f      Finding
		target string
		combos []string
	}
	rows := map[key]*row{}
	var order []key
	for _, p := range r.Results {
		combo := fmt.Sprintf("%s/%s/%s", p.Browser, p.Viewport, p.Scheme)
		for _, f := range p.Violations {
			for _, n := range f.Nodes {
				if n.Allowed {
					continue
				}
				k := key{p.URL, f.Rule, n.Target}
				if rows[k] == nil {
					rows[k] = &row{f: f, target: n.Target}
					order = append(order, k)
				}
				rows[k].combos = append(rows[k].combos, combo)
			}
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].url != order[j].url {
			return order[i].url < order[j].url
		}
		return order[i].rule < order[j].rule
	})
	for i, k := range order {
		if i == maxSummaryLines {
			_, _ = fmt.Fprintf(w, "... and %d more; the JSON report has them all\n", len(order)-i)
			break
		}
		rw := rows[k]
		wcag := ""
		if len(rw.f.WCAG) > 0 {
			wcag = ", WCAG " + strings.Join(rw.f.WCAG, " ")
		}
		_, _ = fmt.Fprintf(w, "FAIL %s\n     %s (%s%s) at %s\n     %s\n     in %s\n     %s\n", k.url, k.rule, orUnknown(rw.f.Impact), wcag, rw.target, rw.f.Help, strings.Join(rw.combos, ", "), rw.f.HelpURL)
	}
	for _, p := range r.Results {
		if p.Skipped != "" {
			_, _ = fmt.Fprintf(w, "SKIPPED %s: %s\n", p.URL, p.Skipped)
		}
		if p.Error != "" {
			_, _ = fmt.Fprintf(w, "ERROR %s (%s/%s/%s): %s\n", p.URL, p.Browser, p.Viewport, p.Scheme, p.Error)
		}
	}
	s := r.Summary
	_, _ = fmt.Fprintf(w, "\n%s %s, axe-core %s: %d pages, %d runs; %d failing, %d allowed, %d need review, %d page errors",
		r.Browser, r.BrowserVersion, r.AxeVersion, s.Pages, s.Runs, s.Failing, s.Allowed, s.NeedsReview, s.PageErrors)
	if s.Skipped > 0 {
		_, _ = fmt.Fprintf(w, ", %d pages skipped for robots.txt", s.Skipped)
	}
	if s.UnusedAllowed > 0 {
		_, _ = fmt.Fprintf(w, ", %d unused allow-list entries", s.UnusedAllowed)
	}
	_, _ = fmt.Fprintln(w)
}

func orUnknown(s string) string {
	if s == "" {
		return "no impact"
	}
	return s
}
