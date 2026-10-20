// Package telemetry turns a run record into the signals described in docs/TELEMETRY.md and sends
// them through the OpenTelemetry SDK.
//
// Build is a pure function from a run record to a Plan. The Plan is what --dry-run prints and what
// Emit sends, so the tests check the plan and the wire format separately.
package telemetry

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/record"
)

// Plan is every signal one run produces.
type Plan struct {
	RunID      string        `json:"run_id"`
	TraceID    string        `json:"trace_id"`
	Root       Span          `json:"root_span"`
	Steps      []Span        `json:"step_spans"`
	Metrics    []Measurement `json:"metrics"`
	Logs       []LogRecord   `json:"logs"`
	Annotation Annotation    `json:"annotation"`
}

// Span is one span with its own historical timestamps.
type Span struct {
	Name       string               `json:"name"`
	SpanID     string               `json:"span_id"`
	Start      time.Time            `json:"start"`
	End        time.Time            `json:"end"`
	Status     codes.Code           `json:"-"`
	Attributes []attribute.KeyValue `json:"-"`
	Events     []Event              `json:"events,omitempty"`
}

// Event is a span event. Findings are events on the root span.
type Event struct {
	Name       string               `json:"name"`
	Time       time.Time            `json:"time"`
	Attributes []attribute.KeyValue `json:"-"`
}

// Measurement is one addition to a counter or one histogram record.
type Measurement struct {
	Instrument string               `json:"instrument"`
	Kind       string               `json:"kind"`
	Unit       string               `json:"unit"`
	Int        int64                `json:"-"`
	Float      float64              `json:"-"`
	IsFloat    bool                 `json:"-"`
	Attributes []attribute.KeyValue `json:"-"`
}

// LogRecord is one log line with trace context from the root span.
type LogRecord struct {
	Time       time.Time            `json:"time"`
	Severity   log.Severity         `json:"-"`
	Body       string               `json:"body"`
	Attributes []attribute.KeyValue `json:"-"`
}

// Annotation is the Grafana region annotation for the run.
type Annotation struct {
	Time    time.Time `json:"time"`
	TimeEnd time.Time `json:"time_end"`
	Tags    []string  `json:"tags"`
	Text    string    `json:"text"`
}

// Instrument names and units from docs/TELEMETRY.md.
const (
	MetricRuns         = "dummyrun.runs"
	MetricRunDuration  = "dummyrun.run.duration"
	MetricSteps        = "dummyrun.steps"
	MetricStepDuration = "dummyrun.step.duration"
	MetricFindings     = "dummyrun.findings"
	MetricTokens       = "dummyrun.tokens"
	MetricCost         = "dummyrun.cost.usd"

	UnitRuns     = "{run}"
	UnitSeconds  = "s"
	UnitSteps    = "{step}"
	UnitMillis   = "ms"
	UnitFindings = "{finding}"
	UnitTokens   = "{token}"
	UnitUSD      = "{USD}"
)

// TraceIDFor derives a stable trace ID from a run ID, so exporting a run twice updates one trace.
func TraceIDFor(runID string) trace.TraceID {
	sum := sha256.Sum256([]byte("dummyrun.run:" + runID))
	var id trace.TraceID
	copy(id[:], sum[:16])
	return id
}

// SpanIDFor derives a stable span ID for a part of a run. The root span's part is "root" and a
// step's part is "step:<n>".
func SpanIDFor(runID, part string) trace.SpanID {
	sum := sha256.Sum256([]byte("dummyrun.span:" + runID + ":" + part))
	var id trace.SpanID
	copy(id[:], sum[:8])
	return id
}

// Build maps a run record to its signals. It does no I/O.
func Build(run *record.Run) Plan {
	rootAttrs := rootAttributes(run)
	p := Plan{
		RunID:   run.ID,
		TraceID: TraceIDFor(run.ID).String(),
		Root: Span{
			Name:       "dummyrun.run",
			SpanID:     SpanIDFor(run.ID, "root").String(),
			Start:      run.StartedAt,
			End:        run.EndedAt,
			Status:     runStatus(run.Outcome),
			Attributes: rootAttrs,
		},
	}

	for _, s := range run.Steps {
		p.Steps = append(p.Steps, stepSpan(run.ID, s))
	}

	for _, f := range run.Findings {
		at := run.StartedAt
		if s := run.FirstStep(f); s != nil {
			at = s.At
		}
		p.Root.Events = append(p.Root.Events, Event{
			Name:       "dummyrun.finding",
			Time:       at,
			Attributes: findingAttributes(f),
		})
	}

	p.Metrics = measurements(run)
	p.Logs = logRecords(run, rootAttrs)
	p.Annotation = annotation(run)
	return p
}

func runStatus(outcome string) codes.Code {
	if outcome == "errored" || outcome == "blocked" || outcome == "provider_filtered" {
		return codes.Error
	}
	return codes.Ok
}

func rootAttributes(run *record.Run) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.String("dummyrun.run.id", run.ID)}
	if run.Batch != "" {
		attrs = append(attrs, attribute.String("dummyrun.batch.id", run.Batch))
	}
	attrs = append(attrs,
		attribute.String("dummyrun.persona.id", run.Persona.ID),
		attribute.String("dummyrun.persona.role", run.Persona.Role),
		attribute.String("dummyrun.persona.device", run.Persona.Device),
		attribute.String("dummyrun.scenario.id", run.Scenario.ID),
		attribute.String("dummyrun.target.adapter", run.Target.Adapter),
		attribute.String("dummyrun.target.environment", run.Target.Environment),
	)
	if run.Persona.Zoom != 0 {
		attrs = append(attrs, attribute.Int("dummyrun.persona.zoom", run.Persona.Zoom))
	}
	if run.Persona.Dimensions != nil && run.Persona.Dimensions.AT != nil {
		attrs = append(attrs, attribute.String("dummyrun.persona.at", *run.Persona.Dimensions.AT))
	}
	if run.Scenario.GoalMet != "" {
		attrs = append(attrs, attribute.String("dummyrun.scenario.goal_met", run.Scenario.GoalMet))
	}
	if run.SchemaVersion >= 2 {
		attrs = append(attrs, attribute.Int("dummyrun.repeat_index", run.RepeatIndex))
	}
	if run.Persona.TextScale != 0 {
		attrs = append(attrs, attribute.Int("dummyrun.persona.text_scale", run.Persona.TextScale))
	}
	if run.Target.Commit != "" {
		attrs = append(attrs, attribute.String("dummyrun.target.commit", run.Target.Commit))
	}
	if run.Target.BuildHash != "" {
		attrs = append(attrs, attribute.String("dummyrun.target.build_hash", run.Target.BuildHash))
	}
	attrs = append(attrs,
		attribute.String("url.full", run.Target.BaseURL),
		attribute.String("gen_ai.system", run.Provider.Kind),
		attribute.String("gen_ai.request.model", run.Provider.Model),
		attribute.String("cloud.region", run.Provider.Region),
		attribute.String("browser.name", run.Browser.Name),
		attribute.String("dummyrun.outcome", run.Outcome),
		attribute.Int("dummyrun.steps.count", len(run.Steps)),
		attribute.Int64("gen_ai.usage.input_tokens", run.Usage.InputTokens),
		attribute.Int64("gen_ai.usage.output_tokens", run.Usage.OutputTokens),
	)
	if run.Usage.CostUSD != nil {
		attrs = append(attrs, attribute.Float64("dummyrun.cost.usd", *run.Usage.CostUSD))
	}
	return attrs
}

func stepSpan(runID string, s record.Step) Span {
	attrs := []attribute.KeyValue{
		attribute.Int("dummyrun.step.n", s.N),
		attribute.String("dummyrun.step.action", s.Action),
	}
	if s.Target != nil {
		attrs = append(attrs, attribute.String("dummyrun.step.target", *s.Target))
	}
	if s.URL != nil {
		attrs = append(attrs, attribute.String("url.full", *s.URL))
	}
	if s.Route != nil {
		attrs = append(attrs, attribute.String("dummyrun.step.route", *s.Route))
	}
	if s.Status != nil {
		attrs = append(attrs, attribute.Int("http.response.status_code", *s.Status))
	}
	if s.ConsoleErrors != nil {
		attrs = append(attrs, attribute.Int("dummyrun.step.console_errors", *s.ConsoleErrors))
	}
	if s.A11yViolations != nil {
		attrs = append(attrs, attribute.Int("dummyrun.step.a11y_violations", *s.A11yViolations))
	}
	status := codes.Unset
	if s.Status != nil && *s.Status >= 500 {
		status = codes.Error
	}
	return Span{
		Name:       "dummyrun.step " + s.Action,
		SpanID:     SpanIDFor(runID, fmt.Sprintf("step:%d", s.N)).String(),
		Start:      s.At,
		End:        s.At.Add(time.Duration(s.DurationMS) * time.Millisecond),
		Status:     status,
		Attributes: attrs,
	}
}

func findingAttributes(f record.Finding) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("dummyrun.finding.id", f.ID),
		attribute.String("dummyrun.finding.kind", f.Kind),
		attribute.String("dummyrun.finding.severity", f.Severity),
		attribute.String("dummyrun.finding.title", f.Title),
		attribute.String("dummyrun.finding.evidence", f.Evidence),
		attribute.String("dummyrun.finding.verdict", f.Verdict),
		attribute.IntSlice("dummyrun.finding.steps", f.Steps),
	}
	for _, kv := range []struct{ key, val string }{
		{"dummyrun.finding.class", f.Class}, {"dummyrun.finding.route", f.Route},
		{"dummyrun.finding.symptom", f.Symptom}, {"dummyrun.finding.fingerprint", f.Fingerprint},
	} {
		if kv.val != "" {
			attrs = append(attrs, attribute.String(kv.key, kv.val))
		}
	}
	return attrs
}

func measurements(run *record.Run) []Measurement {
	persona := attribute.String("dummyrun.persona.id", run.Persona.ID)
	scenario := attribute.String("dummyrun.scenario.id", run.Scenario.ID)
	outcome := attribute.String("dummyrun.outcome", run.Outcome)
	environment := attribute.String("dummyrun.target.environment", run.Target.Environment)
	system := attribute.String("gen_ai.system", run.Provider.Kind)
	model := attribute.String("gen_ai.request.model", run.Provider.Model)

	ms := []Measurement{
		counter(MetricRuns, UnitRuns, 1, persona, scenario, environment, outcome),
		histogram(MetricRunDuration, UnitSeconds, run.EndedAt.Sub(run.StartedAt).Seconds(), persona, scenario, outcome),
	}
	for _, s := range run.Steps {
		action := attribute.String("dummyrun.action", s.Action)
		ms = append(ms,
			counter(MetricSteps, UnitSteps, 1, persona, action),
			histogram(MetricStepDuration, UnitMillis, float64(s.DurationMS), persona, action),
		)
	}
	for _, f := range run.Findings {
		ms = append(ms, counter(MetricFindings, UnitFindings, 1, persona, environment,
			attribute.String("dummyrun.kind", f.Kind),
			attribute.String("dummyrun.severity", f.Severity),
			attribute.String("dummyrun.verdict", f.Verdict)))
	}
	ms = append(ms,
		counter(MetricTokens, UnitTokens, run.Usage.InputTokens, persona, system, model, attribute.String("dummyrun.direction", "input")),
		counter(MetricTokens, UnitTokens, run.Usage.OutputTokens, persona, system, model, attribute.String("dummyrun.direction", "output")),
	)
	if run.Usage.CostUSD != nil {
		ms = append(ms, Measurement{Instrument: MetricCost, Kind: "counter", Unit: UnitUSD, Float: *run.Usage.CostUSD, IsFloat: true,
			Attributes: []attribute.KeyValue{persona, system, model}})
	}
	return ms
}

func counter(name, unit string, v int64, attrs ...attribute.KeyValue) Measurement {
	return Measurement{Instrument: name, Kind: "counter", Unit: unit, Int: v, Attributes: attrs}
}

func histogram(name, unit string, v float64, attrs ...attribute.KeyValue) Measurement {
	return Measurement{Instrument: name, Kind: "histogram", Unit: unit, Float: v, IsFloat: true, Attributes: attrs}
}

func logRecords(run *record.Run, rootAttrs []attribute.KeyValue) []LogRecord {
	sev := log.SeverityInfo
	if run.Outcome == "errored" {
		sev = log.SeverityError
	}
	recs := []LogRecord{{
		Time:       run.EndedAt,
		Severity:   sev,
		Body:       fmt.Sprintf("run %s %s: %s / %s", run.ID, run.Outcome, run.Persona.ID, run.Scenario.ID),
		Attributes: rootAttrs,
	}}
	for _, f := range run.Findings {
		at := run.StartedAt
		if s := run.FirstStep(f); s != nil {
			at = s.At
		}
		fsev := log.SeverityInfo
		if f.Severity == "blocker" || f.Severity == "major" {
			fsev = log.SeverityWarn
		}
		attrs := append(findingAttributes(f),
			attribute.String("dummyrun.run.id", run.ID),
			attribute.String("dummyrun.persona.id", run.Persona.ID),
			attribute.String("dummyrun.scenario.id", run.Scenario.ID),
			attribute.String("dummyrun.target.environment", run.Target.Environment))
		recs = append(recs, LogRecord{Time: at, Severity: fsev, Body: f.Title, Attributes: attrs})
	}
	return recs
}

func annotation(run *record.Run) Annotation {
	tags := []string{
		"dummyrun",
		"persona:" + run.Persona.ID,
		"scenario:" + run.Scenario.ID,
		"env:" + run.Target.Environment,
		"outcome:" + run.Outcome,
	}
	confirmed := run.ConfirmedFindings()
	if confirmed > 0 {
		tags = append(tags, "findings")
	}
	noun := "confirmed findings"
	if confirmed == 1 {
		noun = "confirmed finding"
	}
	return Annotation{
		Time:    run.StartedAt,
		TimeEnd: run.EndedAt,
		Tags:    tags,
		Text:    fmt.Sprintf("%s / %s: %s, %d %s", run.Persona.ID, run.Scenario.ID, run.Outcome, confirmed, noun),
	}
}

// SeverityText is the OpenTelemetry short name for a severity.
func SeverityText(s log.Severity) string {
	switch {
	case s >= log.SeverityError:
		return "ERROR"
	case s >= log.SeverityWarn:
		return "WARN"
	default:
		return "INFO"
	}
}

// JSON rendering for --dry-run. Attributes become an object keyed by attribute name.

type spanJSON struct {
	Name       string         `json:"name"`
	SpanID     string         `json:"span_id"`
	Start      time.Time      `json:"start"`
	End        time.Time      `json:"end"`
	Status     string         `json:"status"`
	Attributes map[string]any `json:"attributes"`
	Events     []Event        `json:"events,omitempty"`
}

// MarshalJSON renders the span with its attributes and status as plain JSON values.
func (s Span) MarshalJSON() ([]byte, error) {
	return json.Marshal(spanJSON{
		Name: s.Name, SpanID: s.SpanID, Start: s.Start, End: s.End,
		Status: strings.ToUpper(s.Status.String()), Attributes: attrsJSON(s.Attributes), Events: s.Events,
	})
}

type eventJSON struct {
	Name       string         `json:"name"`
	Time       time.Time      `json:"time"`
	Attributes map[string]any `json:"attributes"`
}

// MarshalJSON renders the event with its attributes as plain JSON values.
func (e Event) MarshalJSON() ([]byte, error) {
	return json.Marshal(eventJSON{Name: e.Name, Time: e.Time, Attributes: attrsJSON(e.Attributes)})
}

type measurementJSON struct {
	Instrument string         `json:"instrument"`
	Kind       string         `json:"kind"`
	Unit       string         `json:"unit"`
	Value      any            `json:"value"`
	Attributes map[string]any `json:"attributes"`
}

// MarshalJSON renders the measurement with its value and attributes as plain JSON values.
func (m Measurement) MarshalJSON() ([]byte, error) {
	var v any = m.Int
	if m.IsFloat {
		v = m.Float
	}
	return json.Marshal(measurementJSON{Instrument: m.Instrument, Kind: m.Kind, Unit: m.Unit, Value: v, Attributes: attrsJSON(m.Attributes)})
}

type logJSON struct {
	Time       time.Time      `json:"time"`
	Severity   string         `json:"severity"`
	Body       string         `json:"body"`
	Attributes map[string]any `json:"attributes"`
}

// MarshalJSON renders the log record with its severity text and attributes as plain JSON values.
func (l LogRecord) MarshalJSON() ([]byte, error) {
	return json.Marshal(logJSON{Time: l.Time, Severity: SeverityText(l.Severity), Body: l.Body, Attributes: attrsJSON(l.Attributes)})
}

func attrsJSON(attrs []attribute.KeyValue) map[string]any {
	m := make(map[string]any, len(attrs))
	for _, kv := range attrs {
		m[string(kv.Key)] = kv.Value.AsInterface()
	}
	return m
}
