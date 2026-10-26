// Package lighthouse runs Google Lighthouse's accessibility and best-practice audits on a page, in the Chrome the
// sweep already has open, and reads the failures out of its JSON report. Lighthouse runs as a separate Node.js
// program. No model is involved.
package lighthouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Version is the Lighthouse release the docs name and the tests ran.
const Version = "13.5.0"

// ErrNoLighthouse means no lighthouse program was found.
var ErrNoLighthouse = errors.New("lighthouse: not found; install it with npm install -g lighthouse or set DUMMYRUN_LIGHTHOUSE (see docs/ENGINE.md)")

// Find returns the lighthouse program to run: DUMMYRUN_LIGHTHOUSE when set, otherwise lighthouse on the PATH.
func Find() (string, error) {
	if p := os.Getenv("DUMMYRUN_LIGHTHOUSE"); p != "" {
		found, err := exec.LookPath(p)
		if err != nil {
			return "", fmt.Errorf("lighthouse: DUMMYRUN_LIGHTHOUSE: %w", err)
		}
		return found, nil
	}
	p, err := exec.LookPath("lighthouse")
	if err != nil {
		return "", ErrNoLighthouse
	}
	return p, nil
}

// Item is one element an audit failed. For a best-practice audit, it can be a message about the page instead, on
// html, with where in a script it came from.
type Item struct {
	Selector string `json:"selector"`
	Label    string `json:"label,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// Failure is one audit that failed.
type Failure struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Items    []Item `json:"items"`
}

// Result is what one run found.
type Result struct {
	Version  string             `json:"version"`
	Scores   map[string]float64 `json:"scores"`
	Failures []Failure          `json:"failures"`
}

// Categories are the two Lighthouse categories the sweep runs.
var Categories = []string{"accessibility", "best-practices"}

// Run audits url in the Chrome whose DevTools address is debugger, such as localhost:9222. Lighthouse opens its
// own tab there, so the sweep's tab is left alone, and it keeps the browser's cookies, so a signed-in sweep stays
// signed in. Throttling is off and the window is the browser's own.
func Run(ctx context.Context, bin, url, debugger string) (*Result, error) {
	i := strings.LastIndex(debugger, ":")
	if i < 0 {
		return nil, fmt.Errorf("lighthouse: no port in the debugger address %q", debugger)
	}
	host, port := debugger[:i], debugger[i+1:]
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, url, //nolint:gosec // lighthouse is the program the user chose // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- the path comes from DUMMYRUN_LIGHTHOUSE or PATH, never from a page
		"--hostname="+host, "--port="+port,
		"--only-categories="+strings.Join(Categories, ","),
		"--output=json", "--output-path=stdout", "--quiet",
		"--disable-storage-reset", "--form-factor=desktop", "--screenEmulation.disabled", "--throttling-method=provided")
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("lighthouse: %w: %s", err, lastLine(stderr.String()))
	}
	return Parse(out.Bytes())
}

// Parse reads a Lighthouse JSON report. Only audits scored pass or fail count. Manual, informative and
// not-applicable audits are left out.
func Parse(b []byte) (*Result, error) {
	var lhr struct {
		LighthouseVersion string `json:"lighthouseVersion"`
		RuntimeError      *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"runtimeError"`
		Categories map[string]struct {
			Score     *float64 `json:"score"`
			AuditRefs []struct {
				ID string `json:"id"`
			} `json:"auditRefs"`
		} `json:"categories"`
		Audits map[string]struct {
			Title            string   `json:"title"`
			Score            *float64 `json:"score"`
			ScoreDisplayMode string   `json:"scoreDisplayMode"`
			Details          struct {
				Items []struct {
					Node *struct {
						Selector    string `json:"selector"`
						NodeLabel   string `json:"nodeLabel"`
						Explanation string `json:"explanation"`
					} `json:"node"`
					// A best-practice row with no node says what went wrong in one of these, and where in a script.
					Value          json.RawMessage `json:"value"`
					Description    json.RawMessage `json:"description"`
					IssueType      json.RawMessage `json:"issueType"`
					Source         json.RawMessage `json:"source"`
					SourceLocation json.RawMessage `json:"sourceLocation"`
				} `json:"items"`
			} `json:"details"`
		} `json:"audits"`
	}
	if err := json.Unmarshal(b, &lhr); err != nil {
		return nil, fmt.Errorf("lighthouse: read the report: %w", err)
	}
	if lhr.RuntimeError != nil && lhr.RuntimeError.Code != "" {
		return nil, fmt.Errorf("lighthouse: %s: %s", lhr.RuntimeError.Code, lhr.RuntimeError.Message)
	}
	res := &Result{Version: lhr.LighthouseVersion, Scores: map[string]float64{}}
	for _, cat := range Categories {
		c, ok := lhr.Categories[cat]
		if !ok {
			continue
		}
		if c.Score != nil {
			res.Scores[cat] = *c.Score
		}
		for _, ref := range c.AuditRefs {
			a, ok := lhr.Audits[ref.ID]
			if !ok || a.Score == nil || *a.Score >= 1 || (a.ScoreDisplayMode != "binary" && a.ScoreDisplayMode != "numeric") {
				continue
			}
			f := Failure{ID: ref.ID, Category: cat, Title: a.Title, Items: []Item{}}
			for _, it := range a.Details.Items {
				if it.Node != nil {
					f.Items = append(f.Items, Item{Selector: it.Node.Selector, Label: it.Node.NodeLabel, Detail: firstLine(it.Node.Explanation)})
					continue
				}
				// Accessibility audits are axe rules and always name an element. A best-practice row without one,
				// such as a deprecated API a script called, is kept on the page with its message and place.
				if cat != "best-practices" {
					continue
				}
				msg := firstText(it.Value, it.Description, it.IssueType)
				if msg == "" {
					continue
				}
				if at := location(it.Source, it.SourceLocation); at != "" {
					msg += " (" + at + ")"
				}
				f.Items = append(f.Items, Item{Selector: "html", Detail: msg})
			}
			res.Failures = append(res.Failures, f)
		}
	}
	return res, nil
}

// firstText returns the first line of the first field that holds a non-empty string.
func firstText(fields ...json.RawMessage) string {
	for _, f := range fields {
		var s string
		if json.Unmarshal(f, &s) == nil && strings.TrimSpace(s) != "" {
			return firstLine(s)
		}
	}
	return ""
}

// location returns url:line for the first field that is a Lighthouse source location. Lighthouse counts lines from
// 0, and the location counts them from 1, as an editor does.
func location(fields ...json.RawMessage) string {
	for _, f := range fields {
		var l struct {
			Type string `json:"type"`
			URL  string `json:"url"`
			Line *int   `json:"line"`
		}
		if json.Unmarshal(f, &l) != nil || l.Type != "source-location" || l.URL == "" {
			continue
		}
		if l.Line == nil {
			return l.URL
		}
		return fmt.Sprintf("%s:%d", l.URL, *l.Line+1)
	}
	return ""
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
