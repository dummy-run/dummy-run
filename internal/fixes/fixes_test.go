package fixes

import (
	"slices"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/validate"
)

// The rules Dummy Run's own checks report. Each needs an entry.
var ownRules = []string{
	"dummyrun-reflow", "dummyrun-overlap", "dummyrun-clipped-text", "dummyrun-target-size", "dummyrun-focus-obscured",
	"dummyrun-focus-not-visible", "dummyrun-focus-hidden", "dummyrun-keyboard-trap", "dummyrun-skip-link-broken",
	"dummyrun-mouse-only", "dummyrun-skip-link-no-focus",
	"dummyrun-label-in-name", "dummyrun-autocomplete-missing", "dummyrun-contrast-over-image", "dummyrun-consent-no-reject",
	"dummyrun-plain-reading-ease", "dummyrun-plain-reading-grade", "dummyrun-plain-long-sentence",
	"dummyrun-plain-words-to-avoid", "dummyrun-plain-latin", "dummyrun-engines-1.1.1",
	"dummyrun-html-broken-reference", "dummyrun-html-duplicate-id", "dummyrun-html-nested-interactive",
	"dummyrun-html-aria", "dummyrun-html-structure", "dummyrun-html-img-alt",
	"dummyrun-lighthouse-paste-preventing-inputs",
	"dummyrun-journey-keyboard",
}

func TestEveryOwnRuleHasAFix(t *testing.T) {
	for _, r := range ownRules {
		if For(r) == nil {
			t.Errorf("no fix for %s", r)
		}
	}
	for _, r := range validate.Rules {
		if For(r.ID) == nil {
			t.Errorf("no fix for the HTML check's %s", r.ID)
		}
	}
	if For("image-alt") != nil {
		t.Errorf("an axe rule got one of our entries")
	}
	if f := For("dummyrun-engines-4.1.2"); f == nil || f.Rule != "dummyrun-engines-" {
		t.Errorf("an engines rule should get the engines entry, got %+v", f)
	}
}

func TestEntriesAreComplete(t *testing.T) {
	for _, r := range Rules() {
		f := For(r)
		if !slices.Contains(Owners, f.Owner) {
			t.Errorf("%s: owner %q is not one of %v", r, f.Owner, Owners)
		}
		if f.Summary == "" || len(f.Steps) == 0 {
			t.Errorf("%s: needs a summary and at least one step", r)
		}
		for _, s := range append([]string{f.Summary}, f.Steps...) {
			if !strings.HasSuffix(s, ".") || strings.Contains(s, "—") {
				t.Errorf("%s: %q should be a sentence ending in a full stop, with no em dash", r, s)
			}
		}
		for _, u := range f.Sources {
			if !strings.HasPrefix(u, "https://") {
				t.Errorf("%s: source %q is not an https link", r, u)
			}
		}
		if f.Example != nil && (f.Example.Before == "" || f.Example.After == "") {
			t.Errorf("%s: an example needs both before and after", r)
		}
	}
}

func TestLoadRefusesADuplicate(t *testing.T) {
	_, err := load([]byte("- rule: a\n  owner: content\n- rule: a\n  owner: design\n"))
	if err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("want a duplicate error, got %v", err)
	}
}
