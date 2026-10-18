package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const examples = "../../../../../examples/runs"

// fakeOTLP accepts OTLP/HTTP requests and counts them by path.
type fakeOTLP struct {
	mu     sync.Mutex
	counts map[string]int
	bytes  map[string]int
	status int
}

func newFakeOTLP(status int) *fakeOTLP {
	return &fakeOTLP{counts: map[string]int{}, bytes: map[string]int{}, status: status}
}

func (f *fakeOTLP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r.Body)
	f.mu.Lock()
	f.counts[r.URL.Path]++
	f.bytes[r.URL.Path] += buf.Len()
	f.mu.Unlock()
	if r.Header.Get("Content-Type") != "application/x-protobuf" {
		http.Error(w, "want protobuf", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(f.status)
}

func (f *fakeOTLP) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[path]
}

// fakeGrafana stores annotations and answers the lookups the exporter makes.
type fakeGrafana struct {
	mu    sync.Mutex
	store []map[string]any
}

func (f *fakeGrafana) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/annotations" || r.Header.Get("Authorization") != "Bearer glsa_test" {
		http.Error(w, "nope", http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
		to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
		out := []map[string]any{}
		for _, a := range f.store {
			t0, t1 := int64(a["time"].(float64)), int64(a["timeEnd"].(float64))
			if t1 >= from && t0 <= to {
				out = append(out, a)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodPost:
		var a map[string]any
		_ = json.NewDecoder(r.Body).Decode(&a)
		a["id"] = float64(len(f.store) + 1)
		f.store = append(f.store, a)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": a["id"], "message": "Annotation added"})
	}
}

// fakeLoki answers the verification query with a fixed count per run ID.
type fakeLoki struct {
	mu     sync.Mutex
	counts map[string]int
}

func (f *fakeLoki) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var results []map[string]any
	for id, n := range f.counts {
		results = append(results, map[string]any{"metric": map[string]string{"dummyrun_run_id": id}, "value": []any{1.0, strconv.Itoa(n)}})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": results}})
}

func TestExportVerifiesLogsInLoki(t *testing.T) {
	otlp := newFakeOTLP(http.StatusOK)
	srv := httptest.NewServer(otlp)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	// Every run has its lines except one, which Loki dropped as too far behind.
	loki := &fakeLoki{counts: map[string]int{
		"2026-10-15-sam-author-001": 2, "2026-10-16-ada-editor-003": 2, "2026-10-16-lin-contributor-002": 2,
		"2026-10-16-lin-contributor-004": 2, "2026-10-17-lin-contributor-008": 3, "2026-10-17-ravi-reviewer-005": 1,
		"2026-10-17-sam-author-006": 2, "2026-10-17-sam-author-007": 0,
	}}
	lsrv := httptest.NewServer(loki)
	defer lsrv.Close()
	code, _, stderr := run(t, nil, "export", "--logs-max-age", "0", "--verify-loki", lsrv.URL, examples)
	if code != ExitFailure {
		t.Fatalf("exit %d, want %d; stderr %s", code, ExitFailure, stderr)
	}
	if !strings.Contains(stderr, "run 2026-10-17-sam-author-007: 0 of 2 log lines in Loki") || !strings.Contains(stderr, "1 of 8 runs are missing log lines") {
		t.Errorf("stderr %q", stderr)
	}
	loki.mu.Lock()
	loki.counts["2026-10-17-sam-author-007"] = 2
	loki.mu.Unlock()
	code, _, stderr = run(t, nil, "export", "--logs-max-age", "0", "--verify-loki", lsrv.URL, examples)
	if code != ExitOK || !strings.Contains(stderr, "every one of the 16 log records is in Loki") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := run(t, map[string]string{"LOKI_BASIC_AUTH": "nocolon"}, "export", "--verify-loki", lsrv.URL, examples); code != ExitFailure || !strings.Contains(stderr, "LOKI_BASIC_AUTH") {
		t.Errorf("bad LOKI_BASIC_AUTH: exit %d, stderr %q", code, stderr)
	}
}

func run(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Main(context.Background(), args, Env{
		Stdout: &out,
		Stderr: &errb,
		Getenv: func(k string) string { return env[k] },
		Now:    func() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) },
	})
	return code, out.String(), errb.String()
}

func TestUsageErrors(t *testing.T) {
	if code, _, _ := run(t, nil); code != ExitUsage {
		t.Errorf("no args: exit %d, want %d", code, ExitUsage)
	}
	if code, _, stderr := run(t, nil, "frobnicate"); code != ExitUsage || !strings.Contains(stderr, "unknown command") {
		t.Errorf("unknown command: exit %d, stderr %q", code, stderr)
	}
	if code, _, _ := run(t, nil, "export"); code != ExitUsage {
		t.Errorf("export without paths: exit %d, want %d", code, ExitUsage)
	}
	if code, _, _ := run(t, nil, "export", "--protocol", "carrier-pigeon", examples); code != ExitFailure {
		t.Errorf("bad protocol: exit %d, want %d", code, ExitFailure)
	}
	if code, _, _ := run(t, nil, "watch"); code != ExitUsage {
		t.Errorf("watch without a directory: exit %d, want %d", code, ExitUsage)
	}
	if code, out, _ := run(t, nil, "version"); code != ExitOK || !strings.HasPrefix(out, "dummyrun-otel ") {
		t.Errorf("version: exit %d, out %q", code, out)
	}
}

func TestDryRunPrintsPlans(t *testing.T) {
	code, out, stderr := run(t, nil, "export", "--dry-run", examples)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %s", code, stderr)
	}
	var plans []map[string]any
	if err := json.Unmarshal([]byte(out), &plans); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if len(plans) != 8 {
		t.Errorf("%d plans, want 8", len(plans))
	}
	if plans[0]["run_id"] != "2026-10-15-sam-author-001" {
		t.Errorf("first plan %v", plans[0]["run_id"])
	}
	if !strings.Contains(stderr, "nothing sent") {
		t.Errorf("stderr %q", stderr)
	}
}

func TestInvalidRecordFailsAndNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(bad, []byte(`{"schema_version": 2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, nil, "export", "--dry-run", bad)
	if code != ExitFailure {
		t.Errorf("exit %d, want %d", code, ExitFailure)
	}
	if !strings.Contains(stderr, bad) || !strings.Contains(stderr, "invalid run record") {
		t.Errorf("stderr %q does not name the file", stderr)
	}
}

func TestExportSendsAllSignalsAndAnnotations(t *testing.T) {
	otlp := newFakeOTLP(http.StatusOK)
	srv := httptest.NewServer(otlp)
	defer srv.Close()
	grafana := &fakeGrafana{}
	gsrv := httptest.NewServer(grafana)
	defer gsrv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)

	env := map[string]string{"GRAFANA_TOKEN": "glsa_test"}
	code, _, stderr := run(t, env, "export", "--grafana-url", gsrv.URL, "--logs-max-age", "0", examples)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %s", code, stderr)
	}
	for _, p := range []string{"/v1/traces", "/v1/metrics", "/v1/logs"} {
		if otlp.count(p) == 0 {
			t.Errorf("nothing posted to %s", p)
		}
	}
	if !strings.Contains(stderr, "sent 8 runs: 102 spans, 16 log records") {
		t.Errorf("summary %q", stderr)
	}
	if !strings.Contains(stderr, "annotations: 8 created, 0 already present") {
		t.Errorf("summary %q", stderr)
	}
	if len(grafana.store) != 8 {
		t.Errorf("%d annotations stored", len(grafana.store))
	}

	// A second export is idempotent for annotations.
	code, _, stderr = run(t, env, "export", "--grafana-url", gsrv.URL, "--logs-max-age", "0", examples)
	if code != ExitOK || !strings.Contains(stderr, "annotations: 0 created, 8 already present") {
		t.Errorf("second export: exit %d, stderr %q", code, stderr)
	}
}

func TestExportSkipsOldLogs(t *testing.T) {
	otlp := newFakeOTLP(http.StatusOK)
	srv := httptest.NewServer(otlp)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	// The clock is fixed at 2026-10-18T12:00Z; the records run from the 15th to the 17th.
	code, _, stderr := run(t, nil, "export", "--logs-max-age", "36h", examples)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %s", code, stderr)
	}
	if !strings.Contains(stderr, "skipped") || !strings.Contains(stderr, "--logs-max-age") {
		t.Errorf("summary %q should say logs were skipped", stderr)
	}
	// Only the 17th is inside 36h: four runs with four findings, so 8 of the 16 records.
	if !strings.Contains(stderr, "8 log records") || !strings.Contains(stderr, "skipped 8 log records") {
		t.Errorf("summary %q", stderr)
	}
}

func TestExportFailsWhenTheBackendRejects(t *testing.T) {
	otlp := newFakeOTLP(http.StatusBadRequest)
	srv := httptest.NewServer(otlp)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	code, _, stderr := run(t, nil, "export", "--timeout", "10s", filepath.Join(examples, "2026-10-17-ravi-reviewer-005.json"))
	if code != ExitFailure {
		t.Fatalf("exit %d, want %d; stderr %s", code, ExitFailure, stderr)
	}
	if !strings.Contains(stderr, "export failed") {
		t.Errorf("stderr %q", stderr)
	}
}

func TestExportFailsWithoutAToken(t *testing.T) {
	code, _, stderr := run(t, nil, "export", "--grafana-url", "http://127.0.0.1:1", examples)
	if code != ExitFailure || !strings.Contains(stderr, "GRAFANA_TOKEN") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestWatchSendsNewFiles(t *testing.T) {
	otlp := newFakeOTLP(http.StatusOK)
	srv := httptest.NewServer(otlp)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	dir := t.TempDir()
	good, err := os.ReadFile(filepath.Join(examples, "2026-10-17-ravi-reviewer-005.json"))
	if err != nil {
		t.Fatal(err)
	}
	sigs := make(chan os.Signal, 1)
	var out, errb bytes.Buffer
	done := make(chan int)
	go func() {
		done <- Main(context.Background(), []string{"watch", "--interval", "20ms", "--logs-max-age", "0", dir}, Env{
			Stdout: &out, Stderr: &errb, Getenv: func(string) string { return "" }, Signals: sigs,
		})
	}()
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "2026-10-17-ravi-reviewer-005.json"), good, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for otlp.count("/v1/traces") == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	sigs <- os.Interrupt
	code := <-done
	stderr := errb.String()
	if code != ExitFailure {
		t.Errorf("exit %d, want %d because bad.json is invalid; stderr %s", code, ExitFailure, stderr)
	}
	if !strings.Contains(stderr, "sent "+filepath.Join(dir, "2026-10-17-ravi-reviewer-005.json")) {
		t.Errorf("stderr %q does not report the good file", stderr)
	}
	if !strings.Contains(stderr, "bad.json") || !strings.Contains(stderr, "1 files failed") {
		t.Errorf("stderr %q does not report the bad file", stderr)
	}
	if otlp.count("/v1/traces") == 0 || otlp.count("/v1/logs") == 0 {
		t.Error("watch did not send the good record")
	}
}
