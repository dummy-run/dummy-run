package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/plain"
)

// TestAllNamesEveryCheck keeps --all honest: every check flag in the help is one --all turns on.
func TestAllNamesEveryCheck(t *testing.T) {
	for _, flag := range []string{"--consent", "--engines", "--html", "--image-contrast", "--journeys", "--keyboard", "--layout", "--lighthouse", "--links", "--markup", "--content", "--plain-language"} {
		i := strings.Index(a11yUsage, "\n  "+flag+" ")
		if i < 0 {
			t.Fatalf("%s is not in the help", flag)
		}
		if line := a11yUsage[i+1 : i+1+strings.Index(a11yUsage[i+1:], "\n")]; !strings.Contains(line, " also ") {
			t.Errorf("%s does not start with \"also\": %q", flag, line)
		}
	}
	if !strings.Contains(a11yUsage, "--all             run every check listed below except --pdf") {
		t.Error("--all is not described")
	}
}

// TestAllTurnsOnEveryCheck runs the command with a stand-in sweep and checks what --all asks for.
func TestAllTurnsOnEveryCheck(t *testing.T) {
	var got a11y.Config
	a11yRun = func(_ context.Context, cfg a11y.Config) (*a11y.Report, error) {
		got = cfg
		return &a11y.Report{}, nil
	}
	t.Cleanup(func() { a11yRun = a11y.Run })
	checks := func(c a11y.Config) []bool {
		return []bool{c.Layout, c.Keyboard, c.Markup, c.Content, c.Engines, c.Consent, c.PlainLanguage, c.HTML, c.Lighthouse, c.Journeys, c.Links, c.ImageContrast}
	}
	if code := a11yCmd([]string{"https://example.com/"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for i, on := range checks(got) {
		if on {
			t.Errorf("check %d is on without --all", i)
		}
	}
	if code := a11yCmd([]string{"--all", "https://example.com/"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for i, on := range checks(got) {
		if !on {
			t.Errorf("check %d is off with --all", i)
		}
	}
	if got.PDF {
		t.Error("--all turned on --pdf, which downloads files")
	}
}

// TestLayoutAddsTheReflowSettings checks the layout checks bring in the 320 px window and 200 percent text unless
// you chose your own.
func TestLayoutAddsTheReflowSettings(t *testing.T) {
	var got a11y.Config
	a11yRun = func(_ context.Context, cfg a11y.Config) (*a11y.Report, error) {
		got = cfg
		return &a11y.Report{}, nil
	}
	t.Cleanup(func() { a11yRun = a11y.Run })
	names := func(c a11y.Config) string {
		var n []string
		for _, v := range c.Viewports {
			n = append(n, v.Name)
		}
		return strings.Join(n, ",")
	}
	if a11yCmd([]string{"--layout", "https://example.com/"}); names(got) != "desktop,phone,small" || len(got.TextScales) != 2 || got.TextScales[1] != 200 {
		t.Errorf("--layout: viewports %s, scales %v", names(got), got.TextScales)
	}
	if a11yCmd([]string{"--layout", "--viewport", "phone", "--text-scale", "150", "https://example.com/"}); names(got) != "phone" || len(got.TextScales) != 1 || got.TextScales[0] != 150 {
		t.Errorf("chosen settings were changed: viewports %s, scales %v", names(got), got.TextScales)
	}
	if a11yCmd([]string{"https://example.com/"}); names(got) != "desktop,phone" || len(got.TextScales) != 0 {
		t.Errorf("without --layout: viewports %s, scales %v", names(got), got.TextScales)
	}
}

// TestVisualFlagsReachTheSweep checks --visual and each --mask reach the sweep, and that --all leaves --visual off.
func TestVisualFlagsReachTheSweep(t *testing.T) {
	var got a11y.Config
	a11yRun = func(_ context.Context, cfg a11y.Config) (*a11y.Report, error) {
		got = cfg
		return &a11y.Report{}, nil
	}
	t.Cleanup(func() { a11yRun = a11y.Run })
	if code := a11yCmd([]string{"https://example.com/", "--visual", "screens", "--mask", ".clock", "--mask", "#ad"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got.VisualDir != "screens" || strings.Join(got.Masks, " ") != ".clock #ad" {
		t.Errorf("visual %q, masks %q", got.VisualDir, got.Masks)
	}
	if code := a11yCmd([]string{"--all", "https://example.com/"}); code != 0 || got.VisualDir != "" {
		t.Errorf("--all: exit %d, visual %q", code, got.VisualDir)
	}
}

// TestA11yWritesTestReports checks --junit and --ctrf write each failing rule as a failed test.
func TestA11yWritesTestReports(t *testing.T) {
	a11yRun = func(_ context.Context, cfg a11y.Config) (*a11y.Report, error) {
		return &a11y.Report{Browser: "chrome", AxeVersion: "4.13.0", StartedAt: "2026-10-26T10:00:00Z", EndedAt: "2026-10-26T10:01:30Z", Summary: a11y.Summary{Failing: 1}, Results: []a11y.PageResult{{URL: cfg.URLs[0], Viewport: "desktop", Scheme: "light",
			Violations: []a11y.Finding{{Rule: "image-alt", Impact: "critical", WCAG: []string{"1.1.1"}, Help: "Images must have alternative text", Nodes: []a11y.Node{{Target: "#logo"}}}}}}}, nil
	}
	t.Cleanup(func() { a11yRun = a11y.Run })
	dir := t.TempDir()
	junit, ctrf := filepath.Join(dir, "a11y.xml"), filepath.Join(dir, "a11y.ctrf.json")
	if code := a11yCmd([]string{"--junit", junit, "--ctrf", ctrf, "https://example.com/"}); code != 1 {
		t.Fatalf("exit %d, want 1 for a failing rule", code)
	}
	x, err := os.ReadFile(junit)
	if err != nil || !strings.Contains(string(x), `classname="axe.image-alt"`) || !strings.Contains(string(x), "<failure") {
		t.Errorf("junit: %v\n%s", err, x)
	}
	var doc struct {
		ReportFormat string `json:"reportFormat"`
		Results      struct {
			Summary struct {
				Tests, Failed int
				Start, Stop   int64
			} `json:"summary"`
		} `json:"results"`
	}
	b, err := os.ReadFile(ctrf)
	if err != nil || json.Unmarshal(b, &doc) != nil || doc.ReportFormat != "CTRF" || doc.Results.Summary.Tests != 1 || doc.Results.Summary.Failed != 1 ||
		doc.Results.Summary.Start != 1793008800000 || doc.Results.Summary.Stop != 1793008890000 {
		t.Errorf("ctrf: %v %+v\n%s", err, doc, b)
	}
}

// TestA11yRefusesSharedOutputs checks two outputs can't write the same file.
func TestA11yRefusesSharedOutputs(t *testing.T) {
	ran := false
	a11yRun = func(context.Context, a11y.Config) (*a11y.Report, error) { ran = true; return &a11y.Report{}, nil }
	t.Cleanup(func() { a11yRun = a11y.Run })
	f := filepath.Join(t.TempDir(), "out")
	for _, args := range [][]string{{"--junit", f, "--ctrf", f}, {"--json", f, "--ctrf", filepath.Dir(f) + "/./out"}} {
		ran = false
		if code := a11yCmd(append(args, "https://example.com/")); code != 2 || ran {
			t.Errorf("%v: exit %d, sweep ran %v", args, code, ran)
		}
	}
	if code := a11yCmd([]string{"--json", "-", "--ctrf", f, "https://example.com/"}); code != 0 {
		t.Errorf("--json - with a file: exit %d", code)
	}
}

// TestARIASnapshotsFlagReachesTheSweep checks --aria-snapshots passes its directory to the sweep unchanged.
func TestARIASnapshotsFlagReachesTheSweep(t *testing.T) {
	var got a11y.Config
	a11yRun = func(_ context.Context, cfg a11y.Config) (*a11y.Report, error) {
		got = cfg
		return &a11y.Report{}, nil
	}
	t.Cleanup(func() { a11yRun = a11y.Run })
	dir := t.TempDir()
	if code := a11yCmd([]string{"--aria-snapshots", dir, "https://example.com/"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got.ARIASnapshots != dir {
		t.Errorf("ARIASnapshots = %q, want %q", got.ARIASnapshots, dir)
	}
	if a11yCmd([]string{"https://example.com/"}); got.ARIASnapshots != "" {
		t.Errorf("without the flag, ARIASnapshots = %q", got.ARIASnapshots)
	}
}

// TestRepeatReachesTheSweep checks --repeat reaches the sweep, and that it sweeps once without it.
func TestRepeatReachesTheSweep(t *testing.T) {
	var got a11y.Config
	a11yRun = func(_ context.Context, cfg a11y.Config) (*a11y.Report, error) {
		got = cfg
		return &a11y.Report{}, nil
	}
	t.Cleanup(func() { a11yRun = a11y.Run })
	if code := a11yCmd([]string{"https://example.com/"}); code != 0 || got.Repeat != 1 {
		t.Errorf("without --repeat: exit %d, repeat %d", code, got.Repeat)
	}
	if code := a11yCmd([]string{"https://example.com/", "--repeat", "3"}); code != 0 || got.Repeat != 3 {
		t.Errorf("--repeat 3: exit %d, repeat %d", code, got.Repeat)
	}
}

// TestA11yRemovesTheValeRules checks the command removes the plain language rules it wrote, whether the sweep
// succeeds or fails.
func TestA11yRemovesTheValeRules(t *testing.T) {
	for _, fail := range []bool{false, true} {
		tmp := t.TempDir()
		// os.TempDir reads TMPDIR on Unix and TMP or TEMP on Windows.
		for _, v := range []string{"TMPDIR", "TMP", "TEMP"} {
			t.Setenv(v, tmp)
		}
		wrote := false
		a11yRun = func(ctx context.Context, _ a11y.Config) (*a11y.Report, error) {
			// Check writes the rules before it looks for Vale, so a missing Vale still leaves them behind.
			_, _ = plain.Check(ctx, filepath.Join(tmp, "no-vale"), "<p>Words.</p>")
			m, _ := filepath.Glob(filepath.Join(tmp, "dummyrun-vale-*"))
			wrote = len(m) == 1
			if fail {
				return nil, errors.New("the sweep failed")
			}
			return &a11y.Report{}, nil
		}
		a11yCmd([]string{"--plain-language", "https://example.com/"})
		a11yRun = a11y.Run
		if !wrote {
			t.Fatalf("fail %v: the stand-in sweep wrote no rules", fail)
		}
		if m, _ := filepath.Glob(filepath.Join(tmp, "dummyrun-vale-*")); len(m) != 0 {
			t.Errorf("fail %v: the rules are still there: %v", fail, m)
		}
	}
}
