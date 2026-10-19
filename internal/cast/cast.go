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
//
// Optional persona keys:
//
//	zoom: 200                     page zoom in percent, 100 to 500
//	tools-allowed: press look     the only tools the persona may use (finish is always allowed)
//	tools-denied: click           tools the persona may not use
//
// A scenario file can set "Device: phone", "Zoom: 200%" and "Budget: 30 steps"
// on lines of their own; Device and Zoom there replace the persona's. A Device
// line may carry prose after the device name ("Device: desktop. Start at the
// home page."); a Zoom value has the line to itself. A value that is not valid
// is an error, never ignored.
package cast

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dummy-run/dummy-run/internal/record"
)

// Persona is one fictional user.
type Persona struct {
	ID     string
	Role   string
	Device string
	// Name is a display name from the front matter, optional.
	Name string
	// Zoom is the persona's page zoom in percent, or 0 for 100.
	Zoom int
	// ToolsAllowed, when not empty, lists the only tools the persona may use.
	ToolsAllowed []string
	// ToolsDenied lists tools the persona may not use.
	ToolsDenied []string
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
	// Device is the device the scenario asked for, or "" to use the persona's.
	Device string
	// Zoom is the page zoom in percent the scenario asked for, or 0 to use the persona's.
	Zoom int
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
	if p.Zoom, err = ParseZoom(meta["zoom"]); err != nil {
		return nil, fmt.Errorf("cast: %s/profile.md zoom: %w", personaID, err)
	}
	if p.ToolsAllowed, err = parseTools(meta["tools-allowed"]); err != nil {
		return nil, fmt.Errorf("cast: %s/profile.md tools-allowed: %w", personaID, err)
	}
	if p.ToolsDenied, err = parseTools(meta["tools-denied"]); err != nil {
		return nil, fmt.Errorf("cast: %s/profile.md tools-denied: %w", personaID, err)
	}
	if slices.Contains(p.ToolsDenied, "finish") {
		return nil, fmt.Errorf("cast: %s/profile.md tools-denied: finish cannot be denied, it is how a run ends", personaID)
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
	deviceLine   = regexp.MustCompile(`(?mi)^Device:[ \t]*([^\s.]*)`)
	zoomLine     = regexp.MustCompile(`(?mi)^Zoom:[ \t]*([^\n]*)$`)
)

// Zoom limits in percent. Browsers offer 25 to 500; below 100 is not a
// low-vision setting, so the engine starts at 100.
const (
	MinZoom = 100
	MaxZoom = 500
)

// ParseZoom reads a zoom level such as "200", "200%" or "200 percent". Empty
// means no setting and gives 0.
func ParseZoom(v string) (int, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(v, "percent"), "%")))
	if err != nil {
		return 0, fmt.Errorf("%q is not a percentage such as 200", v)
	}
	if n < MinZoom || n > MaxZoom {
		return 0, fmt.Errorf("%d percent is outside %d to %d", n, MinZoom, MaxZoom)
	}
	return n, nil
}

// parseTools reads a list of tool names separated by spaces or commas and
// checks each against the engine's tools.
func parseTools(v string) ([]string, error) {
	var out []string
	for _, name := range strings.FieldsFunc(strings.ToLower(v), func(r rune) bool { return r == ',' || r == ' ' }) {
		if !slices.Contains(record.Actions, name) {
			return nil, fmt.Errorf("%q is not a tool; the tools are %s", name, strings.Join(record.Actions, ", "))
		}
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out, nil
}

// Allows reports whether the persona may call the tool. finish is always
// allowed: it is how a run ends.
func (p *Persona) Allows(tool string) bool {
	if tool == "finish" || p == nil {
		return true
	}
	if slices.Contains(p.ToolsDenied, tool) {
		return false
	}
	return len(p.ToolsAllowed) == 0 || slices.Contains(p.ToolsAllowed, tool)
}

// AT is the assistive technology or modality the persona's tool limits emulate, as the run record's
// persona.dimensions.at names it: keyboard-only when the persona cannot click, screen-reader-view when
// it is not sent screenshots, otherwise none. Nothing else is emulated.
func (p *Persona) AT() string {
	switch {
	case !p.Allows("click"):
		return "keyboard-only"
	case !p.Allows("screenshot"):
		return "screen-reader-view"
	}
	return "none"
}

// Refused lists the tools from names that the persona may not use, in the order given.
func (p *Persona) Refused(names []string) []string {
	var out []string
	for _, n := range names {
		if !p.Allows(n) {
			out = append(out, n)
		}
	}
	return out
}

// Display is how the browser is set up for one run.
type Display struct {
	Device string
	// Width and Height are the window in device pixels.
	Width, Height int
	// Zoom is the page zoom in percent; 100 means none.
	Zoom int
	// CSSWidth and CSSHeight are the viewport the page lays out in, the window divided by the zoom.
	CSSWidth, CSSHeight int
}

// DisplayFor resolves the device and zoom for a scenario: the scenario's own
// setting if it has one, otherwise the persona's.
func (p *Persona) DisplayFor(s Scenario) Display {
	d := Display{Device: p.Device, Zoom: 100}
	if s.Device != "" {
		d.Device = s.Device
	}
	if p.Zoom != 0 {
		d.Zoom = p.Zoom
	}
	if s.Zoom != 0 {
		d.Zoom = s.Zoom
	}
	size := Devices[d.Device]
	d.Width, d.Height = size[0], size[1]
	d.CSSWidth = (d.Width*100 + d.Zoom/2) / d.Zoom
	d.CSSHeight = (d.Height*100 + d.Zoom/2) / d.Zoom
	return d
}

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
	if m := deviceLine.FindStringSubmatch(s.Body); m != nil {
		s.Device = strings.ToLower(m[1])
		if _, ok := Devices[s.Device]; !ok {
			return Scenario{}, fmt.Errorf("cast: %s device %q must be desktop, tablet or phone", path, m[1])
		}
	}
	if m := zoomLine.FindStringSubmatch(s.Body); m != nil {
		z, err := ParseZoom(m[1])
		if err != nil {
			return Scenario{}, fmt.Errorf("cast: %s zoom: %w", path, err)
		}
		s.Zoom = z
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
