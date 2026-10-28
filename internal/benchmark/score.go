package benchmark

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// Outcome is what the sweep did on one planted copy.
type Outcome struct {
	// Page is the fixture the defect was planted in, and Defect names the defect.
	Page, Defect string
	// Rules are the rules that count as finding the defect.
	Rules []string
	// Before and After are the rules the sweep reported on the clean page and on the copy.
	Before, After []string
	// Error says why the pair could not be run.
	Error string
}

// Found says whether the copy shows a counting rule that the clean page did not.
func (o Outcome) Found() bool {
	was := set(o.Before)
	for _, r := range o.Rules {
		if contains(o.After, r) && !was[r] {
			return true
		}
	}
	return false
}

// Check is the recall of one check over the defects planted for it.
type Check struct {
	Rule    string  `json:"rule"`
	Planted int     `json:"planted"`
	Found   int     `json:"found"`
	Recall  float64 `json:"recall"`
	// Missed lists the cases the check did not find, so a miss can be looked at.
	Missed []string `json:"missed,omitempty"`
}

// Score is recall per check over a set of outcomes.
type Score struct {
	Checks []Check `json:"checks"`
	// Errors counts the outcomes that could not be run. They are left out of the recall.
	Errors  int     `json:"errors"`
	Planted int     `json:"planted"`
	Found   int     `json:"found"`
	Recall  float64 `json:"recall"`
}

// ScoreOutcomes works out recall for each check. A case counts toward the first rule it lists.
func ScoreOutcomes(outs []Outcome) Score {
	by := map[string]*Check{}
	var s Score
	for _, o := range outs {
		if o.Error != "" || len(o.Rules) == 0 {
			s.Errors++
			continue
		}
		c := by[o.Rules[0]]
		if c == nil {
			c = &Check{Rule: o.Rules[0]}
			by[o.Rules[0]] = c
		}
		c.Planted++
		s.Planted++
		if o.Found() {
			c.Found++
			s.Found++
		} else {
			c.Missed = append(c.Missed, o.Page+"/"+o.Defect)
		}
	}
	for _, c := range by {
		c.Recall = ratio(c.Found, c.Planted)
		s.Checks = append(s.Checks, *c)
	}
	sort.Slice(s.Checks, func(i, j int) bool { return s.Checks[i].Rule < s.Checks[j].Rule })
	s.Recall = ratio(s.Found, s.Planted)
	return s
}

// Write prints the score as a table.
func (s Score) Write(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "check\tplanted\tfound\trecall\tmissed")
	for _, c := range s.Checks {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%.0f%%\t%s\n", c.Rule, c.Planted, c.Found, c.Recall*100, strings.Join(c.Missed, ", "))
	}
	fmt.Fprintf(tw, "all\t%d\t%d\t%.0f%%\t\n", s.Planted, s.Found, s.Recall*100)
	_ = tw.Flush()
	if s.Errors > 0 {
		fmt.Fprintf(w, "%d cases could not be run and are left out.\n", s.Errors)
	}
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func set(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
