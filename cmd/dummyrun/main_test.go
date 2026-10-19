package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
