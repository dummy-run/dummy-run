package a11y

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/dummy-run/dummy-run/internal/confidence"
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
		detail string
		shot   string
		combos []string
	}
	rows := map[key]*row{}
	var order []key
	for _, p := range r.Results {
		combo := p.Setting()
		for _, f := range p.Violations {
			for _, n := range f.Nodes {
				if n.Allowed {
					continue
				}
				k := key{p.URL, f.Rule, n.Target}
				if rows[k] == nil {
					rows[k] = &row{f: f, target: n.Target, detail: n.Detail, shot: n.Screenshot}
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
		_, _ = fmt.Fprintf(w, "FAIL %s\n     %s (%s%s) at %s\n     %s\n", k.url, k.rule, orUnknown(rw.f.Impact), wcag, rw.target, rw.f.Help)
		if rw.detail != "" {
			_, _ = fmt.Fprintf(w, "     %s\n", rw.detail)
		}
		if rw.shot != "" {
			_, _ = fmt.Fprintf(w, "     screenshot %s\n", rw.shot)
		}
		_, _ = fmt.Fprintf(w, "     in %s\n     %s\n", strings.Join(rw.combos, ", "), rw.f.HelpURL)
	}
	for _, p := range r.Results {
		if p.Skipped != "" {
			_, _ = fmt.Fprintf(w, "SKIPPED %s: %s\n", p.URL, p.Skipped)
		}
		if p.Error != "" {
			_, _ = fmt.Fprintf(w, "ERROR %s (%s): %s\n", p.URL, p.Setting(), p.Error)
		}
		for _, e := range sortedKeys(p.EngineErrors) {
			_, _ = fmt.Fprintf(w, "ENGINE %s %s (%s): %s\n", confidence.EngineName(e), p.URL, p.Setting(), printable(p.EngineErrors[e]))
		}
		if p.JourneysError != "" {
			_, _ = fmt.Fprintf(w, "JOURNEYS %s (%s): %s\n", p.URL, p.Setting(), printable(p.JourneysError))
		}
		if p.LinksError != "" {
			_, _ = fmt.Fprintf(w, "LINKS %s (%s): %s\n", p.URL, p.Setting(), printable(p.LinksError))
		}
		for _, c := range checkErrors(p) {
			_, _ = fmt.Fprintf(w, "%s %s (%s): %s\n", c.check, p.URL, p.Setting(), printable(c.err))
		}
	}
	s := r.Summary
	_, _ = fmt.Fprintf(w, "\n%s %s, axe-core %s: %d pages, %d runs; %d failing, %d allowed, %d need review, %d page errors",
		r.Browser, r.BrowserVersion, r.AxeVersion, s.Pages, s.Runs, s.Failing, s.Allowed, s.NeedsReview, s.PageErrors)
	if s.Advice > 0 {
		_, _ = fmt.Fprintf(w, ", %d elements with best-practice advice (not failures)", s.Advice)
	}
	if s.Candidates > 0 {
		_, _ = fmt.Fprintf(w, ", %d candidates axe did not report (not failures)", s.Candidates)
	}
	if n := engineErrorRuns(r); n > 0 {
		_, _ = fmt.Fprintf(w, ", rule engines failed on %d runs (listed above)", n)
	}
	if n := journeyErrorPages(r); n > 0 {
		_, _ = fmt.Fprintf(w, ", journeys not tried on %d pages (listed above)", n)
	}
	if n := linkErrorPages(r); n > 0 {
		_, _ = fmt.Fprintf(w, ", links not checked on %d pages (listed above)", n)
	}
	if n := checkErrorRuns(r); n > 0 {
		_, _ = fmt.Fprintf(w, ", other checks failed on %d runs (listed above)", n)
	}
	if s.Skipped > 0 {
		_, _ = fmt.Fprintf(w, ", %d pages skipped for robots.txt", s.Skipped)
	}
	if s.UnusedAllowed > 0 {
		_, _ = fmt.Fprintf(w, ", %d unused allow-list entries", s.UnusedAllowed)
	}
	_, _ = fmt.Fprintln(w)
}

type checkError struct{ check, err string }

// checkErrors lists the optional checks that couldn't run on a page, other than the rule engines and the journeys,
// which have lines of their own.
func checkErrors(p PageResult) []checkError {
	var out []checkError
	add := func(check, err string) {
		if err != "" {
			out = append(out, checkError{check, err})
		}
	}
	add("PLAIN", p.PlainLanguageError)
	add("HTML", p.HTMLError)
	add("LIGHTHOUSE", p.LighthouseError)
	add("LIGHTHOUSE MATCH", p.LighthouseMatchError)
	add("STATE", p.StateError)
	if p.Consent != nil {
		add("CONSENT", p.Consent.Error)
	}
	return out
}

// checkErrorRuns counts the runs where an optional check other than the rule engines and the journeys failed.
func checkErrorRuns(r *Report) int {
	n := 0
	for _, p := range r.Results {
		if len(checkErrors(p)) > 0 {
			n++
		}
	}
	return n
}

func orUnknown(s string) string {
	if s == "" {
		return "no impact"
	}
	return s
}

// journeyErrorPages counts the pages whose journeys couldn't be tried.
func journeyErrorPages(r *Report) int {
	n := 0
	for _, p := range r.Results {
		if p.JourneysError != "" {
			n++
		}
	}
	return n
}

// linkErrorPages counts the pages whose links couldn't be checked.
func linkErrorPages(r *Report) int {
	n := 0
	for _, p := range r.Results {
		if p.LinksError != "" {
			n++
		}
	}
	return n
}

// printable drops control characters, so a message with text from the page can't move the terminal's cursor.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// engineErrorRuns counts the runs on which at least one of the other rule engines failed.
func engineErrorRuns(r *Report) int {
	n := 0
	for _, p := range r.Results {
		if len(p.EngineErrors) > 0 {
			n++
		}
	}
	return n
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
