package runner

import (
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/provider"
)

func TestRenderShowsFocusTreeAndZoom(t *testing.T) {
	s := sampleStep(2, "press", "")
	s.Obs.Snapshot.Focus = &page.Focus{Role: "link", Name: "Docs", Ref: "1", Tag: "a"}
	s.Obs.Snapshot.Tree = "banner\n  link \"Docs\" @1  <- focus\nmain\n  heading 1 \"Welcome\""
	s.Obs.Snapshot.Zoom = 200
	s.Obs.Snapshot.DevicePixelRatio = 2
	out := Render(s)
	for _, want := range []string{
		`focus: link "Docs" @1`,
		"accessibility tree (roles and names computed from the page; @n matches the controls):\n  banner\n    link \"Docs\" @1  <- focus\n  main",
		"zoom: 200% (the page lays out in 1400 x 1000 CSS px, device pixel ratio 2)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}
	s.Obs.Snapshot.Focus = nil
	if !strings.Contains(Render(s), "focus: nothing") {
		t.Error("no focus is not said")
	}
	s.Obs.Snapshot.Focus = &page.Focus{Role: "textbox", Tag: "input"}
	if !strings.Contains(Render(s), "focus: textbox (no accessible name)\n") {
		t.Errorf("an unnamed focus is not flagged:\n%s", Render(s))
	}
	s.Obs.Snapshot.Zoom = 100
	if strings.Contains(Render(s), "zoom:") {
		t.Error("a page at 100 percent shows a zoom line")
	}
}

func TestSignatureSeesFocusMoving(t *testing.T) {
	a := sampleStep(1, "press", "")
	b := sampleStep(2, "press", "")
	a.Obs.Snapshot.Focus = &page.Focus{Role: "link", Name: "Docs", Ref: "1"}
	b.Obs.Snapshot.Focus = &page.Focus{Role: "button", Name: "Go", Ref: "2"}
	if Signature(a.Obs) == Signature(b.Obs) {
		t.Error("Tab moved focus but the step counts as nothing changing, so the persona would be told to stop and give up")
	}
}

func TestRefusedStepRendersAsRefused(t *testing.T) {
	s := sampleStep(4, "click", "click is not one of the tools kit may use, so nothing was done. Use another tool.")
	s.Refusal = s.Obs.Error
	out := Render(s)
	if !strings.Contains(out, "REFUSED: click is not one of the tools kit may use") || strings.Contains(out, "ACTION FAILED") {
		t.Errorf("render:\n%s", out)
	}
}

func TestRefusal(t *testing.T) {
	r := &run{cfg: Config{Persona: &cast.Persona{ID: "kit", ToolsDenied: []string{"click"}}}}
	if got := r.refusal("click"); !strings.Contains(got, "click is not one of the tools kit may use") {
		t.Errorf("refusal %q", got)
	}
	for _, tool := range []string{"press", "look", "finish", "no-such-tool"} {
		if got := r.refusal(tool); got != "" {
			t.Errorf("%s refused: %q", tool, got)
		}
	}
	r.opening = true
	if got := r.refusal("click"); got != "" {
		t.Errorf("the harness's own opening step was refused: %q", got)
	}
}

func TestSystemPromptNamesRefusedToolsAndZoom(t *testing.T) {
	r := &run{cfg: Config{Persona: &cast.Persona{ID: "kit", ToolsDenied: []string{"click", "screenshot"}}}, budget: 20,
		display: cast.Display{Device: "tablet", Zoom: 200, CSSWidth: 410, CSSHeight: 590}}
	p := r.systemPrompt()
	for _, want := range []string{
		"You may not use these tools: click, screenshot.",
		"records a blocked step",
		"200 percent zoom on a tablet-sized window, so pages lay out in 410 x 590 CSS pixels",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	r = &run{cfg: Config{Persona: &cast.Persona{ID: "kit"}}, budget: 20, display: cast.Display{Device: "desktop", Zoom: 100}}
	if p := r.systemPrompt(); strings.Contains(p, "may not use") || strings.Contains(p, "percent zoom") {
		t.Error("a persona without limits or zoom was told about them")
	}
}

func TestScreenshotsWithheldWhenTheToolIsDenied(t *testing.T) {
	build := func(p *cast.Persona) []provider.Turn {
		step := &Step{N: 1, PageChanged: true, Obs: page.Observation{Action: "visit", URL: "https://t.test/", Screenshot: []byte("png"), Snapshot: &page.Snapshot{}}}
		r := &run{budget: 25, cfg: Config{Persona: p}, entries: []entry{{role: provider.RoleUser, text: "Begin.", step: step}}}
		return r.turns()
	}
	if got := build(&cast.Persona{}); len(got[0].Images) != 1 {
		t.Error("a persona without limits lost its screenshot")
	}
	if got := build(&cast.Persona{ToolsDenied: []string{"screenshot"}}); len(got[0].Images) != 0 {
		t.Error("a persona that may not use screenshot was shown one anyway")
	}
	if got := build(&cast.Persona{ToolsAllowed: []string{"look", "press"}}); len(got[0].Images) != 0 {
		t.Error("a persona limited to look and press was shown a screenshot")
	}
}
