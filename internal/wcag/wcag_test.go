package wcag

import (
	"reflect"
	"testing"
)

func TestCleanKeepsRealCriteriaInOrder(t *testing.T) {
	got := Clean([]string{"WCAG 2.4.6 Headings and Labels", "SC 1.1.1", "2.4.4", "2.4.4", "9.9.9", "4.1.1", "1.4.10"})
	want := []string{"1.1.1", "1.4.10", "2.4.4", "2.4.6"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Clean = %v, want %v", got, want)
	}
	if Clean(nil) != nil {
		t.Fatal("Clean(nil) should be nil")
	}
}

func TestLabel(t *testing.T) {
	if got := Label("2.4.4"); got != "2.4.4 Link Purpose (In Context)" {
		t.Fatalf("Label = %q", got)
	}
	if got := Label("7.7.7"); got != "7.7.7" {
		t.Fatalf("Label of unknown = %q", got)
	}
	if len(criteria) != 86 {
		t.Fatalf("WCAG 2.2 has 86 success criteria without 4.1.1, the table has %d", len(criteria))
	}
}
