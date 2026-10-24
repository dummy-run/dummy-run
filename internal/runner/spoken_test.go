package runner

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/page"
)

func TestSpokenLines(t *testing.T) {
	if got := spokenLines(&page.Snapshot{}); got != "" {
		t.Errorf("a page not read aloud shows %q", got)
	}
	got := spokenLines(&page.Snapshot{Spoken: &page.Spoken{Phrases: []string{"main", "heading, Hi, level 1"}, Complete: true}})
	if want := "screen reader (what it says reading the page from the top, one phrase per line):\n  main\n  heading, Hi, level 1\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got := spokenLines(&page.Snapshot{Spoken: &page.Spoken{Phrases: []string{"main"}}}); !strings.HasSuffix(got, "stopped before the end of the page\n") {
		t.Errorf("an unfinished reading does not say so: %q", got)
	}
	long := make([]string, spokenMax+5)
	for i := range long {
		long[i] = fmt.Sprint("p", i)
	}
	if got := spokenLines(&page.Snapshot{Spoken: &page.Spoken{Phrases: long, Complete: true}}); !strings.HasSuffix(got, "... 5 more phrases\n") {
		t.Errorf("a long reading is not cut: %q", got[len(got)-40:])
	}
	// A long reading that also stopped early says both.
	if got := spokenLines(&page.Snapshot{Spoken: &page.Spoken{Phrases: long}}); !strings.HasSuffix(got, "... 5 more phrases\n  ... the reading stopped before the end of the page\n") {
		t.Errorf("a long unfinished reading: %q", got[len(got)-80:])
	}
	if got := spokenLines(&page.Snapshot{SpokenError: "boom"}); !strings.Contains(got, "could not read the page aloud (boom)") {
		t.Errorf("error: %q", got)
	}
}
