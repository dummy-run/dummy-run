package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/testexporters"
)

type harness struct {
	spans   *tracetest.InMemoryExporter
	metrics *testexporters.Metrics
	reader  *sdkmetric.ManualReader
	logs    *testexporters.Logs
	tp      *sdktrace.TracerProvider
	mp      *sdkmetric.MeterProvider
	lp      *sdklog.LoggerProvider
	emitter *Emitter
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		spans:   tracetest.NewInMemoryExporter(),
		metrics: &testexporters.Metrics{},
		reader:  sdkmetric.NewManualReader(),
		logs:    &testexporters.Logs{},
	}
	res := resource.NewSchemaless(semconv.ServiceName("dummyrun"))
	h.tp = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithIDGenerator(IDGenerator{}),
		sdktrace.WithSyncer(h.spans),
	)
	h.mp = sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(h.reader))
	h.lp = sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewSimpleProcessor(h.logs)))
	var err error
	h.emitter, err = NewEmitter(h.tp, h.mp, h.lp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_ = h.tp.Shutdown(ctx)
		_ = h.mp.Shutdown(ctx)
		_ = h.lp.Shutdown(ctx)
	})
	return h
}

func TestEmitTrace(t *testing.T) {
	h := newHarness(t)
	run := load(t, "2026-10-16-ada-editor-003.json")
	p := Build(run)
	if err := h.emitter.Emit(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	spans := h.spans.GetSpans()
	if len(spans) != 9 {
		t.Fatalf("%d spans, want 1 root + 8 steps", len(spans))
	}
	var root *tracetest.SpanStub
	steps := 0
	for i := range spans {
		s := &spans[i]
		if s.SpanContext.TraceID() != TraceIDFor(run.ID) {
			t.Errorf("span %s has trace id %s, want %s", s.Name, s.SpanContext.TraceID(), TraceIDFor(run.ID))
		}
		if s.Name == "dummyrun.run" {
			root = s
			continue
		}
		steps++
		if s.Parent.SpanID() != SpanIDFor(run.ID, "root") {
			t.Errorf("step span %s has parent %s, want the root span", s.Name, s.Parent.SpanID())
		}
	}
	if root == nil {
		t.Fatal("no root span")
	}
	if root.SpanContext.SpanID() != SpanIDFor(run.ID, "root") {
		t.Errorf("root span id %s", root.SpanContext.SpanID())
	}
	if !root.StartTime.Equal(run.StartedAt) || !root.EndTime.Equal(run.EndedAt) {
		t.Errorf("root span %s to %s, want the run's own timestamps %s to %s", root.StartTime, root.EndTime, run.StartedAt, run.EndedAt)
	}
	if root.Status.Code != codes.Error {
		t.Errorf("root status %v", root.Status.Code)
	}
	attrsEqual(t, "exported root span", root.Attributes, p.Root.Attributes)
	if len(root.Events) != 1 || root.Events[0].Name != "dummyrun.finding" || !root.Events[0].Time.Equal(p.Root.Events[0].Time) {
		t.Fatalf("root events %+v", root.Events)
	}
	attrsEqual(t, "exported finding event", root.Events[0].Attributes, p.Root.Events[0].Attributes)
	if root.ChildSpanCount != 8 || steps != 8 {
		t.Errorf("child spans %d, step spans %d", root.ChildSpanCount, steps)
	}
	for i := range spans {
		s := &spans[i]
		if s.Name == "dummyrun.step fill" && s.StartTime.Equal(p.Steps[6].Start) {
			if s.SpanContext.SpanID() != SpanIDFor(run.ID, "step:7") {
				t.Errorf("step 7 span id %s", s.SpanContext.SpanID())
			}
			if !s.EndTime.Equal(p.Steps[6].End) || s.Status.Code != codes.Error {
				t.Errorf("step 7 ends %s with status %v", s.EndTime, s.Status.Code)
			}
			attrsEqual(t, "exported step 7", s.Attributes, p.Steps[6].Attributes)
		}
	}
}

func TestEmitTwiceGivesTheSameIDs(t *testing.T) {
	h := newHarness(t)
	p := Build(load(t, "2026-10-17-ravi-reviewer-005.json"))
	for i := 0; i < 2; i++ {
		if err := h.emitter.Emit(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	ids := map[string]int{}
	for _, s := range h.spans.GetSpans() {
		ids[s.SpanContext.TraceID().String()+"/"+s.SpanContext.SpanID().String()]++
	}
	for id, n := range ids {
		if n != 2 {
			t.Errorf("span %s seen %d times, want 2 (ids must be stable across exports)", id, n)
		}
	}
}

func TestEmitMetrics(t *testing.T) {
	h := newHarness(t)
	run := load(t, "2026-10-16-ada-editor-003.json")
	if err := h.emitter.Emit(context.Background(), Build(run)); err != nil {
		t.Fatal(err)
	}
	var rm metricdata.ResourceMetrics
	if err := h.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	if len(rm.ScopeMetrics) != 1 || rm.ScopeMetrics[0].Scope.Name != ScopeName {
		t.Fatalf("scope metrics %+v", rm.ScopeMetrics)
	}
	byName := map[string]metricdata.Metrics{}
	for _, m := range rm.ScopeMetrics[0].Metrics {
		byName[m.Name] = m
	}
	want := map[string]string{
		MetricRuns: "{run}", MetricRunDuration: "s", MetricSteps: "{step}", MetricStepDuration: "ms",
		MetricFindings: "{finding}", MetricTokens: "{token}", MetricCost: "{USD}",
	}
	for name, unit := range want {
		m, ok := byName[name]
		if !ok {
			t.Errorf("metric %s missing", name)
			continue
		}
		if m.Unit != unit {
			t.Errorf("%s unit %q, want %q", name, m.Unit, unit)
		}
	}

	runs, ok := byName[MetricRuns].Data.(metricdata.Sum[int64])
	if !ok || !runs.IsMonotonic || runs.Temporality != metricdata.CumulativeTemporality || len(runs.DataPoints) != 1 {
		t.Fatalf("dummyrun.runs data %+v", byName[MetricRuns].Data)
	}
	if runs.DataPoints[0].Value != 1 {
		t.Errorf("dummyrun.runs = %d", runs.DataPoints[0].Value)
	}
	wantRunAttrs := attribute.NewSet(
		attribute.String("dummyrun.persona.id", "ada-editor"),
		attribute.String("dummyrun.scenario.id", "reassign-draft"),
		attribute.String("dummyrun.target.environment", "pr-41"),
		attribute.String("dummyrun.outcome", "blocked"),
	)
	if !runs.DataPoints[0].Attributes.Equals(&wantRunAttrs) {
		t.Errorf("dummyrun.runs attributes %s", runs.DataPoints[0].Attributes.Encoded(attribute.DefaultEncoder()))
	}
	if !runs.DataPoints[0].Time.After(run.EndedAt) {
		t.Error("metric points carry the export time, which must be after the run ended")
	}

	dur, ok := byName[MetricRunDuration].Data.(metricdata.Histogram[float64])
	if !ok || len(dur.DataPoints) != 1 || dur.DataPoints[0].Count != 1 || dur.DataPoints[0].Sum != 42.535 {
		t.Errorf("dummyrun.run.duration %+v", byName[MetricRunDuration].Data)
	}

	steps, ok := byName[MetricSteps].Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatal("dummyrun.steps is not an int sum")
	}
	total := int64(0)
	for _, dp := range steps.DataPoints {
		total += dp.Value
		if _, has := dp.Attributes.Value("dummyrun.action"); !has {
			t.Errorf("dummyrun.steps point without dummyrun.action: %s", dp.Attributes.Encoded(attribute.DefaultEncoder()))
		}
	}
	if total != 8 {
		t.Errorf("dummyrun.steps total %d, want 8", total)
	}

	tokens, ok := byName[MetricTokens].Data.(metricdata.Sum[int64])
	if !ok || len(tokens.DataPoints) != 2 {
		t.Fatalf("dummyrun.tokens %+v", byName[MetricTokens].Data)
	}
	for _, dp := range tokens.DataPoints {
		dir, _ := dp.Attributes.Value("dummyrun.direction")
		switch dir.AsString() {
		case "input":
			if dp.Value != 64570 {
				t.Errorf("input tokens %d", dp.Value)
			}
		case "output":
			if dp.Value != 6600 {
				t.Errorf("output tokens %d", dp.Value)
			}
		default:
			t.Errorf("unexpected direction %q", dir.AsString())
		}
	}

	cost, ok := byName[MetricCost].Data.(metricdata.Sum[float64])
	if !ok || len(cost.DataPoints) != 1 || cost.DataPoints[0].Value != 0.0359 {
		t.Errorf("dummyrun.cost %+v", byName[MetricCost].Data)
	}
}

func TestEmitLogsCarryTraceContext(t *testing.T) {
	h := newHarness(t)
	run := load(t, "2026-10-16-ada-editor-003.json")
	p := Build(run)
	if err := h.emitter.Emit(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	recs := h.logs.Records()
	if len(recs) != 2 {
		t.Fatalf("%d log records, want 2", len(recs))
	}
	for i, r := range recs {
		if r.TraceID() != TraceIDFor(run.ID) || r.SpanID() != SpanIDFor(run.ID, "root") {
			t.Errorf("record %d trace %s span %s, want the root span's", i, r.TraceID(), r.SpanID())
		}
		if !r.Timestamp().Equal(p.Logs[i].Time) {
			t.Errorf("record %d at %s, want %s", i, r.Timestamp(), p.Logs[i].Time)
		}
		if r.ObservedTimestamp().IsZero() {
			t.Errorf("record %d has no observed timestamp", i)
		}
		if r.Body().AsString() != p.Logs[i].Body {
			t.Errorf("record %d body %q", i, r.Body().AsString())
		}
		var attrs []attribute.KeyValue
		r.WalkAttributes(func(kv attribute.KeyValue) bool {
			attrs = append(attrs, kv)
			return true
		})
		attrsEqual(t, "log record", attrs, p.Logs[i].Attributes)
	}
	if recs[0].Severity() != log.SeverityInfo || recs[0].SeverityText() != "INFO" {
		t.Errorf("run end severity %v %q", recs[0].Severity(), recs[0].SeverityText())
	}
	if recs[1].Severity() != log.SeverityWarn || recs[1].SeverityText() != "WARN" {
		t.Errorf("finding severity %v %q", recs[1].Severity(), recs[1].SeverityText())
	}
}
