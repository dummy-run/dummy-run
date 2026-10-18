// Package telemetry builds the identifiers and headers that link a run to the
// target app's own traces, as docs/TELEMETRY.md defines them.
package telemetry

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// Version is the Dummy Run version. The browser's user agent ends with
// "DummyRun/<Version>".
var Version = "0.1.0-dev"

// UserAgentSuffix is appended to the browser's own user agent.
func UserAgentSuffix() string { return "DummyRun/" + Version }

// UserAgent returns the browser's user agent with the Dummy Run suffix, unless
// it already ends with one.
func UserAgent(browserUA string) string {
	suffix := UserAgentSuffix()
	if strings.HasSuffix(browserUA, suffix) {
		return browserUA
	}
	return strings.TrimSpace(browserUA) + " " + suffix
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// Slug lowers a string and keeps only letters, digits and hyphens.
func Slug(s string) string {
	s = slugUnsafe.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

// NewRunID builds a run ID of the form the schema requires: the date, then a
// slug made from the persona, the scenario and four random hex characters.
func NewRunID(now time.Time, persona, scenario string) string {
	return fmt.Sprintf("%s-%s-%s-%s", now.UTC().Format("2006-01-02"), Slug(persona), Slug(scenario), randomHex(2))
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("telemetry: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Trace holds the IDs that tie the run's requests to the exporter's trace.
//
// The exporter builds the run's trace from the run record alone, so both sides
// derive the same IDs from the run ID: the trace ID is the first 16 bytes of
// SHA-256 of "dummyrun.run:<run id>" and the root span ID is the first 8 bytes
// of SHA-256 of "dummyrun.span:<run id>:root". The app's spans then join the
// trace as children of the root span.
type Trace struct {
	TraceID    string
	RootSpanID string
}

// NewTrace derives the trace for a run ID.
func NewTrace(runID string) Trace {
	trace := sha256.Sum256([]byte("dummyrun.run:" + runID))
	span := sha256.Sum256([]byte("dummyrun.span:" + runID + ":root"))
	return Trace{TraceID: hex.EncodeToString(trace[:16]), RootSpanID: hex.EncodeToString(span[:8])}
}

// Traceparent is the W3C header naming the run's root span as the parent.
func (t Trace) Traceparent() string {
	return "00-" + t.TraceID + "-" + t.RootSpanID + "-01"
}

// Baggage is the W3C baggage header carrying the run and persona IDs.
func Baggage(runID, personaID string) string {
	return "dummyrun.run.id=" + runID + ",dummyrun.persona.id=" + personaID
}

// Headers returns the headers to set on a request to the target. The page may
// already carry a traceparent from its own instrumentation; that one is kept
// and only the baggage is added. Existing baggage is extended, not replaced.
func Headers(existing []bidi.Header, runID, personaID, traceparent string) []bidi.Header {
	out := []bidi.Header{}
	bag := Baggage(runID, personaID)
	if prev, ok := bidi.HeaderValue(existing, "baggage"); ok && strings.TrimSpace(prev) != "" {
		if !strings.Contains(prev, "dummyrun.run.id=") {
			bag = strings.TrimSpace(prev) + "," + bag
		}
	}
	out = append(out, bidi.StringHeader("baggage", bag))
	if _, ok := bidi.HeaderValue(existing, "traceparent"); !ok {
		out = append(out, bidi.StringHeader("traceparent", traceparent))
	}
	return out
}
