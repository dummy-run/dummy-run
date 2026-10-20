package coverage

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/dummy-run/dummy-run/internal/identity"
	"github.com/dummy-run/dummy-run/internal/record"
)

// ReportVersion is the version of the report's JSON shape.
const ReportVersion = 1

// Options choose what a report counts.
type Options struct {
	// Build selects records by commit (or build hash). A prefix of at least 7 characters is enough.
	// Empty means the records must all be from one build.
	Build string
	// Strata is "device" (the default) or "persona".
	Strata string
	// ConfirmedOnly counts only reviewer-confirmed findings. When false, pending findings count too and
	// the report says it is a demonstration on unreviewed findings. Rejected findings never count.
	ConfirmedOnly bool
	Thresholds    Thresholds
	// Clusters are reviewers' groupings of findings into problems. Two files give a kappa.
	Clusters []*ClusterFile
}

// Refusal is returned when the command must not produce a report. The message says why and what to do.
type Refusal struct{ Msg string }

func (r *Refusal) Error() string { return r.Msg }

func refuse(format string, a ...any) error { return &Refusal{Msg: fmt.Sprintf(format, a...)} }

// Report is everything `dummyrun coverage` prints.
type Report struct {
	ReportVersion int           `json:"report_version"`
	Build         string        `json:"build"`
	Basis         string        `json:"basis"`
	Strata        string        `json:"strata"`
	Thresholds    ThresholdsOut `json:"thresholds"`
	Runs          RunsBlock     `json:"runs"`
	Review        ReviewBlock   `json:"review"`
	Reach         ReachBlock    `json:"reach"`
	Completeness  Completeness  `json:"completeness"`
	Stopping      StoppingBlock `json:"stopping"`
	Findings      []FindingRow  `json:"findings"`
	Warnings      []string      `json:"warnings"`
}

// ThresholdsOut echoes the policy numbers used.
type ThresholdsOut struct {
	Coverage float64 `json:"coverage"`
	MinRuns  int     `json:"min_runs"`
	NoNew    int     `json:"no_new"`
	Kappa    float64 `json:"kappa"`
}

// RunsBlock counts runs by outcome. Only completed runs are sampling units.
type RunsBlock struct {
	Total              int     `json:"total"`
	Completed          int     `json:"completed"`
	Blocked            int     `json:"blocked"`
	Errored            int     `json:"errored"`
	BudgetExhausted    int     `json:"budget_exhausted"`
	HarnessFailureRate float64 `json:"harness_failure_rate"`
	NotFingerprinted   int     `json:"not_fingerprinted"`
}

// ReviewBlock counts problem findings by verdict.
type ReviewBlock struct {
	Confirmed    int         `json:"confirmed"`
	Rejected     int         `json:"rejected"`
	Pending      int         `json:"pending"`
	Observations int         `json:"observations"`
	Kappa        *KappaBlock `json:"kappa"`
}

// KappaBlock is the agreement between two groupings of the same findings.
type KappaBlock struct {
	Value     *float64 `json:"value"`
	Pairs     int      `json:"pairs"`
	Reviewers int      `json:"reviewers"`
	Status    string   `json:"status"`
}

// Kappa statuses.
const (
	KappaPrint   = "print_estimates"
	KappaBand    = "print_band"
	KappaSObs    = "s_obs_only"
	KappaSingle  = "single_reviewer_vs_fingerprint"
	KappaUnknown = "not_measured"
)

// ReachBlock says how much of the app and how many kinds of persona were touched.
type ReachBlock struct {
	RouteCoverage *RouteCoverage          `json:"route_coverage"`
	Dimensions    map[string]DimensionUse `json:"dimensions"`
	DevicePairs   *PairCoverage           `json:"device_browser_pairs"`
}

// RouteCoverage is visited routes over the stated universe.
type RouteCoverage struct {
	Visited  int      `json:"visited"`
	Universe int      `json:"universe"`
	Sources  []string `json:"sources"`
	Outside  int      `json:"visited_outside_universe"`
	Missed   []string `json:"never_visited"`
}

// DimensionUse is how many values of a persona dimension were used. Possible is the number of values
// the record format allows, or nil where the values are open-ended.
type DimensionUse struct {
	Used     int      `json:"used"`
	Possible *int     `json:"possible"`
	Values   []string `json:"values"`
}

// PairCoverage is pairs of two dimensions that were used over the pairs that exist.
type PairCoverage struct {
	Used     int `json:"used"`
	Possible int `json:"possible"`
}

// Completeness is the remaining-problem block.
type Completeness struct {
	SObs         int          `json:"s_obs"`
	T            int          `json:"t"`
	Label        string       `json:"label"`
	IdentityBand []IdentityOp `json:"identity_band"`
	Jackknife2   *float64     `json:"jackknife2"`
	Note         string       `json:"note"`
}

// IdentityOp is the estimates under one way of deciding which findings are the same problem.
type IdentityOp struct {
	Operation      string      `json:"operation"`
	SObs           int         `json:"s_obs"`
	Chao2          *Chao2Block `json:"chao2"`
	Coverage       *float64    `json:"coverage"`
	CoverageLabel  string      `json:"coverage_label"`
	NewPerExtraRun *float64    `json:"new_per_extra_run"`
	Q1             int         `json:"q1"`
	Q2             int         `json:"q2"`
}

// Chao2Block is a Chao2 result with its form named. AtLeast is a lower bound.
type Chao2Block struct {
	Form    string      `json:"form"`
	AtLeast float64     `json:"at_least"`
	CI95    *[2]float64 `json:"ci95"`
	Label   string      `json:"label"`
}

// StoppingBlock is the stratified stopping rule.
type StoppingBlock struct {
	Stop    bool            `json:"stop"`
	Summary string          `json:"summary"`
	Strata  []StratumStatus `json:"strata"`
}

// FindingRow is one problem with its band.
type FindingRow struct {
	Fingerprint  string     `json:"fingerprint"`
	Route        string     `json:"route"`
	Locus        string     `json:"locus"`
	Symptom      string     `json:"symptom"`
	Title        string     `json:"title"`
	Band         *string    `json:"band"`
	Severity     string     `json:"severity"`
	Verdict      string     `json:"verdict"`
	K            int        `json:"k"`
	N            int        `json:"n"`
	Wilson95     [2]float64 `json:"wilson95"`
	Evidence     string     `json:"evidence"`
	Unreplicated bool       `json:"unreplicated"`
	StateBound   bool       `json:"state_conditional"`
	Display      string     `json:"display"`
	Runs         []string   `json:"runs"`
}

type instance struct {
	key, run, fid, fp, cluster string
	rec                        *record.Record
	f                          record.Finding
}

func round(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(x*p) / p
}

func ptr[T any](v T) *T { return &v }

// commitOf is the build a record belongs to.
func commitOf(r *record.Record) string {
	if r.Target.Commit != "" {
		return r.Target.Commit
	}
	if r.Target.BuildHash != "" {
		return r.Target.BuildHash
	}
	return "unknown"
}

func matchesBuild(commit, want string) bool {
	return len(want) >= 7 && (strings.HasPrefix(commit, want) || strings.HasPrefix(want, commit)) && commit != "unknown"
}

func pathOf(route string) string {
	p, _, _ := strings.Cut(route, "?")
	return p
}

// Build computes the report from run records.
func Build(recs []*record.Record, opt Options) (*Report, error) {
	if opt.Strata == "" {
		opt.Strata = "device"
	}
	if opt.Strata != "device" && opt.Strata != "persona" {
		return nil, refuse("--strata must be device or persona, not %q", opt.Strata)
	}
	if opt.Thresholds == (Thresholds{}) {
		opt.Thresholds = DefaultThresholds()
	}
	th := opt.Thresholds

	var v2 []*record.Record
	notFP := 0
	for _, r := range recs {
		if r.SchemaVersion < 2 {
			notFP++
			continue
		}
		v2 = append(v2, r)
	}
	if opt.Build != "" {
		if len(opt.Build) < 7 {
			return nil, refuse("--build needs at least 7 characters of the commit or build hash")
		}
		var keep []*record.Record
		for _, r := range v2 {
			if matchesBuild(commitOf(r), opt.Build) {
				keep = append(keep, r)
			}
		}
		v2 = keep
	}
	builds := map[string]int{}
	for _, r := range v2 {
		builds[commitOf(r)]++
	}
	if len(builds) > 1 {
		var list []string
		for b, n := range builds {
			list = append(list, fmt.Sprintf("%s (%d runs)", b, n))
		}
		sort.Strings(list)
		return nil, refuse("these records are from %d builds: %s. Problems found on one build say nothing about another, so runs are not pooled across builds. Pass --build COMMIT to choose one", len(builds), strings.Join(list, ", "))
	}
	if len(v2) == 0 {
		msg := "no schema version 2 run records to count"
		if opt.Build != "" {
			msg += " for build " + opt.Build
		}
		if notFP > 0 {
			msg += fmt.Sprintf("; %d version 1 record(s) have no fingerprint and are left out of incidence statistics", notFP)
		}
		return nil, refuse("%s", msg)
	}
	build := commitOf(v2[0])

	// A fingerprint must recompute from its parts, so a record cannot claim a problem it does not describe.
	for _, r := range v2 {
		for _, f := range r.Findings {
			want, err := identity.Fingerprint(f.Route, f.Locus, f.Symptom)
			if err != nil {
				return nil, refuse("%s finding %s: %v", r.ID, f.ID, err)
			}
			if want != f.Fingerprint {
				return nil, refuse("%s finding %s: fingerprint %s does not recompute from route, locus and symptom (that gives %s)", r.ID, f.ID, f.Fingerprint, want)
			}
		}
	}

	rep := &Report{ReportVersion: ReportVersion, Build: build, Strata: opt.Strata, Warnings: []string{},
		Thresholds: ThresholdsOut(th)}

	// Runs by outcome.
	for _, r := range v2 {
		rep.Runs.Total++
		switch r.Outcome {
		case record.OutcomeCompleted:
			rep.Runs.Completed++
		case record.OutcomeBlocked:
			rep.Runs.Blocked++
		case record.OutcomeErrored:
			rep.Runs.Errored++
		case record.OutcomeBudgetExhausted:
			rep.Runs.BudgetExhausted++
		}
	}
	rep.Runs.NotFingerprinted = notFP
	rep.Runs.HarnessFailureRate = round(float64(rep.Runs.Errored)/float64(rep.Runs.Total), 3)
	if rep.Runs.Completed < 1 {
		return nil, refuse("fewer than 1 completed run for build %s (%d blocked, %d errored, %d budget exhausted); nothing to count", build, rep.Runs.Blocked, rep.Runs.Errored, rep.Runs.BudgetExhausted)
	}
	if notFP > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d version 1 record(s) are not fingerprinted and are left out of every count", notFP))
	}

	// Review counts, over every run of the build.
	pendingIncluded := false
	for _, r := range v2 {
		for _, f := range r.Findings {
			if f.Class == record.ClassObservation {
				rep.Review.Observations++
				continue
			}
			switch f.Verdict {
			case "confirmed":
				rep.Review.Confirmed++
			case "rejected":
				rep.Review.Rejected++
			default:
				rep.Review.Pending++
			}
		}
	}

	// Instances: eligible findings in completed runs.
	var inst []instance
	for _, r := range v2 {
		if r.Outcome != record.OutcomeCompleted {
			continue
		}
		for _, f := range r.Findings {
			if f.Class == record.ClassObservation || f.Verdict == "rejected" {
				continue
			}
			if f.Verdict != "confirmed" {
				if opt.ConfirmedOnly {
					continue
				}
				pendingIncluded = true
			}
			inst = append(inst, instance{key: r.ID + "/" + f.ID, run: r.ID, fid: f.ID, fp: f.Fingerprint, cluster: f.ClusterID, rec: r, f: f})
		}
	}
	rep.Basis = "reviewer-confirmed findings"
	if pendingIncluded {
		rep.Basis = "unreviewed findings: a demonstration of the method, not an estimate"
		rep.Warnings = append(rep.Warnings, "pending findings are counted. Remaining-problem estimates need reviewer-confirmed findings; read these numbers as a demonstration. Use --confirmed-only for the real figure")
	}
	if opt.ConfirmedOnly && rep.Review.Pending > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d finding(s) are still pending review and are not counted", rep.Review.Pending))
	}
	if len(inst) == 0 {
		rep.Warnings = append(rep.Warnings, "no counted findings: richness is not estimable until a reviewer confirms findings")
	}

	t := rep.Runs.Completed
	rep.Completeness.T = t

	// Identity operations.
	keys := make([]string, len(inst))
	fps := make([]string, len(inst))
	for i, in := range inst {
		keys[i], fps[i] = in.key, in.fp
	}
	ops := []struct {
		name   string
		labels []int
	}{{"fingerprint", ClusterLabels(fps, nil)}}
	var joinPairs [][2]int
	byCluster := map[string]int{}
	for i, in := range inst {
		if in.cluster == "" {
			continue
		}
		if j, ok := byCluster[in.cluster]; ok {
			joinPairs = append(joinPairs, [2]int{i, j})
		} else {
			byCluster[in.cluster] = i
		}
	}
	if len(joinPairs) > 0 {
		ops = append(ops, struct {
			name   string
			labels []int
		}{"fingerprint+cluster_id", ClusterLabels(fps, joinPairs)})
	}
	for i, c := range opt.Clusters {
		name := "reviewer-clusters"
		if len(opt.Clusters) > 1 {
			name = fmt.Sprintf("reviewer-clusters-%c", 'A'+i)
		}
		ops = append(ops, struct {
			name   string
			labels []int
		}{name, c.labels(keys)})
	}

	// Kappa.
	switch {
	case len(opt.Clusters) >= 2:
		k, err := CohenKappaPairs(ops[len(ops)-2].labels, ops[len(ops)-1].labels)
		if err != nil {
			return nil, err
		}
		kb := &KappaBlock{Pairs: k.Pairs, Reviewers: 2}
		if !math.IsNaN(k.Kappa) {
			kb.Value = ptr(round(k.Kappa, 3))
		}
		switch {
		case kb.Value == nil:
			kb.Status = KappaSObs
		case k.Kappa >= KappaAccept:
			kb.Status = KappaPrint
		case k.Kappa >= th.Kappa:
			kb.Status = KappaBand
		default:
			kb.Status = KappaSObs
		}
		rep.Review.Kappa = kb
	case len(opt.Clusters) == 1:
		k, err := CohenKappaPairs(ops[0].labels, ops[len(ops)-1].labels)
		if err != nil {
			return nil, err
		}
		kb := &KappaBlock{Pairs: k.Pairs, Reviewers: 1, Status: KappaSingle}
		if !math.IsNaN(k.Kappa) {
			kb.Value = ptr(round(k.Kappa, 3))
		}
		rep.Review.Kappa = kb
		rep.Warnings = append(rep.Warnings, "one reviewer file: kappa compares the engine's fingerprints with that one reviewer, which is not the two-reviewer measure that gates the estimates")
	default:
		rep.Review.Kappa = &KappaBlock{Status: KappaUnknown}
		rep.Warnings = append(rep.Warnings, "identity reliability is not measured (no reviewer clusters): every problem count is conditional on the fingerprint rule")
	}
	gateSObsOnly := rep.Review.Kappa != nil && rep.Review.Kappa.Status == KappaSObs && rep.Review.Kappa.Reviewers == 2
	if gateSObsOnly {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("reviewer agreement (kappa) is below %.1f: only the observed count is printed", th.Kappa))
	}

	for _, op := range ops {
		perRun := map[string]map[string]bool{}
		for i, in := range inst {
			if perRun[in.run] == nil {
				perRun[in.run] = map[string]bool{}
			}
			perRun[in.run][fmt.Sprint(op.labels[i])] = true
		}
		var runs []map[string]bool
		for _, m := range perRun {
			runs = append(runs, m)
		}
		counts := Incidence(runs)
		row := IdentityOp{Operation: op.name, SObs: len(counts)}
		f := Freq(counts)
		row.Q1, row.Q2 = f.Q1, f.Q2
		if !gateSObsOnly {
			c2 := Chao2(counts, t)
			cb := &Chao2Block{Form: c2.Form, AtLeast: round(c2.Estimate, 1), Label: c2.Label()}
			if c2.CI != nil {
				cb.CI95 = &[2]float64{round(c2.CI[0], 1), round(c2.CI[1], 1)}
			}
			row.Chao2 = cb
			if cov, ok := SampleCoverage(counts, t); ok {
				row.Coverage = ptr(round(cov, 3))
				if cov < TrustCoverageFloor {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: sample coverage %.2f is below %.2f, so do not extrapolate from it", op.name, cov, TrustCoverageFloor))
				}
			} else {
				row.CoverageLabel = "not estimable"
			}
			row.NewPerExtraRun = ptr(round(RarefactionSlope(counts, t), 3))
			if op.name == "fingerprint" {
				if jk, ok := Jackknife2(counts, t); ok {
					rep.Completeness.Jackknife2 = ptr(round(jk, 1))
				}
			}
		}
		rep.Completeness.IdentityBand = append(rep.Completeness.IdentityBand, row)
	}
	head := rep.Completeness.IdentityBand[0]
	rep.Completeness.SObs = head.SObs
	switch {
	case head.Chao2 == nil:
		rep.Completeness.Label = fmt.Sprintf("%d observed (estimates withheld)", head.SObs)
	case head.Chao2.Form == FormNotEstimable:
		rep.Completeness.Label = fmt.Sprintf("%d observed; Chao2 not estimable (Q1=%d, Q2=%d)", head.SObs, head.Q1, head.Q2)
	default:
		rep.Completeness.Label = fmt.Sprintf("%d observed; at least %.1f (Chao2 %s form, T=%d)", head.SObs, head.Chao2.AtLeast, head.Chao2.Form, t)
	}
	rep.Completeness.Note = "The Chao2 figure is a lower bound on the number of distinct problems, never a count of how many remain."
	if t < 5 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("T=%d completed runs: Chao recommends at least 5, so read the interval as decorative", t))
	}

	rep.Findings = findingRows(v2, inst, ops[0].labels)
	rep.Reach = reach(v2)
	rep.Stopping = stopping(v2, inst, opt.Strata, th)
	for _, s := range rep.Stopping.Strata {
		if s.Completed < th.MinRuns {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("stratum %s has %d completed runs; remaining-problem estimates need enough runs per stratum (at least %d)", s.Stratum, s.Completed, th.MinRuns))
		}
	}
	if rep.Runs.Errored > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d of %d runs errored (harness failure rate %.1f%%); they are left out of every count", rep.Runs.Errored, rep.Runs.Total, 100*rep.Runs.HarnessFailureRate))
	}
	for _, r := range v2 {
		if r.Target.Commit == "" && r.Target.BuildHash == "" {
			rep.Warnings = append(rep.Warnings, "records without a commit or build hash were treated as one unknown build")
			break
		}
	}
	return rep, nil
}

func stratumOf(r *record.Record, by string) string {
	if by == "persona" {
		return r.Persona.ID
	}
	return r.Persona.Device
}

func stopping(v2 []*record.Record, inst []instance, by string, th Thresholds) StoppingBlock {
	probs := map[string]map[string]bool{}
	for _, in := range inst {
		if probs[in.run] == nil {
			probs[in.run] = map[string]bool{}
		}
		probs[in.run][in.fp] = true
	}
	var obs []RunObs
	set := map[string]bool{}
	for _, r := range v2 {
		s := stratumOf(r, by)
		set[s] = true
		p := probs[r.ID]
		if p == nil {
			p = map[string]bool{}
		}
		obs = append(obs, RunObs{Stratum: s, Outcome: r.Outcome, Problems: p})
	}
	strata := make([]string, 0, len(set))
	for s := range set {
		strata = append(strata, s)
	}
	sort.Strings(strata)
	stop, st := Decide(obs, strata, th)
	for i := range st {
		if st[i].Coverage != nil {
			st[i].Coverage = ptr(round(*st[i].Coverage, 3))
		}
	}
	var yes, no []string
	for _, s := range st {
		if s.Stopped {
			yes = append(yes, s.Stratum)
		} else {
			no = append(no, fmt.Sprintf("%s (%s)", s.Stratum, s.Reason))
		}
	}
	sum := fmt.Sprintf("Stopped: %s; not stopped: %s", orNone(strings.Join(yes, ", ")), orNone(strings.Join(no, "; ")))
	return StoppingBlock{Stop: stop, Summary: sum, Strata: st}
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func findingRows(v2 []*record.Record, inst []instance, labels []int) []FindingRow {
	// Routes each completed run reached, by path.
	reached := map[string]map[string]bool{}
	nCompleted := 0
	for _, r := range v2 {
		if r.Outcome != record.OutcomeCompleted {
			continue
		}
		nCompleted++
		m := map[string]bool{}
		for _, s := range r.Steps {
			if s.Route != "" {
				m[pathOf(s.Route)] = true
			}
		}
		reached[r.ID] = m
	}
	type agg struct {
		first     instance
		runs      map[string]bool
		severity  string
		evidence  string
		confirmed bool
		state     bool
	}
	by := map[string]*agg{}
	var order []string
	for _, in := range inst {
		a := by[in.fp]
		if a == nil {
			a = &agg{first: in, runs: map[string]bool{}, severity: in.f.Severity, evidence: in.f.Evidence}
			by[in.fp] = a
			order = append(order, in.fp)
		}
		a.runs[in.run] = true
		if severityRank(in.f.Severity) > severityRank(a.severity) {
			a.severity = in.f.Severity
		}
		if EvidenceRank(in.f.Evidence) > EvidenceRank(a.evidence) {
			a.evidence = in.f.Evidence
		}
		if in.f.Verdict == "confirmed" {
			a.confirmed = true
		}
		if in.f.State != "" {
			a.state = true
		}
	}
	_ = labels
	var rows []FindingRow
	var vecs []FindingVector
	for _, fp := range order {
		a := by[fp]
		n := 0
		for id, m := range reached {
			if m[pathOf(a.first.f.Route)] || a.runs[id] {
				n++
			}
		}
		n = max(n, len(a.runs))
		verdict := "pending"
		if a.confirmed {
			verdict = "confirmed"
		}
		v := FindingVector{Severity: a.severity, Evidence: a.evidence, Verdict: verdict, K: len(a.runs), N: n}
		row := FindingRow{Fingerprint: fp, Route: a.first.f.Route, Locus: a.first.f.Locus, Symptom: a.first.f.Symptom,
			Title: a.first.f.Title, Severity: a.severity, Verdict: verdict, K: v.K, N: n, Evidence: a.evidence,
			Unreplicated: v.K < 2, StateBound: a.state, Display: Display(v)}
		lo, hi := Wilson(v.K, v.N)
		row.Wilson95 = [2]float64{round(lo, 2), round(hi, 2)}
		if b, ok := Band(v); ok {
			row.Band = ptr(b)
		}
		for id := range a.runs {
			row.Runs = append(row.Runs, id)
		}
		sort.Strings(row.Runs)
		rows = append(rows, row)
		vecs = append(vecs, v)
	}
	idx := make([]int, len(rows))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return Less(vecs[idx[a]], vecs[idx[b]]) })
	out := make([]FindingRow, 0, len(rows))
	for _, i := range idx {
		out = append(out, rows[i])
	}
	return out
}

var (
	devices     = []string{"desktop", "tablet", "phone"}
	browsers    = []string{"chrome", "firefox"}
	ats         = []string{"none", "keyboard-only", "screen-reader-view", "magnifier", "switch", "voice"}
	connections = []string{"unthrottled", "fast-4g", "slow-4g", "3g", "offline-flaky"}
	motions     = []string{"no-preference", "reduce"}
)

func reach(v2 []*record.Record) ReachBlock {
	var rb ReachBlock
	// Route coverage against the stated universe.
	universe := map[string]bool{}
	sources := map[string]bool{}
	visited := map[string]bool{}
	for _, r := range v2 {
		for _, u := range r.Target.RouteUniverse {
			universe[pathOf(u.Route)] = true
			sources[u.Source] = true
		}
		for _, s := range r.Steps {
			if s.Route != "" {
				visited[pathOf(s.Route)] = true
			}
		}
	}
	if len(universe) > 0 {
		rc := &RouteCoverage{Universe: len(universe), Missed: []string{}}
		for u := range universe {
			if visited[u] {
				rc.Visited++
			} else {
				rc.Missed = append(rc.Missed, u)
			}
		}
		for v := range visited {
			if !universe[v] {
				rc.Outside++
			}
		}
		sort.Strings(rc.Missed)
		for s := range sources {
			rc.Sources = append(rc.Sources, s)
		}
		sort.Strings(rc.Sources)
		rb.RouteCoverage = rc
	}
	// Persona dimensions.
	vals := map[string]map[string]bool{}
	add := func(dim, v string) {
		if v == "" {
			return
		}
		if vals[dim] == nil {
			vals[dim] = map[string]bool{}
		}
		vals[dim][v] = true
	}
	pairs := map[string]bool{}
	for _, r := range v2 {
		add("device", r.Persona.Device)
		add("browser", r.Browser.Name)
		add("model", r.Provider.Model)
		pairs[r.Persona.Device+" x "+r.Browser.Name] = true
		d := r.Persona.Dimensions
		if d == nil {
			continue
		}
		if d.AT != nil {
			add("at", *d.AT)
		}
		if d.Connection != nil {
			add("connection", *d.Connection)
		}
		if d.Motion != nil {
			add("motion", *d.Motion)
		}
		if d.Locale != nil {
			add("locale", *d.Locale)
		}
		if d.Zoom != nil {
			add("zoom", fmt.Sprintf("%g", *d.Zoom))
		}
		if d.Viewport != nil {
			add("viewport", fmt.Sprintf("%dx%d", d.Viewport.Width, d.Viewport.Height))
		}
	}
	possible := map[string][]string{"device": devices, "browser": browsers, "at": ats, "connection": connections, "motion": motions}
	rb.Dimensions = map[string]DimensionUse{}
	for _, dim := range []string{"device", "browser", "at", "connection", "motion", "locale", "zoom", "viewport", "model"} {
		u := DimensionUse{Values: []string{}}
		for v := range vals[dim] {
			u.Values = append(u.Values, v)
		}
		sort.Strings(u.Values)
		u.Used = len(u.Values)
		if p, ok := possible[dim]; ok {
			u.Possible = ptr(len(p))
		}
		rb.Dimensions[dim] = u
	}
	rb.DevicePairs = &PairCoverage{Used: len(pairs), Possible: len(devices) * len(browsers)}
	return rb
}
