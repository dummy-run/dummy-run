package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const (
	traceID  = "4bf92f3577b34da6a3ce929d0e0e4736"
	parentID = "00f067aa0ab902b7"
	runID    = "2026-10-17-ravi-reviewer-005"
	persona  = "ravi-reviewer"
)

// TestBaggageReachesEverySpan sends one request shaped like the persona browser's
// and checks the server span and its child both join the trace and carry the
// two dummyrun attributes. Any other baggage member must be dropped.
func TestBaggageReachesEverySpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTracerProvider(sdktrace.NewSimpleSpanProcessor(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	srv := httptest.NewServer(newHandler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/drafts", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("traceparent", "00-"+traceID+"-"+parentID+"-01")
	req.Header.Set("baggage", "dummyrun.run.id="+runID+",dummyrun.persona.id="+persona+",session.id=should-not-copy,dummyrun.extra=should-not-copy")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Chrome/148.0.0.0 DummyRun/0.1.0")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("want 2 spans (server + render), got %d", len(spans))
	}
	for _, s := range spans {
		if got := s.SpanContext.TraceID().String(); got != traceID {
			t.Errorf("%s: trace ID %s, want %s (span did not join the persona's trace)", s.Name, got, traceID)
		}
		attrs := attribute.NewSet(s.Attributes...)
		for key, want := range map[attribute.Key]string{"dummyrun.run.id": runID, "dummyrun.persona.id": persona} {
			v, ok := attrs.Value(key)
			if !ok || v.AsString() != want {
				t.Errorf("%s: %s = %q, want %q", s.Name, key, v.AsString(), want)
			}
		}
		for _, key := range []attribute.Key{"session.id", "dummyrun.extra"} {
			if _, ok := attrs.Value(key); ok {
				t.Errorf("%s: %s was copied; the filter should allow only the two contract members", s.Name, key)
			}
		}
	}
	var server bool
	for _, s := range spans {
		if s.Name == "GET /drafts" {
			server = true
			if got := s.Parent.SpanID().String(); got != parentID {
				t.Errorf("server span parent %s, want the persona's step span %s", got, parentID)
			}
		}
	}
	if !server {
		t.Error("no server span named GET /drafts")
	}
}

// TestUnknownPathHasBoundedSpanName checks a request to an unlisted path is named after the
// catch-all route, so a scanner cannot create a span name per URL.
func TestUnknownPathHasBoundedSpanName(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTracerProvider(sdktrace.NewSimpleSpanProcessor(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	rec := httptest.NewRecorder()
	newHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no/such/page/12345", nil))
	for _, s := range exporter.GetSpans() {
		if s.Name != "GET /" && s.Name != "inkwell.render" {
			t.Errorf("span name %q leaks the request path", s.Name)
		}
	}
}

// TestPreflightAllowsTraceHeaders checks the CORS answer a cross-origin frontend needs.
func TestPreflightAllowsTraceHeaders(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTracerProvider(sdktrace.NewSimpleSpanProcessor(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	rec := httptest.NewRecorder()
	newHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/drafts", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	for _, h := range []string{"traceparent", "tracestate", "baggage"} {
		if !containsToken(rec.Header().Get("Access-Control-Allow-Headers"), h) {
			t.Errorf("Access-Control-Allow-Headers lacks %s: %q", h, rec.Header().Get("Access-Control-Allow-Headers"))
		}
	}
}

func containsToken(list, token string) bool {
	for _, part := range splitTrim(list) {
		if part == token {
			return true
		}
	}
	return false
}

func splitTrim(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			for len(part) > 0 && part[0] == ' ' {
				part = part[1:]
			}
			for len(part) > 0 && part[len(part)-1] == ' ' {
				part = part[:len(part)-1]
			}
			out = append(out, part)
			start = i + 1
		}
	}
	return out
}
