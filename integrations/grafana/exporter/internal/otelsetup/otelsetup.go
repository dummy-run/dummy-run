// Package otelsetup builds the OpenTelemetry providers from the standard OTEL_* environment
// variables and a few flags, and captures export errors so the command can exit non-zero.
package otelsetup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/telemetry"
)

// Protocol is the OTLP transport.
type Protocol string

// The two supported transports. OTEL_EXPORTER_OTLP_PROTOCOL selects between them.
const (
	ProtocolHTTP Protocol = "http/protobuf"
	ProtocolGRPC Protocol = "grpc"
)

// ParseProtocol accepts the OTEL_EXPORTER_OTLP_PROTOCOL spellings.
func ParseProtocol(s string) (Protocol, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "http/protobuf", "http":
		return ProtocolHTTP, nil
	case "grpc":
		return ProtocolGRPC, nil
	default:
		return "", fmt.Errorf("unknown OTLP protocol %q (use http/protobuf or grpc)", s)
	}
}

// Temporality is how counters and histograms report over time.
type Temporality string

// Cumulative is what Prometheus and Mimir accept by default. Delta needs a backend that converts
// deltas, such as Prometheus with --enable-feature=otlp-deltatocumulative.
const (
	TemporalityCumulative Temporality = "cumulative"
	TemporalityDelta      Temporality = "delta"
)

// ParseTemporality accepts cumulative or delta.
func ParseTemporality(s string) (Temporality, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "cumulative":
		return TemporalityCumulative, nil
	case "delta":
		return TemporalityDelta, nil
	default:
		return "", fmt.Errorf("unknown metrics temporality %q (use cumulative or delta)", s)
	}
}

// Config selects transport and temporality. Endpoints, headers and TLS come from the OTEL_*
// environment variables that the exporters read themselves.
type Config struct {
	Protocol    Protocol
	Temporality Temporality
	// Version becomes service.version.
	Version string
	// Stderr receives the SDK's error reports. nil means os.Stderr.
	Stderr io.Writer
}

// Providers holds the three SDK providers and the error sink.
type Providers struct {
	Tracer *sdktrace.TracerProvider
	Meter  *sdkmetric.MeterProvider
	Logger *sdklog.LoggerProvider
	errs   *errorSink
}

// New builds the providers. Spans and logs are batched; metrics use a periodic reader whose
// interval OTEL_METRIC_EXPORT_INTERVAL controls.
func New(ctx context.Context, cfg Config) (*Providers, error) {
	res, err := Resource(ctx, cfg.Version)
	if err != nil {
		return nil, err
	}
	sink := &errorSink{w: cfg.Stderr}
	if sink.w == nil {
		sink.w = os.Stderr
	}
	otel.SetErrorHandler(sink)

	var (
		traceExp  sdktrace.SpanExporter
		metricExp sdkmetric.Exporter
		logExp    sdklog.Exporter
	)
	tempo := temporalitySelector(cfg.Temporality)
	switch cfg.Protocol {
	case ProtocolGRPC:
		if traceExp, err = otlptracegrpc.New(ctx); err != nil {
			return nil, fmt.Errorf("trace exporter: %w", err)
		}
		if metricExp, err = otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithTemporalitySelector(tempo)); err != nil {
			return nil, fmt.Errorf("metric exporter: %w", err)
		}
		if logExp, err = otlploggrpc.New(ctx); err != nil {
			return nil, fmt.Errorf("log exporter: %w", err)
		}
	default:
		if traceExp, err = otlptracehttp.New(ctx); err != nil {
			return nil, fmt.Errorf("trace exporter: %w", err)
		}
		if metricExp, err = otlpmetrichttp.New(ctx, otlpmetrichttp.WithTemporalitySelector(tempo)); err != nil {
			return nil, fmt.Errorf("metric exporter: %w", err)
		}
		if logExp, err = otlploghttp.New(ctx); err != nil {
			return nil, fmt.Errorf("log exporter: %w", err)
		}
	}
	return Assemble(res, &spanExporter{SpanExporter: traceExp, sink: sink}, &metricExporter{Exporter: metricExp, sink: sink}, &logExporter{Exporter: logExp, sink: sink}, sink), nil
}

// Assemble wires exporters into providers. Tests call it with in-memory exporters.
func Assemble(res *resource.Resource, traceExp sdktrace.SpanExporter, metricExp sdkmetric.Exporter, logExp sdklog.Exporter, sink *errorSink) *Providers {
	if sink == nil {
		sink = &errorSink{w: os.Stderr}
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithIDGenerator(telemetry.IDGenerator{}),
		sdktrace.WithBatcher(traceExp),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithView(Views()...),
	)
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
	)
	return &Providers{Tracer: tp, Meter: mp, Logger: lp, errs: sink}
}

// Resource is service.name=dummyrun and service.version, then whatever OTEL_RESOURCE_ATTRIBUTES
// and OTEL_SERVICE_NAME add or override.
func Resource(ctx context.Context, version string) (*resource.Resource, error) {
	base, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("dummyrun"),
			semconv.ServiceVersion(version),
		),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}
	env, err := resource.New(ctx, resource.WithFromEnv())
	if err != nil {
		return nil, fmt.Errorf("resource from environment: %w", err)
	}
	res, err := resource.Merge(base, env)
	if err != nil && !errors.Is(err, resource.ErrSchemaURLConflict) {
		return nil, fmt.Errorf("merge resource: %w", err)
	}
	return res, nil
}

// Views set histogram buckets that suit runs measured in seconds and steps measured in
// milliseconds. The SDK defaults top out at 10000 and start at 5, which fits neither.
func Views() []sdkmetric.View {
	return []sdkmetric.View{
		sdkmetric.NewView(
			sdkmetric.Instrument{Name: telemetry.MetricRunDuration},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: []float64{15, 30, 60, 120, 180, 300, 600, 900, 1800, 3600},
			}},
		),
		sdkmetric.NewView(
			sdkmetric.Instrument{Name: telemetry.MetricStepDuration},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: []float64{100, 250, 500, 1000, 2000, 5000, 10000, 30000, 60000},
			}},
		),
	}
}

func temporalitySelector(t Temporality) sdkmetric.TemporalitySelector {
	if t == TemporalityDelta {
		return func(sdkmetric.InstrumentKind) metricdata.Temporality { return metricdata.DeltaTemporality }
	}
	return sdkmetric.DefaultTemporalitySelector
}

// Flush pushes everything queued so far. It returns the first export error seen.
func (p *Providers) Flush(ctx context.Context) error {
	var errs []error
	if err := p.Tracer.ForceFlush(ctx); err != nil {
		errs = append(errs, fmt.Errorf("traces: %w", err))
	}
	if err := p.Meter.ForceFlush(ctx); err != nil {
		errs = append(errs, fmt.Errorf("metrics: %w", err))
	}
	if err := p.Logger.ForceFlush(ctx); err != nil {
		errs = append(errs, fmt.Errorf("logs: %w", err))
	}
	if err := p.errs.take(); err != nil && len(errs) == 0 {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Shutdown flushes and releases the providers. It returns the first export error seen.
func (p *Providers) Shutdown(ctx context.Context) error {
	var errs []error
	if err := p.Tracer.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("traces: %w", err))
	}
	if err := p.Meter.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("metrics: %w", err))
	}
	if err := p.Logger.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("logs: %w", err))
	}
	if err := p.errs.take(); err != nil && len(errs) == 0 {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// errorSink remembers export errors. The SDK reports batch export failures to the global error
// handler rather than returning them, so the exporter wrappers below record them.
type errorSink struct {
	w    io.Writer
	mu   sync.Mutex
	errs []error
}

// Handle implements otel.ErrorHandler. It prints; the wrappers store.
func (s *errorSink) Handle(err error) {
	if err != nil {
		fmt.Fprintf(s.w, "dummyrun-otel: %v\n", err)
	}
}

func (s *errorSink) take() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.errs) == 0 {
		return nil
	}
	err := errors.Join(s.errs...)
	s.errs = nil
	return err
}

type spanExporter struct {
	sdktrace.SpanExporter
	sink *errorSink
}

func (e *spanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	err := e.SpanExporter.ExportSpans(ctx, spans)
	if err != nil {
		e.sink.record(fmt.Errorf("traces: export %d spans: %w", len(spans), err))
	}
	return err
}

type metricExporter struct {
	sdkmetric.Exporter
	sink *errorSink
}

func (e *metricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	err := e.Exporter.Export(ctx, rm)
	if err != nil {
		e.sink.record(fmt.Errorf("metrics: export: %w", err))
	}
	return err
}

type logExporter struct {
	sdklog.Exporter
	sink *errorSink
}

func (e *logExporter) Export(ctx context.Context, recs []sdklog.Record) error {
	err := e.Exporter.Export(ctx, recs)
	if err != nil {
		e.sink.record(fmt.Errorf("logs: export %d records: %w", len(recs), err))
	}
	return err
}

// record stores an error without printing it, because the SDK also passes it to Handle.
func (s *errorSink) record(err error) {
	s.mu.Lock()
	s.errs = append(s.errs, err)
	s.mu.Unlock()
}

// Timeout is the default wait for a flush or shutdown.
const Timeout = 30 * time.Second
