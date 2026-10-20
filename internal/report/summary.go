package report

import (
	"fmt"
	"strings"
)

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func verb(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// summaryText writes the executive summary in plain language, one paragraph per entry.
func summaryText(rep *Report) []string {
	var out []string
	sc, s := rep.Scope, rep.Summary

	if sc.Runs.Total > 0 {
		target := "the app"
		if len(sc.Targets) == 1 && sc.Targets[0].BaseURL != "" {
			target = sc.Targets[0].BaseURL
		}
		p := fmt.Sprintf("Dummy Run played %s in %s against %s between %s and %s.",
			plural(len(sc.Personas), "fictional user", "fictional users"), plural(sc.Runs.Total, "run", "runs"),
			target, sc.StartedAt[:10], sc.EndedAt[:10])
		if len(sc.Targets) > 1 {
			p = fmt.Sprintf("Dummy Run played %s in %s against %s between %s and %s.",
				plural(len(sc.Personas), "fictional user", "fictional users"), plural(sc.Runs.Total, "run", "runs"),
				plural(len(sc.Targets), "build", "builds"), sc.StartedAt[:10], sc.EndedAt[:10])
		}
		out = append(out, p)
	}

	switch {
	case s.FindingsTotal == 0 && sc.Runs.Total > 0:
		p := "The runs found no problems that this report includes. That says little on its own. The runs are a sample and the personas did not visit every page."
		if s.PendingOmitted > 0 {
			p += fmt.Sprintf(" %s still waiting for review and %s left out.", plural(s.PendingOmitted, "finding is", "findings are"), verb(s.PendingOmitted, "is", "are"))
		}
		out = append(out, p)
	case s.FindingsTotal > 0:
		var parts []string
		for _, sev := range Severities {
			if n := s.BySeverity[sev]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, sev))
			}
		}
		p := fmt.Sprintf("The runs found %s: %s.", plural(s.FindingsTotal, "problem", "distinct problems"), strings.Join(parts, ", "))
		if s.Confirmed > 0 {
			p += fmt.Sprintf(" A person has confirmed %d of them.", s.Confirmed)
		}
		if s.NotReviewed > 0 {
			p += fmt.Sprintf(" %d %s marked not reviewed. No person has checked %s yet.", s.NotReviewed, verb(s.NotReviewed, "is", "are"), verb(s.NotReviewed, "it", "them"))
		}
		out = append(out, p)
		if s.PendingOmitted > 0 {
			out = append(out, fmt.Sprintf("%s waiting for review and %s left out of this report.", plural(s.PendingOmitted, "more finding is", "more findings are"), verb(s.PendingOmitted, "is", "are")))
		}
	case s.PendingOmitted > 0:
		out = append(out, fmt.Sprintf("%s waiting for review and %s left out of this report.", plural(s.PendingOmitted, "finding is", "findings are"), verb(s.PendingOmitted, "is", "are")))
	}

	if a := rep.Axe; a != nil {
		if a.FailingNodes == 0 && a.PageErrors == 0 {
			out = append(out, fmt.Sprintf("The axe sweep tested %s in %s and found no failing elements. Automated checks find only some accessibility problems.", plural(a.Pages, "page", "pages"), plural(a.Runs, "setting", "settings")))
		} else {
			p := fmt.Sprintf("The axe sweep tested %s and found %s on %s.", plural(a.Pages, "page", "pages"),
				plural(a.FailingNodes, "failing element", "failing elements"), plural(len(a.Rules), "rule", "rules"))
			if a.PageErrors > 0 {
				p += fmt.Sprintf(" %s could not be tested.", plural(a.PageErrors, "setting", "settings"))
			}
			out = append(out, p)
		}
	}

	if c := rep.Coverage; c.Available && c.UnfoundAtLeast != nil && c.CompletedRuns > 0 {
		p := fmt.Sprintf("From the pattern of repeats, there are at least %.1f more problems that these runs have not found. That is a floor and not a count.", *c.UnfoundAtLeast)
		if c.Basis != "confirmed findings" {
			p += " This counts findings that no person has confirmed, so it demonstrates the method and is not an estimate."
		}
		out = append(out, p)
	} else if c.Available {
		out = append(out, "There are too few repeated findings to estimate how many problems are still unfound.")
	}

	if rep.Baseline != nil {
		out = append(out, fmt.Sprintf("Against the baseline report of %s, %s new, %d still present and %d not seen again.",
			rep.Baseline.Date, plural(s.New, "finding is", "findings are"), s.StillPresent, s.Fixed))
	}
	return out
}
