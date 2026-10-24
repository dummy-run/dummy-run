// Package plain checks how plainly a page is written, with Vale and a small rule set of our own written from the
// public GOV.UK style guide, plus two readability scores from Vale's Readability style. Vale runs as a separate
// program. No model is involved.
package plain

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
)

// ValeVersion is the Vale release the container image ships and the docs name. Other 3.x releases read the same
// rules.
const ValeVersion = "3.24.0"

//go:embed all:styles
var styles embed.FS

// Alert is one thing Vale found.
type Alert struct {
	Check    string `json:"Check"`
	Message  string `json:"Message"`
	Match    string `json:"Match"`
	Link     string `json:"Link"`
	Severity string `json:"Severity"`
	Line     int    `json:"Line"`
}

// ErrNoVale means no Vale program was found.
var ErrNoVale = errors.New("plain language: vale not found; install it or set DUMMYRUN_VALE (see docs/ENGINE.md)")

// Find returns the Vale program to run: DUMMYRUN_VALE when set, otherwise vale on the PATH.
func Find() (string, error) {
	if p := os.Getenv("DUMMYRUN_VALE"); p != "" {
		// LookPath checks a path with a slash in it directly, and that it can be run.
		found, err := exec.LookPath(p)
		if err != nil {
			return "", fmt.Errorf("plain language: DUMMYRUN_VALE: %w", err)
		}
		return found, nil
	}
	p, err := exec.LookPath("vale")
	if err != nil {
		return "", ErrNoVale
	}
	return p, nil
}

var (
	stylesOnce sync.Once
	stylesDir  string
	stylesErr  error
)

// configPath writes the embedded rules to a private directory once per process and returns its .vale.ini.
func configPath() (string, error) {
	stylesOnce.Do(func() {
		dir, err := os.MkdirTemp("", "dummyrun-vale-")
		if err != nil {
			stylesErr = err
			return
		}
		stylesErr = fs.WalkDir(styles, "styles", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel("styles", path)
			dst := filepath.Join(dir, rel)
			if d.IsDir() {
				return os.MkdirAll(dst, 0o700)
			}
			b, err := styles.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(dst, b, 0o600)
		})
		stylesDir = dir
	})
	if stylesErr != nil {
		return "", fmt.Errorf("plain language: write the rules: %w", stylesErr)
	}
	return filepath.Join(stylesDir, ".vale.ini"), nil
}

// Check runs Vale on one page of HTML and returns its alerts, in line order.
func Check(ctx context.Context, vale, html string) ([]Alert, error) {
	cfg, err := configPath()
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Dir(cfg), "page-*.html")
	if err != nil {
		return nil, fmt.Errorf("plain language: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(html); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("plain language: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("plain language: %w", err)
	}
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, vale, "--config", cfg, "--output", "JSON", "--no-exit", f.Name()) //nolint:gosec // vale is the program the user chose
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("plain language: vale: %w: %s", err, firstLine(stderr.String()))
	}
	return parse(out.Bytes())
}

// parse reads Vale's JSON output, which maps each file to its alerts.
func parse(b []byte) ([]Alert, error) {
	var byFile map[string][]Alert
	if err := json.Unmarshal(bytes.TrimSpace(b), &byFile); err != nil {
		return nil, fmt.Errorf("plain language: read vale's output: %w", err)
	}
	var all []Alert
	for _, a := range byFile {
		all = append(all, a...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Line < all[j].Line })
	return all, nil
}

func firstLine(s string) string {
	if i := bytes.IndexByte([]byte(s), '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
