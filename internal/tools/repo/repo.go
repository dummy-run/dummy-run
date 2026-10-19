// Package repo has the helpers shared by the documentation tools in internal/tools.
package repo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const modulePath = "module github.com/dummy-run/dummy-run"

// Root finds the repository root, which holds the go.mod of the main module, by walking up from dir.
func Root(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod")) //nolint:gosec // walking up from the working directory is the point
		if err == nil && bytes.Contains(data, []byte(modulePath)) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repo: no go.mod for the dummy-run module above the working directory")
		}
		dir = parent
	}
}

// Binaries are the commands the documentation describes, built from the working tree.
type Binaries struct {
	Dir      string // holds both commands, so it can go on PATH
	DummyRun string
	OTel     string
}

// Build compiles dummyrun and dummyrun-otel into dir. The exporter is its own Go module.
func Build(root, dir string) (*Binaries, error) {
	b := &Binaries{Dir: dir, DummyRun: filepath.Join(dir, "dummyrun"), OTel: filepath.Join(dir, "dummyrun-otel")}
	steps := []struct{ dir, out, pkg string }{
		{root, b.DummyRun, "./cmd/dummyrun"},
		{filepath.Join(root, "integrations", "grafana", "exporter"), b.OTel, "./cmd/dummyrun-otel"},
	}
	for _, s := range steps {
		cmd := exec.Command("go", "build", "-o", s.out, s.pkg) //nolint:gosec // fixed arguments
		cmd.Dir = s.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("repo: go build %s: %w\n%s", s.pkg, err, out)
		}
	}
	return b, nil
}
