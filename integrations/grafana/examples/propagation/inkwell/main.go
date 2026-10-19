// Inkwell is a stub of the fictional app that Dummy Run personas use in the
// examples. It exists to prove one thing: a request that arrives with W3C
// traceparent and baggage headers produces server spans that join the
// persona's trace and carry dummyrun.run.id and dummyrun.persona.id.
//
// The app is plain net/http with the OpenTelemetry Go SDK. The two pieces that
// matter are the composite propagator (trace context plus baggage) and the
// baggagecopy span processor, which copies baggage members onto every span.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/processors/baggagecopy"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// allowedBaggage lists the two members docs/TELEMETRY.md says the runner sends.
// Copying anything else would let any caller write attributes into your traces.
var allowedBaggage = map[string]bool{"dummyrun.run.id": true, "dummyrun.persona.id": true}

// dummyRunMembers is the filter handed to the baggagecopy processor.
func dummyRunMembers(m baggage.Member) bool {
	return allowedBaggage[m.Key()]
}

// newTracerProvider wires the SDK. The caller supplies the exporting span
// processor so tests can pass an in-memory one.
func newTracerProvider(exporting sdktrace.SpanProcessor) *sdktrace.TracerProvider {
	tp := sdktrace.NewTracerProvider(
		// Order matters: baggagecopy must run OnStart before the span is exported.
		sdktrace.WithSpanProcessor(baggagecopy.NewSpanProcessor(dummyRunMembers)),
		sdktrace.WithSpanProcessor(exporting),
	)
	otel.SetTracerProvider(tp)
	// Without the Baggage propagator the baggage header is never parsed, and
	// without TraceContext the app starts a new trace instead of joining the persona's.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp
}

// allowTraceHeaders answers CORS preflight for a frontend on another origin.
// traceparent, tracestate and baggage are not CORS-safelisted, so a browser
// refuses to send them cross-origin unless the server lists them here.
func allowTraceHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "traceparent, tracestate, baggage, content-type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type page struct {
	Page       string `json:"page"`
	TraceID    string `json:"trace_id"`
	RunID      string `json:"dummyrun_run_id,omitempty"`
	PersonaID  string `json:"dummyrun_persona_id,omitempty"`
	UserAgent  string `json:"user_agent"`
	PersonaRun bool   `json:"persona_run"`
}

// handlePage echoes what the server saw, so the example script can show the
// join without opening Grafana. Real apps would not do this.
func handlePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bag := baggage.FromContext(ctx)
	ua := r.UserAgent()
	resp := page{
		Page:       r.URL.Path,
		TraceID:    trace.SpanContextFromContext(ctx).TraceID().String(),
		RunID:      bag.Member("dummyrun.run.id").Value(),
		PersonaID:  bag.Member("dummyrun.persona.id").Value(),
		UserAgent:  ua,
		PersonaRun: strings.Contains(ua, "DummyRun/"),
	}
	// A child span, to show the attributes reach every span in the app, not only the server span.
	_, span := otel.Tracer("inkwell").Start(ctx, "inkwell.render")
	time.Sleep(5 * time.Millisecond)
	span.End()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, route := range []string{"/", "/drafts", "/payments", "/reviews"} {
		mux.HandleFunc(route, handlePage)
	}
	// otelhttp extracts traceparent and baggage with the global propagator and
	// starts the server span. Once the mux has matched, otelhttp names the span
	// "{method} {pattern}" from r.Pattern, so the set of span names is bounded by
	// the routes above and an unlisted path is named "GET /".
	return allowTraceHeaders(otelhttp.NewHandler(mux, "inkwell"))
}

// healthy is the compose health check. The runtime image is scratch, so the
// binary checks itself: `inkwell healthz` exits 0 when the server answers.
func healthy(addr string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Get("http://127.0.0.1" + addr + "/healthz")
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errors.New("healthz returned " + res.Status)
	}
	return nil
}

func listenAddr() string {
	if addr := os.Getenv("INKWELL_ADDR"); addr != "" {
		return addr
	}
	return ":8080"
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthz" {
		if err := healthy(listenAddr()); err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx := context.Background()
	// Endpoint, headers and protocol come from OTEL_EXPORTER_OTLP_* variables.
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		log.Fatalf("otlp exporter: %v", err)
	}
	tp := newTracerProvider(sdktrace.NewBatchSpanProcessor(exporter))
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	addr := listenAddr()
	srv := &http.Server{Addr: addr, Handler: newHandler(), ReadHeaderTimeout: 5 * time.Second}

	stop, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() {
		log.Printf("inkwell listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()
	<-stop.Done()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = srv.Shutdown(shutdownCtx)
}
