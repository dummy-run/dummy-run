package main

import (
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/provider"
)

func TestReplaceBetween(t *testing.T) {
	doc := "before\n" + providersBegin + "\nold\n" + providersEnd + "\nafter\n"
	got, err := replaceBetween(doc, "new\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "before\n" + providersBegin + "\n\nnew\n\n" + providersEnd + "\nafter\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	again, _ := replaceBetween(got, "new\n")
	if again != got {
		t.Error("a second pass changed the text, so the check would never settle")
	}
	if _, err := replaceBetween("no markers", "x"); err == nil {
		t.Error("missing markers were accepted")
	}
	if _, err := replaceBetween(providersEnd+providersBegin, "x"); err == nil {
		t.Error("reversed markers were accepted")
	}
}

func TestProviderTableHasEveryKind(t *testing.T) {
	table := renderProviderTable()
	for _, kind := range provider.Kinds() {
		if !strings.Contains(table, "| `"+kind+"` |") {
			t.Errorf("no row for %s", kind)
		}
	}
	if !strings.Contains(table, "`--region` decides the endpoint, `eu-west-2` by default") {
		t.Error("the Bedrock region text is missing")
	}
	if !strings.Contains(table, "Every kind supports tool calling") {
		t.Error("the closing sentence is missing")
	}
}

func TestRegionText(t *testing.T) {
	cases := []struct {
		c    provider.Capabilities
		want string
	}{
		{provider.Capabilities{}, "you declare it"},
		{provider.Capabilities{DefaultRegion: "global"}, "`global` unless declared"},
		{provider.Capabilities{RegionSelectable: true}, "`--region` decides the endpoint"},
	}
	for _, c := range cases {
		if got := regionText(c.c); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
