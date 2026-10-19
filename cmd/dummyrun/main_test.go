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
)

// Asking for help is not an error, so the documented help commands exit 0.
func TestHelpExitsZero(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {"run", "-h"}, {"coverage", "-h"}, {"a11y", "-h"}, {"audit", "a11y", "-h"}, {"version"}} {
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

func TestRequestClientUserAgent(t *testing.T) {
	var agents []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents = append(agents, r.Header.Get("User-Agent"))
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)
		}
	}))
	t.Cleanup(srv.Close)
	for _, identify := range []bool{true, false} {
		agents = nil
		resp, err := requestClient(identify).Get(srv.URL + "/start")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if len(agents) != 2 {
			t.Fatalf("identify=%v: %d requests, want 2 (a redirect)", identify, len(agents))
		}
		for _, ua := range agents {
			if got := strings.Contains(ua, "(+https://dummyrun.dev/scanner/)"); got != identify {
				t.Errorf("identify=%v: user agent %q", identify, ua)
			}
		}
	}
}
