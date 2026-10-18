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

// fakeLoki answers the instant query with fixed counts per run and records the query it got. The
// range query returns the newest line when one is set.
type fakeLoki struct {
	counts map[string]int
	newest time.Time
	query  string
	user   string
}

func (f *fakeLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/loki/api/v1/query_range" {
		var results []map[string]any
		if !f.newest.IsZero() {
			results = append(results, map[string]any{"stream": map[string]string{}, "values": [][]string{{strconv.FormatInt(f.newest.UnixNano(), 10), "run x"}}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": results}})
		return
	}
	if r.URL.Path != "/loki/api/v1/query" {
		http.NotFound(w, r)
		return
	}
	f.query = r.URL.Query().Get("query")
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
	if !strings.Contains(fake.query, `{service_name="dummyrun"}`) || !strings.Contains(fake.query, "sum by (dummyrun_run_id)") || !strings.Contains(fake.query, "[93603s]") {
		t.Errorf("query %q", fake.query)
	}
	if fake.user != "user" {
		t.Errorf("basic auth user %q", fake.user)
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
}

func TestNewestLine(t *testing.T) {
	fake := &fakeLoki{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	l, err := NewLoki(srv.URL, "", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := l.Newest(context.Background(), "dummyrun"); err != nil || ok {
		t.Fatalf("empty stream: ok=%v err=%v", ok, err)
	}
	fake.newest = time.Date(2026, 10, 17, 13, 54, 3, 183000000, time.UTC)
	got, ok, err := l.Newest(context.Background(), "dummyrun")
	if err != nil || !ok || !got.Equal(fake.newest) {
		t.Fatalf("newest = %s ok=%v err=%v, want %s", got, ok, err, fake.newest)
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
	if _, err := l.Count(context.Background(), "dummyrun", time.Now().Add(-time.Hour), time.Now()); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("got %v, want an HTTP 502 error", err)
	}
}
