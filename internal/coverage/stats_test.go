package coverage

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// inc builds incidence counts: q1 problems seen once, q2 seen twice, then others.
func inc(q1, q2 int, others ...int) []int {
	var out []int
	for i := 0; i < q1; i++ {
		out = append(out, 1)
	}
	for i := 0; i < q2; i++ {
		out = append(out, 2)
	}
	return append(out, others...)
}

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v +/- %v", name, got, want, tol)
	}
}

// The numbers come from the review's recomputation with Chao's reference code, not from this package.
func TestChao2MatchesReviewTable(t *testing.T) {
	for _, c := range []struct {
		name        string
		counts      []int
		t           int
		form        string
		est, lo, hi float64
		cov         float64
	}{
		{"dd strict ids", inc(8, 0), 7, FormBiasCorrected, 32.0, 14.4, 97.9, 0.045},
		{"dd mine", inc(5, 1), 7, FormClassic, 16.7, 7.4, 86.8, 0.330},
		{"sb strict ids", inc(22, 0), 12, FormBiasCorrected, 233.7, 115.4, 502.1, 0.009},
		{"sb mine", inc(4, 1, 4), 12, FormClassic, 13.3, 6.9, 65.6, 0.617},
	} {
		r := Chao2(c.counts, c.t)
		if r.Form != c.form {
			t.Errorf("%s: form %s, want %s", c.name, r.Form, c.form)
		}
		near(t, c.name+" estimate", r.Estimate, c.est, 0.06)
		near(t, c.name+" lo", r.CI[0], c.lo, 0.06)
		near(t, c.name+" hi", r.CI[1], c.hi, 0.06)
		cov, ok := SampleCoverage(c.counts, c.t)
		if !ok {
			t.Fatalf("%s: coverage not estimable", c.name)
		}
		near(t, c.name+" coverage", cov, c.cov, 0.0006)
	}
}

func TestSBLumpedBiasCorrectedAndCoverage(t *testing.T) {
	counts := []int{1, 1, 3, 4} // S=4 f1=2 f2=0 U=9, T=12
	r := Chao2(counts, 12)
	if r.Form != FormBiasCorrected {
		t.Fatal(r.Form)
	}
	near(t, "estimate", r.Estimate, 4.9, 0.05)
	near(t, "lo", r.CI[0], 4.1, 0.06)
	near(t, "hi", r.CI[1], 16.3, 0.06)
	cov, _ := SampleCoverage(counts, 12)
	near(t, "coverage", cov, 0.812, 0.0006)
}

func TestDDLumpedNotEstimable(t *testing.T) {
	counts := []int{1, 3} // S=2 f1=1 f2=0 U=4
	r := Chao2(counts, 7)
	if r.Form != FormNotEstimable || r.Estimate != 2 || r.CI != nil {
		t.Fatalf("%+v", r)
	}
	if _, ok := SampleCoverage(counts, 7); ok {
		t.Error("coverage reported for Q1=1, Q2=0")
	}
	if !strings.Contains(r.Label(), "not estimable") {
		t.Error(r.Label())
	}
}

func TestLabelSaysAtLeastAndNamesTheForm(t *testing.T) {
	l := Chao2(inc(5, 1), 7).Label()
	if !strings.HasPrefix(l, "at least") || !strings.Contains(l, "classic") || !strings.Contains(l, "T=7") {
		t.Error(l)
	}
	l = Chao2(inc(8, 0), 7).Label()
	if !strings.HasPrefix(l, "at least 32.0 (Chao2 bias-corrected form;") {
		t.Error(l)
	}
}

func TestVarianceValuesFromReview(t *testing.T) {
	near(t, "dd strict", *Chao2(inc(8, 0), 7).Var, 331.1, 0.1)
	near(t, "dd mine", *Chao2(inc(5, 1), 7).Var, 217.3, 0.1)
	near(t, "sb strict", *Chao2(inc(22, 0), 12).Var, 8546.4, 0.2)
}

func TestClassicFormWhenQ2PositiveNeverBiasCorrected(t *testing.T) {
	r := Chao2(inc(5, 1), 7)
	near(t, "estimate", r.Estimate, 6+(6.0/7)*25/2, 1e-9)
}

func TestNoSingletonsMeansNoUnseenEstimate(t *testing.T) {
	r := Chao2([]int{2, 3, 3}, 8)
	if r.Form != FormNotEstimable {
		t.Fatal(r.Form)
	}
	if c, ok := SampleCoverage([]int{2, 3, 3}, 8); !ok || c != 1 {
		t.Errorf("coverage %v %v", c, ok)
	}
}

func TestStratifiedSpektrabotCoverage(t *testing.T) {
	phone := []int{1, 1, 1, 2} // T=4, U=5
	c, _ := SampleCoverage(phone, 4)
	near(t, "phone", c, 0.509, 0.0006)
	desk := []int{1, 2} // T=6
	c, _ = SampleCoverage(desk, 6)
	near(t, "desktop coverage", c, 0.762, 0.0006)
	near(t, "desktop estimate", Chao2(desk, 6).Estimate, 2.4, 0.05)
}

func TestRarefactionSlopeValues(t *testing.T) {
	near(t, "dd", RarefactionSlope(inc(5, 1), 7), 0.71, 0.005)
	near(t, "sb", RarefactionSlope(inc(4, 1, 4), 12), 0.33, 0.005)
}

func TestSlopeIdentityWithRarefactionCurve(t *testing.T) {
	for _, c := range []struct {
		counts []int
		t      int
	}{{inc(5, 1), 7}, {inc(4, 1, 4), 12}, {[]int{1, 1, 2, 3, 5}, 9}} {
		curve := RarefactionCurve(c.counts, c.t)
		near(t, "slope", curve[len(curve)-1]-curve[len(curve)-2], RarefactionSlope(c.counts, c.t), 1e-9)
		near(t, "end", curve[len(curve)-1], float64(len(c.counts)), 1e-9)
	}
}

func TestJackknife2(t *testing.T) {
	got, ok := Jackknife2(inc(5, 1), 7)
	if !ok {
		t.Fatal("not ok")
	}
	near(t, "jackknife2", got, 6+5*11.0/7-1*25.0/42, 1e-9)
	if _, ok := Jackknife2(inc(5, 1), 2); ok {
		t.Error("estimated with T=2")
	}
}

func TestWilson(t *testing.T) {
	lo, hi := Wilson(4, 5)
	near(t, "lo", lo, 0.376, 0.002)
	near(t, "hi", hi, 0.964, 0.002)
	if lo, _ := Wilson(0, 0); !math.IsNaN(lo) {
		t.Error("no NaN for n=0")
	}
}

func TestClusterLabels(t *testing.T) {
	lab := ClusterLabels([]string{"a", "a", "b", "c"}, [][2]int{{2, 3}})
	if lab[0] != lab[1] || lab[2] != lab[3] || lab[0] == lab[2] {
		t.Error(lab)
	}
}

func TestKappaPerfectAndChance(t *testing.T) {
	k, _ := CohenKappaPairs([]int{0, 0, 1, 1}, []int{5, 5, 6, 6})
	near(t, "perfect", k.Kappa, 1, 1e-9)
	k, _ = CohenKappaPairs([]int{0, 0, 1, 1, 2, 2}, []int{0, 1, 0, 1, 2, 3})
	if k.Kappa >= 0.5 {
		t.Errorf("kappa %v", k.Kappa)
	}
}

func TestKappaKnownValue(t *testing.T) {
	// A: {0,1} {2} {3}. B: {0,1,2} {3}. po = 4/6, pa = 1/6, pb = 3/6, pe = 0.5, kappa = 1/3.
	k, _ := CohenKappaPairs([]int{0, 0, 1, 2}, []int{0, 0, 0, 1})
	near(t, "kappa", k.Kappa, 1.0/3, 1e-9)
	if k.Pairs != 6 {
		t.Error(k.Pairs)
	}
	if _, err := CohenKappaPairs([]int{0}, []int{0, 1}); err == nil {
		t.Error("length mismatch accepted")
	}
}

// ---- stopping rule

func ro(stratum, outcome string, probs ...string) RunObs {
	m := map[string]bool{}
	for _, p := range probs {
		m[p] = true
	}
	return RunObs{Stratum: stratum, Outcome: outcome, Problems: m}
}

func repeat(n int, r RunObs) []RunObs {
	out := make([]RunObs, n)
	for i := range out {
		out[i] = r
	}
	return out
}

func TestErroredAndBlockedRunsDoNotCount(t *testing.T) {
	runs := []RunObs{ro("d", "completed", "1", "2"), ro("d", "errored"), ro("d", "blocked"), ro("d", "errored")}
	s := StatusOf(runs, "d", DefaultThresholds())
	if s.Completed != 1 || s.Stopped || !strings.Contains(s.Reason, "need 4") {
		t.Errorf("%+v", s)
	}
}

func TestNeedsFourCompletedPerStratum(t *testing.T) {
	if StatusOf(repeat(3, ro("d", "completed", "1", "2", "3")), "d", DefaultThresholds()).Stopped {
		t.Error("stopped at 3 runs")
	}
}

func TestStratumStopsWhenCoveredAndQuiet(t *testing.T) {
	s := StatusOf(repeat(6, ro("d", "completed", "1", "2", "3", "4")), "d", DefaultThresholds())
	if !s.Stopped || s.Coverage == nil || *s.Coverage != 1 || s.RunsSinceNew != 5 {
		t.Errorf("%+v", s)
	}
}

func TestNewProblemBlocksStop(t *testing.T) {
	runs := append(repeat(5, ro("d", "completed", "1", "2")), ro("d", "completed", "1", "2", "9"))
	s := StatusOf(runs, "d", DefaultThresholds())
	if s.Stopped || s.RunsSinceNew != 0 {
		t.Errorf("%+v", s)
	}
}

func TestAllStrataMustStopAndReportNames(t *testing.T) {
	runs := append(repeat(6, ro("d", "completed", "1", "2")), repeat(2, ro("p", "completed", "1", "2"))...)
	ok, st := Decide(runs, []string{"d", "p"}, DefaultThresholds())
	if ok || !st[0].Stopped || st[1].Stopped {
		t.Errorf("%v %+v", ok, st)
	}
}

func TestThresholdsChangeTheRule(t *testing.T) {
	runs := repeat(3, ro("d", "completed", "1", "2"))
	th := DefaultThresholds()
	th.MinRuns, th.NoNew = 2, 1
	if !StatusOf(runs, "d", th).Stopped {
		t.Error("lower thresholds did not stop")
	}
}

func TestNoStrataNeverStops(t *testing.T) {
	if ok, _ := Decide(nil, nil, DefaultThresholds()); ok {
		t.Error("stopped with no strata")
	}
}

// pooled is the v1 rule: coverage >= 0.90 and the last three runs add nothing, every run counted.
func pooled(history []map[string]bool) bool {
	t := len(history)
	counts := map[string]int{}
	seen := map[string]bool{}
	var seenAfter []int
	for _, h := range history {
		for p := range h {
			counts[p]++
			seen[p] = true
		}
		seenAfter = append(seenAfter, len(seen))
	}
	if len(counts) == 0 || t < 4 {
		return false
	}
	var inc []int
	for _, c := range counts {
		inc = append(inc, c)
	}
	cov, ok := SampleCoverage(inc, t)
	return ok && cov >= 0.90 && seenAfter[t-1] == seenAfter[t-4]
}

func TestPooledRuleFalseStopExample(t *testing.T) {
	var hist []map[string]bool
	for i := 0; i < 6; i++ {
		hist = append(hist, map[string]bool{"1": true, "2": true, "3": true})
	}
	if !pooled(hist) {
		t.Fatal("pooled rule did not fire")
	}
	runs := append(repeat(6, ro("d", "completed", "1", "2", "3")), ro("p", "completed"))
	if ok, _ := Decide(runs, []string{"d", "p"}, DefaultThresholds()); ok {
		t.Error("stratified rule stopped with an unexposed phone stratum")
	}
}

type simConfig struct {
	nTrue, nAny   int
	pAny, pPhone  float64
	phoneShare    float64
	pBlocked      float64
	maxRuns, sims int
	seed          uint64
}

// simulate runs the stopping rule in a world with known truth, like the reference's simulate.
func simulate(rule string, c simConfig) (fire, falseStop80 float64) {
	rng := rand.New(rand.NewPCG(c.seed, 0))
	fired, bad := 0, 0
	for s := 0; s < c.sims; s++ {
		var hist []map[string]bool
		var obs []RunObs
		found := map[int]bool{}
		for t := 1; t <= c.maxRuns; t++ {
			dev := "desktop"
			if rng.Float64() < c.phoneShare {
				dev = "phone"
			}
			blocked := rng.Float64() < c.pBlocked
			probs := map[string]bool{}
			if !blocked {
				for i := 0; i < c.nTrue; i++ {
					p := 0.0
					switch {
					case i < c.nAny:
						p = c.pAny
					case dev == "phone":
						p = c.pPhone
					}
					if rng.Float64() < p {
						probs[string(rune('A'+i))] = true
						found[i] = true
					}
				}
			}
			hist = append(hist, probs)
			outcome := "completed"
			if blocked {
				outcome = "blocked"
			}
			obs = append(obs, RunObs{Stratum: dev, Outcome: outcome, Problems: probs})
			var stop bool
			if rule == "pooled" {
				stop = pooled(hist)
			} else {
				stop, _ = Decide(obs, []string{"desktop", "phone"}, DefaultThresholds())
			}
			if stop {
				fired++
				if float64(len(found))/float64(c.nTrue) < 0.8 {
					bad++
				}
				break
			}
		}
	}
	if fired == 0 {
		return 0, 0
	}
	return float64(fired) / float64(c.sims), float64(bad) / float64(fired)
}

func TestStratifiedRuleHasNoFalseStopsInASmallWorld(t *testing.T) {
	c := simConfig{nTrue: 20, nAny: 12, pAny: 0.35, pPhone: 0.6, phoneShare: 0.2, pBlocked: 0.3, maxRuns: 80, sims: 150, seed: 3}
	_, bad := simulate("stratified", c)
	if bad != 0 {
		t.Errorf("false-stop share %v", bad)
	}
}

func TestPooledRuleFalseStopsExceedTheTenPercentLine(t *testing.T) {
	c := simConfig{nTrue: 20, nAny: 12, pAny: 0.35, pPhone: 0.6, phoneShare: 0.2, pBlocked: 0.3, maxRuns: 80, sims: 400, seed: 2}
	fire, bad := simulate("pooled", c)
	if fire <= 0.95 || bad <= 0.10 {
		t.Errorf("fire %v, false-stop share %v", fire, bad)
	}
}

// ---- scoring

func fv(sev, ev string, k, n int, verdict ...string) FindingVector {
	v := "confirmed"
	if len(verdict) > 0 {
		v = verdict[0]
	}
	return FindingVector{Severity: sev, Evidence: ev, Verdict: v, K: k, N: n}
}

func band(t *testing.T, v FindingVector) string {
	t.Helper()
	b, ok := Band(v)
	if !ok {
		return ""
	}
	return b
}

func TestReviewCounterexamplesResolved(t *testing.T) {
	if got := band(t, fv("blocker", "inferred", 1, 1)); got != "major" {
		t.Error(got)
	}
	if got := band(t, fv("major", "measured", 1, 1)); got != "major" {
		t.Error(got)
	}
	if got := band(t, fv("blocker", "measured", 1, 3)); got != "blocker" {
		t.Error(got)
	}
}

func TestDemotionNeedsBothConditions(t *testing.T) {
	for _, c := range []struct {
		v    FindingVector
		want string
	}{
		{fv("blocker", "inferred", 2, 3), "blocker"},
		{fv("blocker", "measured", 1, 1), "blocker"},
		{fv("blocker", "sourced", 1, 1), "blocker"},
		{fv("info", "inferred", 1, 1), "info"},
		{fv("minor", "inferred", 1, 1), "info"},
		{fv("blocker", "measured", 4, 4), "blocker"},
		{fv("blocker", "inferred", 4, 4), "blocker"},
	} {
		if got := band(t, c.v); got != c.want {
			t.Errorf("%+v = %s, want %s", c.v, got, c.want)
		}
	}
}

func TestPendingAndRejectedAreNotScored(t *testing.T) {
	for _, v := range []string{"pending", "rejected"} {
		if _, ok := Band(fv("blocker", "measured", 4, 4, v)); ok {
			t.Errorf("%s was banded", v)
		}
	}
	if !strings.Contains(Display(fv("blocker", "measured", 4, 4, "pending")), "unscored") {
		t.Error("no unscored label")
	}
}

func TestSortKeyOrdersByBandThenEvidence(t *testing.T) {
	items := []FindingVector{fv("major", "measured", 4, 4), fv("blocker", "inferred", 1, 1), fv("blocker", "measured", 1, 1), fv("minor", "measured", 5, 5)}
	slices.SortFunc(items, func(a, b FindingVector) int {
		switch {
		case Less(a, b):
			return -1
		case Less(b, a):
			return 1
		}
		return 0
	})
	var bands []string
	for _, x := range items {
		bands = append(bands, band(t, x))
	}
	if !slices.Equal(bands, []string{"blocker", "major", "major", "minor"}) {
		t.Fatal(bands)
	}
	if items[0].Evidence != "measured" || items[1].Evidence != "measured" || items[2].Severity != "blocker" {
		t.Errorf("%+v", items)
	}
}

func TestDisplayShowsKNEvidence(t *testing.T) {
	s := Display(fv("blocker", "inferred", 1, 1))
	for _, w := range []string{"1 of 1", "inferred", "unreplicated"} {
		if !strings.Contains(s, w) {
			t.Errorf("%q missing %q", s, w)
		}
	}
	if !strings.HasPrefix(s, "major") {
		t.Error(s)
	}
}
