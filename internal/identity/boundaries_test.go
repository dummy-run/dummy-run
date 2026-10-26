package identity

import "testing"

// TestCharacterClassEdges checks each character class at the characters either side of its ranges, so a range that
// grows or shrinks by one fails.
func TestCharacterClassEdges(t *testing.T) {
	for _, c := range []struct {
		name string
		f    func(r rune) bool
		in   string
		out  string
	}{
		{"isSpace", isSpace, "\x1c\x1d\x1e\x1f \t\n", "\x1ba!"},
		{"isHex", func(r rune) bool { return isHex(byte(r)) }, "09afAF", "/:`g@G"},
		{"isASCIIAlpha", func(r rune) bool { return isASCIIAlpha(byte(r)) }, "azAZ", "`{@[09"},
	} {
		for _, r := range c.in {
			if !c.f(r) {
				t.Errorf("%s(%q) = false, want true", c.name, r)
			}
		}
		for _, r := range c.out {
			if c.f(r) {
				t.Errorf("%s(%q) = true, want false", c.name, r)
			}
		}
	}
}

// TestSchemeEdges checks which leading words count as a scheme. A scheme starts with a letter, needs at least one
// character before the colon and goes on with letters, digits, plus, minus and full stops only.
func TestSchemeEdges(t *testing.T) {
	for in, want := range map[string]string{
		"a0:/p":    "/p",
		"a9+-.:/p": "/p",
		"az:/p":    "/p",
		"AZ:/p":    "/p",
		":/p":      ":/p",
		"0a:/p":    "0a:/p",
		"a/:x":     "a/:x",
		"a;:x":     "a;:x",
		"a@:x":     "a@:x",
	} {
		if got, _ := split(in); got != want {
			t.Errorf("split(%q) path = %q, want %q", in, got, want)
		}
	}
}

// TestSplitEdges checks the marks split looks for when they come first: leading spaces and control characters go,
// an authority can be empty, and a fragment or query can start the string.
func TestSplitEdges(t *testing.T) {
	for _, c := range []struct{ in, path, query string }{
		{" \x01/p", "/p", ""},
		{"///p", "/p", ""},
		{"#f", "", ""},
		{"?x", "", "x"},
		{"/p?x#f", "/p", "x"},
	} {
		if p, q := split(c.in); p != c.path || q != c.query {
			t.Errorf("split(%q) = %q, %q, want %q, %q", c.in, p, q, c.path, c.query)
		}
	}
}

// TestUnquoteEdges checks percent escapes at the end of a string and next to the edges of the hex digits.
func TestUnquoteEdges(t *testing.T) {
	for in, want := range map[string]string{
		"%41":    "A",
		"x%41":   "xA",
		"%4":     "%4",
		"x%4":    "x%4",
		"%":      "%",
		"%4g":    "%4g",
		"%g4":    "%g4",
		"%7e%7E": "~~",
		"a+b":    "a b",
		"%ff":    "\uFFFD",
	} {
		if got := unquote(in); got != want {
			t.Errorf("unquote(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCleanEdges checks clean keeps lower-case letters, digits and spaces and turns the characters just past them
// into spaces.
func TestCleanEdges(t *testing.T) {
	for in, want := range map[string]string{
		"az09":     "az09",
		"`a{z/0:9": "a z 0 9",
		"AZ":       "az",
		"  a  b  ": "a b",
	} {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}
