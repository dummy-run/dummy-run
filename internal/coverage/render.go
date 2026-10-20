package coverage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// JSON renders the report as indented JSON.
func (r *Report) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func pct(p *float64) string {
	if p == nil {
		return "not estimable"
	}
	return fmt.Sprintf("%.2f", *p)
}

// Markdown renders the same blocks as the JSON for a person to read.
func (r *Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Coverage report\n\nBuild `%s`. Counted: %s. Strata: %s.\n\n", r.Build, r.Basis, r.Strata)

	b.WriteString("## Runs\n\n")
	fmt.Fprintf(&b, "%d runs: %d completed, %d blocked, %d errored, %d budget exhausted. Only completed runs are counted as samples. Harness failure rate %.1f%%.",
		r.Runs.Total, r.Runs.Completed, r.Runs.Blocked, r.Runs.Errored, r.Runs.BudgetExhausted, 100*r.Runs.HarnessFailureRate)
	if r.Runs.NotFingerprinted > 0 {
		fmt.Fprintf(&b, " %d version 1 record(s) have no fingerprint and are left out.", r.Runs.NotFingerprinted)
	}
	b.WriteString("\n\n## Review\n\n")
	fmt.Fprintf(&b, "Problems: %d confirmed, %d rejected, %d pending. Observations (never counted): %d.", r.Review.Confirmed, r.Review.Rejected, r.Review.Pending, r.Review.Observations)
	if k := r.Review.Kappa; k != nil {
		switch {
		case k.Value != nil:
			fmt.Fprintf(&b, " Kappa %.2f over %d pairs (%d reviewer file(s); %s).", *k.Value, k.Pairs, k.Reviewers, k.Status)
		default:
			fmt.Fprintf(&b, " Kappa: %s.", k.Status)
		}
	}

	b.WriteString("\n\n## Reach\n\n")
	if rc := r.Reach.RouteCoverage; rc != nil {
		fmt.Fprintf(&b, "Route coverage: %d of %d routes in the stated universe (%s) were visited; %d visited routes are outside it.", rc.Visited, rc.Universe, strings.Join(rc.Sources, ", "), rc.Outside)
	} else {
		b.WriteString("Route coverage: not reported, because the records state no route universe.")
	}
	b.WriteString("\n\n| Dimension | Used | Possible values | Values |\n|---|---|---|---|\n")
	dims := make([]string, 0, len(r.Reach.Dimensions))
	for d := range r.Reach.Dimensions {
		dims = append(dims, d)
	}
	sort.Strings(dims)
	for _, d := range dims {
		u := r.Reach.Dimensions[d]
		poss := "open"
		if u.Possible != nil {
			poss = fmt.Sprint(*u.Possible)
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s |\n", d, u.Used, poss, strings.Join(u.Values, ", "))
	}
	if p := r.Reach.DevicePairs; p != nil {
		fmt.Fprintf(&b, "\nDevice and browser pairs: %d of %d.\n", p.Used, p.Possible)
	}

	b.WriteString("\n## Completeness\n\n")
	fmt.Fprintf(&b, "%s\n\n%s\n\n", r.Completeness.Label, r.Completeness.Note)
	b.WriteString("| Identity operation | Observed | Chao2 | Coverage | New problems per extra run |\n|---|---|---|---|---|\n")
	for _, op := range r.Completeness.IdentityBand {
		c2, cov, slope := "withheld", "withheld", "withheld"
		if op.Chao2 != nil {
			c2 = op.Chao2.Label
			if op.Chao2.Form == FormNotEstimable {
				c2 = "not estimable"
			}
			cov = pct(op.Coverage)
			slope = fmt.Sprintf("%.2f", *op.NewPerExtraRun)
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n", op.Operation, op.SObs, c2, cov, slope)
	}
	if r.Completeness.Jackknife2 != nil {
		fmt.Fprintf(&b, "\nJackknife-2 (a sensitivity check only): %.1f.\n", *r.Completeness.Jackknife2)
	}

	b.WriteString("\n## Stopping rule\n\n")
	fmt.Fprintf(&b, "Stop: %v. %s.\n\n| Stratum | Completed | Coverage | Runs since new | Stopped | Reason |\n|---|---|---|---|---|---|\n", r.Stopping.Stop, r.Stopping.Summary)
	for _, s := range r.Stopping.Strata {
		fmt.Fprintf(&b, "| %s | %d | %s | %d | %v | %s |\n", s.Stratum, s.Completed, pct(s.Coverage), s.RunsSinceNew, s.Stopped, s.Reason)
	}

	b.WriteString("\n## Problems\n\n")
	if len(r.Findings) == 0 {
		b.WriteString("None counted.\n")
	}
	for _, f := range r.Findings {
		band := "unscored"
		if f.Band != nil {
			band = *f.Band
		}
		fmt.Fprintf(&b, "- **%s** `%s` %s on `%s` (%s): %s. %s. Wilson 95%% %.2f to %.2f.\n", band, f.Fingerprint, f.Symptom, f.Route, f.Locus, f.Title, f.Display, f.Wilson95[0], f.Wilson95[1])
	}
	if len(r.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}
	return b.String()
}
