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
//	text_scale: 200               system text size in percent, 100 to 300 (the browser's default font size)
//	tools-allowed: press look     the only tools the persona may use (finish is always allowed)
//	tools-denied: click           tools the persona may not use
//
// The persona key "signin: editor" and the scenario line "Sign in as: editor" pick which of the target's
// credentials roles the engine signs in as before the run. "none" turns it off.
//
// A scenario file can set "Device: phone", "Zoom: 200%", "Text scale: 200%", "Budget: 30 steps" and
// "Min steps: 8" on lines of their own; Device, Zoom and Text scale there replace the persona's. A Device
// line may carry prose after the device name ("Device: desktop. Start at the
// home page."); a Zoom or Text scale value has the line to itself. A value that is not valid
// is an error, never ignored.
//
// A "Site: personal site - people who want to know who Joel is" line says what kind of site this is and who it is
// for. The engine gives it to the self-check, which judges each finding against it. The planner writes it.
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
	// Signin is the credentials role the persona signs in as, from the front matter key "signin".
	// "none" means the persona never signs in. Empty means the target decides: see SigninRole.
	Signin string
	// Zoom is the persona's page zoom in percent, or 0 for 100.
	Zoom int
	// TextScale is the persona's system text size in percent, or 0 for 100.
	TextScale int
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
	// MinSteps is how many steps the persona must take before finish is accepted, or 0 for no minimum.
	MinSteps int
	// Device is the device the scenario asked for, or "" to use the persona's.
	Device string
	// Zoom is the page zoom in percent the scenario asked for, or 0 to use the persona's.
	Zoom int
	// TextScale is the system text size in percent the scenario asked for, or 0 to use the persona's.
	TextScale int
	// Signin is the role from a "Sign in as: ROLE" line, "none" for no sign-in, or "" to use the persona's.
	Signin string
	// Site says what kind of site this is and who it is for, from a "Site:" line, or "".
	Site string
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
	p := &Persona{ID: meta["id"], Role: meta["role"], Device: meta["device"], Name: meta["name"], Signin: strings.ToLower(meta["signin"]), Profile: body}
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
	if p.TextScale, err = ParseTextScale(meta["text_scale"]); err != nil {
		return nil, fmt.Errorf("cast: %s/profile.md text_scale: %w", personaID, err)
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
	numberPrefix  = regexp.MustCompile(`^[0-9]+-`)
	budgetLine    = regexp.MustCompile(`(?m)^Budget:\s*([0-9]+)`)
	minStepsLine  = regexp.MustCompile(`(?mi)^Min steps:[ \t]*([^\n]*)$`)
	minStepsValue = regexp.MustCompile(`^([0-9]+)(?: steps?)?$`)
	titleLine     = regexp.MustCompile(`(?m)^#\s+(.+)$`)
	deviceLine    = regexp.MustCompile(`(?mi)^Device:[ \t]*([^\s.]*)`)
	signinLine    = regexp.MustCompile(`(?mi)^Sign in(?: as)?:[ \t]*([^\s.]*)`)
	zoomLine      = regexp.MustCompile(`(?mi)^Zoom:[ \t]*([^\n]*)$`)
	textScaleLine = regexp.MustCompile(`(?mi)^Text[ _-]scale:[ \t]*([^\n]*)$`)
	siteLine      = regexp.MustCompile(`(?mi)^Site:[ \t]*([^\n]*)$`)
)

// maxSiteRunes bounds the Site line, which goes into a model prompt.
const maxSiteRunes = 300

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

// Text scale limits in percent. 100 is the browser's own size. Phones offer up to about 200 or 300.
const (
	MinTextScale = 100
	MaxTextScale = 300
)

// ParseTextScale reads a system text size such as "200", "200%" or "200 percent". Empty means no
// setting and gives 0.
func ParseTextScale(v string) (int, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(v, "percent"), "%")))
	if err != nil {
		return 0, fmt.Errorf("%q is not a percentage such as 200", v)
	}
	if n < MinTextScale || n > MaxTextScale {
		return 0, fmt.Errorf("%d percent is outside %d to %d", n, MinTextScale, MaxTextScale)
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

// SigninRole says which credentials role to sign in as for a scenario, and whether the choice was
// explicit. The scenario's "Sign in as:" line comes first, then the persona's "signin" key. With
// neither, the persona's role is used when the target has credentials for it (the caller checks),
// so the library's role-admin signs in as admin on a target that has an admin. "none" gives "".
func (p *Persona) SigninRole(s Scenario) (role string, explicit bool) {
	for _, v := range []string{s.Signin, p.Signin} {
		if v == "none" {
			return "", true
		}
		if v != "" {
			return v, true
		}
	}
	return strings.ToLower(p.Role), false
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
	// TextScale is the system text size in percent; 100 means none. It changes the browser's default
	// font size and leaves the viewport as it is.
	TextScale int
	// CSSWidth and CSSHeight are the viewport the page lays out in, the window divided by the zoom.
	CSSWidth, CSSHeight int
}

// DisplayFor resolves the device and zoom for a scenario: the scenario's own
// setting if it has one, otherwise the persona's.
func (p *Persona) DisplayFor(s Scenario) Display {
	d := Display{Device: p.Device, Zoom: 100, TextScale: 100}
	if s.Device != "" {
		d.Device = s.Device
	}
	if p.Zoom != 0 {
		d.Zoom = p.Zoom
	}
	if s.Zoom != 0 {
		d.Zoom = s.Zoom
	}
	if p.TextScale != 0 {
		d.TextScale = p.TextScale
	}
	if s.TextScale != 0 {
		d.TextScale = s.TextScale
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
	if m := minStepsLine.FindStringSubmatch(s.Body); m != nil {
		v := minStepsValue.FindStringSubmatch(strings.TrimSpace(strings.ToLower(m[1])))
		if v == nil {
			return Scenario{}, fmt.Errorf("cast: %s min steps %q must be a whole number such as \"Min steps: 8\"", path, strings.TrimSpace(m[1]))
		}
		n, err := strconv.Atoi(v[1])
		if err != nil {
			return Scenario{}, fmt.Errorf("cast: %s min steps: %w", path, err)
		}
		s.MinSteps = n
	}
	if m := deviceLine.FindStringSubmatch(s.Body); m != nil {
		s.Device = strings.ToLower(m[1])
		if _, ok := Devices[s.Device]; !ok {
			return Scenario{}, fmt.Errorf("cast: %s device %q must be desktop, tablet or phone", path, m[1])
		}
	}
	if m := signinLine.FindStringSubmatch(s.Body); m != nil {
		if m[1] == "" {
			return Scenario{}, fmt.Errorf("cast: %s has a Sign in line with no role; write \"Sign in as: admin\" or \"Sign in as: none\"", path)
		}
		s.Signin = strings.ToLower(m[1])
	}
	if m := zoomLine.FindStringSubmatch(s.Body); m != nil {
		z, err := ParseZoom(m[1])
		if err != nil {
			return Scenario{}, fmt.Errorf("cast: %s zoom: %w", path, err)
		}
		s.Zoom = z
	}
	if m := siteLine.FindStringSubmatch(s.Body); m != nil {
		site := []rune(strings.TrimSpace(m[1]))
		if len(site) > maxSiteRunes {
			site = site[:maxSiteRunes]
		}
		s.Site = string(site)
	}
	if m := textScaleLine.FindStringSubmatch(s.Body); m != nil {
		t, err := ParseTextScale(m[1])
		if err != nil {
			return Scenario{}, fmt.Errorf("cast: %s text scale: %w", path, err)
		}
		s.TextScale = t
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
