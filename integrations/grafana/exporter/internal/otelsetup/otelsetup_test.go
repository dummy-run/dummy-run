package otelsetup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/record"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/telemetry"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/testexporters"
)

func TestParsers(t *testing.T) {
	for in, want := range map[string]Protocol{"": ProtocolHTTP, "http/protobuf": ProtocolHTTP, "HTTP": ProtocolHTTP, "grpc": ProtocolGRPC} {
		got, err := ParseProtocol(in)
		if err != nil || got != want {
			t.Errorf("ParseProtocol(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseProtocol("http/json"); err == nil {
		t.Error("http/json is not supported and must be rejected")
	}
	for in, want := range map[string]Temporality{"": TemporalityCumulative, "cumulative": TemporalityCumulative, "Delta": TemporalityDelta} {
		got, err := ParseTemporality(in)
		if err != nil || got != want {
			t.Errorf("ParseTemporality(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseTemporality("sometimes"); err == nil {
		t.Error("bad temporality must be rejected")
	}
}

func TestResourceFollowsTheContractAndTheEnvironment(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.namespace=inkwell,deployment.environment.name=preview")
	t.Setenv("OTEL_SERVICE_NAME", "")
	res, err := Resource(context.Background(), "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, kv := range res.Attributes() {
		got[string(kv.Key)] = kv.Value.String()
	}
	for k, want := range map[string]string{
		"service.name":                "dummyrun",
		"service.version":             "v1.2.3",
		"service.namespace":           "inkwell",
		"deployment.environment.name": "preview",
	} {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
	host, _ := os.Hostname()
	if want := fmt.Sprintf("%s-%d-", host, os.Getpid()); !strings.HasPrefix(got["service.instance.id"], want) {
		t.Errorf("service.instance.id = %q, want a prefix of %q", got["service.instance.id"], want)
	}
	if got["service.instance.id"] != InstanceID() {
		t.Error("service.instance.id must be stable within the process")
	}
}

func TestDefaultEndpointsArePlainHTTPOnLocalhost(t *testing.T) {
	none := func(string) string { return "" }
	want := map[Signal]string{
		SignalTraces:  "http://localhost:4318/v1/traces",
		SignalMetrics: "http://localhost:4318/v1/metrics",
		SignalLogs:    "http://localhost:4318/v1/logs",
	}
	for sig, u := range want {
		if got := DefaultEndpoint(none, ProtocolHTTP, sig); got != u {
			t.Errorf("%s over HTTP: %q, want %q", sig, got, u)
		}
		if got := DefaultEndpoint(none, ProtocolGRPC, sig); got != "http://localhost:4317" {
			t.Errorf("%s over gRPC: %q, want http://localhost:4317", sig, got)
		}
	}
	generic := func(k string) string {
		if k == "OTEL_EXPORTER_OTLP_ENDPOINT" {
			return "https://otlp.example.net"
		}
		return ""
	}
	for _, sig := range []Signal{SignalTraces, SignalMetrics, SignalLogs} {
		if got := DefaultEndpoint(generic, ProtocolHTTP, sig); got != "" {
			t.Errorf("%s with OTEL_EXPORTER_OTLP_ENDPOINT set: %q, want the SDK's own parsing", sig, got)
		}
	}
	logsOnly := func(k string) string {
		if k == "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT" {
			return "http://loki:3100/otlp/v1/logs"
		}
		return ""
	}
	if got := DefaultEndpoint(logsOnly, ProtocolHTTP, SignalLogs); got != "" {
		t.Errorf("logs with their own endpoint: %q, want the SDK's own parsing", got)
	}
	if got := DefaultEndpoint(logsOnly, ProtocolHTTP, SignalTraces); got != "http://localhost:4318/v1/traces" {
		t.Errorf("traces with only the logs endpoint set: %q", got)
	}
}

func TestViewsSetHistogramBuckets(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithView(Views()...))
	defer func() { _ = mp.Shutdown(context.Background()) }()
	lp := sdklog.NewLoggerProvider()
	tp := sdktrace.NewTracerProvider()
	em, err := telemetry.NewEmitter(tp, mp, lp)
	if err != nil {
		t.Fatal(err)
	}
	v, err := record.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	run, err := v.LoadFile("../../../../../examples/runs/2026-10-16-ada-editor-003.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := em.Emit(context.Background(), telemetry.Build(run)); err != nil {
		t.Fatal(err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	bounds := map[string][]float64{}
	for _, m := range rm.ScopeMetrics[0].Metrics {
		if h, ok := m.Data.(metricdata.Histogram[float64]); ok {
			bounds[m.Name] = h.DataPoints[0].Bounds
		}
	}
	if b := bounds[telemetry.MetricRunDuration]; len(b) != 10 || b[0] != 15 || b[9] != 3600 {
		t.Errorf("run duration bounds %v", b)
	}
	if b := bounds[telemetry.MetricStepDuration]; len(b) != 9 || b[0] != 100 || b[8] != 60000 {
		t.Errorf("step duration bounds %v", b)
	}
}

type failingSpans struct{ *tracetest.NoopExporter }

func (failingSpans) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return errors.New("tempo said no")
}

func TestShutdownReportsExportErrors(t *testing.T) {
	var stderr bytes.Buffer
	sink := &errorSink{w: &stderr}
	otel.SetErrorHandler(sink)
	res, err := Resource(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	p := Assemble(res, &spanExporter{SpanExporter: failingSpans{tracetest.NewNoopExporter()}, sink: sink}, &testexporters.Metrics{}, &testexporters.Logs{}, sink)
	_, span := p.Tracer.Tracer("t").Start(context.Background(), "x")
	span.End()
	err = p.Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "tempo said no") || !strings.Contains(err.Error(), "traces") {
		t.Fatalf("Shutdown error = %v, want the span export failure", err)
	}
	if res.Set().HasValue(semconv.ServiceNameKey) == false {
		t.Error("resource lost service.name")
	}
	if !strings.Contains(stderr.String(), "tempo said no") {
		t.Errorf("SDK error was not written to the configured stderr: %q", stderr.String())
	}
}

func TestAssembleUsesStableIDs(t *testing.T) {
	spans := tracetest.NewInMemoryExporter()
	res, err := Resource(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	p := Assemble(res, spans, &testexporters.Metrics{}, &testexporters.Logs{}, nil)
	em, err := telemetry.NewEmitter(p.Tracer, p.Meter, p.Logger)
	if err != nil {
		t.Fatal(err)
	}
	v, err := record.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	run, err := v.LoadFile("../../../../../examples/runs/2026-10-17-ravi-reviewer-005.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := em.Emit(context.Background(), telemetry.Build(run)); err != nil {
		t.Fatal(err)
	}
	if err := p.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := spans.GetSpans()
	defer func() { _ = p.Shutdown(context.Background()) }()
	if len(got) != 14 {
		t.Fatalf("%d spans, want 14", len(got))
	}
	for _, s := range got {
		if s.SpanContext.TraceID() != telemetry.TraceIDFor(run.ID) {
			t.Errorf("span %s has trace id %s", s.Name, s.SpanContext.TraceID())
		}
	}
}
