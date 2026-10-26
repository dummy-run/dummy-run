package a11y

import (
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

var (
	globTextGen    = rapid.StringMatching(`[ab/.#]{0,8}`)
	globPatternGen = rapid.StringMatching(`[ab/.#*]{0,6}`)
)

// TestPropertyGlobMatchAgreesWithARegexp checks globMatch against an independent reading of its patterns: the
// whole string must match, with "*" as any run of characters.
func TestPropertyGlobMatchAgreesWithARegexp(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p, s := globPatternGen.Draw(t, "pattern"), globTextGen.Draw(t, "text")
		want := globRegexp(p).MatchString(s)
		if got := globMatch(p, s); got != want {
			t.Fatalf("globMatch(%q, %q) = %v, the regexp says %v", p, s, got, want)
		}
	})
}

var allowEntryGen = rapid.Custom(func(t *rapid.T) *AllowEntry {
	return &AllowEntry{
		Rule:     rapid.SampledFrom([]string{"image-alt", "label", "color-contrast"}).Draw(t, "rule"),
		Page:     "/" + globPatternGen.Draw(t, "page"),
		Selector: rapid.StringMatching(`[ab#.*]{1,5}`).Draw(t, "selector"),
		Reason:   "accepted",
	}
})

// globRegexp is an independent reading of an allow-list pattern: the whole string must match, with "*" as any run.
func globRegexp(p string) *regexp.Regexp {
	parts := strings.Split(p, "*")
	for i := range parts {
		parts[i] = regexp.QuoteMeta(parts[i])
	}
	return regexp.MustCompile("^(?s)" + strings.Join(parts, ".*") + "$")
}

// accepts is an independent reading of what an allow-list accepts. A selector with "*" must match the whole
// element selector, and one without may match any part of it.
func accepts(entries []*AllowEntry, rule, path, sel string) bool {
	for _, e := range entries {
		if e.Rule != rule || !globRegexp(e.Page).MatchString(path) {
			continue
		}
		if strings.Contains(e.Selector, "*") {
			if globRegexp(e.Selector).MatchString(sel) {
				return true
			}
		} else if strings.Contains(sel, e.Selector) {
			return true
		}
	}
	return false
}

// fill replaces each "*" in a pattern with text, so the result is a string the pattern matches.
func fill(t *rapid.T, p, label string) string {
	parts := strings.Split(p, "*")
	for i := 1; i < len(parts); i++ {
		parts[i] = globTextGen.Draw(t, label) + parts[i]
	}
	return strings.Join(parts, "")
}

// Metamorphic relation 3: adding entries to an allow-list never stops it accepting a violation it accepted before,
// and every violation the longer list accepts that the shorter one didn't is accepted by an added entry. Both
// lists must also agree with an independent reading of the patterns. Half the violations are built from an added
// entry, wildcards filled in, so the accepting case comes up often.
func TestMetamorphicAddingAllowEntriesOnlyAccepts(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		base := rapid.SliceOfN(allowEntryGen, 0, 4).Draw(t, "base")
		extra := rapid.SliceOfN(allowEntryGen, 1, 3).Draw(t, "extra")
		rule := rapid.SampledFrom([]string{"image-alt", "label", "color-contrast"}).Draw(t, "violation rule")
		path, sel := "/"+globTextGen.Draw(t, "path"), globTextGen.Draw(t, "violation selector")
		if rapid.Bool().Draw(t, "from an added entry") {
			e := rapid.SampledFrom(extra).Draw(t, "entry")
			rule, path, sel = e.Rule, fill(t, e.Page, "page text"), fill(t, e.Selector, "selector text")
		}
		short := &AllowList{Entries: base}
		long := &AllowList{Entries: append(append([]*AllowEntry{}, base...), extra...)}
		before, after := short.Match(rule, path, sel) != nil, long.Match(rule, path, sel) != nil
		if want := accepts(base, rule, path, sel); before != want {
			t.Fatalf("the list accepts %s on %s at %q: %v, the regexp reading says %v", rule, path, sel, before, want)
		}
		if want := accepts(long.Entries, rule, path, sel); after != want {
			t.Fatalf("the longer list accepts %s on %s at %q: %v, the regexp reading says %v", rule, path, sel, after, want)
		}
		if before && !after {
			t.Fatalf("adding entries stopped accepting %s on %s at %q", rule, path, sel)
		}
		if after && !before && (&AllowList{Entries: extra}).Match(rule, path, sel) == nil {
			t.Fatalf("the longer list accepts %s on %s at %q, but no added entry does", rule, path, sel)
		}
	})
}
