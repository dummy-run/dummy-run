package confidence

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/record"
)

func TestSeverityRule(t *testing.T) {
	cases := []struct {
		name string
		in   SeverityInputs
		want string
	}{
		{"critical is a blocker", SeverityInputs{Impact: "critical"}, SeverityBlocker},
		{"serious is major", SeverityInputs{Impact: "serious"}, SeverityMajor},
		{"moderate is minor", SeverityInputs{Impact: "moderate"}, SeverityMinor},
		{"minor is info", SeverityInputs{Impact: "minor"}, SeverityInfo},
		{"no impact is info", SeverityInputs{}, SeverityInfo},
		{"serious on a journey stays major: only critical makes a blocker", SeverityInputs{Impact: "serious", OnJourney: true}, SeverityMajor},
		{"moderate on a journey is major", SeverityInputs{Impact: "moderate", OnJourney: true}, SeverityMajor},
		{"minor on a journey is minor", SeverityInputs{Impact: "minor", OnJourney: true}, SeverityMinor},
		{"a blocker on a journey stays a blocker", SeverityInputs{Impact: "critical", OnJourney: true}, SeverityBlocker},
		{"moderate on many pages is major", SeverityInputs{Impact: "moderate", Pages: 3}, SeverityMajor},
		{"serious on many pages stays major", SeverityInputs{Impact: "serious", Pages: 12}, SeverityMajor},
		{"moderate on two pages stays minor", SeverityInputs{Impact: "moderate", Pages: 2}, SeverityMinor},
		{"journey and many pages move one step only", SeverityInputs{Impact: "moderate", OnJourney: true, Pages: 9}, SeverityMajor},
		{"a failed task is a blocker whatever the impact", SeverityInputs{Evidence: record.EvidenceTaskVerdict, Impact: "minor"}, SeverityBlocker},
		{"level A floors at minor", SeverityInputs{Impact: "minor", Level: "A"}, SeverityMinor},
		{"level AA has no floor", SeverityInputs{Impact: "minor", Level: "AA"}, SeverityInfo},
		{"level A does not raise a minor", SeverityInputs{Impact: "moderate", Level: "A"}, SeverityMinor},
	}
	for _, c := range cases {
		if got := Severity(c.in); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestEverySymptomHasAnImpact(t *testing.T) {
	for sym, impact := range SymptomImpact {
		if _, ok := impactRank[impact]; !ok || impact == "" {
			t.Errorf("%s: impact %q is not a class", sym, impact)
		}
	}
	if ImpactOfSymptom("missing-information:prices") != "minor" || ImpactOfSymptom("made-up") != "moderate" {
		t.Error("ImpactOfSymptom strips the parameter and defaults to moderate")
	}
}

func TestCandidatesMostSpecificFirstThenSubsetsOfEngines(t *testing.T) {
	in := record.FindingInputs{EvidenceKind: record.EvidenceEngineRule, Engines: []string{"ibm", "axe", "alfa"}, Check: "image-alt",
		Reproduced: &record.Reproduced{Runs: 3, Of: 3}}
	got := Candidates(in)
	var keys []string
	for _, k := range got {
		keys = append(keys, k.String())
	}
	wantFirst := []string{
		"engine-rule|image-alt|alfa+axe+ibm|all", "engine-rule|image-alt|alfa+axe+ibm|*",
		"engine-rule|image-alt|axe+ibm|all", "engine-rule|image-alt|axe+ibm|*",
		"engine-rule|image-alt|alfa+ibm|all", "engine-rule|image-alt|alfa+ibm|*",
		"engine-rule|image-alt|alfa+axe|all", "engine-rule|image-alt|alfa+axe|*",
		"engine-rule|image-alt|ibm|all", "engine-rule|image-alt|ibm|*",
		"engine-rule|image-alt|axe|all", "engine-rule|image-alt|axe|*",
		"engine-rule|image-alt|alfa|all", "engine-rule|image-alt|alfa|*",
		"engine-rule|*|alfa+axe+ibm|all",
	}
	if !slices.Equal(keys[:len(wantFirst)], wantFirst) {
		t.Errorf("candidates:\n%s", strings.Join(keys, "\n"))
	}
	if last := keys[len(keys)-1]; last != "engine-rule|*|alfa|*" {
		t.Errorf("last candidate %s", last)
	}
	if len(keys) != len(slices.Compact(slices.Clone(keys))) {
		t.Error("duplicate candidates")
	}
	plain := Candidates(record.FindingInputs{Check: "unclear-label"})
	if plain[0].String() != "model-only|unclear-label|-|-" || plain[len(plain)-1].String() != "model-only|*|-|*" {
		t.Errorf("model-only candidates: %v", plain)
	}
}

func TestReproClass(t *testing.T) {
	cases := []struct {
		r    *record.Reproduced
		want string
	}{
		{nil, None}, {&record.Reproduced{Runs: 1, Of: 1}, None}, {&record.Reproduced{Runs: 3, Of: 3}, ReproAll},
		{&record.Reproduced{Runs: 2, Of: 3}, ReproSome}, {&record.Reproduced{Runs: 1, Of: 4}, ReproOnce},
	}
	for _, c := range cases {
		if got := ReproClass(c.r); got != c.want {
			t.Errorf("%+v: %s, want %s", c.r, got, c.want)
		}
	}
}

func TestWilson(t *testing.T) {
	low, high := Wilson(0, 0)
	if low != 0 || high != 1 {
		t.Errorf("empty: %v %v", low, high)
	}
	low, high = Wilson(10, 10)
	if math.Abs(low-0.722) > 0.002 || math.Abs(high-1) > 1e-9 {
		t.Errorf("10 of 10: %.3f %.3f", low, high)
	}
	low, _ = Wilson(143, 149)
	if math.Abs(low-0.912) > 0.003 {
		t.Errorf("143 of 149: %.3f", low)
	}
	low, high = Wilson(1, 47)
	if low <= 0 || high >= 0.12 {
		t.Errorf("1 of 47: %.3f %.3f", low, high)
	}
}

func table(buckets ...Bucket) *Table {
	return &Table{MinSamples: DefaultMinSamples, Buckets: buckets}
}

func bucket(key string, correct, total int) Bucket {
	k := parseKey(key)
	low, high := Wilson(correct, total)
	return Bucket{Key: k, Correct: correct, Total: total, Accuracy: float64(correct) / float64(total), Low: low, High: high}
}

func TestRateUsesTheMostSpecificBucketWithEnoughSamples(t *testing.T) {
	tb := table(
		bucket("engine-rule|*|axe|*", 143, 149),
		bucket("engine-rule|avoid-inline-spacing|axe|*", 14, 19),
		bucket("engine-rule|image-alt|axe|*", 5, 5),
	)
	axe := record.FindingInputs{EvidenceKind: record.EvidenceEngineRule, Engines: []string{"axe"}, Check: "image-alt"}
	if r := tb.Rate(axe); r.Band != BandHigh || r.Bucket != "engine-rule|*|axe|*" || r.Samples != 149 || !strings.HasSuffix(r.Reason, "so the band comes from all rule engine findings together (149 checked).") {
		t.Errorf("image-alt with five samples falls back to all axe rules and says so: %+v", r)
	}
	spacing := record.FindingInputs{EvidenceKind: record.EvidenceEngineRule, Engines: []string{"axe"}, Check: "avoid-inline-spacing"}
	if r := tb.Rate(spacing); r.Band != BandMedium || r.Bucket != "engine-rule|avoid-inline-spacing|axe|*" || strings.Contains(r.Reason, "too few") {
		t.Errorf("a check with its own evidence uses it: %+v", r)
	}
	three := record.FindingInputs{EvidenceKind: record.EvidenceEngineRule, Engines: []string{"alfa", "axe", "ibm"}, Check: "label"}
	if r := tb.Rate(three); r.Band != BandHigh || r.Bucket != "engine-rule|*|axe|*" {
		t.Errorf("three engines fall back to the axe subset: %+v", r)
	}
	if !strings.HasPrefix(tb.Rate(three).Reason, "Three engines agree (axe-core, IBM Equal Access, Siteimprove Alfa)") {
		t.Errorf("reason %q", tb.Rate(three).Reason)
	}
	alone := record.FindingInputs{EvidenceKind: record.EvidenceEngineRule, Engines: []string{"ibm"}, Check: "dummyrun-engines-1.3.1"}
	if r := tb.Rate(alone); r.Band != BandUnknown || r.Bucket != "" || !strings.Contains(r.Reason, "has not been measured yet") {
		t.Errorf("an engine with no data on its own is unknown, never high: %+v", r)
	}
	layout := record.FindingInputs{EvidenceKind: record.EvidenceMeasurement, Check: "dummyrun-target-size"}
	if r := tb.Rate(layout); r.Band != BandUnknown || r.Reason != "Measured in the browser. How often this kind of check is right has not been measured yet." {
		t.Errorf("measurement without data: %+v", r)
	}
}

func TestModelOnlyIsNeverAboveLowWithoutData(t *testing.T) {
	tb := table(bucket("engine-rule|*|axe|*", 143, 149))
	in := record.FindingInputs{Check: "unclear-label", OnJourney: true}
	if r := tb.Rate(in); r.Band != BandLow || r.Bucket != "" || r.Reason != "The persona's own report, with no check behind it." {
		t.Errorf("model-only with no data: %+v", r)
	}
	// Data can say otherwise.
	tb = table(bucket("model-only|*|-|*", 48, 50))
	if r := tb.Rate(in); r.Band != BandHigh {
		t.Errorf("model-only with strong data: %+v", r)
	}
	in.Reproduced = &record.Reproduced{Runs: 2, Of: 3}
	if r := tb.Rate(in); !strings.Contains(r.Reason, "and it happened in 2 of 3 runs. This check has too few checked findings of its own, so the band comes from all persona-only findings together (50 checked).") {
		t.Errorf("reason %q", r.Reason)
	}
	in.Reproduced = &record.Reproduced{Runs: 3, Of: 3}
	if r := tb.Rate(in); !strings.Contains(r.Reason, "and it happened in all 3 runs.") {
		t.Errorf("reason %q", r.Reason)
	}
}

func TestSmallBucketsAreUnknown(t *testing.T) {
	b := bucket("measurement|dummyrun-reflow|-|*", 9, 9)
	if b.Band(DefaultMinSamples) != BandUnknown {
		t.Error("nine samples must not give a band")
	}
	if bucket("measurement|dummyrun-reflow|-|*", 10, 10).Band(DefaultMinSamples) != BandMedium {
		t.Error("ten of ten is medium: the interval still reaches below 0.8")
	}
	if bucket("measurement|dummyrun-reflow|-|*", 16, 16).Band(DefaultMinSamples) != BandHigh {
		t.Error("sixteen of sixteen is high")
	}
	if bucket("x|*|*|*", 4, 10).Band(DefaultMinSamples) != BandLow {
		t.Error("four of ten is low")
	}
}

func synthetic() []Labelled {
	var out []Labelled
	add := func(n, correct int, in record.FindingInputs, src string) {
		for i := 0; i < n; i++ {
			out = append(out, Labelled{ID: src, Inputs: in, True: i < correct, Source: src})
		}
	}
	axe := func(check string) record.FindingInputs {
		return record.FindingInputs{EvidenceKind: record.EvidenceEngineRule, Engines: []string{"axe"}, Check: check}
	}
	add(30, 29, axe("image-alt"), "set a")
	add(20, 12, axe("color-contrast"), "set b") //nolint:misspell // an axe rule ID
	add(12, 3, record.FindingInputs{Check: "unclear-label"}, "set c")
	add(4, 4, record.FindingInputs{EvidenceKind: record.EvidenceMeasurement, Check: "dummyrun-reflow"}, "set d")
	return out
}

func TestCalibrateBuildsBucketsAndGeneralisations(t *testing.T) {
	tb := Calibrate(synthetic(), 10)
	b := tb.Find(parseKey("engine-rule|*|axe|-"))
	if b == nil || b.Total != 50 || b.Correct != 41 || b.Source != "set a; set b" {
		t.Fatalf("pooled axe bucket %+v", b)
	}
	if b := tb.Find(parseKey("engine-rule|color-contrast|axe|-")); b == nil || b.Band(10) != BandLow { //nolint:misspell // an axe rule ID
		t.Errorf("contrast at 12 of 20 is low: %+v", b)
	}
	if b := tb.Find(parseKey("engine-rule|image-alt|axe|-")); b == nil || b.Band(10) != BandHigh {
		t.Errorf("image-alt at 29 of 30 is high: %+v", b)
	}
	if b := tb.Find(parseKey("measurement|dummyrun-reflow|-|-")); b == nil || b.Total != 4 || b.Band(10) != BandUnknown {
		t.Errorf("a small bucket is written and unknown: %+v", b)
	}
	if b := tb.Find(parseKey("model-only|*|-|*")); b == nil || b.Total != 12 {
		t.Errorf("model-only generalisation %+v", b)
	}
	data, err := tb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseTable(data)
	if err != nil || len(back.Buckets) != len(tb.Buckets) || back.MinSamples != 10 {
		t.Errorf("round trip: %v, %d buckets", err, len(back.Buckets))
	}
}

func TestEvaluateReportsBandsAndDiscrimination(t *testing.T) {
	labels := synthetic()
	tb := Calibrate(labels, 10)
	c := Evaluate(tb, labels)
	if c.Labels != len(labels) || c.Unknown != 4 {
		t.Errorf("labels %d unknown %d", c.Labels, c.Unknown)
	}
	bands := map[string]BandCheck{}
	for _, b := range c.Bands {
		bands[b.Band] = b
	}
	if bands[BandHigh].Labels != 30 || bands[BandLow].Labels != 32 || bands[BandUnknown].Labels != 4 {
		t.Errorf("bands %+v", c.Bands)
	}
	if !c.Discriminates || c.Separation == nil || math.Abs(*c.Separation-(29.0/30-15.0/32)) > 0.002 {
		t.Errorf("discriminates %v separation %v", c.Discriminates, c.Separation)
	}
	md := c.Markdown()
	for _, want := range []string{"| high | 30 | 29 | 0.967 |", "`engine-rule|image-alt|axe|-` | high | 0.967 | 30 | 29 | 0.967", "The bands discriminate", "4 could not be rated"} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	// A table whose low band is right more often than its high band does not discriminate.
	flipped := table(bucket("engine-rule|*|-|*", 50, 50), bucket("model-only|*|-|*", 10, 50))
	bad := []Labelled{}
	for i := 0; i < 10; i++ {
		bad = append(bad, Labelled{Inputs: record.FindingInputs{EvidenceKind: record.EvidenceEngineRule, Check: "x"}, True: false})
		bad = append(bad, Labelled{Inputs: record.FindingInputs{Check: "y"}, True: true})
	}
	if c := Evaluate(flipped, bad); c.Discriminates || !strings.Contains(c.Markdown(), "do not discriminate") {
		t.Errorf("flipped table: %+v", c)
	}
}

func TestReadLabelsRejectsUnknownEvidence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "l.json")
	if err := os.WriteFile(p, []byte(`[{"inputs":{"evidence_kind":"vibes","engines":[],"check":"x","on_journey":false},"true":true}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLabels(p); err == nil || !strings.Contains(err.Error(), "vibes") {
		t.Errorf("err %v", err)
	}
}

// The shipped table is what dummyrun calibrate writes from the label files beside it, so the two never drift.
func TestShippedTableMatchesLabels(t *testing.T) {
	files, err := filepath.Glob("labels/*.json")
	if err != nil || len(files) == 0 {
		t.Fatal("no label files")
	}
	var labels []Labelled
	for _, f := range files {
		l, err := ReadLabels(f)
		if err != nil {
			t.Fatal(err)
		}
		labels = append(labels, l...)
	}
	want, err := Calibrate(labels, DefaultMinSamples).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(defaultJSON) {
		t.Error("internal/confidence/table.json is out of date. Run: go run ./cmd/dummyrun calibrate --labels internal/confidence/labels/round-0-act.json --labels internal/confidence/labels/round-1-judge.json --out-table internal/confidence/table.json")
	}
}

// This checks the shipped table against the labels it was built from, so it shows the bands are consistent with
// the seed data, not that they hold on new findings. Held-out labels are for dummyrun calibrate --eval.
func TestShippedTableRatesAsPromised(t *testing.T) {
	tb := Default()
	axe := record.FindingInputs{EvidenceKind: record.EvidenceEngineRule, Engines: []string{"axe"}, Check: "image-alt"}
	if r := tb.Rate(axe); r.Band != BandHigh {
		t.Errorf("axe alone on image-alt: %+v", r)
	}
	model := record.FindingInputs{Check: "missing-information", OnJourney: true}
	if r := tb.Rate(model); r.Band != BandLow {
		t.Errorf("model-only: %+v", r)
	}
	for _, kind := range []string{record.EvidenceMeasurement, record.EvidenceAxFact, record.EvidenceTaskVerdict} {
		if r := tb.Rate(record.FindingInputs{EvidenceKind: kind, Check: "dummyrun-x"}); r.Band != BandUnknown {
			t.Errorf("%s has no data yet and must be unknown: %+v", kind, r)
		}
	}
	c := Evaluate(tb, mustLabels(t))
	if !c.Discriminates {
		t.Errorf("the shipped table's bands do not discriminate: %+v", c.Bands)
	}
}

func mustLabels(t *testing.T) []Labelled {
	t.Helper()
	files, _ := filepath.Glob("labels/*.json")
	var out []Labelled
	for _, f := range files {
		l, err := ReadLabels(f)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, l...)
	}
	return out
}
