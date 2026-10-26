package main

import (
	"context"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/a11y"
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
	if !strings.Contains(a11yUsage, "--all             run every check listed below") {
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
