package price

import (
	"math"
	"testing"
)

func TestEmbeddedTableIsValid(t *testing.T) {
	tab, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Models) == 0 {
		t.Fatal("empty table")
	}
}

func TestCost(t *testing.T) {
	// claude-sonnet-5-5: 2 in, 0.10 cached, 10 out, per million tokens.
	got := Cost("claude-sonnet-5-5", 1_000_000, 400_000, 100_000)
	want := (600_000*2.0 + 400_000*0.10 + 100_000*10.0) / 1e6
	if got == nil || math.Abs(*got-want) > 1e-9 {
		t.Fatalf("cost %v, want %v", got, want)
	}
}

func TestUnknownModelHasNoCost(t *testing.T) {
	if Cost("my-azure-deployment", 10, 0, 10) != nil {
		t.Fatal("priced an unknown model")
	}
}

func TestCachedWithoutListedPriceCostsInput(t *testing.T) {
	tab, err := Parse([]byte(`{"models":[{"model":"m","input":1,"output":2,"source":"s","retrieved":"2026-01-01"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := tab.Cost("m", 1_000_000, 500_000, 0); got == nil || *got != 1 {
		t.Fatalf("cost %v", got)
	}
}

func TestParseRejectsBadEntries(t *testing.T) {
	for _, in := range []string{
		`{"models":[{"model":"m","input":1,"output":2,"source":"","retrieved":"d"}]}`,
		`{"models":[{"model":"m","input":-1,"output":2,"source":"s","retrieved":"d"}]}`,
		`{"models":[{"model":"m","input":1,"output":2,"source":"s","retrieved":"d"},{"model":"m","input":1,"output":2,"source":"s","retrieved":"d"}]}`,
		`nope`,
	} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("accepted %s", in)
		}
	}
}
