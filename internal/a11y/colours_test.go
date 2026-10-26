package a11y

import (
	"strings"
	"testing"
)

func TestSuggestColours(t *testing.T) {
	// Colours axe couldn't read, or a ratio that any pair meets, give no suggestion at all.
	for _, c := range []axeColours{{FG: "rgb(1,2,3)", BG: "#fff", Need: 4.5}, {FG: "#777", BG: "", Need: 4.5}, {FG: "#777", BG: "#fff", Need: 1}} {
		if got := suggestColours(c); got != nil {
			t.Errorf("%+v: want nil, got %+v", c, got)
		}
	}
	// A missing ratio is worked out from the colours.
	got := suggestColours(axeColours{FG: "#777777", BG: "#ffffff", Need: 4.5})
	if got == nil || got.Ratio < 4.47 || got.Ratio > 4.49 || got.TextFix != "#767676" || got.BackgroundFix == "" {
		t.Fatalf("grey on white: %+v", got)
	}
	if d := got.Detail(); !strings.Contains(d, "#777777 on #ffffff is 4.48:1, and needs 4.5:1.") || !strings.Contains(d, "Text #767676") {
		t.Errorf("detail %q", d)
	}
	// Only black on white reaches 21:1, so against a mid grey neither side has a suggestion.
	mid := suggestColours(axeColours{FG: "#808080", BG: "#7f7f7f", Need: 21})
	if mid == nil || mid.TextFix != "" || mid.BackgroundFix != "" {
		t.Errorf("an impossible ratio: %+v", mid)
	}
	if d := mid.Detail(); strings.Contains(d, "meets it") {
		t.Errorf("detail suggests a colour when there is none: %q", d)
	}
	// Only one side can move far enough.
	one := &Colours{Text: "#111111", Background: "#222222", Ratio: 1.1, Need: 4.5, BackgroundFix: "#888888"}
	if d := one.Detail(); !strings.Contains(d, "The same text on #888888 meets it.") || strings.Contains(d, "Text ") {
		t.Errorf("background only: %q", d)
	}
}
