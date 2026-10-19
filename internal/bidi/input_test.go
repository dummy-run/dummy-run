package bidi

import "testing"

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
