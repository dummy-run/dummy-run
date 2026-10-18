package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

func TestNewRunIDMatchesSchema(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}-[a-z0-9-]+$`)
	id := NewRunID(time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC), "QA Lead", "First Look!")
	if !pattern.MatchString(id) {
		t.Errorf("run ID %q does not match the schema pattern", id)
	}
	if !strings.HasPrefix(id, "2026-10-18-qa-lead-first-look-") {
		t.Errorf("run ID %q", id)
	}
	first, second := NewRunID(time.Now(), "a", "b"), NewRunID(time.Now(), "a", "b")
	if first == second {
		t.Error("two run IDs collided")
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Hello World": "hello-world", "--a__b--": "a-b", "x": "x", "Über cool": "ber-cool"} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTraceIsDerivedFromTheRunID(t *testing.T) {
	tr := NewTrace("2026-10-18-qa-lead-first-look-ab12")
	// sha256("dummyrun.run:<id>")[:16] and sha256("dummyrun.span:<id>:root")[:8], as docs/TELEMETRY.md defines them.
	wantTrace := sha256.Sum256([]byte("dummyrun.run:2026-10-18-qa-lead-first-look-ab12"))
	wantSpan := sha256.Sum256([]byte("dummyrun.span:2026-10-18-qa-lead-first-look-ab12:root"))
	if tr.TraceID != hex.EncodeToString(wantTrace[:16]) || tr.RootSpanID != hex.EncodeToString(wantSpan[:8]) {
		t.Errorf("ids %q %q", tr.TraceID, tr.RootSpanID)
	}
	if !regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`).MatchString(tr.Traceparent()) {
		t.Errorf("traceparent %q", tr.Traceparent())
	}
	if NewTrace("2026-10-18-other-run-0000").TraceID == tr.TraceID {
		t.Error("two runs share a trace ID")
	}
	if NewTrace("2026-10-18-qa-lead-first-look-ab12") != tr {
		t.Error("the derivation is not stable")
	}
}

func TestHeaders(t *testing.T) {
	tp := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	t.Run("adds both when the page set neither", func(t *testing.T) {
		got := Headers([]bidi.Header{bidi.StringHeader("Host", "a")}, "r1", "p1", tp)
		if len(got) != 2 || got[0].Value.Value != "dummyrun.run.id=r1,dummyrun.persona.id=p1" || got[1].Value.Value != tp {
			t.Errorf("got %v", got)
		}
	})
	t.Run("keeps the page's traceparent", func(t *testing.T) {
		got := Headers([]bidi.Header{bidi.StringHeader("traceparent", "00-page-span-01")}, "r1", "p1", tp)
		if len(got) != 1 || got[0].Name != "baggage" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("extends the page's baggage", func(t *testing.T) {
		got := Headers([]bidi.Header{bidi.StringHeader("baggage", "session.id=abc")}, "r1", "p1", tp)
		if got[0].Value.Value != "session.id=abc,dummyrun.run.id=r1,dummyrun.persona.id=p1" {
			t.Errorf("got %q", got[0].Value.Value)
		}
	})
	t.Run("does not repeat its own baggage on a redirect", func(t *testing.T) {
		got := Headers([]bidi.Header{bidi.StringHeader("baggage", "dummyrun.run.id=r1,dummyrun.persona.id=p1")}, "r1", "p1", tp)
		if got[0].Value.Value != "dummyrun.run.id=r1,dummyrun.persona.id=p1" {
			t.Errorf("got %q", got[0].Value.Value)
		}
	})
}

func TestUserAgent(t *testing.T) {
	ua := UserAgent("Mozilla/5.0 Firefox/158.0")
	if !strings.HasSuffix(ua, "DummyRun/"+Version) {
		t.Errorf("got %q", ua)
	}
	if UserAgent(ua) != ua {
		t.Error("suffix added twice")
	}
}
