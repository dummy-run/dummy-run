package verify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeLoki answers the instant query with fixed counts per run and records the query it got.
type fakeLoki struct {
	counts  map[string]int
	query   string
	queries int
	user    string
}

func (f *fakeLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/loki/api/v1/query" {
		http.NotFound(w, r)
		return
	}
	f.query = r.URL.Query().Get("query")
	f.queries++
	f.user, _, _ = r.BasicAuth()
	type result struct {
		Metric map[string]string `json:"metric"`
		Value  []any             `json:"value"`
	}
	var results []result
	for id, n := range f.counts {
		results = append(results, result{Metric: map[string]string{"dummyrun_run_id": id}, Value: []any{1.0, strconv.Itoa(n)}})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": results}})
}

func TestCheckReportsShortRuns(t *testing.T) {
	fake := &fakeLoki{counts: map[string]int{"2026-10-16-a": 2, "2026-10-17-b": 1}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	l, err := NewLoki(srv.URL+"/", "user:secret", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	oldest := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	newest := oldest.Add(26 * time.Hour)
	wants := []Want{{RunID: "2026-10-17-b", Lines: 3}, {RunID: "2026-10-16-a", Lines: 2}, {RunID: "2026-10-18-c", Lines: 1}}
	short, err := l.Check(context.Background(), "dummyrun", wants, oldest, newest, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(short) != 2 || short[0].RunID != "2026-10-17-b" || short[0].Got != 1 || short[0].Want != 3 || short[1].RunID != "2026-10-18-c" || short[1].Got != 0 {
		t.Errorf("shortfalls %+v", short)
	}
	if short[0].String() != "run 2026-10-17-b: 1 of 3 log lines in Loki" {
		t.Errorf("message %q", short[0].String())
	}
	if !strings.Contains(fake.query, `{service_name="dummyrun"}`) || !strings.Contains(fake.query, "sum by (dummyrun_run_id)") || !strings.Contains(fake.query, "[93602s]") {
		t.Errorf("query %q", fake.query)
	}
	if fake.queries != 1 {
		t.Errorf("%d queries for a 26 hour span, want 1", fake.queries)
	}
	if !strings.Contains(fake.query, `dummyrun_run_id=~"2026-10-17-b|2026-10-16-a|2026-10-18-c"`) {
		t.Errorf("query %q does not limit itself to the runs being checked", fake.query)
	}
	if fake.user != "user" {
		t.Errorf("basic auth user %q", fake.user)
	}
}

func TestCountSplitsLongSpansIntoWindows(t *testing.T) {
	fake := &fakeLoki{counts: map[string]int{"2026-09-20-a": 1}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	l, err := NewLoki(srv.URL, "", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	oldest := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	newest := oldest.Add(45 * 24 * time.Hour)
	counts, err := l.Count(context.Background(), "dummyrun", []string{"2026-09-20-a"}, oldest, newest)
	if err != nil {
		t.Fatal(err)
	}
	if fake.queries != 7 {
		t.Errorf("%d queries for a 45 day span, want 7 windows of 7 days", fake.queries)
	}
	// The fake answers every window with one line, so the windows must add up.
	if counts["2026-09-20-a"] != 7 {
		t.Errorf("count %d, want the windows summed", counts["2026-09-20-a"])
	}
}

func TestCheckPassesWhenEveryLineIsThere(t *testing.T) {
	fake := &fakeLoki{counts: map[string]int{"2026-10-16-a": 2}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	l, err := NewLoki(srv.URL, "", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	short, err := l.Check(context.Background(), "dummyrun", []Want{{RunID: "2026-10-16-a", Lines: 2}}, time.Now().Add(-time.Hour), time.Now(), 0)
	if err != nil || len(short) != 0 {
		t.Fatalf("short %v, err %v", short, err)
	}
}

func TestNewLokiRejectsBadInput(t *testing.T) {
	if _, err := NewLoki("localhost:3100", "", nil); err == nil {
		t.Error("a URL without a scheme must be rejected")
	}
	if _, err := NewLoki("http://localhost:3100", "token-only", nil); err == nil {
		t.Error("basic auth without a colon must be rejected")
	}
	if _, err := NewLoki("http://loki.example.net:3100", "user:secret", nil); err == nil || !strings.Contains(err.Error(), "clear text") {
		t.Errorf("credentials over plain http to a remote host must be rejected, got %v", err)
	}
	for _, ok := range []string{"http://localhost:3100", "http://127.0.0.1:3100", "http://[::1]:3100", "https://loki.example.net"} {
		if _, err := NewLoki(ok, "user:secret", nil); err != nil {
			t.Errorf("%s with credentials must be accepted: %v", ok, err)
		}
	}
	if _, err := NewLoki("http://loki.example.net:3100", "", nil); err != nil {
		t.Errorf("plain http without credentials must be accepted: %v", err)
	}
}

func TestRefuseInsecureRedirect(t *testing.T) {
	from, _ := http.NewRequest(http.MethodGet, "https://loki.example.net/", nil)
	toHTTP, _ := http.NewRequest(http.MethodGet, "http://loki.example.net/", nil)
	toHTTPS, _ := http.NewRequest(http.MethodGet, "https://other.example.net/", nil)
	if err := RefuseInsecureRedirect(toHTTP, []*http.Request{from}); err == nil {
		t.Error("https to http must be refused")
	}
	if err := RefuseInsecureRedirect(toHTTPS, []*http.Request{from}); err != nil {
		t.Errorf("https to https must be allowed: %v", err)
	}
}

func TestCountReportsHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	defer srv.Close()
	l, err := NewLoki(srv.URL, "", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Count(context.Background(), "dummyrun", []string{"x"}, time.Now().Add(-time.Hour), time.Now()); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("got %v, want an HTTP 502 error", err)
	}
}
