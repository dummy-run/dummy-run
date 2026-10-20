package coverage

import "fmt"

// Thresholds are the policy numbers of the stopping rule and the identity gate. They are defaults, not
// calibrated: the study tests the rule, it does not tune it afterwards.
type Thresholds struct {
	// Coverage is the sample coverage a stratum needs (0.90).
	Coverage float64
	// MinRuns is the number of completed runs a stratum needs (4).
	MinRuns int
	// NoNew is how many of the latest completed runs must add no new problem (3).
	NoNew int
	// Kappa is the two-reviewer agreement below which only S_obs is printed (0.6).
	Kappa float64
}

// DefaultThresholds are the pre-registered values.
func DefaultThresholds() Thresholds {
	return Thresholds{Coverage: 0.90, MinRuns: 4, NoNew: 3, Kappa: 0.6}
}

// KappaAccept is the pre-registered kappa for Tier 3 and for accepting the identity method.
const KappaAccept = 0.8

// TrustCoverageFloor is the coverage below which Chao and Jost 2012 advise against extrapolation.
const TrustCoverageFloor = 0.50

// Outcomes of a run.
const (
	OutcomeCompleted = "completed"
)

// RunObs is one run as the stopping rule sees it.
type RunObs struct {
	Stratum  string
	Outcome  string
	Problems map[string]bool
}

// StratumStatus is the stopping status of one stratum.
type StratumStatus struct {
	Stratum      string   `json:"stratum"`
	Completed    int      `json:"completed"`
	Coverage     *float64 `json:"coverage"`
	RunsSinceNew int      `json:"runs_since_new"`
	Stopped      bool     `json:"stopped"`
	Reason       string   `json:"reason"`
}

// StatusOf is the status of one stratum. Only completed runs count. Blocked, errored and budget-exhausted
// runs are left out of every count. A completed run with no findings counts as a run that added nothing new.
func StatusOf(runs []RunObs, stratum string, th Thresholds) StratumStatus {
	var comp []RunObs
	for _, r := range runs {
		if r.Stratum == stratum && r.Outcome == OutcomeCompleted {
			comp = append(comp, r)
		}
	}
	n := len(comp)
	seen := map[string]bool{}
	lastNew := -1
	counts := map[string]int{}
	for i, r := range comp {
		isNew := false
		for p := range r.Problems {
			if !seen[p] {
				isNew = true
			}
			seen[p] = true
			counts[p]++
		}
		if isNew {
			lastNew = i
		}
	}
	since := n
	if lastNew >= 0 {
		since = n - 1 - lastNew
	}
	var cov *float64
	var inc []int
	for _, c := range counts {
		inc = append(inc, c)
	}
	if len(inc) > 0 {
		if c, ok := SampleCoverage(inc, n); ok {
			cov = &c
		}
	}
	st := StratumStatus{Stratum: stratum, Completed: n, Coverage: cov, RunsSinceNew: since}
	switch {
	case n < th.MinRuns:
		st.Reason = fmt.Sprintf("%d completed runs, need %d", n, th.MinRuns)
	case len(inc) == 0:
		st.Coverage = nil
		st.Reason = "no confirmed problems yet; coverage not estimable"
	case cov == nil:
		st.Reason = "coverage not estimable (Q1 <= 1, Q2 = 0)"
	case *cov < th.Coverage:
		st.Reason = fmt.Sprintf("coverage %.2f < %.2f", *cov, th.Coverage)
	case since < th.NoNew:
		st.Reason = fmt.Sprintf("only %d completed runs since the last new problem, need %d", since, th.NoNew)
	default:
		st.Stopped, st.Reason = true, "coverage and no-new-problem conditions met"
	}
	return st
}

// Decide stops only when every declared stratum has stopped, and names each stratum's status.
func Decide(runs []RunObs, strata []string, th Thresholds) (bool, []StratumStatus) {
	all := len(strata) > 0
	out := make([]StratumStatus, 0, len(strata))
	for _, s := range strata {
		st := StatusOf(runs, s, th)
		all = all && st.Stopped
		out = append(out, st)
	}
	return all, out
}
