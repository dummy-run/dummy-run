package coverage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/identity"
	"github.com/dummy-run/dummy-run/internal/record"
)

// item describes one finding for mk.
type item struct {
	name     string // the problem: becomes the locus, so each name is its own fingerprint
	verdict  string
	severity string
	evidence string
	class    string
	route    string
}

func mk(id, outcome, device, commit string, finds ...item) *record.Record {
	r := &record.Record{
		SchemaVersion: 2, ID: id, Batch: "b", StartedAt: time.Now(), EndedAt: time.Now(),
		Persona:  record.Persona{ID: "p-" + device, Role: "visitor", Device: device, Dimensions: &record.Dimensions{}},
		Scenario: record.Scenario{ID: "s", Title: "S", GoalMet: "yes"},
		Target:   record.Target{Adapter: "url", Environment: "e", BaseURL: "https://t.test", Commit: commit},
		Provider: record.Provider{Kind: "anthropic", Model: "m", Region: "global"},
		Browser:  record.Browser{Name: "chrome", Version: "1"}, Outcome: outcome,
		Steps:    []record.Step{{N: 1, Action: "visit", URL: "https://t.test/docs", Route: "/docs"}},
		Findings: []record.Finding{}, Usage: record.Usage{},
	}
	for _, f := range finds {
		if f.verdict == "" {
			f.verdict = "confirmed"
		}
		if f.severity == "" {
			f.severity = "major"
		}
		if f.evidence == "" {
			f.evidence = "measured"
		}
		if f.class == "" {
			f.class = "problem"
		}
		if f.route == "" {
			f.route = "/docs"
		}
		locus := "button:" + f.name
		fp, err := identity.Fingerprint(f.route, locus, "obscured")
		if err != nil {
			panic(err)
		}
		r.Findings = append(r.Findings, record.Finding{ID: "f-" + f.name, Kind: "bug", Severity: f.severity, Title: f.name,
			Steps: []int{1}, Evidence: f.evidence, Verdict: f.verdict, Class: f.class, Route: f.route, Locus: locus,
			Symptom: "obscured", TaxonomyVersion: 1, Fingerprint: fp})
	}
	return r
}

// world builds completed runs from a list of problem sets, one run each.
func world(commit string, sets ...[]string) []*record.Record {
	var out []*record.Record
	for i, set := range sets {
		var fs []item
		for _, n := range set {
			fs = append(fs, item{name: n})
		}
		out = append(out, mk(fmt.Sprintf("2026-10-19-r%02d", i+1), "completed", "desktop", commit, fs...))
	}
	return out
}

func opts() Options { return Options{ConfirmedOnly: true} }

func build(t *testing.T, recs []*record.Record, o Options) *Report {
	t.Helper()
	rep, err := Build(recs, o)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

// The review's recomputed numbers, reached through the whole pipeline: T=7, one problem seen twice and five once.
func TestReportReproducesTheReviewNumbers(t *testing.T) {
	rep := build(t, world("abc1234", []string{"A", "B"}, []string{"A", "C"}, []string{"D"}, []string{"E"}, []string{"F"}, nil, nil), opts())
	op := rep.Completeness.IdentityBand[0]
	if op.Operation != "fingerprint" || op.SObs != 6 || op.Q1 != 5 || op.Q2 != 1 || rep.Completeness.T != 7 {
		t.Fatalf("%+v", op)
	}
	c := op.Chao2
	if c.Form != FormClassic || c.AtLeast != 16.7 || c.CI95[0] != 7.4 || c.CI95[1] != 86.8 {
		t.Errorf("chao2 %+v", c)
	}
	if !strings.HasPrefix(c.Label, "at least 16.7 (Chao2 classic form;") {
		t.Error(c.Label)
	}
	near(t, "coverage", *op.Coverage, 0.330, 0.0006)
	near(t, "new per extra run", *op.NewPerExtraRun, 0.714, 0.0006)
	near(t, "jackknife2", *rep.Completeness.Jackknife2, 6+5*11.0/7-25.0/42, 0.05)
	if rep.Completeness.Label != "6 observed; at least 16.7 (Chao2 classic form, T=7)" {
		t.Error(rep.Completeness.Label)
	}
}

func TestReportBiasCorrectedWhenNoDoubletons(t *testing.T) {
	sets := [][]string{{"A"}, {"B"}, {"C"}, {"D"}, {"E"}, {"F"}, {"G", "H"}}
	rep := build(t, world("abc1234", sets...), opts())
	op := rep.Completeness.IdentityBand[0]
	if op.Q1 != 8 || op.Q2 != 0 || op.Chao2.Form != FormBiasCorrected || op.Chao2.AtLeast != 32.0 {
		t.Errorf("%+v %+v", op, op.Chao2)
	}
	near(t, "coverage", *op.Coverage, 0.045, 0.0006)
}

func TestReportNotEstimable(t *testing.T) {
	rep := build(t, world("abc1234", []string{"A", "B"}, []string{"B"}, []string{"B"}, nil, nil, nil, nil), opts())
	op := rep.Completeness.IdentityBand[0]
	if op.Chao2.Form != FormNotEstimable || op.Coverage != nil || op.CoverageLabel != "not estimable" {
		t.Errorf("%+v %+v", op, op.Chao2)
	}
	if !strings.Contains(rep.Completeness.Label, "not estimable") {
		t.Error(rep.Completeness.Label)
	}
}

func TestOnlyCompletedRunsAreSamplesAndEstimatesNeedConfirmation(t *testing.T) {
	recs := world("abc1234", []string{"A"}, []string{"A"})
	recs = append(recs,
		mk("2026-10-19-b1", "blocked", "desktop", "abc1234", item{name: "Blocked"}),
		mk("2026-10-19-e1", "errored", "desktop", "abc1234", item{name: "Errored"}),
		mk("2026-10-19-x1", "budget_exhausted", "desktop", "abc1234"),
		mk("2026-10-19-c3", "completed", "desktop", "abc1234",
			item{name: "Pending", verdict: "pending"}, item{name: "Rejected", verdict: "rejected"}, item{name: "Note", class: "observation"}))
	rep := build(t, recs, opts())
	r := rep.Runs
	if r.Total != 6 || r.Completed != 3 || r.Blocked != 1 || r.Errored != 1 || r.BudgetExhausted != 1 {
		t.Errorf("%+v", r)
	}
	near(t, "harness failure rate", r.HarnessFailureRate, 1.0/6, 0.001)
	if rep.Completeness.T != 3 || rep.Completeness.SObs != 1 {
		t.Errorf("T=%d SObs=%d", rep.Completeness.T, rep.Completeness.SObs)
	}
	if rv := rep.Review; rv.Confirmed != 4 || rv.Pending != 1 || rv.Rejected != 1 || rv.Observations != 1 {
		t.Errorf("review %+v", rv)
	}
	if !strings.HasPrefix(rep.Basis, "reviewer-confirmed") || !hasWarning(rep, "still pending") {
		t.Errorf("basis %q warnings %v", rep.Basis, rep.Warnings)
	}
	// Demonstration mode counts the pending finding and says so; rejected and observations never count.
	rep = build(t, recs, Options{ConfirmedOnly: false})
	if rep.Completeness.SObs != 2 || !strings.Contains(rep.Basis, "unreviewed") || !hasWarning(rep, "need reviewer-confirmed findings") {
		t.Errorf("SObs %d basis %q warnings %v", rep.Completeness.SObs, rep.Basis, rep.Warnings)
	}
}

func hasWarning(rep *Report, sub string) bool {
	for _, w := range rep.Warnings {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func TestReportRefuses(t *testing.T) {
	refusal := func(recs []*record.Record, o Options, want string) {
		t.Helper()
		_, err := Build(recs, o)
		var ref *Refusal
		if !errors.As(err, &ref) || !strings.Contains(ref.Msg, want) {
			t.Errorf("error %v, want a refusal containing %q", err, want)
		}
	}
	a, b := world("aaaaaaa", []string{"A"}), world("bbbbbbb", []string{"A"})
	b[0].ID = "2026-10-19-other"
	refusal(append(a, b...), opts(), "2 builds")
	refusal(append(a, b...), Options{Build: "abc"}, "at least 7 characters")
	// A build can be chosen, by prefix or by a longer hash.
	for _, sel := range []string{"aaaaaaa", "aaaaaaa1234567"} {
		rep := build(t, append(a, b...), Options{Build: sel, ConfirmedOnly: true})
		if rep.Build != "aaaaaaa" || rep.Runs.Total != 1 {
			t.Errorf("--build %s gave %+v", sel, rep.Runs)
		}
	}
	refusal(append(a, b...), Options{Build: "ccccccc"}, "no schema version 2")
	// Unknown builds are not pooled with known ones.
	unk := world("", []string{"A"})
	unk[0].ID = "2026-10-19-unk"
	refusal(append(world("aaaaaaa", []string{"A"}), unk...), opts(), "unknown")
	refusal([]*record.Record{mk("2026-10-19-b", "blocked", "desktop", "abc1234")}, opts(), "fewer than 1 completed run")
	refusal(nil, opts(), "no schema version 2")
	bad := world("abc1234", []string{"A"})
	bad[0].Findings[0].Fingerprint = "0000000000000000"
	refusal(bad, opts(), "does not recompute")
	bad = world("abc1234", []string{"A"})
	bad[0].Findings[0].Symptom = "looks weird"
	refusal(bad, opts(), "closed taxonomy")
	refusal(world("abc1234", []string{"A"}), Options{Strata: "colour"}, "--strata")
}

func TestVersionOneRecordsAreListedAndLeftOut(t *testing.T) {
	v1 := &record.Record{SchemaVersion: 1, ID: "2026-10-19-old", Outcome: "completed", Persona: record.Persona{ID: "p", Device: "desktop"}}
	rep := build(t, append(world("abc1234", []string{"A"}), v1), opts())
	if rep.Runs.NotFingerprinted != 1 || rep.Runs.Total != 1 || !hasWarning(rep, "not fingerprinted") {
		t.Errorf("%+v %v", rep.Runs, rep.Warnings)
	}
	if _, err := Build([]*record.Record{v1}, opts()); err == nil || !strings.Contains(err.Error(), "version 1") {
		t.Errorf("only v1: %v", err)
	}
}

func TestFindingRowsCarryBandsAndExposure(t *testing.T) {
	recs := world("abc1234", []string{"A"}, []string{"A"}, nil, nil)
	// Two more completed runs that never reached /docs do not count as exposed.
	for _, id := range []string{"x1", "x2"} {
		r := mk("2026-10-19-"+id, "completed", "desktop", "abc1234")
		r.Steps[0].Route = "/other"
		recs = append(recs, r)
	}
	recs = append(recs, mk("2026-10-19-y1", "completed", "desktop", "abc1234", item{name: "Once", severity: "blocker", evidence: "inferred"}))
	rep := build(t, recs, opts())
	byLocus := map[string]FindingRow{}
	for _, f := range rep.Findings {
		byLocus[f.Locus] = f
	}
	a := byLocus["button:A"]
	if a.K != 2 || a.N != 5 || a.Band == nil || *a.Band != "major" || a.Unreplicated {
		t.Errorf("A %+v", a)
	}
	once := byLocus["button:Once"]
	if once.K != 1 || once.Band == nil || *once.Band != "major" || !once.Unreplicated || !strings.Contains(once.Display, "unreplicated") {
		t.Errorf("Once (blocker, inferred, unreplicated: demoted one step) %+v", once)
	}
	if rep.Findings[0].Locus != "button:A" && rep.Findings[0].Locus != "button:Once" {
		t.Error("findings not sorted")
	}
	// Pending findings are listed unscored in demonstration mode.
	recs = append(recs, mk("2026-10-19-z1", "completed", "desktop", "abc1234", item{name: "Maybe", verdict: "pending"}))
	rep = build(t, recs, Options{})
	for _, f := range rep.Findings {
		if f.Locus == "button:Maybe" && (f.Band != nil || !strings.HasPrefix(f.Display, "unscored")) {
			t.Errorf("pending finding banded: %+v", f)
		}
	}
}

func TestStratifiedStoppingFromRecords(t *testing.T) {
	var recs []*record.Record
	for i := 0; i < 6; i++ {
		recs = append(recs, mk(fmt.Sprintf("2026-10-19-d%d", i), "completed", "desktop", "abc1234", item{name: "A"}, item{name: "B"}))
	}
	rep := build(t, recs, opts())
	if !rep.Stopping.Stop || len(rep.Stopping.Strata) != 1 || !strings.HasPrefix(rep.Stopping.Summary, "Stopped: desktop") {
		t.Errorf("%+v", rep.Stopping)
	}
	// A phone stratum with one run keeps the batch from stopping, and the report names both.
	recs = append(recs, mk("2026-10-19-p0", "completed", "phone", "abc1234"))
	rep = build(t, recs, opts())
	if rep.Stopping.Stop || !strings.Contains(rep.Stopping.Summary, "phone (1 completed runs, need 4)") {
		t.Errorf("%+v", rep.Stopping)
	}
	if !hasWarning(rep, "stratum phone has 1 completed runs") {
		t.Errorf("%v", rep.Warnings)
	}
	// Thresholds change the rule.
	th := DefaultThresholds()
	th.MinRuns = 1
	rep = build(t, recs, Options{ConfirmedOnly: true, Thresholds: th})
	if rep.Thresholds.MinRuns != 1 {
		t.Error("thresholds not echoed")
	}
	// Persona strata.
	rep = build(t, recs, Options{ConfirmedOnly: true, Strata: "persona"})
	if len(rep.Stopping.Strata) != 2 || rep.Stopping.Strata[0].Stratum != "p-desktop" {
		t.Errorf("%+v", rep.Stopping.Strata)
	}
}

func TestReachRouteAndDimensions(t *testing.T) {
	recs := world("abc1234", []string{"A"})
	recs[0].Steps = append(recs[0].Steps, record.Step{N: 2, Action: "visit", URL: "https://t.test/x?y=1", Route: "/x?y"},
		record.Step{N: 3, Action: "visit", URL: "https://t.test/elsewhere", Route: "/elsewhere"})
	recs[0].Target.RouteUniverse = []record.RouteUniverseEntry{{Route: "/docs", Source: "sitemap"}, {Route: "/x", Source: "sitemap"},
		{Route: "/never", Source: "sitemap"}, {Route: "/", Source: "scenario"}}
	at, conn := "keyboard-only", "unthrottled"
	recs[0].Persona.Dimensions = &record.Dimensions{AT: &at, Connection: &conn, Viewport: &record.Viewport{Width: 1400, Height: 1000}}
	rep := build(t, recs, opts())
	rc := rep.Reach.RouteCoverage
	if rc == nil || rc.Visited != 2 || rc.Universe != 4 || rc.Outside != 1 || strings.Join(rc.Missed, ",") != "/,/never" || strings.Join(rc.Sources, ",") != "scenario,sitemap" {
		t.Errorf("%+v", rc)
	}
	d := rep.Reach.Dimensions
	if d["at"].Used != 1 || *d["at"].Possible != 6 || d["device"].Used != 1 || *d["device"].Possible != 3 || d["locale"].Possible != nil || d["viewport"].Used != 1 {
		t.Errorf("%+v", d)
	}
	if p := rep.Reach.DevicePairs; p.Used != 1 || p.Possible != 6 {
		t.Errorf("%+v", p)
	}
	// With no stated universe route coverage is not reported at all.
	recs[0].Target.RouteUniverse = nil
	if build(t, recs, opts()).Reach.RouteCoverage != nil {
		t.Error("route coverage reported without a universe")
	}
}

func cf(t *testing.T, name string, clusters ...[]string) *ClusterFile {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"clusters": clusters})
	c, err := ParseClusters(name, data)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestKappaGatesTheEstimates(t *testing.T) {
	recs := world("abc1234", []string{"A", "B"}, []string{"A", "C"}, []string{"D"}, []string{"E"}, nil)
	k := func(run, fid string) string { return "2026-10-19-" + run + "/f-" + fid }
	same := [][]string{{k("r01", "A"), k("r02", "A")}}
	// Two reviewers who agree with each other: estimates are printed.
	rep := build(t, recs, Options{ConfirmedOnly: true, Clusters: []*ClusterFile{cf(t, "a", same...), cf(t, "b", same...)}})
	kb := rep.Review.Kappa
	if kb.Status != KappaPrint || kb.Value == nil || *kb.Value != 1 || kb.Reviewers != 2 {
		t.Errorf("%+v", kb)
	}
	if len(rep.Completeness.IdentityBand) != 3 || rep.Completeness.IdentityBand[1].Operation != "reviewer-clusters-A" || rep.Completeness.IdentityBand[1].Chao2 == nil {
		t.Errorf("%+v", rep.Completeness.IdentityBand)
	}
	// Reviewers who disagree: only S_obs is printed.
	other := [][]string{{k("r01", "B"), k("r03", "D"), k("r04", "E")}, {k("r01", "A"), k("r02", "C")}}
	rep = build(t, recs, Options{ConfirmedOnly: true, Clusters: []*ClusterFile{cf(t, "a", same...), cf(t, "b", other...)}})
	if rep.Review.Kappa.Status != KappaSObs {
		t.Fatalf("%+v", rep.Review.Kappa)
	}
	for _, op := range rep.Completeness.IdentityBand {
		if op.Chao2 != nil || op.SObs == 0 {
			t.Errorf("estimate printed under low kappa: %+v", op)
		}
	}
	if !strings.Contains(rep.Completeness.Label, "withheld") || !hasWarning(rep, "kappa") {
		t.Errorf("%q %v", rep.Completeness.Label, rep.Warnings)
	}
	// One reviewer: kappa against the fingerprints is reported but does not gate.
	rep = build(t, recs, Options{ConfirmedOnly: true, Clusters: []*ClusterFile{cf(t, "a", same...)}})
	if rep.Review.Kappa.Status != KappaSingle || rep.Completeness.IdentityBand[0].Chao2 == nil {
		t.Errorf("%+v", rep.Review.Kappa)
	}
	// A cluster can merge findings with different fingerprints, which lowers S_obs.
	merge := [][]string{{k("r01", "A"), k("r02", "A"), k("r03", "D")}} // a finding the file leaves out is a problem of its own
	rep = build(t, recs, Options{ConfirmedOnly: true, Clusters: []*ClusterFile{cf(t, "a", merge...)}})
	if rep.Completeness.IdentityBand[0].SObs != 5 || rep.Completeness.IdentityBand[1].SObs != 4 {
		t.Errorf("%+v", rep.Completeness.IdentityBand)
	}
}

func TestClusterIDFromRecordsAddsAnIdentityOperation(t *testing.T) {
	recs := world("abc1234", []string{"A"}, []string{"B"}, nil, nil)
	recs[0].Findings[0].ClusterID = "c1"
	recs[1].Findings[0].ClusterID = "c1"
	rep := build(t, recs, opts())
	ops := rep.Completeness.IdentityBand
	if len(ops) != 2 || ops[0].SObs != 2 || ops[1].Operation != "fingerprint+cluster_id" || ops[1].SObs != 1 {
		t.Errorf("%+v", ops)
	}
}

func TestParseClustersRejectsBadFiles(t *testing.T) {
	for _, in := range []string{`{"clusters":[["a"]]}`, `{"clusters":[["a/b"],["a/b"]]}`, `{"nope":1}`, `[]`, ``, `{"clusters":[[1]]}`} {
		if _, err := ParseClusters("f", []byte(in)); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
	if _, err := ParseClusters("f", []byte(`{"clusters":[]}`)); err != nil {
		t.Error(err)
	}
}

func FuzzParseClusters(f *testing.F) {
	for _, s := range []string{`{"clusters":[["a/b","c/d"]]}`, `{}`, `[]`, ``, `{"clusters":[[]]}`, `{"clusters":null}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := ParseClusters("f", data)
		if err != nil {
			return
		}
		seen := map[string]bool{}
		for _, cl := range c.Clusters {
			for _, k := range cl {
				if !strings.Contains(k, "/") || seen[k] {
					t.Fatalf("accepted key %q", k)
				}
				seen[k] = true
			}
		}
		l := c.labels([]string{"x/y", "z/w"})
		if len(l) != 2 {
			t.Fatal(l)
		}
	})
}

func TestReportRendersBothFormats(t *testing.T) {
	rep := build(t, world("abc1234", []string{"A", "B"}, []string{"A"}, []string{"C"}), opts())
	data, err := rep.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"report_version", "build", "runs", "review", "reach", "completeness", "stopping", "findings", "warnings"} {
		if _, ok := back[key]; !ok {
			t.Errorf("JSON lacks %q", key)
		}
	}
	md := rep.Markdown()
	for _, want := range []string{"# Coverage report", "at least", "Chao2", "## Stopping rule", "## Problems", "## Warnings"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	if strings.Contains(strings.ToLower(md), "remain:") || strings.Contains(strings.ToLower(md), "problems remain") {
		t.Error("the report says how many problems remain")
	}
}

// The example records in the repository: one build with three completed runs.
func TestExampleRecords(t *testing.T) {
	recs, err := Load(filepath.Join("..", "..", "examples", "runs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 8 {
		t.Fatalf("%d records", len(recs))
	}
	if _, err := Build(recs, opts()); err == nil || !strings.Contains(err.Error(), "3 builds") {
		t.Errorf("the examples span three builds and must not be pooled: %v", err)
	}
	for _, r := range recs {
		for _, f := range r.Findings {
			want, _ := identity.Fingerprint(f.Route, f.Locus, f.Symptom)
			if f.Fingerprint != want {
				t.Errorf("%s %s: fingerprint %s does not recompute (%s)", r.ID, f.ID, f.Fingerprint, want)
			}
		}
	}
	rep := build(t, recs, Options{Build: "fc2958b", ConfirmedOnly: true})
	if rep.Runs.Total != 4 || rep.Runs.Completed != 3 || rep.Runs.Blocked != 1 || rep.Completeness.SObs != 2 || rep.Completeness.T != 3 {
		t.Errorf("%+v %+v", rep.Runs, rep.Completeness)
	}
	op := rep.Completeness.IdentityBand[0]
	if op.Chao2.Form != FormBiasCorrected || op.Chao2.AtLeast != 2.7 {
		t.Errorf("%+v", op.Chao2)
	}
}

func TestLoadRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Error("an empty directory loaded")
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"schema_version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("error %v does not name the file", err)
	}
	if _, err := Load(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing path loaded")
	}
	ex := filepath.Join("..", "..", "examples", "runs", "2026-10-16-ada-editor-003.json")
	if _, err := Load(ex, ex); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("duplicate run: %v", err)
	}
}
