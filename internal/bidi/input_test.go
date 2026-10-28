package bidi

import (
	"slices"
	"testing"
)

func TestKey(t *testing.T) {
	for name, want := range map[string]string{
		"Enter": KeyEnter, "return": KeyEnter, "ArrowDown": KeyArrowDown, "arrow_down": KeyArrowDown,
		"Page Up": KeyPageUp, "esc": KeyEscape, "a": "a", "7": "7",
	} {
		got, err := Key(name)
		if err != nil || got != want {
			t.Errorf("Key(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := Key("F13"); err == nil {
		t.Error("Key accepted an unknown name")
	}
}

func TestChord(t *testing.T) {
	for name, want := range map[string]struct {
		mods []string
		key  string
	}{
		"Tab":                {nil, KeyTab},
		"Shift+Tab":          {[]string{KeyShift}, KeyTab},
		"shift + tab":        {[]string{KeyShift}, KeyTab},
		"Control+Alt+Delete": {[]string{KeyControl, KeyAlt}, KeyDelete},
		"ctrl+a":             {[]string{KeyControl}, "a"},
		"Cmd+Enter":          {[]string{KeyMeta}, KeyEnter},
		"+":                  {nil, "+"},
		"Shift++":            {[]string{KeyShift}, "+"},
	} {
		mods, key, err := Chord(name)
		if err != nil || key != want.key || !slices.Equal(mods, want.mods) {
			t.Errorf("Chord(%q) = %q, %q, %v; want %q, %q", name, mods, key, err, want.mods, want.key)
		}
	}
	for _, name := range []string{"Hyper+Tab", "Shift+Shift+Tab", "Shift+F13", "+Tab", "Shift+"} {
		if _, _, err := Chord(name); err == nil {
			t.Errorf("Chord(%q) was accepted", name)
		}
	}
}
