package review

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/dummy-run/dummy-run/internal/record"
)

// clean makes text from a model or a page safe to print: control characters, which could carry
// terminal escape sequences, become spaces. Newlines and tabs stay.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

// stepExtra is what steps.jsonl holds about a step that the record does not.
type stepExtra struct {
	Step int `json:"step"`
	Page *struct {
		Focus *struct {
			Role string `json:"role"`
			Name string `json:"name"`
		} `json:"focus"`
	} `json:"page"`
}

// readExtras reads <dir>/<run id>/steps.jsonl, which the runner writes next to the record. A missing
// or damaged file gives fewer details and no error: the record alone is enough to review.
func readExtras(recordPath, runID string) map[int]stepExtra {
	out := map[int]stepExtra{}
	f, err := os.Open(filepath.Join(filepath.Dir(recordPath), runID, "steps.jsonl")) //nolint:gosec // beside the record the user named
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(bufio.NewReader(f))
	for {
		var e stepExtra
		if err := dec.Decode(&e); err != nil {
			return out
		}
		out[e.Step] = e
	}
}

// observation is the one-line summary of what a step saw.
func observation(s record.Step) string {
	var parts []string
	if s.Status != nil {
		parts = append(parts, fmt.Sprintf("status %d", *s.Status))
	}
	if s.ConsoleErrors != nil && *s.ConsoleErrors > 0 {
		parts = append(parts, plural(*s.ConsoleErrors, "console error"))
	}
	if s.A11yViolations != nil && *s.A11yViolations > 0 {
		parts = append(parts, plural(*s.A11yViolations, "accessibility violation"))
	}
	if s.Route != "" {
		parts = append(parts, "route "+s.Route)
	}
	if len(parts) == 0 {
		return "nothing flagged"
	}
	return strings.Join(parts, ", ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// screenshotPath returns the screenshot file for a step, or "" when the run saved none.
func screenshotPath(recordPath, runID string, s record.Step) (path string, found bool) {
	dir := filepath.Dir(recordPath)
	cands := []string{}
	if s.Screenshot != "" {
		cands = append(cands, filepath.Join(dir, s.Screenshot))
	}
	cands = append(cands, filepath.Join(dir, runID, "screens", fmt.Sprintf("%03d.png", s.N)))
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c, true
		}
	}
	if s.Screenshot != "" {
		return cands[0], false
	}
	return "", false
}

// Render prints one problem for the reviewer: who found it, what it says and the steps it cites.
func Render(w io.Writer, g *Group) {
	lead := g.Lead()
	f, rec := lead.Finding, lead.Record
	pending := len(g.Pending())
	p := func(format string, a ...any) { fmt.Fprintf(w, format+"\n", a...) }

	p("%s  %s  evidence: %s  class: %s", f.Severity, f.Kind, f.Evidence, f.Class)
	p("  %s", clean(f.Title))
	p("  fingerprint %s  route %s  locus %s  symptom %s", g.Fingerprint, clean(f.Route), clean(f.Locus), clean(f.Symptom))
	p("  persona %s (%s)  scenario %s  run %s", rec.Persona.ID, rec.Persona.Role, rec.Scenario.ID, rec.ID)
	if len(g.Occurrences) > 1 {
		p("  %s of this problem in all. One verdict covers the %d still pending. The others:", plural(len(g.Occurrences), "finding"), pending)
		for _, o := range g.Occurrences {
			if o.Record.ID == rec.ID && o.Finding.ID == f.ID {
				continue
			}
			p("    %s  persona %s  %s  %s", o.Record.ID, o.Record.Persona.ID, o.Finding.Verdict, o.Finding.ID)
		}
	}
	extras := readExtras(lead.Path, rec.ID)
	byN := map[int]record.Step{}
	for _, s := range rec.Steps {
		byN[s.N] = s
	}
	steps := append([]int(nil), f.Steps...)
	sort.Ints(steps)
	p("  cited steps:")
	for _, n := range steps {
		s, ok := byN[n]
		if !ok {
			p("    step %d  (not in the record)", n)
			continue
		}
		action := s.Action
		if s.Target != "" {
			action += " " + clean(s.Target)
		}
		p("    step %d  %s", n, action)
		p("      saw: %s", observation(s))
		if s.Note != "" {
			p("      persona said: %s", clean(s.Note))
		}
		if x, ok := extras[n]; ok && x.Page != nil {
			if x.Page.Focus != nil {
				p("      focus: %s %q", clean(x.Page.Focus.Role), clean(x.Page.Focus.Name))
			} else {
				p("      focus: none")
			}
		}
		if path, found := screenshotPath(lead.Path, rec.ID, s); path != "" {
			if found {
				p("      screenshot: %s", path)
			} else {
				p("      screenshot: %s (file not found)", path)
			}
		}
	}
}
