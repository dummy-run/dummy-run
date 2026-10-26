package robots

import (
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Paths and patterns come from a small alphabet, so the generator often hits the cases that matter: a shared
// prefix, a "*" in the middle, a "$" at the end and an escape.
var (
	pathGen    = rapid.StringMatching(`/[ab/%2F.]{0,8}`)
	patternGen = rapid.StringMatching(`/[ab/*.]{0,6}\$?`)
)

// patternRegexp is an independent reading of a robots.txt pattern: a prefix match where "*" is any run and a
// final "$" anchors the end.
func patternRegexp(p string) *regexp.Regexp {
	anchored := strings.HasSuffix(p, "$")
	p = strings.TrimSuffix(p, "$")
	parts := strings.Split(p, "*")
	for i := range parts {
		parts[i] = regexp.QuoteMeta(parts[i])
	}
	re := "^" + strings.Join(parts, ".*")
	if anchored {
		re += "$"
	}
	return regexp.MustCompile("(?s)" + re)
}

func TestPropertyMatchesAgreesWithARegexp(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p, s := patternGen.Draw(t, "pattern"), pathGen.Draw(t, "path")
		if got, want := matches(p, s), patternRegexp(p).MatchString(s); got != want {
			t.Fatalf("matches(%q, %q) = %v, the regexp says %v", p, s, got, want)
		}
	})
}

func TestPropertyNoRulesAllowEverything(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := pathGen.Draw(t, "path")
		if !AllowAll().Allowed(s) || !Parse(nil, "dummyrun").Allowed(s) {
			t.Fatalf("%q is disallowed with no rules", s)
		}
		if DenyAll().Allowed(s) != (normalise(s) == "/robots.txt") {
			t.Fatalf("DenyAll gives %v for %q", DenyAll().Allowed(s), s)
		}
		if !DenyAll().Allowed("/robots.txt") {
			t.Fatal("DenyAll disallows /robots.txt")
		}
	})
}

type line struct {
	allow   bool
	pattern string
}

func (l line) String() string {
	if l.allow {
		return "Allow: " + l.pattern
	}
	return "Disallow: " + l.pattern
}

var lineGen = rapid.Custom(func(t *rapid.T) line {
	return line{allow: rapid.Bool().Draw(t, "allow"), pattern: patternGen.Draw(t, "pattern")}
})

func file(lines []line) []byte {
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	for _, l := range lines {
		b.WriteString(l.String() + "\n")
	}
	return []byte(b.String())
}

// Metamorphic relation 1: adding a Disallow rule never lets in a path that was kept out, and adding an Allow rule
// never keeps out a path that was let in.
func TestMetamorphicAddingARuleOnlyMovesOneWay(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lines := rapid.SliceOfN(lineGen, 0, 6).Draw(t, "lines")
		extra := lineGen.Draw(t, "extra")
		s := pathGen.Draw(t, "path")
		before := Parse(file(lines), "dummyrun").Allowed(s)
		after := Parse(file(append(lines, extra)), "dummyrun").Allowed(s)
		if extra.allow && before && !after {
			t.Fatalf("adding %q kept out %q", extra, s)
		}
		if !extra.allow && !before && after {
			t.Fatalf("adding %q let in %q", extra, s)
		}
	})
}

// Metamorphic relation 2: the order of the rules in a group doesn't change any answer, since the longest match
// decides and Allow wins a tie.
func TestMetamorphicRuleOrderDoesNotMatter(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lines := rapid.SliceOfN(lineGen, 0, 6).Draw(t, "lines")
		shuffled := rapid.Permutation(lines).Draw(t, "shuffled")
		s := pathGen.Draw(t, "path")
		if a, b := Parse(file(lines), "dummyrun").Allowed(s), Parse(file(shuffled), "dummyrun").Allowed(s); a != b {
			t.Fatalf("%q: %v in one order and %v in another", s, a, b)
		}
	})
}
