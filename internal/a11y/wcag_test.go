package a11y

import (
	"slices"
	"testing"
)

func TestEN301549(t *testing.T) {
	got := EN301549([]string{"1.4.3", "2.5.8", "4.1.2", "1.4.3", "9.9.9", "1.4.10"})
	if want := []string{"9.1.4.3", "9.4.1.2", "9.1.4.10"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Every A and AA criterion WCAG 2.1 has maps to a clause, and the six WCAG 2.2 added map to none.
	n := 0
	for _, c := range CriteriaAA() {
		if len(EN301549([]string{c})) == 1 {
			n++
		}
	}
	if n != len(CriteriaAA())-6 {
		t.Errorf("%d of %d criteria have a clause", n, len(CriteriaAA()))
	}
}
