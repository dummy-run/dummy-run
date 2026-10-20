package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/record"
)

const examples = "../../../../../examples/runs"

func load(t *testing.T, name string) *record.Run {
	t.Helper()
	v, err := record.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	run, err := v.LoadFile(filepath.Join(examples, name))
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func ts(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

// attrsEqual compares attribute sets in order, because the contract lists them in order.
func attrsEqual(t *testing.T, what string, got, want []attribute.KeyValue) {
	t.Helper()
	gotSet := attribute.NewSet(got...)
	wantSet := attribute.NewSet(want...)
	if !gotSet.Equals(&wantSet) {
		t.Errorf("%s attributes:\n got  %v\n want %v", what, gotSet.Encoded(attribute.DefaultEncoder()), wantSet.Encoded(attribute.DefaultEncoder()))
	}
}

func TestBuildBlockedRunWithBugFinding(t *testing.T) {
	run := load(t, "2026-10-16-ada-editor-003.json")
	p := Build(run)

	if p.RunID != "2026-10-16-ada-editor-003" {
		t.Errorf("run id %q", p.RunID)
	}
	if p.TraceID != TraceIDFor(run.ID).String() || len(p.TraceID) != 32 {
		t.Errorf("trace id %q", p.TraceID)
	}

	root := p.Root
	if root.Name != "dummyrun.run" {
		t.Errorf("root name %q", root.Name)
	}
	if !root.Start.Equal(ts(t, "2026-10-16T19:33:44.000Z")) || !root.End.Equal(ts(t, "2026-10-16T19:34:26.535Z")) {
		t.Errorf("root span runs %s to %s", root.Start, root.End)
	}
	if root.Status != codes.Error {
		t.Errorf("root status %v, want ERROR for a blocked run", root.Status)
	}
	attrsEqual(t, "root span", root.Attributes, []attribute.KeyValue{
		attribute.String("dummyrun.run.id", "2026-10-16-ada-editor-003"),
		attribute.String("dummyrun.batch.id", "2026-10-16-pr-41"),
		attribute.String("dummyrun.persona.id", "ada-editor"),
		attribute.String("dummyrun.persona.role", "editor"),
		attribute.String("dummyrun.persona.device", "desktop"),
		attribute.String("dummyrun.persona.at", "none"),
		attribute.String("dummyrun.scenario.id", "reassign-draft"),
		attribute.String("dummyrun.scenario.goal_met", "no"),
		attribute.Int("dummyrun.repeat_index", 0),
		attribute.String("dummyrun.target.adapter", "url"),
		attribute.String("dummyrun.target.environment", "pr-41"),
		attribute.String("dummyrun.target.commit", "a865ffd"),
		attribute.String("url.full", "https://pr-41.inkwell.example"),
		attribute.String("gen_ai.system", "vertex-gemini"),
		attribute.String("gen_ai.request.model", "gemini-3.5-flash-lite"),
		attribute.String("cloud.region", "europe-west2"),
		attribute.String("browser.name", "chrome"),
		attribute.String("dummyrun.outcome", "blocked"),
		attribute.Int("dummyrun.steps.count", 8),
		attribute.Int64("gen_ai.usage.input_tokens", 64570),
		attribute.Int64("gen_ai.usage.output_tokens", 6600),
		attribute.Float64("dummyrun.cost.usd", 0.0359),
	})

	if len(p.Steps) != 8 {
		t.Fatalf("%d step spans, want 8", len(p.Steps))
	}
	s7 := p.Steps[6]
	if s7.Name != "dummyrun.step fill" {
		t.Errorf("step 7 name %q", s7.Name)
	}
	if !s7.Start.Equal(ts(t, "2026-10-16T19:34:16.937Z")) || !s7.End.Equal(ts(t, "2026-10-16T19:34:18.071Z")) {
		t.Errorf("step 7 runs %s to %s, want at + 1134ms", s7.Start, s7.End)
	}
	if s7.Status != codes.Error {
		t.Errorf("step 7 status %v, want ERROR for HTTP 500", s7.Status)
	}
	attrsEqual(t, "step 7", s7.Attributes, []attribute.KeyValue{
		attribute.Int("dummyrun.step.n", 7),
		attribute.String("dummyrun.step.action", "fill"),
		attribute.String("dummyrun.step.target", "Submit"),
		attribute.String("dummyrun.step.route", "/issues"),
		attribute.String("url.full", "https://pr-41.inkwell.example/issues"),
		attribute.Int("http.response.status_code", 500),
		attribute.Int("dummyrun.step.console_errors", 2),
	})
	s3 := p.Steps[2]
	if s3.Status != codes.Unset {
		t.Errorf("step 3 status %v, want UNSET", s3.Status)
	}
	attrsEqual(t, "step 3 (no target)", s3.Attributes, []attribute.KeyValue{
		attribute.Int("dummyrun.step.n", 3),
		attribute.String("dummyrun.step.action", "visit"),
		attribute.String("dummyrun.step.route", "/drafts"),
		attribute.String("url.full", "https://pr-41.inkwell.example/drafts"),
		attribute.Int("http.response.status_code", 200),
		attribute.Int("dummyrun.step.console_errors", 0),
	})
	if s7.SpanID == s3.SpanID || s7.SpanID == root.SpanID {
		t.Error("span ids must differ")
	}

	if len(root.Events) != 1 {
		t.Fatalf("%d finding events, want 1", len(root.Events))
	}
	ev := root.Events[0]
	if ev.Name != "dummyrun.finding" || !ev.Time.Equal(ts(t, "2026-10-16T19:34:16.937Z")) {
		t.Errorf("event %q at %s, want dummyrun.finding at step 7", ev.Name, ev.Time)
	}
	findingAttrs := []attribute.KeyValue{
		attribute.String("dummyrun.finding.id", "schedule-500-on-past-date"),
		attribute.String("dummyrun.finding.kind", "bug"),
		attribute.String("dummyrun.finding.severity", "major"),
		attribute.String("dummyrun.finding.title", "Scheduling an issue for a past date returns a server error"),
		attribute.String("dummyrun.finding.evidence", "measured"),
		attribute.String("dummyrun.finding.verdict", "confirmed"),
		attribute.IntSlice("dummyrun.finding.steps", []int{7}),
		attribute.String("dummyrun.finding.class", "problem"),
		attribute.String("dummyrun.finding.route", "/issues"),
		attribute.String("dummyrun.finding.symptom", "error-shown"),
		attribute.String("dummyrun.finding.fingerprint", "72be5e1e780d79bb"),
	}
	attrsEqual(t, "finding event", ev.Attributes, findingAttrs)

	// Metrics: one run, 8 steps, one finding, two token directions, one cost.
	persona := attribute.String("dummyrun.persona.id", "ada-editor")
	byName := map[string][]Measurement{}
	for _, m := range p.Metrics {
		byName[m.Instrument] = append(byName[m.Instrument], m)
	}
	if n := len(byName[MetricRuns]); n != 1 {
		t.Errorf("%d dummyrun.runs measurements, want 1", n)
	} else {
		m := byName[MetricRuns][0]
		if m.Kind != "counter" || m.Unit != "{run}" || m.Int != 1 {
			t.Errorf("dummyrun.runs = %+v", m)
		}
		attrsEqual(t, "dummyrun.runs", m.Attributes, []attribute.KeyValue{
			persona,
			attribute.String("dummyrun.scenario.id", "reassign-draft"),
			attribute.String("dummyrun.target.environment", "pr-41"),
			attribute.String("dummyrun.outcome", "blocked"),
		})
	}
	if m := byName[MetricRunDuration]; len(m) != 1 || m[0].Kind != "histogram" || m[0].Unit != "s" || m[0].Float != 42.535 {
		t.Errorf("dummyrun.run.duration = %+v, want one histogram record of 42.535 s", m)
	} else {
		attrsEqual(t, "dummyrun.run.duration", m[0].Attributes, []attribute.KeyValue{
			persona,
			attribute.String("dummyrun.scenario.id", "reassign-draft"),
			attribute.String("dummyrun.outcome", "blocked"),
		})
	}
	if n := len(byName[MetricSteps]); n != 8 {
		t.Errorf("%d dummyrun.steps measurements, want 8", n)
	}
	if m := byName[MetricStepDuration]; len(m) != 8 || m[6].Float != 1134 || m[6].Unit != "ms" {
		t.Errorf("dummyrun.step.duration = %+v", m)
	} else {
		attrsEqual(t, "dummyrun.step.duration", m[6].Attributes, []attribute.KeyValue{persona, attribute.String("dummyrun.action", "fill")})
	}
	if m := byName[MetricFindings]; len(m) != 1 || m[0].Int != 1 || m[0].Unit != "{finding}" {
		t.Errorf("dummyrun.findings = %+v", m)
	} else {
		attrsEqual(t, "dummyrun.findings", m[0].Attributes, []attribute.KeyValue{
			persona,
			attribute.String("dummyrun.target.environment", "pr-41"),
			attribute.String("dummyrun.kind", "bug"),
			attribute.String("dummyrun.severity", "major"),
			attribute.String("dummyrun.verdict", "confirmed"),
		})
	}
	tokens := byName[MetricTokens]
	if len(tokens) != 2 || tokens[0].Int != 64570 || tokens[1].Int != 6600 || tokens[0].Unit != "{token}" {
		t.Errorf("dummyrun.tokens = %+v", tokens)
	} else {
		attrsEqual(t, "dummyrun.tokens input", tokens[0].Attributes, []attribute.KeyValue{
			persona,
			attribute.String("gen_ai.system", "vertex-gemini"),
			attribute.String("gen_ai.request.model", "gemini-3.5-flash-lite"),
			attribute.String("dummyrun.direction", "input"),
		})
		attrsEqual(t, "dummyrun.tokens output", tokens[1].Attributes, []attribute.KeyValue{
			persona,
			attribute.String("gen_ai.system", "vertex-gemini"),
			attribute.String("gen_ai.request.model", "gemini-3.5-flash-lite"),
			attribute.String("dummyrun.direction", "output"),
		})
	}
	if m := byName[MetricCost]; len(m) != 1 || !m[0].IsFloat || m[0].Float != 0.0359 || m[0].Unit != "{USD}" || m[0].Instrument != "dummyrun.cost.usd" {
		t.Errorf("dummyrun.cost.usd = %+v", m)
	} else {
		attrsEqual(t, "dummyrun.cost.usd", m[0].Attributes, []attribute.KeyValue{
			persona,
			attribute.String("gen_ai.system", "vertex-gemini"),
			attribute.String("gen_ai.request.model", "gemini-3.5-flash-lite"),
		})
	}

	// Logs: run end then one finding.
	if len(p.Logs) != 2 {
		t.Fatalf("%d log records, want 2", len(p.Logs))
	}
	end := p.Logs[0]
	if end.Severity != log.SeverityInfo {
		t.Errorf("run end severity %v, want INFO (blocked is not errored)", end.Severity)
	}
	if end.Body != "run 2026-10-16-ada-editor-003 blocked: ada-editor / reassign-draft" {
		t.Errorf("run end body %q", end.Body)
	}
	if !end.Time.Equal(root.End) {
		t.Errorf("run end log at %s, want ended_at", end.Time)
	}
	attrsEqual(t, "run end log", end.Attributes, root.Attributes)
	fl := p.Logs[1]
	if fl.Severity != log.SeverityWarn || fl.Body != "Scheduling an issue for a past date returns a server error" || !fl.Time.Equal(ev.Time) {
		t.Errorf("finding log = %+v", fl)
	}
	attrsEqual(t, "finding log", fl.Attributes, append(findingAttrs,
		attribute.String("dummyrun.run.id", "2026-10-16-ada-editor-003"),
		attribute.String("dummyrun.persona.id", "ada-editor"),
		attribute.String("dummyrun.scenario.id", "reassign-draft"),
		attribute.String("dummyrun.target.environment", "pr-41")))

	// Annotation.
	a := p.Annotation
	if !a.Time.Equal(root.Start) || !a.TimeEnd.Equal(root.End) {
		t.Errorf("annotation %s to %s", a.Time, a.TimeEnd)
	}
	wantTags := []string{"dummyrun", "persona:ada-editor", "scenario:reassign-draft", "env:pr-41", "outcome:blocked", "findings"}
	if len(a.Tags) != len(wantTags) {
		t.Fatalf("tags %v, want %v", a.Tags, wantTags)
	}
	for i := range wantTags {
		if a.Tags[i] != wantTags[i] {
			t.Errorf("tag %d = %q, want %q", i, a.Tags[i], wantTags[i])
		}
	}
	if a.Text != "ada-editor / reassign-draft: blocked, 1 confirmed finding" {
		t.Errorf("annotation text %q", a.Text)
	}
}

func TestBuildErroredRunWithPendingFinding(t *testing.T) {
	run := load(t, "2026-10-15-sam-author-001.json")
	p := Build(run)
	if p.Root.Status != codes.Error {
		t.Errorf("root status %v, want ERROR for errored", p.Root.Status)
	}
	if p.Logs[0].Severity != log.SeverityError {
		t.Errorf("run end severity %v, want ERROR for an errored run", p.Logs[0].Severity)
	}
	if p.Logs[1].Severity != log.SeverityInfo {
		t.Errorf("minor finding severity %v, want INFO", p.Logs[1].Severity)
	}
	for _, tag := range p.Annotation.Tags {
		if tag == "findings" {
			t.Error("a run with no confirmed finding must not carry the findings tag")
		}
	}
	if p.Annotation.Text != "sam-author / submit-draft: errored, 0 confirmed findings" {
		t.Errorf("annotation text %q", p.Annotation.Text)
	}
	for _, s := range p.Steps {
		if s.Status != codes.Unset {
			t.Errorf("step %s status %v, want UNSET for HTTP 200", s.Name, s.Status)
		}
	}
}

func TestBuildCompletedRunWithoutFindings(t *testing.T) {
	run := load(t, "2026-10-17-ravi-reviewer-005.json")
	p := Build(run)
	if p.Root.Status != codes.Ok {
		t.Errorf("root status %v, want OK", p.Root.Status)
	}
	if len(p.Root.Events) != 0 || len(p.Logs) != 1 {
		t.Errorf("%d events and %d logs, want 0 and 1", len(p.Root.Events), len(p.Logs))
	}
	for _, m := range p.Metrics {
		if m.Instrument == MetricFindings {
			t.Error("no findings counter without findings")
		}
	}
}

func TestOptionalFieldsAreOmitted(t *testing.T) {
	run := load(t, "2026-10-17-ravi-reviewer-005.json")
	run.Batch = ""
	run.Target.Commit = ""
	run.Usage.CostUSD = nil
	p := Build(run)
	for _, kv := range p.Root.Attributes {
		if kv.Key == "dummyrun.batch.id" || kv.Key == "dummyrun.target.commit" || kv.Key == "dummyrun.cost.usd" {
			t.Errorf("%s must be left out when the record has no value", kv.Key)
		}
	}
	for _, m := range p.Metrics {
		if m.Instrument == MetricCost {
			t.Error("no cost counter without cost_usd")
		}
	}
}

func TestPersonaZoomIsExportedWhenPresent(t *testing.T) {
	zoomed := load(t, "2026-10-16-lin-contributor-002.json")
	if zoomed.Persona.Zoom != 200 {
		t.Fatalf("the example run has zoom %d, want 200", zoomed.Persona.Zoom)
	}
	var got attribute.Value
	for _, kv := range Build(zoomed).Root.Attributes {
		if kv.Key == "dummyrun.persona.zoom" {
			got = kv.Value
		}
	}
	if got.AsInt64() != 200 {
		t.Errorf("dummyrun.persona.zoom = %v, want 200", got.AsInt64())
	}
	for _, kv := range Build(load(t, "2026-10-17-ravi-reviewer-005.json")).Root.Attributes {
		if kv.Key == "dummyrun.persona.zoom" {
			t.Error("a record without zoom exported one")
		}
	}
}

func TestPersonaTextScaleIsExportedWhenPresent(t *testing.T) {
	run := load(t, "2026-10-17-ravi-reviewer-005.json")
	attr := func() int64 {
		for _, kv := range Build(run).Root.Attributes {
			if kv.Key == "dummyrun.persona.text_scale" {
				return kv.Value.AsInt64()
			}
		}
		return 0
	}
	if got := attr(); got != 0 {
		t.Errorf("a record without text_scale exported %d", got)
	}
	run.Persona.TextScale = 200
	if got := attr(); got != 200 {
		t.Errorf("dummyrun.persona.text_scale = %d, want 200", got)
	}
}

// The contract fixes the derivation: trace ID is the first 16 bytes of SHA-256 of
// "dummyrun.run:<id>", root span ID the first 8 bytes of SHA-256 of "dummyrun.span:<id>:root", and
// step span IDs the first 8 bytes of SHA-256 of "dummyrun.span:<id>:step:<n>".
func TestIDsFollowTheContract(t *testing.T) {
	id := "2026-10-16-ada-editor-003"
	traceSum := sha256.Sum256([]byte("dummyrun.run:" + id))
	if got := TraceIDFor(id).String(); got != hex.EncodeToString(traceSum[:16]) {
		t.Errorf("trace id %s, want %s", got, hex.EncodeToString(traceSum[:16]))
	}
	rootSum := sha256.Sum256([]byte("dummyrun.span:" + id + ":root"))
	if got := SpanIDFor(id, "root").String(); got != hex.EncodeToString(rootSum[:8]) {
		t.Errorf("root span id %s, want %s", got, hex.EncodeToString(rootSum[:8]))
	}
	// Step n's span ID is the first 8 bytes of SHA-256 of "dummyrun.span:<id>:step:<n>", so the
	// runner's traceparent for a step names the span the exporter writes.
	stepSum := sha256.Sum256([]byte("dummyrun.span:" + id + ":step:7"))
	p := Build(load(t, id+".json"))
	if p.Steps[6].SpanID != hex.EncodeToString(stepSum[:8]) {
		t.Errorf("step 7 span id %s, want %s", p.Steps[6].SpanID, hex.EncodeToString(stepSum[:8]))
	}
	if got := RootSpanContext(id); got.TraceID() != TraceIDFor(id) || got.SpanID() != SpanIDFor(id, "root") || !got.IsSampled() {
		t.Errorf("root span context %+v", got)
	}
}

func TestIDsAreStable(t *testing.T) {
	a := TraceIDFor("2026-10-17-ravi-reviewer-005")
	b := TraceIDFor("2026-10-17-ravi-reviewer-005")
	c := TraceIDFor("2026-10-17-ravi-reviewer-006")
	if a != b || a == c || !a.IsValid() {
		t.Errorf("trace ids: %s %s %s", a, b, c)
	}
	if SpanIDFor("x", "root") == SpanIDFor("x", "step:1") || !SpanIDFor("x", "root").IsValid() {
		t.Error("span ids must differ by part and be valid")
	}
}

func TestDryRunJSON(t *testing.T) {
	p := Build(load(t, "2026-10-16-ada-editor-003.json"))
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		RunID string `json:"run_id"`
		Root  struct {
			Status     string         `json:"status"`
			Attributes map[string]any `json:"attributes"`
			Events     []struct {
				Attributes map[string]any `json:"attributes"`
			} `json:"events"`
		} `json:"root_span"`
		Metrics []struct {
			Instrument string         `json:"instrument"`
			Value      any            `json:"value"`
			Attributes map[string]any `json:"attributes"`
		} `json:"metrics"`
		Logs []struct {
			Severity string `json:"severity"`
			Body     string `json:"body"`
		} `json:"logs"`
		Annotation struct {
			Tags []string `json:"tags"`
		} `json:"annotation"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Root.Status != "ERROR" || out.Root.Attributes["dummyrun.outcome"] != "blocked" {
		t.Errorf("root json %+v", out.Root)
	}
	if steps, ok := out.Root.Events[0].Attributes["dummyrun.finding.steps"].([]any); !ok || len(steps) != 1 || steps[0] != float64(7) {
		t.Errorf("finding steps json %v", out.Root.Events[0].Attributes["dummyrun.finding.steps"])
	}
	if out.Metrics[0].Instrument != MetricRuns || out.Metrics[0].Value != float64(1) {
		t.Errorf("first metric json %+v", out.Metrics[0])
	}
	if out.Logs[0].Severity != "INFO" || out.Logs[1].Severity != "WARN" {
		t.Errorf("log severities %+v", out.Logs)
	}
	if len(out.Annotation.Tags) != 6 {
		t.Errorf("annotation tags %v", out.Annotation.Tags)
	}
}
