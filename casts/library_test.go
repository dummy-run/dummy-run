package casts_test

import (
	"os"
	"testing"

	"github.com/dummy-run/dummy-run/casts"
	"github.com/dummy-run/dummy-run/internal/cast"
)

// TestLargeTextPhone loads the large-text-phone persona from the built-in library and checks the
// settings it exists for, and the keys the website's persona library requires of every persona.
func TestLargeTextPhone(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, casts.Library); err != nil {
		t.Fatal(err)
	}
	p, err := cast.Load(dir+"/library", "large-text-phone")
	if err != nil {
		t.Fatal(err)
	}
	if p.Device != "phone" || p.TextScale != 200 || p.Zoom != 0 {
		t.Errorf("persona %+v, want a phone at text scale 200 and no zoom", p)
	}
	if len(p.Scenarios) != 4 {
		t.Errorf("%d scenarios, want 4", len(p.Scenarios))
	}
	d := p.DisplayFor(p.Scenarios[0])
	if d.TextScale != 200 || d.Zoom != 100 || d.CSSWidth != 390 {
		t.Errorf("display %+v, want text 200, no zoom and the phone's 390 px", d)
	}
	data, err := os.ReadFile(dir + "/library/large-text-phone/profile.md")
	if err != nil {
		t.Fatal(err)
	}
	meta, _, err := cast.FrontMatter(string(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"name", "label", "need", "summary", "finds", "limit", "device", "role"} {
		if meta[key] == "" {
			t.Errorf("profile.md lacks %q, which the persona library requires", key)
		}
	}
	if meta["need"] != "vision" {
		t.Errorf("need %q, want vision", meta["need"])
	}
}
