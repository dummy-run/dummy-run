// Package validate checks a page's HTML with the Nu HTML Checker (vnu), which runs as a separate Java program, and
// keeps only the errors that change what assistive technology gets. No model is involved.
package validate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// VnuVersion is the Nu HTML Checker release the container image ships and the docs name.
const VnuVersion = "26.10.12"

// ErrNoVnu means no vnu program was found.
var ErrNoVnu = errors.New("html check: vnu not found; install the Nu HTML Checker or set DUMMYRUN_VNU (see docs/ENGINE.md)")

// Find returns the vnu program to run: DUMMYRUN_VNU when set, otherwise vnu on the PATH.
func Find() (string, error) {
	if p := os.Getenv("DUMMYRUN_VNU"); p != "" {
		found, err := exec.LookPath(p)
		if err != nil {
			return "", fmt.Errorf("html check: DUMMYRUN_VNU: %w", err)
		}
		return found, nil
	}
	p, err := exec.LookPath("vnu")
	if err != nil {
		return "", ErrNoVnu
	}
	return p, nil
}

// Message is one thing vnu reported.
type Message struct {
	URL         string `json:"url,omitempty"`
	Type        string `json:"type"`
	SubType     string `json:"subType,omitempty"`
	Message     string `json:"message"`
	Extract     string `json:"extract,omitempty"`
	LastLine    int    `json:"lastLine,omitempty"`
	FirstColumn int    `json:"firstColumn,omitempty"`
}

// Check runs vnu once on each document given and returns what it reported about each, in the same order. One run
// for several documents starts Java once.
func Check(ctx context.Context, vnu string, docs ...string) ([][]Message, error) {
	dir, err := os.MkdirTemp("", "dummyrun-vnu-")
	if err != nil {
		return nil, fmt.Errorf("html check: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	args := []string{"--format", "json", "--stdout", "--exit-zero-always"}
	index := map[string]int{}
	for i, d := range docs {
		name := filepath.Join(dir, fmt.Sprintf("doc%d.html", i))
		if err := os.WriteFile(name, []byte(d), 0o600); err != nil {
			return nil, fmt.Errorf("html check: %w", err)
		}
		args = append(args, name)
		index[filepath.Base(name)] = i
	}
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, vnu, args...) //nolint:gosec // vnu is the program the user chose // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- the path comes from DUMMYRUN_VNU or PATH, never from a page
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("html check: vnu: %w: %s", err, firstLine(stderr.String()))
	}
	var r struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		return nil, fmt.Errorf("html check: read vnu's output: %w", err)
	}
	res := make([][]Message, len(docs))
	for _, m := range r.Messages {
		// An I/O or internal failure comes with no document, and it means vnu did not read the pages.
		if m.Type == "non-document-error" {
			return nil, fmt.Errorf("html check: vnu: %s", firstLine(m.Message))
		}
		if i, ok := index[path.Base(m.URL)]; ok {
			m.URL = ""
			res[i] = append(res[i], m)
		}
	}
	return res, nil
}

// A Rule is a kind of vnu error that changes what assistive technology gets.
type Rule struct {
	ID   string
	Help string
	// WCAG is the criterion the error most often breaks. The errors go to advice either way, because WCAG 2.2
	// dropped parsing (4.1.1) as a criterion and a validator can't tell whether the page also fails another one.
	WCAG string
	re   *regexp.Regexp
}

// Rules are checked in order and the first that matches names the error.
var Rules = []Rule{
	{"dummyrun-html-broken-reference", "An attribute that points at another element by ID, such as aria-labelledby or a label's for, names no element that can take it", "1.3.1",
		regexp.MustCompile(`must point to an element|must refer to|“for” attribute of the “label” element must be the ID`)},
	{"dummyrun-html-duplicate-id", "Two elements share an ID, so a label, aria-labelledby or skip link that points at it can reach the wrong one", "4.1.2",
		regexp.MustCompile(`^Duplicate ID`)},
	{"dummyrun-html-nested-interactive", "A control inside another control, such as a link inside a button, which screen readers announce in different ways or not at all", "4.1.2",
		regexp.MustCompile(`must not appear as a descendant of the “(a|button|label|summary|select)” element|attribute “tabindex” must not appear as a descendant`)},
	{"dummyrun-html-aria", "A role or aria-* attribute the element can't have, or a value it can't take, so assistive technology may ignore it", "4.1.2",
		regexp.MustCompile(`“(aria-[a-z]+|role)”|role=`)},
	{"dummyrun-html-structure", "A list, table or select with children that don't belong there, or tags the browser had to rearrange, so the structure screen readers report is not the one you wrote", "1.3.1",
		regexp.MustCompile(`not allowed as child of element “(ul|ol|dl|menu|table|thead|tbody|tfoot|tr|select|optgroup|datalist)”|violates nesting rules|^Unclosed element|seen in “table”`)},
	{"dummyrun-html-img-alt", "An image with no alt attribute", "1.1.1",
		regexp.MustCompile(`must have an “alt” attribute`)},
}

// Relevant returns the rule an error falls under, or false for one that doesn't change what assistive technology
// gets, such as an obsolete attribute or a stray end tag the browser ignores. Warnings and notes are never relevant.
func Relevant(m Message) (Rule, bool) {
	if m.Type != "error" {
		return Rule{}, false
	}
	for _, r := range Rules {
		if r.re.MatchString(m.Message) {
			return r, true
		}
	}
	return Rule{}, false
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
