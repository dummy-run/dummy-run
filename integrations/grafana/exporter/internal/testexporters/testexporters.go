// Package testexporters holds in-memory metric and log exporters for tests. The trace SDK ships
// its own in tracetest.
package testexporters

import (
	"context"
	"sync"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Metrics keeps every exported ResourceMetrics.
type Metrics struct {
	mu   sync.Mutex
	data []metricdata.ResourceMetrics
}

// Temporality implements sdkmetric.Exporter.
func (*Metrics) Temporality(k sdkmetric.InstrumentKind) metricdata.Temporality {
	return sdkmetric.DefaultTemporalitySelector(k)
}

// Aggregation implements sdkmetric.Exporter.
func (*Metrics) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(k)
}

// Export implements sdkmetric.Exporter.
func (m *Metrics) Export(_ context.Context, rm *metricdata.ResourceMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = append(m.data, *rm)
	return nil
}

// ForceFlush implements sdkmetric.Exporter.
func (*Metrics) ForceFlush(context.Context) error { return nil }

// Shutdown implements sdkmetric.Exporter.
func (*Metrics) Shutdown(context.Context) error { return nil }

// Collected returns every export so far.
func (m *Metrics) Collected() []metricdata.ResourceMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]metricdata.ResourceMetrics(nil), m.data...)
}

// Logs keeps every exported log record.
type Logs struct {
	mu   sync.Mutex
	recs []sdklog.Record
}

// Export implements sdklog.Exporter.
func (l *Logs) Export(_ context.Context, recs []sdklog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range recs {
		l.recs = append(l.recs, r.Clone())
	}
	return nil
}

// ForceFlush implements sdklog.Exporter.
func (*Logs) ForceFlush(context.Context) error { return nil }

// Shutdown implements sdklog.Exporter.
func (*Logs) Shutdown(context.Context) error { return nil }

// Records returns every record so far.
func (l *Logs) Records() []sdklog.Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]sdklog.Record(nil), l.recs...)
}
