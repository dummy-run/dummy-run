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

func TestNoSandbox(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("DUMMYRUN_NO_SANDBOX", "")
	if noSandbox() {
		t.Error("sandbox is off with neither variable set")
	}
	t.Setenv("DUMMYRUN_NO_SANDBOX", "1")
	if !noSandbox() {
		t.Error("DUMMYRUN_NO_SANDBOX does not switch the sandbox off")
	}
	t.Setenv("DUMMYRUN_NO_SANDBOX", "")
	t.Setenv("CI", "true")
	if !noSandbox() {
		t.Error("CI does not switch the sandbox off")
	}
}
