package a11y

import (
	"strings"
	"testing"
)

func TestParseAllowList(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		entries int
		wantErr string
	}{
		{"empty", "", 0, ""},
		{"comments and blanks", "# a note\n\n   \n# another\n", 0, ""},
		{"one entry", "label | /docs/* | .code | Syntax colours are decorative", 1, ""},
		{"reason with a bar", "region | * | * | One | two", 1, ""},
		{"crlf", "region | * | * | reason\r\nlandmark-one-main | /a/ | main | reason\r\n", 2, ""},
		{"too few fields", "link-name | /docs/ | .code", 0, "line 1"},
		{"no reason", "link-name | /docs/ | .code | ", 0, "line 1"},
		{"empty rule", " | /docs/ | .code | why", 0, "rule is empty"},
		{"wildcard rule", "lab* | /docs/ | .code | why", 0, "one axe rule ID"},
		{"empty page", "region |  | .code | why", 0, "page is empty"},
		{"empty selector", "region | * |  | why", 0, "selector is empty"},
		{"error names the line", "# ok\nregion | * | * | why\nbad line", 0, "line 3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, err := ParseAllowList(strings.NewReader(c.in))
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want error containing %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(l.Entries) != c.entries {
				t.Fatalf("want %d entries, got %d", c.entries, len(l.Entries))
			}
		})
	}
}

func TestAllowMatch(t *testing.T) {
	l, err := ParseAllowList(strings.NewReader(strings.Join([]string{
		"label | /docs/* | .code | Syntax colours",
		"region | * | * | Everywhere",
		"image-alt | /about/ | img.logo* | Exact page, glob selector",
	}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		rule, path, sel string
		want            bool
	}{
		{"label", "/docs/a/", "div > .code span", true},
		{"label", "/docs/", "div > .code", true},
		{"label", "/other/", ".code", false},
		{"label", "/docs/a/", "p", false},
		{"region", "/anything", "html", true},
		{"image-alt", "/about/", "img.logo-small", true},
		{"image-alt", "/about/x", "img.logo", false},
		{"image-alt", "/about/", "main img.logo", false}, // a glob selector matches the whole string
		{"link-name", "/docs/", ".code", false},
	}
	for _, c := range cases {
		if got := l.Match(c.rule, c.path, c.sel) != nil; got != c.want {
			t.Errorf("Match(%q, %q, %q) = %v, want %v", c.rule, c.path, c.sel, got, c.want)
		}
	}
	if u := l.Unused(); len(u) != 0 {
		t.Errorf("all entries were used, got %d unused", len(u))
	}
	var nilList *AllowList
	if nilList.Match("region", "/", "html") != nil {
		t.Error("a nil list must accept nothing")
	}
}

func TestAllowUnused(t *testing.T) {
	l, _ := ParseAllowList(strings.NewReader("region | * | * | why\nlink-name | * | * | why"))
	l.Match("region", "/", "html")
	u := l.Unused()
	if len(u) != 1 || u[0].Rule != "link-name" || u[0].Line != 2 {
		t.Fatalf("unexpected unused entries: %+v", u)
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		p, s string
		want bool
	}{
		{"*", "", true}, {"*", "abc", true}, {"abc", "abc", true}, {"abc", "abcd", false},
		{"a*c", "abc", true}, {"a*c", "ac", true}, {"a*c", "a", false}, {"a*b*c", "aXbYc", true},
		{"a*b*c", "acb", false}, {"ab*bc", "abc", false}, {"ab*bc", "abbc", true}, {"**", "x", true},
	}
	for _, c := range cases {
		if got := globMatch(c.p, c.s); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.p, c.s, got, c.want)
		}
	}
}

// FuzzParseAllowList checks that no input makes the parser panic, and that a list it accepts
// holds only complete entries.
func FuzzParseAllowList(f *testing.F) {
	for _, s := range []string{"", "# c", "region | * | * | why", "a | b | c", "x | | | ", "a | b | c | d | e", "\x00 | \xff | * | r"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		l, err := ParseAllowList(strings.NewReader(in))
		if err != nil {
			return
		}
		for _, e := range l.Entries {
			if e.Rule == "" || e.Page == "" || e.Selector == "" || e.Reason == "" {
				t.Fatalf("incomplete entry accepted: %+v", e)
			}
			l.Match(e.Rule, "/x", "html")
		}
	})
}
