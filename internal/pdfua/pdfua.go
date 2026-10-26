// Package pdfua checks PDF files against PDF/UA-1, the ISO standard for accessible PDF, with veraPDF, which runs as a
// separate Java program. No model is involved.
package pdfua

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoVeraPDF means no veraPDF program was found.
var ErrNoVeraPDF = errors.New("pdf check: verapdf not found; install veraPDF or set DUMMYRUN_VERAPDF (see docs/ENGINE.md)")

// Find returns the veraPDF program to run: DUMMYRUN_VERAPDF when set, otherwise verapdf on the PATH.
func Find() (string, error) {
	if p := os.Getenv("DUMMYRUN_VERAPDF"); p != "" {
		found, err := exec.LookPath(p)
		if err != nil {
			return "", fmt.Errorf("pdf check: DUMMYRUN_VERAPDF: %w", err)
		}
		return found, nil
	}
	p, err := exec.LookPath("verapdf")
	if err != nil {
		return "", ErrNoVeraPDF
	}
	return p, nil
}

// Rule is one PDF/UA-1 requirement a file fails.
type Rule struct {
	// Clause is the clause of ISO 14289-1, such as 7.1, with the test number after a dash when there is more than one.
	Clause      string `json:"clause"`
	Description string `json:"description"`
	// Checks counts the places in the file that fail it.
	Checks int `json:"checks"`
}

// Result is what veraPDF found in one file.
type Result struct {
	Compliant bool   `json:"compliant"`
	Rules     []Rule `json:"rules,omitempty"`
	// Error says why veraPDF couldn't read the file, such as a file that isn't a PDF or is encrypted.
	Error string `json:"error,omitempty"`
}

// Check runs veraPDF once on the files and returns a result for each, in the same order. veraPDF exits with a
// non-zero status when a file fails, so the status is ignored and its report is read instead.
func Check(ctx context.Context, verapdf string, files ...string) ([]Result, error) {
	args := append([]string{"--flavour", "ua1", "--format", "json"}, files...)
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, verapdf, args...) //nolint:gosec // veraPDF is the program the user chose // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- the path comes from DUMMYRUN_VERAPDF or PATH, never from a page
	cmd.Stdout, cmd.Stderr = &out, &stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("pdf check: %w", ctx.Err())
	}
	var r struct {
		Report *struct {
			Jobs []struct {
				Item struct {
					Name string `json:"name"`
				} `json:"itemDetails"`
				Validation []struct {
					Compliant bool `json:"compliant"`
					Details   struct {
						Rules []struct {
							Clause      string `json:"clause"`
							TestNumber  int    `json:"testNumber"`
							Status      string `json:"status"`
							Description string `json:"description"`
							Failed      int    `json:"failedChecks"`
						} `json:"ruleSummaries"`
					} `json:"details"`
				} `json:"validationResult"`
				Exception any `json:"taskException"`
			} `json:"jobs"`
		} `json:"report"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.Report == nil {
		if runErr != nil {
			return nil, fmt.Errorf("pdf check: verapdf: %w: %s", runErr, firstLine(stderr.String()))
		}
		return nil, errors.New("pdf check: read veraPDF's output: it has no report")
	}
	index := map[string]int{}
	for i, f := range files {
		abs, _ := filepath.Abs(f)
		index[abs] = i
	}
	res := make([]Result, len(files))
	seen := make([]bool, len(files))
	for _, j := range r.Report.Jobs {
		abs, _ := filepath.Abs(j.Item.Name)
		i, ok := index[abs]
		if !ok {
			return nil, fmt.Errorf("pdf check: veraPDF reported on a file it wasn't given (%q)", j.Item.Name)
		}
		seen[i] = true
		if len(j.Validation) == 0 {
			res[i].Error = "veraPDF couldn't read the file"
			if j.Exception != nil {
				res[i].Error = "veraPDF couldn't read the file: " + firstLine(fmt.Sprint(j.Exception))
			}
			continue
		}
		v := j.Validation[0]
		res[i].Compliant = v.Compliant
		multi := map[string]int{}
		for _, rule := range v.Details.Rules {
			multi[rule.Clause]++
		}
		for _, rule := range v.Details.Rules {
			if rule.Status != "failed" {
				continue
			}
			clause := rule.Clause
			if multi[rule.Clause] > 1 {
				clause = fmt.Sprintf("%s-%d", rule.Clause, rule.TestNumber)
			}
			res[i].Rules = append(res[i].Rules, Rule{Clause: clause, Description: rule.Description, Checks: rule.Failed})
		}
	}
	for i, ok := range seen {
		if !ok {
			res[i].Error = "veraPDF didn't report on the file"
		}
	}
	return res, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
