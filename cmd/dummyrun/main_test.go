package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dummy-run/dummy-run/internal/telemetry"
)

// Asking for help is not an error, so the documented help commands exit 0.
func TestHelpExitsZero(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {"run", "-h"}, {"coverage", "-h"}, {"report", "-h"}, {"review", "-h"}, {"a11y", "-h"}, {"audit", "a11y", "-h"}, {"version"}} {
		if got := run(args); got != 0 {
			t.Errorf("dummyrun %v exited %d, want 0", args, got)
		}
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{{}, {"nonsense"}, {"run"}, {"run", "--no-such-flag"}} {
		if got := run(args); got != 2 {
			t.Errorf("dummyrun %v exited %d, want 2", args, got)
		}
	}
}

const examples = "../../examples/runs"

func TestCoverageExitCodes(t *testing.T) {
	for name, c := range map[string]struct {
		args []string
		want int
	}{
		"no runs":               {[]string{"coverage"}, 2},
		"mixed builds":          {[]string{"coverage", "--runs", examples}, 2},
		"one build":             {[]string{"coverage", "--runs", examples, "--build", "a865ffd"}, 0},
		"build too short":       {[]string{"coverage", "--runs", examples, "--build", "fc"}, 2},
		"bad format":            {[]string{"coverage", "--runs", examples, "--build", "a865ffd", "--format", "xml"}, 2},
		"bad threshold name":    {[]string{"coverage", "--runs", examples, "--build", "a865ffd", "--threshold", "speed=1"}, 2},
		"bad threshold value":   {[]string{"coverage", "--runs", examples, "--build", "a865ffd", "--threshold", "coverage=2"}, 2},
		"missing directory":     {[]string{"coverage", "--runs", "/no/such/dir"}, 2},
		"bad strata":            {[]string{"coverage", "--runs", examples, "--build", "a865ffd", "--strata", "colour"}, 2},
		"not stopped":           {[]string{"coverage", "--runs", examples, "--build", "a865ffd", "--fail-unless-stopped"}, 3},
		"demonstration":         {[]string{"coverage", "--runs", examples, "--build", "a865ffd", "--confirmed-only=false", "--format", "json"}, 0},
		"missing clusters file": {[]string{"coverage", "--runs", examples, "--build", "a865ffd", "--clusters", "/no/such.json"}, 2},
	} {
		if got := run(c.args); got != c.want {
			t.Errorf("%s: dummyrun %v exited %d, want %d", name, c.args, got, c.want)
		}
	}
}

func TestCoverageWritesJSONToAFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report.json")
	if got := run([]string{"coverage", "--runs", examples, "--build", "a865ffd", "--format", "json", "--out", out}); got != 0 {
		t.Fatalf("exit %d", got)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Build        string `json:"build"`
		Completeness struct {
			Label string `json:"label"`
		} `json:"completeness"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Build != "a865ffd" || !strings.Contains(rep.Completeness.Label, "at least") {
		t.Errorf("%+v", rep)
	}
}

func TestParseThresholds(t *testing.T) {
	th, err := parseThresholds([]string{"coverage=0.8", "min-runs=5", "no-new=2", "kappa=0.7"})
	if err != nil || th.Coverage != 0.8 || th.MinRuns != 5 || th.NoNew != 2 || th.Kappa != 0.7 {
		t.Errorf("%+v %v", th, err)
	}
	for _, bad := range []string{"coverage", "coverage=0", "min-runs=0", "min-runs=x", "other=1"} {
		if _, err := parseThresholds([]string{bad}); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestVersionLineHasVersionAndCommit(t *testing.T) {
	oldV, oldC := telemetry.Version, telemetry.Commit
	t.Cleanup(func() { telemetry.Version, telemetry.Commit = oldV, oldC })
	telemetry.Version, telemetry.Commit = "1.2.3", "abc1234"
	if got, want := versionLine(), "dummyrun 1.2.3 (commit abc1234)"; got != want {
		t.Errorf("versionLine() = %q, want %q", got, want)
	}
}

// denyAllSite serves a robots.txt that keeps DummyRun out and records every request's user agent.
func denyAllSite(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path+" "+r.Header.Get("User-Agent"))
		mu.Unlock()
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, "User-agent: DummyRun\nDisallow: /\n")
			return
		}
		fmt.Fprint(w, "<html><title>hi</title></html>")
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), seen...) }
}

func TestRobotsDisallowExitsThree(t *testing.T) {
	srv, seen := denyAllSite(t)
	jsonOut := filepath.Join(t.TempDir(), "report.json")
	if got := run([]string{"a11y", "--respect-robots", "--identify", "--json", jsonOut, srv.URL + "/page"}); got != 3 {
		t.Errorf("a11y exited %d, want 3", got)
	}
	data, err := os.ReadFile(jsonOut)
	if err != nil || !strings.Contains(string(data), `"skipped"`) {
		t.Errorf("the JSON report should list the skipped page: %v %s", err, data)
	}

	target := filepath.Join(t.TempDir(), "target.json")
	cfg := fmt.Sprintf(`{"environment":"test","base_url":%q,"allowed_hosts":["127.0.0.1"],"read_only":true,"commit":"abcdef1"}`, srv.URL+"/")
	if err := os.WriteFile(target, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if got := run([]string{"run", "--cast", "../../casts/dummyrun-dev", "--persona", "tom-developer", "--scenario", "find-the-quickstart",
		"--target", target, "--out", out, "--provider", "openai-compatible", "--endpoint", "http://127.0.0.1:1/v1", "--region", "local",
		"--model", "none", "--respect-robots"}); got != 3 {
		t.Errorf("run exited %d, want 3", got)
	}
	for _, line := range seen() {
		if strings.HasPrefix(line, "/page") || strings.HasPrefix(line, "/ ") {
			t.Errorf("a disallowed page was requested: %s", line)
		}
	}
}

func TestLoadTargetWithSignin(t *testing.T) {
	tg, err := loadTarget("../../examples/targets/signin.json")
	if err != nil {
		t.Fatal(err)
	}
	if tg.Signin == nil || len(tg.Signin.Credentials) != 3 || tg.Signin.Dir == "" {
		t.Errorf("signin %+v", tg.Signin)
	}
	for _, f := range []string{"../../examples/targets/local.json", "../../examples/targets/dummyrun-dev.json"} {
		if got, err := loadTarget(f); err != nil || got.Signin != nil {
			t.Errorf("%s: %v %v", f, got.Signin, err)
		}
	}
}

func TestRequestClientUserAgent(t *testing.T) {
	var mu sync.Mutex
	var agents []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		agents = append(agents, r.Header.Get("User-Agent"))
		mu.Unlock()
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)
		}
	}))
	t.Cleanup(srv.Close)
	for _, identify := range []bool{true, false} {
		mu.Lock()
		agents = nil
		mu.Unlock()
		resp, err := requestClient(identify).Get(srv.URL + "/start")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		mu.Lock()
		seen := append([]string(nil), agents...)
		mu.Unlock()
		if len(seen) != 2 {
			t.Fatalf("identify=%v: %d requests, want 2 (a redirect)", identify, len(seen))
		}
		for _, ua := range seen {
			if got := strings.Contains(ua, "(+https://dummyrun.dev/scanner/)"); got != identify {
				t.Errorf("identify=%v: user agent %q", identify, ua)
			}
		}
	}
}

func TestReportExitCodes(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report")
	for name, c := range map[string]struct {
		args []string
		want int
	}{
		"nothing given":    {[]string{"report"}, 2},
		"html":             {[]string{"report", "--runs", examples, "--build", "a865ffd", "--out", out}, 0},
		"positional path":  {[]string{"report", "--format", "md", "--build", "a865ffd", "--out", out, examples}, 0},
		"all formats":      {[]string{"report", "--runs", examples, "--include", "pending", "--format", "sarif", "--out", out}, 0},
		"unknown format":   {[]string{"report", "--runs", examples, "--format", "pdf"}, 2},
		"unknown include":  {[]string{"report", "--runs", examples, "--include", "rejected"}, 2},
		"bad date":         {[]string{"report", "--runs", examples, "--date", "yesterday"}, 2},
		"missing records":  {[]string{"report", "--runs", "no-such-dir"}, 2},
		"short build":      {[]string{"report", "--runs", examples, "--build", "ab"}, 2},
		"missing baseline": {[]string{"report", "--runs", examples, "--baseline", "no-such.json"}, 2},
	} {
		if got := run(c.args); got != c.want {
			t.Errorf("%s: dummyrun %v exited %d, want %d", name, c.args, got, c.want)
		}
	}
}

func TestSessionCommandUsage(t *testing.T) {
	if got := run([]string{"session"}); got != 2 {
		t.Errorf("session with no command: %d", got)
	}
	if got := run([]string{"session", "-h"}); got != 0 {
		t.Errorf("session -h: %d", got)
	}
	if got := run([]string{"session", "save"}); got != 2 {
		t.Errorf("session save with no flags: %d", got)
	}
	if got := run([]string{"session", "bogus"}); got != 2 {
		t.Errorf("session bogus: %d", got)
	}
}

func TestReportBaselineRoundTrip(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.md")
	if got := run([]string{"report", "--runs", examples, "--build", "a865ffd", "--format", "json", "--out", first}); got != 0 {
		t.Fatalf("first report exited %d", got)
	}
	if got := run([]string{"report", "--runs", examples, "--build", "1953351", "--format", "md", "--baseline", first, "--out", second}); got != 0 {
		t.Fatalf("second report exited %d", got)
	}
	data, err := os.ReadFile(second) //nolint:gosec // a temporary file this test wrote
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Since the baseline") {
		t.Error("the second report has no baseline section")
	}
}

// The committed example report is regenerated by examples/report/generate.sh. The version is masked so
// a release does not fail this test.
func TestExampleReportIsCurrent(t *testing.T) {
	dir := t.TempDir()
	for _, format := range []string{"html", "json"} {
		out := filepath.Join(dir, "report."+format)
		args := []string{"report", "--runs", examples, "--build", "a865ffd", "--a11y", "../../examples/report/a11y-sweep.json",
			"--include", "pending", "--title", "Inkwell preview audit", "--client", "Inkwell Press (a fictional client)",
			"--date", "2026-10-19", "--format", format, "--out", out}
		if got := run(args); got != 0 {
			t.Fatalf("report exited %d", got)
		}
		fresh, err := os.ReadFile(out) //nolint:gosec // a temporary file this test wrote
		if err != nil {
			t.Fatal(err)
		}
		committed, err := os.ReadFile("../../examples/report/report." + format)
		if err != nil {
			t.Fatal(err)
		}
		if mask(fresh) != mask(committed) {
			t.Errorf("examples/report/report.%s is out of date. Run sh examples/report/generate.sh", format)
		}
	}
}

func mask(b []byte) string { return strings.ReplaceAll(string(b), telemetry.Version, "VERSION") }
