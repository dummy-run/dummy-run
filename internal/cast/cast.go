// Package cast loads personas and scenarios from a cast directory.
//
// A cast is a directory with one folder per persona:
//
//	casts/<cast>/<persona-id>/profile.md     a front-matter block, then the profile
//	casts/<cast>/<persona-id>/prompt.md      the system prompt: who the persona is and how it behaves
//	casts/<cast>/<persona-id>/scenarios/NN-<scenario-id>.md
//
// The front matter is a block between two "---" lines with "key: value" pairs.
// Required keys are id, role and device (desktop, tablet or phone).
package cast

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Persona is one fictional user.
type Persona struct {
	ID     string
	Role   string
	Device string
	// Name is a display name from the front matter, optional.
	Name string
	// Profile is the Markdown after the front matter.
	Profile string
	// Prompt is prompt.md.
	Prompt string
	// Scenarios are sorted by file name.
	Scenarios []Scenario
}

// Scenario is one job for the persona.
type Scenario struct {
	ID    string
	Title string
	// Budget is the step budget the scenario asked for, or 0 for the default.
	Budget int
	// Body is the whole Markdown file.
	Body string
}

// Devices are the viewports per device.
var Devices = map[string][2]int{
	"desktop": {1400, 1000},
	"tablet":  {820, 1180},
	"phone":   {390, 844},
}

var idPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// Load reads one persona from a cast directory.
func Load(castDir, personaID string) (*Persona, error) {
	if !idPattern.MatchString(personaID) {
		return nil, fmt.Errorf("cast: persona ID %q must be lower-case letters, digits and hyphens", personaID)
	}
	dir := filepath.Join(castDir, personaID)
	profile, err := os.ReadFile(filepath.Join(dir, "profile.md"))
	if err != nil {
		return nil, fmt.Errorf("cast: %w", err)
	}
	meta, body, err := FrontMatter(string(profile))
	if err != nil {
		return nil, fmt.Errorf("cast: %s/profile.md: %w", personaID, err)
	}
	p := &Persona{ID: meta["id"], Role: meta["role"], Device: meta["device"], Name: meta["name"], Profile: body}
	if p.ID != personaID {
		return nil, fmt.Errorf("cast: %s/profile.md has id %q", personaID, p.ID)
	}
	if p.Role == "" {
		return nil, fmt.Errorf("cast: %s/profile.md has no role", personaID)
	}
	if _, ok := Devices[p.Device]; !ok {
		return nil, fmt.Errorf("cast: %s/profile.md device %q must be desktop, tablet or phone", personaID, p.Device)
	}
	prompt, err := os.ReadFile(filepath.Join(dir, "prompt.md"))
	if err != nil {
		return nil, fmt.Errorf("cast: %w", err)
	}
	p.Prompt = string(prompt)
	files, err := filepath.Glob(filepath.Join(dir, "scenarios", "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	for _, f := range files {
		s, err := LoadScenario(f)
		if err != nil {
			return nil, err
		}
		p.Scenarios = append(p.Scenarios, s)
	}
	if len(p.Scenarios) == 0 {
		return nil, fmt.Errorf("cast: %s has no scenarios", personaID)
	}
	return p, nil
}

// Scenario returns the scenario with the given ID.
func (p *Persona) Scenario(id string) (Scenario, error) {
	for _, s := range p.Scenarios {
		if s.ID == id {
			return s, nil
		}
	}
	ids := make([]string, 0, len(p.Scenarios))
	for _, s := range p.Scenarios {
		ids = append(ids, s.ID)
	}
	return Scenario{}, fmt.Errorf("cast: %s has no scenario %q; it has %s", p.ID, id, strings.Join(ids, ", "))
}

var (
	numberPrefix = regexp.MustCompile(`^[0-9]+-`)
	budgetLine   = regexp.MustCompile(`(?m)^Budget:\s*([0-9]+)`)
	titleLine    = regexp.MustCompile(`(?m)^#\s+(.+)$`)
)

// LoadScenario reads one scenario file. The ID is the file name without its
// number prefix and extension; the title is the first heading.
func LoadScenario(path string) (Scenario, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the path comes from the cast directory on purpose
	if err != nil {
		return Scenario{}, fmt.Errorf("cast: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(path), ".md")
	id := numberPrefix.ReplaceAllString(base, "")
	if !idPattern.MatchString(id) {
		return Scenario{}, fmt.Errorf("cast: scenario file %s must be named NN-<id>.md with a lower-case id", path)
	}
	s := Scenario{ID: id, Body: string(data)}
	if m := titleLine.FindStringSubmatch(s.Body); m != nil {
		s.Title = strings.TrimSpace(m[1])
	}
	if s.Title == "" {
		return Scenario{}, fmt.Errorf("cast: %s has no heading", path)
	}
	if m := budgetLine.FindStringSubmatch(s.Body); m != nil {
		s.Budget, _ = strconv.Atoi(m[1])
	}
	return s, nil
}

// FrontMatter splits a "---" delimited key: value block from the Markdown after it.
func FrontMatter(text string) (map[string]string, string, error) {
	sc := bufio.NewScanner(strings.NewReader(text))
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return nil, "", errors.New("front matter must start with ---")
	}
	meta := map[string]string{}
	closed := false
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			closed = true
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("front matter line %q is not key: value", line)
		}
		meta[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	if !closed {
		return nil, "", errors.New("front matter is not closed with ---")
	}
	var body strings.Builder
	for sc.Scan() {
		body.WriteString(sc.Text())
		body.WriteByte('\n')
	}
	return meta, strings.TrimLeft(body.String(), "\n"), nil
}

// List returns the persona IDs in a cast directory.
func List(castDir string) ([]string, error) {
	entries, err := os.ReadDir(castDir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && idPattern.MatchString(e.Name()) {
			if _, err := os.Stat(filepath.Join(castDir, e.Name(), "profile.md")); err == nil {
				ids = append(ids, e.Name())
			}
		}
	}
	return ids, nil
}
