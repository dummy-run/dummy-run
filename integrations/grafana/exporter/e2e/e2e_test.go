//go:build e2e

// Package e2e starts grafana/otel-lgtm with Docker, exports run records into it and queries Tempo,
// Loki, Prometheus and Grafana to prove the data arrived.
//
// Run it with: go test -tags e2e -timeout 15m ./e2e/
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/cli"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/record"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/telemetry"
)

// image is grafana/otel-lgtm 0.35.0, pinned by digest. DUMMYRUN_E2E_IMAGE overrides it.
const image = "grafana/otel-lgtm@sha256:2de1094c593c671cbfca878a28ffcd7e46ff5e32f2ce966c0cf33003f6cf4266"

const repoRoot = "../../../.."

type stack struct {
	container string
	otlp      string // http://host:port
	grafana   string
	tempo     string
	loki      string
	prom      string
	client    *http.Client
}

func TestEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	st := startStack(ctx, t)
	token := st.serviceAccountToken(ctx, t)

	// Two sets of records: the shipped examples with every timestamp shifted so the newest run
	// ended two days ago, and a fresh set generated to end yesterday. Both stay inside Loki's seven
	// day window and Tempo's 30 day slack whenever the test runs.
	examples := shiftedExamples(t, 2)
	generated := generateRuns(ctx, t, 12, 1, "-e2e")
	validator, err := record.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	files, err := validator.Load(examples, generated)
	if err != nil {
		t.Fatal(err)
	}
	wantRuns, wantSteps, wantFindings, wantLogs := 0, 0, 0, 0
	logCutoff := time.Now().Add(-168 * time.Hour)
	for _, f := range files {
		wantRuns++
		wantSteps += len(f.Run.Steps)
		wantFindings += len(f.Run.Findings)
		for _, l := range telemetry.Build(f.Run).Logs {
			if !l.Time.Before(logCutoff) {
				wantLogs++
			}
		}
	}
	t.Logf("exporting %d runs with %d steps, %d findings and %d log records inside Loki's window", wantRuns, wantSteps, wantFindings, wantLogs)

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", st.otlp)
	exportStart := time.Now()
	runExport := func() string {
		var out, errb bytes.Buffer
		code := cli.Main(ctx, []string{"export", "--grafana-url", st.grafana, examples, generated}, cli.Env{
			Stdout: &out, Stderr: &errb,
			Getenv: func(k string) string {
				if k == "GRAFANA_TOKEN" {
					return token
				}
				return os.Getenv(k)
			},
		})
		if code != cli.ExitOK {
			t.Fatalf("export exited %d: %s", code, errb.String())
		}
		return errb.String()
	}
	summary := runExport()
	t.Logf("export: %s", strings.TrimSpace(summary))
	if !strings.Contains(summary, fmt.Sprintf("sent %d runs: %d spans, %d log records", wantRuns, wantRuns+wantSteps, wantLogs)) {
		t.Errorf("summary %q does not match the records", summary)
	}
	if !strings.Contains(summary, fmt.Sprintf("annotations: %d created, 0 already present", wantRuns)) {
		t.Errorf("summary %q: every annotation should be new", summary)
	}

	t.Run("tempo", func(t *testing.T) {
		// tempo-config.yaml raises ingestion_time_range_slack, so the blocks keep the runs' own
		// time range and a search over that range finds them.
		oldest := exportStart
		for _, f := range files {
			if f.Run.StartedAt.Before(oldest) {
				oldest = f.Run.StartedAt
			}
		}
		start, end := oldest.Add(-time.Hour).Unix(), time.Now().Add(10*time.Minute).Unix()
		for _, f := range files {
			run := f.Run
			q := fmt.Sprintf(`{ span.dummyrun.run.id = "%s" }`, run.ID)
			var res struct {
				Traces []struct {
					TraceID string `json:"traceID"`
				} `json:"traces"`
			}
			eventually(t, 3*time.Minute, func() error {
				if err := st.getJSON(ctx, st.tempo+"/api/search?q="+url.QueryEscape(q)+"&start="+strconv.FormatInt(start, 10)+"&end="+strconv.FormatInt(end, 10), &res); err != nil {
					return err
				}
				if len(res.Traces) != 1 {
					return fmt.Errorf("%d traces for %s", len(res.Traces), run.ID)
				}
				return nil
			})
			want := telemetry.TraceIDFor(run.ID).String()
			// Tempo prints trace IDs without leading zeros.
			if strings.TrimLeft(res.Traces[0].TraceID, "0") != strings.TrimLeft(want, "0") {
				t.Errorf("%s: trace id %s, want %s", run.ID, res.Traces[0].TraceID, want)
			}
			var tr struct {
				Trace struct {
					ResourceSpans []struct {
						ScopeSpans []struct {
							Spans []struct {
								Name              string `json:"name"`
								StartTimeUnixNano string `json:"startTimeUnixNano"`
								EndTimeUnixNano   string `json:"endTimeUnixNano"`
								Events            []any  `json:"events"`
							} `json:"spans"`
						} `json:"scopeSpans"`
					} `json:"resourceSpans"`
				} `json:"trace"`
			}
			if err := st.getJSON(ctx, st.tempo+"/api/v2/traces/"+want, &tr); err != nil {
				t.Fatalf("%s: %v", run.ID, err)
			}
			spans, events := 0, 0
			for _, rs := range tr.Trace.ResourceSpans {
				for _, ss := range rs.ScopeSpans {
					for _, s := range ss.Spans {
						spans++
						if s.Name == "dummyrun.run" {
							events = len(s.Events)
							if s.StartTimeUnixNano != strconv.FormatInt(run.StartedAt.UnixNano(), 10) || s.EndTimeUnixNano != strconv.FormatInt(run.EndedAt.UnixNano(), 10) {
								t.Errorf("%s: root span %s to %s, want the run's own timestamps", run.ID, s.StartTimeUnixNano, s.EndTimeUnixNano)
							}
						}
					}
				}
			}
			if spans != 1+len(run.Steps) || events != len(run.Findings) {
				t.Errorf("%s: %d spans and %d finding events, want %d and %d", run.ID, spans, events, 1+len(run.Steps), len(run.Findings))
			}
		}
		t.Logf("tempo: %d traces found by dummyrun.run.id, each with the right span count and historical timestamps", len(files))
	})

	t.Run("loki", func(t *testing.T) {
		q := url.Values{}
		q.Set("query", `{service_name="dummyrun"}`)
		q.Set("start", strconv.FormatInt(logCutoff.Add(-time.Hour).UnixNano(), 10))
		q.Set("end", strconv.FormatInt(time.Now().Add(time.Hour).UnixNano(), 10))
		q.Set("limit", "5000")
		var res struct {
			Data struct {
				Result []struct {
					Stream map[string]string `json:"stream"`
					Values [][]string        `json:"values"`
				} `json:"result"`
			} `json:"data"`
		}
		lines := 0
		eventually(t, 60*time.Second, func() error {
			if err := st.getJSON(ctx, st.loki+"/loki/api/v1/query_range?"+q.Encode(), &res); err != nil {
				return err
			}
			lines = 0
			for _, r := range res.Data.Result {
				lines += len(r.Values)
			}
			if lines != wantLogs {
				return fmt.Errorf("%d log lines, want %d", lines, wantLogs)
			}
			return nil
		})
		withTrace := 0
		for _, r := range res.Data.Result {
			if len(r.Stream["trace_id"]) == 32 && len(r.Stream["span_id"]) == 16 {
				withTrace += len(r.Values)
			}
		}
		if withTrace != lines {
			t.Errorf("%d of %d log lines carry trace_id and span_id", withTrace, lines)
		}
		t.Logf("loki: %d log lines, all with trace context", lines)
	})

	t.Run("prometheus", func(t *testing.T) {
		sums := map[string]int{
			"sum(dummyrun_runs_total)":                 wantRuns,
			"sum(dummyrun_steps_total)":                wantSteps,
			"sum(dummyrun_findings_total)":             wantFindings,
			"sum(dummyrun_run_duration_seconds_count)": wantRuns,
		}
		for query, want := range sums {
			eventually(t, 60*time.Second, func() error {
				got, err := st.promScalar(ctx, query)
				if err != nil {
					return err
				}
				if got != float64(want) {
					return fmt.Errorf("%s = %v, want %d", query, got, want)
				}
				return nil
			})
		}
		var series struct {
			Data []map[string]string `json:"data"`
		}
		if err := st.getJSON(ctx, st.prom+"/api/v1/series?match[]="+url.QueryEscape(`{__name__=~"dummyrun_.*"}`), &series); err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		for _, s := range series.Data {
			names[s["__name__"]] = true
		}
		for _, want := range []string{"dummyrun_runs_total", "dummyrun_steps_total", "dummyrun_findings_total", "dummyrun_tokens_total",
			"dummyrun_cost_usd_total", "dummyrun_run_duration_seconds_bucket", "dummyrun_step_duration_milliseconds_bucket"} {
			if !names[want] {
				t.Errorf("metric %s missing; have %v", want, keys(names))
			}
		}
		t.Logf("prometheus: %d series across %d metric names: %v", len(series.Data), len(names), keys(names))
	})

	t.Run("annotations", func(t *testing.T) {
		count := func() int {
			var anns []map[string]any
			if err := st.getJSON(ctx, st.grafana+"/api/annotations?tags=dummyrun&limit=5000", &anns); err != nil {
				t.Fatal(err)
			}
			return len(anns)
		}
		if got := count(); got != wantRuns {
			t.Errorf("%d annotations, want %d", got, wantRuns)
		}
		second := runExport()
		if !strings.Contains(second, fmt.Sprintf("annotations: 0 created, %d already present", wantRuns)) {
			t.Errorf("second export summary %q: annotations must be idempotent", second)
		}
		if got := count(); got != wantRuns {
			t.Errorf("%d annotations after a second export, want %d", got, wantRuns)
		}
		t.Logf("grafana: %d region annotations, unchanged by a second export", wantRuns)
	})

	// The rest runs the built binary, so each export is a fresh process with fresh counters.
	bin := filepath.Join(t.TempDir(), "dummyrun-otel")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", bin, "../cmd/dummyrun-otel").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, out)
	}
	runBinary := func(args ...string) (int, string) {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(os.Environ(), "OTEL_EXPORTER_OTLP_ENDPOINT="+st.otlp, "GRAFANA_TOKEN="+token)
		out, err := cmd.CombinedOutput()
		code := 0
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("%s: %v", bin, err)
			}
			code = exit.ExitCode()
		}
		return code, string(out)
	}

	t.Run("second process repeats the export", func(t *testing.T) {
		code, out := runBinary("export", "--grafana-url", st.grafana, "--verify-loki", st.loki, examples, generated)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, out)
		}
		if !strings.Contains(out, fmt.Sprintf("annotations: 0 created, %d already present", wantRuns)) {
			t.Errorf("annotations were not idempotent: %s", out)
		}
		if !strings.Contains(out, "sent 0 runs") || !strings.Contains(out, fmt.Sprintf("all %d runs were already in Loki", wantRuns)) {
			t.Errorf("the second process should have recognised every run as already exported: %s", out)
		}
		// Counters start at zero in every process, so the same records add nothing to the totals.
		time.Sleep(3 * time.Second)
		got, err := st.promScalar(ctx, "sum(dummyrun_runs_total)")
		if err != nil {
			t.Fatal(err)
		}
		if got != float64(wantRuns) {
			t.Errorf("sum(dummyrun_runs_total) = %v after a second export, want %d because each process restarts its counters", got, wantRuns)
		}
		t.Logf("second process: all %d runs recognised as already in Loki and not sent again, annotations unchanged, dummyrun_runs_total still %d", wantRuns, wantRuns)
	})

	t.Run("third process exports an older batch", func(t *testing.T) {
		// A set that ended three days ago is more than an hour behind the newest line already in Loki.
		// Its first run is five days old, so every line stays inside the 168h log window at any time of day.
		// Whether Loki keeps it depends on whether service.instance.id becomes a stream label in this
		// Loki, which differs between environments. Either way the exporter must not lose lines
		// silently: exit 0 means every line arrived, exit 1 means it names every run that is short.
		older := generateRuns(ctx, t, 6, 3, "-old")
		olderFiles, err := validator.Load(older)
		if err != nil {
			t.Fatal(err)
		}
		olderLogs := 0
		for _, f := range olderFiles {
			olderLogs += 1 + len(f.Run.Findings)
		}
		code, out := runBinary("export", "--verify-loki", st.loki, older)
		switch code {
		case 0:
			if !strings.Contains(out, fmt.Sprintf("every one of the %d log records is in Loki", olderLogs)) {
				t.Fatalf("older batch exited 0 without confirming all %d lines: %s", olderLogs, out)
			}
			t.Logf("third process: Loki kept the %d older runs in this process's own stream", len(olderFiles))
		case 1:
			for _, f := range olderFiles {
				if !strings.Contains(out, "run "+f.Run.ID+":") {
					t.Errorf("older batch: run %s not named in the output: %s", f.Run.ID, out)
				}
			}
			t.Logf("third process: Loki dropped the %d older runs and the exporter exited 1 naming each one", len(olderFiles))
		default:
			t.Fatalf("older batch: unexpected exit %d: %s", code, out)
		}
	})
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func startStack(ctx context.Context, t *testing.T) *stack {
	t.Helper()
	img := image
	if v := os.Getenv("DUMMYRUN_E2E_IMAGE"); v != "" {
		img = v
	}
	tempoConfig, err := filepath.Abs("tempo-config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--rm",
		"-v", tempoConfig+":/otel-lgtm/tempo-config.yaml:ro",
		"-p", "127.0.0.1:0:4318", "-p", "127.0.0.1:0:3000", "-p", "127.0.0.1:0:3200", "-p", "127.0.0.1:0:3100", "-p", "127.0.0.1:0:9090",
		img).Output()
	if err != nil {
		t.Fatalf("docker run: %v", err)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", id).Run()
	})
	port := func(p string) string {
		out, err := exec.CommandContext(ctx, "docker", "port", id, p+"/tcp").Output()
		if err != nil {
			t.Fatalf("docker port %s: %v", p, err)
		}
		return "http://" + strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	}
	st := &stack{
		container: id,
		otlp:      port("4318"),
		grafana:   port("3000"),
		tempo:     port("3200"),
		loki:      port("3100"),
		prom:      port("9090"),
		client:    &http.Client{Timeout: 20 * time.Second},
	}
	for _, u := range []string{st.grafana + "/api/health", st.tempo + "/ready", st.loki + "/ready", st.prom + "/-/ready"} {
		eventually(t, 3*time.Minute, func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return err
			}
			resp, err := st.client.Do(req)
			if err != nil {
				return err
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
			}
			return nil
		})
	}
	// The collector accepts an empty OTLP request once it is up.
	eventually(t, time.Minute, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, st.otlp+"/v1/traces", bytes.NewReader(nil))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-protobuf")
		resp, err := st.client.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("otlp: HTTP %d", resp.StatusCode)
		}
		return nil
	})
	t.Logf("stack ready: otlp %s grafana %s tempo %s loki %s prometheus %s", st.otlp, st.grafana, st.tempo, st.loki, st.prom)
	return st
}

// serviceAccountToken creates an Editor service account with the default admin login and returns a
// token for it, which is how a user would set GRAFANA_TOKEN.
func (st *stack) serviceAccountToken(ctx context.Context, t *testing.T) string {
	t.Helper()
	var sa struct {
		ID int64 `json:"id"`
	}
	st.adminPost(ctx, t, "/api/serviceaccounts", `{"name":"dummyrun-otel-e2e","role":"Editor"}`, &sa)
	var tok struct {
		Key string `json:"key"`
	}
	st.adminPost(ctx, t, fmt.Sprintf("/api/serviceaccounts/%d/tokens", sa.ID), `{"name":"e2e"}`, &tok)
	if tok.Key == "" {
		t.Fatal("no service account token")
	}
	return tok.Key
}

func (st *stack) adminPost(ctx context.Context, t *testing.T, path, body string, out any) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, st.grafana+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("admin", "admin")
	req.Header.Set("Content-Type", "application/json")
	resp, err := st.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		t.Fatalf("POST %s: HTTP %d: %s", path, resp.StatusCode, data)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
}

func (st *stack) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth("admin", "admin")
	resp, err := st.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d: %s", u, resp.StatusCode, data)
	}
	return json.Unmarshal(data, out)
}

func (st *stack) promScalar(ctx context.Context, query string) (float64, error) {
	var res struct {
		Data struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := st.getJSON(ctx, st.prom+"/api/v1/query?query="+url.QueryEscape(query), &res); err != nil {
		return 0, err
	}
	if len(res.Data.Result) != 1 || len(res.Data.Result[0].Value) != 2 {
		return 0, fmt.Errorf("%s: no result", query)
	}
	s, _ := res.Data.Result[0].Value[1].(string)
	return strconv.ParseFloat(s, 64)
}

// shiftedExamples copies examples/runs into a temporary directory with every timestamp moved by one
// common offset, so the newest ended_at falls daysAgo days before now at 18:00 UTC. The checked-in
// fixtures stay unchanged and keep their IDs.
func shiftedExamples(t *testing.T, daysAgo int) string {
	t.Helper()
	src := filepath.Join(repoRoot, "examples", "runs")
	names, err := filepath.Glob(filepath.Join(src, "*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("no example records in %s: %v", src, err)
	}
	type rec = map[string]any
	parse := func(v any) time.Time {
		tm, err := time.Parse(time.RFC3339Nano, v.(string))
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	records := make([]rec, 0, len(names))
	var newest time.Time
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var r rec
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		if ended := parse(r["ended_at"]); ended.After(newest) {
			newest = ended
		}
		records = append(records, r)
	}
	target := time.Now().UTC().AddDate(0, 0, -daysAgo).Truncate(24 * time.Hour).Add(18 * time.Hour)
	offset := target.Sub(newest)
	format := func(v any) string { return parse(v).Add(offset).UTC().Format("2006-01-02T15:04:05.000Z07:00") }
	dir := t.TempDir()
	for i, r := range records {
		r["started_at"] = format(r["started_at"])
		r["ended_at"] = format(r["ended_at"])
		for _, st := range r["steps"].([]any) {
			step := st.(map[string]any)
			step["at"] = format(step["at"])
		}
		out, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(names[i])), out, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// generateRuns writes n synthetic runs ending daysAgo days ago with examples/generate-runs.py. Each
// run ID gets the suffix, because the generator names runs by date and persona and a generated ID can
// otherwise collide with an example, which the exporter would then merge into one trace on purpose.
func generateRuns(ctx context.Context, t *testing.T, n, daysAgo int, suffix string) string {
	t.Helper()
	dir := t.TempDir()
	end := time.Now().UTC().AddDate(0, 0, -daysAgo).Format("2006-01-02")
	cmd := exec.CommandContext(ctx, "python3", "-I", filepath.Join(repoRoot, "examples", "generate-runs.py"),
		"--runs", strconv.Itoa(n), "--days", "2", "--end", end, "--seed", "7", "--out", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate-runs.py: %v: %s", err, out)
	}
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(names) != n {
		t.Fatalf("generated %d files, want %d (%v)", len(names), n, err)
	}
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var rec map[string]any
		if err := json.Unmarshal(data, &rec); err != nil {
			t.Fatal(err)
		}
		id := rec["id"].(string) + suffix
		rec["id"] = id
		out, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".json"), out, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(name); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// eventually retries fn until it returns nil or the timeout passes.
func eventually(t *testing.T, timeout time.Duration, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var err error
	for {
		if err = fn(); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("gave up after %s: %v", timeout, err)
		}
		time.Sleep(2 * time.Second)
	}
}
