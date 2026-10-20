// Package coverage computes the Dummy Run coverage model v2 from run records: problem richness with
// Chao2, sample coverage, the rarefaction slope, route and persona-dimension coverage, a stratified
// stopping rule and the lexicographic scoring bands. The estimators follow the reference implementation
// in the coverage research (analysis/coverage_lib.py), and the tests carry its numbers.
package coverage

import (
	"fmt"
	"math"
	"sort"
)

// FreqCounts holds the incidence frequency counts of the observed problems.
type FreqCounts struct {
	SObs int // distinct problems seen
	Q1   int // problems seen in exactly one run
	Q2   int // problems seen in exactly two runs
	U    int // total incidences
}

// Freq counts S_obs, Q1, Q2 and U from the number of runs each observed problem appears in.
func Freq(incidence []int) FreqCounts {
	f := FreqCounts{SObs: len(incidence)}
	for _, n := range incidence {
		switch n {
		case 1:
			f.Q1++
		case 2:
			f.Q2++
		}
		f.U += n
	}
	return f
}

// Chao2 forms.
const (
	FormClassic       = "classic"
	FormBiasCorrected = "bias-corrected"
	FormNotEstimable  = "not-estimable"
)

// Chao2Result is the Chao2 lower bound on the number of problems, in one of its two forms.
type Chao2Result struct {
	SObs     int
	Q1, Q2   int
	T        int
	Form     string
	Estimate float64
	Var      *float64
	CI       *[2]float64
}

// Label reads the result as "at least", with the form named. It never says how many remain.
func (r Chao2Result) Label() string {
	if r.Form == FormNotEstimable {
		return fmt.Sprintf("%d observed; Chao2 not estimable (Q1=%d, Q2=%d)", r.SObs, r.Q1, r.Q2)
	}
	lo, hi := r.Estimate, r.Estimate
	if r.CI != nil {
		lo, hi = r.CI[0], r.CI[1]
	}
	return fmt.Sprintf("at least %.1f (Chao2 %s form; 95%% interval %.1f to %.1f; T=%d)", r.Estimate, r.Form, lo, hi, r.T)
}

// Chao2 follows Chao's reference code: the classic form when Q1 > 0 and Q2 > 0, the bias-corrected form
// when Q1 > 1 and Q2 = 0, otherwise S_obs with no estimate. The interval is the log-normal interval of
// Chao 1987 eq. 12. Read the result as "at least", never as how many remain.
func Chao2(incidence []int, t int) Chao2Result {
	f := Freq(incidence)
	q1, q2 := float64(f.Q1), float64(f.Q2)
	a := 0.0
	if t > 0 {
		a = float64(t-1) / float64(t)
	}
	var est, v float64
	var form string
	switch {
	case f.Q1 > 0 && f.Q2 > 0:
		est = float64(f.SObs) + a*q1*q1/(2*q2)
		r := q1 / q2
		v = q2 * (a/2*r*r + a*a*r*r*r + a*a/4*r*r*r*r)
		form = FormClassic
	case f.Q1 > 1 && f.Q2 == 0:
		est = float64(f.SObs) + a*q1*(q1-1)/2
		v = a*q1*(q1-1)/2 + a*a*q1*(2*q1-1)*(2*q1-1)/4 - a*a*q1*q1*q1*q1/(4*est)
		form = FormBiasCorrected
	default:
		return Chao2Result{SObs: f.SObs, Q1: f.Q1, Q2: f.Q2, T: t, Form: FormNotEstimable, Estimate: float64(f.SObs)}
	}
	s := float64(f.SObs)
	f0 := est - s
	ci := [2]float64{s, s}
	if f0 > 0 && v > 0 {
		c := math.Exp(1.96 * math.Sqrt(math.Log(1+v/(f0*f0))))
		ci = [2]float64{s + f0/c, s + f0*c}
	}
	return Chao2Result{SObs: f.SObs, Q1: f.Q1, Q2: f.Q2, T: t, Form: form, Estimate: est, Var: &v, CI: &ci}
}

// SampleCoverage is Chao et al. 2014 eq. C.7 with the SpadeR and iNEXT branch for Q2 = 0. ok is false
// ("not estimable") when no incidence was seen or when Q1 <= 1 and Q2 = 0, where the software returns 1
// or S_obs, which are not measurements.
func SampleCoverage(incidence []int, t int) (c float64, ok bool) {
	f := Freq(incidence)
	if f.U == 0 {
		return 0, false
	}
	q1, q2, u, tm := float64(f.Q1), float64(f.Q2), float64(f.U), float64(t-1)
	switch {
	case f.Q1 == 0:
		return 1, true
	case f.Q2 > 0:
		return 1 - (q1/u)*(tm*q1/(tm*q1+2*q2)), true
	case f.Q1 > 1:
		return 1 - (q1/u)*(tm*(q1-1)/(tm*(q1-1)+2)), true
	}
	return 0, false
}

// Jackknife2 is the second-order jackknife for incidence data, a sensitivity check only. Chao 1987
// warns that it works badly when most problems are seen once or twice. ok is false for T <= 2.
func Jackknife2(incidence []int, t int) (float64, bool) {
	if t <= 2 {
		return 0, false
	}
	f := Freq(incidence)
	tf := float64(t)
	return float64(f.SObs) + float64(f.Q1)*(2*tf-3)/tf - float64(f.Q2)*(tf-2)*(tf-2)/(tf*(tf-1)), true
}

// RarefactionCurve is the expected number of distinct problems after m = 1..T runs (Colwell et al. 2012).
func RarefactionCurve(incidence []int, t int) []float64 {
	out := make([]float64, 0, t)
	for m := 1; m <= t; m++ {
		sum := 0.0
		for _, q := range incidence {
			if t-q >= m {
				ratio := 1.0
				for i := 0; i < m; i++ {
					ratio *= float64(t-q-i) / float64(t-i)
				}
				sum += 1 - ratio
			} else {
				sum++
			}
		}
		out = append(out, sum)
	}
	return out
}

// RarefactionSlope is the expected number of new problems the last run added, E[S(T)] - E[S(T-1)],
// which is exactly Q1/T. It is an expected count and not a probability that one more run finds something.
func RarefactionSlope(incidence []int, t int) float64 {
	if t == 0 {
		return 0
	}
	return float64(Freq(incidence).Q1) / float64(t)
}

// Wilson is the 95 percent Wilson score interval for k of n.
func Wilson(k, n int) (lo, hi float64) {
	if n == 0 {
		return math.NaN(), math.NaN()
	}
	const z = 1.96
	nf := float64(n)
	p := float64(k) / nf
	d := 1 + z*z/nf
	c := (p + z*z/(2*nf)) / d
	h := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / d
	return c - h, c + h
}

// Kappa is Cohen's kappa on same-problem pair decisions.
type Kappa struct {
	Kappa    float64
	Po       float64
	Pairs    int
	SameA    int
	SameB    int
	SameBoth int
}

// CohenKappaPairs compares two labelings. For every pair of findings each rater says same or different.
// Kappa is NaN when chance agreement is 1 or there are no pairs.
func CohenKappaPairs(a, b []int) (Kappa, error) {
	if len(a) != len(b) {
		return Kappa{}, fmt.Errorf("coverage: labelings differ in length (%d and %d)", len(a), len(b))
	}
	n := len(a)
	var k Kappa
	agree := 0
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			sa, sb := a[i] == a[j], b[i] == b[j]
			k.Pairs++
			if sa {
				k.SameA++
			}
			if sb {
				k.SameB++
			}
			if sa && sb {
				k.SameBoth++
			}
			if sa == sb {
				agree++
			}
		}
	}
	if k.Pairs == 0 {
		k.Kappa, k.Po = math.NaN(), math.NaN()
		return k, nil
	}
	total := float64(k.Pairs)
	k.Po = float64(agree) / total
	pa, pb := float64(k.SameA)/total, float64(k.SameB)/total
	pe := pa*pb + (1-pa)*(1-pb)
	if pe == 1 {
		k.Kappa = math.NaN()
	} else {
		k.Kappa = (k.Po - pe) / (1 - pe)
	}
	return k, nil
}

// ClusterLabels joins findings: those with an equal key are one problem, and each accepted pair joins two
// clusters. Labels count up from 0 in order of first appearance.
func ClusterLabels(keys []string, pairs [][2]int) []int {
	parent := make([]int, len(keys))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) { parent[find(a)] = find(b) }
	first := map[string]int{}
	for i, k := range keys {
		if j, ok := first[k]; ok {
			union(i, j)
		} else {
			first[k] = i
		}
	}
	for _, p := range pairs {
		union(p[0], p[1])
	}
	roots := map[int]int{}
	out := make([]int, len(keys))
	for i := range keys {
		r := find(i)
		if _, ok := roots[r]; !ok {
			roots[r] = len(roots)
		}
		out[i] = roots[r]
	}
	return out
}

// Incidence turns problem labels per run into the number of runs each problem appears in, in a stable order.
func Incidence(perRun []map[string]bool) []int {
	counts := map[string]int{}
	for _, run := range perRun {
		for p := range run {
			counts[p]++
		}
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]int, len(keys))
	for i, k := range keys {
		out[i] = counts[k]
	}
	return out
}
