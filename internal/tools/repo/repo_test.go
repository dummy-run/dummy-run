package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootFindsTheModule(t *testing.T) {
	root, err := Root(".")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "dummyrun", "main.go")); err != nil {
		t.Errorf("root %s is not the repository root: %v", root, err)
	}
}

func TestRootFailsOutsideTheModule(t *testing.T) {
	if _, err := Root(t.TempDir()); err == nil {
		t.Error("a directory outside the module gave a root")
	}
}
