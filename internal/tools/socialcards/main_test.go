package main

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateThenCheckPasses(t *testing.T) {
	dir := t.TempDir()
	if err := run(dir, false); err != nil {
		t.Fatal(err)
	}
	for _, c := range cards {
		if _, err := os.Stat(filepath.Join(dir, c.Name+".png")); err != nil {
			t.Errorf("card %s was not written: %v", c.Name, err)
		}
	}
	if err := run(dir, true); err != nil {
		t.Errorf("fresh cards failed the check: %v", err)
	}
}

func TestCheckFailsForMissingCards(t *testing.T) {
	dir := t.TempDir()
	err := run(dir, true)
	if err == nil || !strings.Contains(err.Error(), "home.png") {
		t.Errorf("got %v, want an error that names the missing card", err)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil || len(entries) != 0 {
		t.Errorf("the check wrote %d files", len(entries))
	}
}

func TestCheckFailsForAStaleCard(t *testing.T) {
	dir := t.TempDir()
	if err := run(dir, false); err != nil {
		t.Fatal(err)
	}
	// Overwrite one card with another one's picture.
	data, err := os.ReadFile(filepath.Join(dir, "docs.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "home.png"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(dir, true); err == nil || !strings.Contains(err.Error(), "home.png") {
		t.Errorf("got %v, want the stale card named", err)
	}
}

func TestCheckFailsAfterASmallBrandColourChange(t *testing.T) {
	dir := t.TempDir()
	if err := run(dir, false); err != nil {
		t.Fatal(err)
	}
	old := amber
	defer func() { amber = old }()
	// A change of 10 levels is inside the per-pixel tolerance, but it moves the whole ground.
	amber = color.RGBA{amber.R - 10, amber.G - 10, amber.B - 10, 0xff}
	if err := run(dir, true); err == nil {
		t.Error("the check accepted cards drawn with another brand colour")
	}
}
