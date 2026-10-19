package bidi

import (
	"os"
	"path/filepath"
	"testing"
)

// removeProfile may only delete what the engine made.
func TestRemoveProfileRefusesOtherPaths(t *testing.T) {
	outside := t.TempDir()
	keep := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(outside, chromeProfilePrefix+"nested") // right prefix, wrong parent
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	wrongName, err := os.MkdirTemp("", "other-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(wrongName) })
	for _, p := range []string{"", ".", "/", outside, nested, wrongName, "relative/" + chromeProfilePrefix + "x", filepath.Join(os.TempDir(), "..")} {
		if err := removeProfile(p); err == nil {
			t.Errorf("removeProfile(%q) did not refuse", p)
		}
	}
	for _, p := range []string{keep, nested, wrongName} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed", p)
		}
	}
	good, err := os.MkdirTemp("", firefoxProfilePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := removeProfile(good); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(good); !os.IsNotExist(err) {
		t.Error("an engine profile was not removed")
	}
}
