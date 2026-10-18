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
