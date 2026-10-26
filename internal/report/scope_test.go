package report

import (
	"slices"
	"testing"

	"github.com/dummy-run/dummy-run/internal/record"
)

// TestScopeDescribesEachPersonaRun checks how the scope words a persona's device and dimensions, and that it
// counts each outcome. A setting at its default, such as text at 100 percent or no assistive technology, isn't
// named.
func TestScopeDescribesEachPersonaRun(t *testing.T) {
	str := func(s string) *string { return &s }
	run := func(p record.Persona, outcome string) *Run {
		return &Run{Rec: &record.Record{Persona: p, Outcome: outcome}}
	}
	plain := &record.Dimensions{AT: str("none"), Connection: str("unthrottled"), Motion: str("no-preference")}
	varied := &record.Dimensions{Viewport: &record.Viewport{Width: 390, Height: 844}, Locale: str("en-GB"),
		AT: str("voiceover"), Connection: str("slow-3g"), Motion: str("reduce")}
	sc := buildScope([]*Run{
		run(record.Persona{ID: "a", Device: "phone", Zoom: 100, TextScale: 100, Dimensions: plain}, record.OutcomeCompleted),
		run(record.Persona{ID: "a", Device: "phone", TextScale: 101}, record.OutcomeBlocked),
		run(record.Persona{ID: "b", Device: "desktop", Zoom: 200, Dimensions: varied}, record.OutcomeErrored),
		run(record.Persona{ID: "b", Device: "desktop"}, record.OutcomeBudgetExhausted),
		run(record.Persona{ID: "b", Device: "desktop"}, record.OutcomeBudgetExhausted),
	})
	if c := sc.Runs; c.Total != 5 || c.Completed != 1 || c.Blocked != 1 || c.Errored != 1 || c.BudgetExhausted != 2 {
		t.Errorf("runs %+v", c)
	}
	if len(sc.Personas) != 2 {
		t.Fatalf("personas %+v", sc.Personas)
	}
	a, b := sc.Personas[0], sc.Personas[1]
	if want := []string{"phone", "phone with text at 101 percent"}; !slices.Equal(a.Devices, want) || len(a.Dimensions) != 0 {
		t.Errorf("a: devices %q, dimensions %q", a.Devices, a.Dimensions)
	}
	if want := []string{"desktop", "desktop at 200 percent zoom"}; !slices.Equal(b.Devices, want) {
		t.Errorf("b: devices %q", b.Devices)
	}
	want := []string{"assistive technology voiceover", "connection slow-3g", "locale en-GB", "motion reduce", "viewport 390x844"}
	if !slices.Equal(b.Dimensions, want) {
		t.Errorf("b: dimensions %q, want %q", b.Dimensions, want)
	}
}
