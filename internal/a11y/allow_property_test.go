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
		parts := strings.Split(p, "*")
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		want := regexp.MustCompile("^(?s)" + strings.Join(parts, ".*") + "$").MatchString(s)
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

// Metamorphic relation 3: adding entries to an allow-list never stops it accepting a violation it accepted before,
// and every violation the longer list accepts that the shorter one didn't is accepted by an added entry.
func TestMetamorphicAddingAllowEntriesOnlyAccepts(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		base := rapid.SliceOfN(allowEntryGen, 0, 4).Draw(t, "base")
		extra := rapid.SliceOfN(allowEntryGen, 1, 3).Draw(t, "extra")
		rule := rapid.SampledFrom([]string{"image-alt", "label", "color-contrast"}).Draw(t, "violation rule")
		path, sel := "/"+globTextGen.Draw(t, "path"), globTextGen.Draw(t, "violation selector")
		short := &AllowList{Entries: base}
		long := &AllowList{Entries: append(append([]*AllowEntry{}, base...), extra...)}
		before, after := short.Match(rule, path, sel) != nil, long.Match(rule, path, sel) != nil
		if before && !after {
			t.Fatalf("adding entries stopped accepting %s on %s at %q", rule, path, sel)
		}
		if after && !before && (&AllowList{Entries: extra}).Match(rule, path, sel) == nil {
			t.Fatalf("the longer list accepts %s on %s at %q, but no added entry does", rule, path, sel)
		}
	})
}
