package colour

import (
	"math"
	"testing"
)

func TestRatioMatchesWCAG(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want float64
	}{
		{"#000000", "#ffffff", 21},
		{"#777777", "#ffffff", 4.48},
		{"#767676", "#ffffff", 4.54},
		{"#fff", "#fff", 1},
	} {
		a, _ := Parse(tc.a)
		b, _ := Parse(tc.b)
		if got := Ratio(a, b); math.Abs(got-tc.want) > 0.01 {
			t.Errorf("%s on %s: %.3f, want %.2f", tc.a, tc.b, got, tc.want)
		}
	}
	if _, err := Parse("#12345"); err == nil {
		t.Error("a five-digit colour parsed")
	}
}

// OKLCH round-trips sRGB, so a colour that passes already is kept as it is.
func TestLCHRoundTrip(t *testing.T) {
	for _, s := range []string{"#000000", "#ffffff", "#ff0000", "#0b6e4f", "#d4a017", "#7f7f7f"} {
		c, _ := Parse(s)
		if got := ToLCH(c).ToRGB(); got != c {
			t.Errorf("%s came back as %s", s, got.Hex())
		}
	}
}

// The suggestion passes, keeps the hue, and moves no further than it has to.
func TestNearestPassesAndKeepsHue(t *testing.T) {
	for _, tc := range []struct {
		fg, bg string
		need   float64
	}{
		{"#777777", "#ffffff", 4.5},
		{"#3b82f6", "#ffffff", 4.5},
		{"#22c55e", "#ffffff", 4.5},
		{"#f87171", "#1f2937", 7},
		{"#9ca3af", "#f3f4f6", 3},
	} {
		fg, _ := Parse(tc.fg)
		bg, _ := Parse(tc.bg)
		got, ok := Nearest(fg, bg, tc.need)
		if !ok {
			t.Errorf("%s on %s: no suggestion", tc.fg, tc.bg)
			continue
		}
		if r := Ratio(got, bg); r < tc.need {
			t.Errorf("%s on %s: suggestion %s is %.2f:1, want %.1f", tc.fg, tc.bg, got.Hex(), r, tc.need)
		}
		before, after := ToLCH(fg), ToLCH(got)
		if before.C > 0.02 && math.Abs(math.Remainder(after.H-before.H, 2*math.Pi)) > 0.1 {
			t.Errorf("%s on %s: hue moved from %.2f to %.2f (%s)", tc.fg, tc.bg, before.H, after.H, got.Hex())
		}
		// One step less lightness change would fail: the suggestion is the nearest.
		back := LCH{after.L + (before.L-after.L)*0.02, after.C, after.H}.ToRGB()
		if back != got && Ratio(back, bg) >= tc.need+0.05 {
			t.Errorf("%s on %s: %s isn't the nearest, %s passes too", tc.fg, tc.bg, got.Hex(), back.Hex())
		}
	}
	// #777777 needs only one step: #767676 is the well-known answer on white.
	fg, _ := Parse("#777777")
	if got, _ := Nearest(fg, RGB{255, 255, 255}, 4.5); got.Hex() != "#767676" {
		t.Errorf("#777777 on white: %s, want #767676", got.Hex())
	}
	// Mid grey can't reach 21:1 against anything but black or white.
	if _, ok := Nearest(RGB{128, 128, 128}, RGB{128, 128, 128}, 21); ok {
		t.Error("an impossible ratio had a suggestion")
	}
}
