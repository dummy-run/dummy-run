package bidi

import (
	"os"
	"path/filepath"
	"testing"
)

// firstExisting must never run a bare name out of the working directory: a
// cast repository could carry a file called chromedriver.
func TestFirstExistingIgnoresBareNamesInTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.WriteFile(filepath.Join(dir, "chromedriver-planted"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+"-not-on-path")
	if got := firstExisting("chromedriver-planted"); got != "" {
		t.Errorf("bare name resolved to %q from the working directory", got)
	}
	abs := filepath.Join(dir, "chromedriver-planted")
	if got := firstExisting("", abs); got != abs {
		t.Errorf("absolute path not accepted: %q", got)
	}
	if got := firstExisting(dir); got != "" {
		t.Errorf("a directory was accepted: %q", got)
	}
	t.Setenv("PATH", dir)
	if got := firstExisting("chromedriver-planted"); got != abs {
		t.Errorf("name on PATH not found: %q", got)
	}
}

func TestChromeOptionsPrefs(t *testing.T) {
	prefs := map[string]any{"webkit": map[string]any{"webprefs": map[string]any{"default_font_size": 32}}}
	got := chromeOptions([]string{"--x"}, "/chrome", prefs)
	if _, ok := got["prefs"]; !ok {
		t.Fatalf("prefs missing from %v", got)
	}
	if got["binary"] != "/chrome" {
		t.Errorf("binary = %v", got["binary"])
	}
	for _, empty := range []map[string]any{nil, {}} {
		got = chromeOptions([]string{"--x"}, "", empty)
		if _, ok := got["prefs"]; ok {
			t.Errorf("prefs present for an empty map: %v", got)
		}
		if _, ok := got["binary"]; ok {
			t.Errorf("binary present for an empty path: %v", got)
		}
	}
}
