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
	"sync"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// Version is the Dummy Run version. The browser's user agent ends with
// "DummyRun/<Version>".
var Version = "0.1.0-dev"

// ScannerURL is the page that says who Dummy Run is and how to opt out. An identified user agent names it.
const ScannerURL = "https://dummyrun.dev/scanner/"

// RobotsToken is the product token a robots.txt file uses to address Dummy Run.
const RobotsToken = "DummyRun"

// UserAgentSuffix is appended to the browser's own user agent.
func UserAgentSuffix() string { return "DummyRun/" + Version }

// IdentifiedSuffix is the suffix with a contact URL, as --identify adds it.
func IdentifiedSuffix() string { return UserAgentSuffix() + " (+" + ScannerURL + ")" }

// UserAgent returns the browser's user agent with the Dummy Run suffix, unless
// it already ends with one. With identify the suffix also names ScannerURL. The
// browser's own user agent stays in front so sites render as they do for the browser.
func UserAgent(browserUA string, identify bool) string {
	suffix := UserAgentSuffix()
	if identify {
		suffix = IdentifiedSuffix()
	}
	if strings.HasSuffix(browserUA, suffix) {
		return browserUA
	}
	return strings.TrimSpace(browserUA) + " " + suffix
}

// RequestUserAgent is the user agent for the engine's own requests, such as robots.txt and sitemaps.
func RequestUserAgent(identify bool) string {
	if identify {
		return IdentifiedSuffix()
	}
	return UserAgentSuffix()
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
// slug made from the persona, the scenario and sixteen random hex characters.
func NewRunID(now time.Time, persona, scenario string) string {
	return fmt.Sprintf("%s-%s-%s-%s", now.UTC().Format("2006-01-02"), Slug(persona), Slug(scenario), randomHex(8))
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
// derive the same IDs from the run ID. The trace ID is the first 16 bytes of
// SHA-256 of "dummyrun.run:<run id>". The root span ID is the first 8 bytes of
// SHA-256 of "dummyrun.span:<run id>:root". Step n's span ID is the first 8
// bytes of SHA-256 of "dummyrun.span:<run id>:step:<n>". A request sent during
// step n names that step's span as its parent, so the app's spans nest under
// the step.
type Trace struct {
	runID      string
	TraceID    string
	RootSpanID string
	mu         sync.Mutex
	step       int
}

// NewTrace derives the trace for a run ID. The current step starts at 1.
func NewTrace(runID string) *Trace {
	trace := sha256.Sum256([]byte("dummyrun.run:" + runID))
	root := sha256.Sum256([]byte("dummyrun.span:" + runID + ":root"))
	return &Trace{runID: runID, TraceID: hex.EncodeToString(trace[:16]), RootSpanID: hex.EncodeToString(root[:8]), step: 1}
}

// StepSpanID is the span ID of step n.
func (t *Trace) StepSpanID(n int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("dummyrun.span:%s:step:%d", t.runID, n)))
	return hex.EncodeToString(sum[:8])
}

// SetStep says which step is running, so Traceparent names its span.
func (t *Trace) SetStep(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.step = n
}

// Step returns the step whose span requests currently join.
func (t *Trace) Step() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.step
}

// Traceparent is the W3C header for a request sent during the current step.
func (t *Trace) Traceparent() string {
	return "00-" + t.TraceID + "-" + t.StepSpanID(t.Step()) + "-01"
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
