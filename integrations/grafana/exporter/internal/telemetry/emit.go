package telemetry

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// ScopeName is the instrumentation scope for every signal the exporter sends.
const ScopeName = "github.com/dummy-run/dummy-run/integrations/grafana/exporter"

// Emitter sends plans through OpenTelemetry providers.
type Emitter struct {
	tracer       trace.Tracer
	logger       log.Logger
	runs         metric.Int64Counter
	runDuration  metric.Float64Histogram
	steps        metric.Int64Counter
	stepDuration metric.Float64Histogram
	findings     metric.Int64Counter
	tokens       metric.Int64Counter
	cost         metric.Float64Counter
}

// NewEmitter creates the instruments. The tracer provider must use IDGenerator so the trace and
// span IDs match the plan.
func NewEmitter(tp trace.TracerProvider, mp metric.MeterProvider, lp log.LoggerProvider) (*Emitter, error) {
	m := mp.Meter(ScopeName)
	e := &Emitter{tracer: tp.Tracer(ScopeName), logger: lp.Logger(ScopeName)}
	var err error
	if e.runs, err = m.Int64Counter(MetricRuns, metric.WithUnit(UnitRuns), metric.WithDescription("Runs by persona, scenario, environment and outcome")); err != nil {
		return nil, err
	}
	if e.runDuration, err = m.Float64Histogram(MetricRunDuration, metric.WithUnit(UnitSeconds), metric.WithDescription("Run duration from started_at to ended_at")); err != nil {
		return nil, err
	}
	if e.steps, err = m.Int64Counter(MetricSteps, metric.WithUnit(UnitSteps), metric.WithDescription("Steps by persona and action")); err != nil {
		return nil, err
	}
	if e.stepDuration, err = m.Float64Histogram(MetricStepDuration, metric.WithUnit(UnitMillis), metric.WithDescription("Step duration")); err != nil {
		return nil, err
	}
	if e.findings, err = m.Int64Counter(MetricFindings, metric.WithUnit(UnitFindings), metric.WithDescription("Findings by persona, kind, severity and verdict")); err != nil {
		return nil, err
	}
	if e.tokens, err = m.Int64Counter(MetricTokens, metric.WithUnit(UnitTokens), metric.WithDescription("Model tokens by direction")); err != nil {
		return nil, err
	}
	if e.cost, err = m.Float64Counter(MetricCost, metric.WithUnit(UnitUSD), metric.WithDescription("Model cost in US dollars")); err != nil {
		return nil, err
	}
	return e, nil
}

// Emit sends one plan: the trace, the metrics and the logs. Annotations are sent separately
// because they go to Grafana's HTTP API rather than OTLP.
func (e *Emitter) Emit(ctx context.Context, p Plan) error {
	e.EmitTrace(ctx, p)
	if err := e.EmitMetrics(ctx, p); err != nil {
		return err
	}
	e.EmitLogs(ctx, p.RunID, p.Logs)
	return nil
}

// EmitTrace sends the root span and the step spans with the run's own timestamps.
func (e *Emitter) EmitTrace(ctx context.Context, p Plan) {
	ctx = withSpanPart(ctx, p.RunID, "root")
	ctx, root := e.tracer.Start(ctx, p.Root.Name,
		trace.WithTimestamp(p.Root.Start),
		trace.WithAttributes(p.Root.Attributes...))
	for _, ev := range p.Root.Events {
		root.AddEvent(ev.Name, trace.WithTimestamp(ev.Time), trace.WithAttributes(ev.Attributes...))
	}
	root.SetStatus(p.Root.Status, "")

	for _, s := range p.Steps {
		stepCtx := withSpanPart(ctx, p.RunID, fmt.Sprintf("step:%d", stepN(s)))
		_, span := e.tracer.Start(stepCtx, s.Name, trace.WithTimestamp(s.Start), trace.WithAttributes(s.Attributes...))
		span.SetStatus(s.Status, "")
		span.End(trace.WithTimestamp(s.End))
	}
	root.End(trace.WithTimestamp(p.Root.End))
}

// EmitMetrics adds the run's measurements to the counters and histograms.
func (e *Emitter) EmitMetrics(ctx context.Context, p Plan) error {
	for _, m := range p.Metrics {
		if err := e.record(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// EmitLogs sends log records with the trace context of the run's root span. Callers may pass the
// records of many runs sorted by time, because Loki accepts a line only when it is less than an hour
// older than the newest line already in the stream.
func (e *Emitter) EmitLogs(ctx context.Context, runID string, logs []LogRecord) {
	ctx = trace.ContextWithSpanContext(ctx, RootSpanContext(runID))
	now := time.Now()
	for _, l := range logs {
		var rec log.Record
		rec.SetTimestamp(l.Time)
		rec.SetObservedTimestamp(now)
		rec.SetSeverity(l.Severity)
		rec.SetSeverityText(SeverityText(l.Severity))
		rec.SetBody(attribute.StringValue(l.Body))
		rec.AddAttributes(l.Attributes...)
		e.logger.Emit(ctx, rec)
	}
}

// RootSpanContext is the span context of a run's root span, derived from the run ID alone.
func RootSpanContext(runID string) trace.SpanContext {
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    TraceIDFor(runID),
		SpanID:     SpanIDFor(runID, "root"),
		TraceFlags: trace.FlagsSampled,
	})
}

func (e *Emitter) record(ctx context.Context, m Measurement) error {
	opt := metric.WithAttributes(m.Attributes...)
	switch m.Instrument {
	case MetricRuns:
		e.runs.Add(ctx, m.Int, opt)
	case MetricRunDuration:
		e.runDuration.Record(ctx, m.Float, opt)
	case MetricSteps:
		e.steps.Add(ctx, m.Int, opt)
	case MetricStepDuration:
		e.stepDuration.Record(ctx, m.Float, opt)
	case MetricFindings:
		e.findings.Add(ctx, m.Int, opt)
	case MetricTokens:
		e.tokens.Add(ctx, m.Int, opt)
	case MetricCost:
		e.cost.Add(ctx, m.Float, opt)
	default:
		return fmt.Errorf("unknown instrument %q", m.Instrument)
	}
	return nil
}

// stepN reads the step number back from the span's attributes.
func stepN(s Span) int {
	for _, kv := range s.Attributes {
		if kv.Key == "dummyrun.step.n" {
			return int(kv.Value.AsInt64())
		}
	}
	return 0
}

type spanPartKey struct{}

type spanPart struct {
	runID string
	part  string
}

func withSpanPart(ctx context.Context, runID, part string) context.Context {
	return context.WithValue(ctx, spanPartKey{}, spanPart{runID: runID, part: part})
}

// IDGenerator derives trace and span IDs from the run ID in the context, so a run always maps to
// the same trace. Spans started without a run in the context get random IDs.
type IDGenerator struct{}

// NewIDs returns the run's trace ID and the span ID for the context's span part.
func (IDGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	if p, ok := ctx.Value(spanPartKey{}).(spanPart); ok {
		return TraceIDFor(p.runID), SpanIDFor(p.runID, p.part)
	}
	var tid trace.TraceID
	var sid trace.SpanID
	randomBytes(tid[:])
	randomBytes(sid[:])
	return tid, sid
}

// NewSpanID returns the span ID for the context's span part.
func (IDGenerator) NewSpanID(ctx context.Context, _ trace.TraceID) trace.SpanID {
	if p, ok := ctx.Value(spanPartKey{}).(spanPart); ok {
		return SpanIDFor(p.runID, p.part)
	}
	var sid trace.SpanID
	randomBytes(sid[:])
	return sid
}

func randomBytes(b []byte) {
	for {
		_, _ = rand.Read(b)
		for _, x := range b {
			if x != 0 {
				return
			}
		}
	}
}
