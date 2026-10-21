package plan

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// lookFor is what every persona notes on every page. It is general on purpose: the plan decides where the
// persona goes, not what counts as a problem.
var lookFor = []string{
	"Links and buttons whose name does not say where they go or what they do, such as several links all called \"Read more\", or controls shown as \"(no label)\".",
	"Form fields with no visible label, and groups of choices with no question above them.",
	"Images that carry meaning but have no description, and pages with no headings or with headings in a strange order.",
	"Text that is small, faint or cut off, and content that overlaps or runs off the screen.",
	"Links that go somewhere you did not expect, pages that fail to load or answer with an error, and pages that are very slow.",
	"Words or instructions you do not understand, and a next step you cannot find.",
}

// WriteCast writes the plan as a cast folder: for each chosen persona its profile.md and prompt.md from
// lib, and one scenario at scenarios/01-<id>.md. It also writes plan.json at the top of dir. dir may exist;
// files in it are replaced.
func WriteCast(dir string, lib fs.FS, p *Plan, libPersonas []LibraryPersona) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	finds := map[string]string{}
	for _, l := range libPersonas {
		finds[l.ID] = l.Finds
	}
	for _, pick := range p.Personas {
		pdir := filepath.Join(dir, pick.ID)
		if err := os.MkdirAll(filepath.Join(pdir, "scenarios"), 0o750); err != nil {
			return err
		}
		for _, name := range []string{"profile.md", "prompt.md"} {
			data, err := fs.ReadFile(lib, path.Join(pick.ID, name))
			if err != nil {
				if name == "prompt.md" {
					continue
				}
				return fmt.Errorf("plan: persona %s: %w", pick.ID, err)
			}
			if err := os.WriteFile(filepath.Join(pdir, name), data, 0o600); err != nil {
				return err
			}
		}
		text := ScenarioText(p, pick, finds[pick.ID])
		if err := os.WriteFile(filepath.Join(pdir, "scenarios", "01-"+pick.Scenario.ID+".md"), []byte(text), 0o600); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "plan.json"), append(data, '\n'), 0o600)
}

// ScenarioText renders one persona's scenario in the cast format that internal/cast reads.
func ScenarioText(p *Plan, pick Pick, finds string) string {
	sc := pick.Scenario
	name := pick.Name
	if name == "" {
		name = "The visitor"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", oneLine(sc.Title))
	device := sc.Device
	if device == "" {
		device = "the persona's own"
	}
	if sc.Device != "" {
		fmt.Fprintf(&b, "Device: %s. Start at the page you were given.\n", sc.Device)
	} else {
		fmt.Fprintf(&b, "Start at the page you were given, on %s device.\n", device)
	}
	if sc.Zoom > 0 {
		fmt.Fprintf(&b, "Zoom: %d%%\n", sc.Zoom)
	}
	if sc.TextScale > 0 {
		fmt.Fprintf(&b, "Text scale: %d%%\n", sc.TextScale)
	}
	fmt.Fprintf(&b, "Min steps: %d\n", sc.MinSteps)
	b.WriteString("Sign in as: none\n")
	site := SiteWords(p.SiteType)
	if p.Audience != "" {
		site += " - " + oneLine(p.Audience)
	}
	fmt.Fprintf(&b, "Site: %s\n\n", site)
	fmt.Fprintf(&b, "## Given\n\n%s Nobody is signed in and nothing gets sent on this visit.\n\n", oneLine(sc.Given))
	fmt.Fprintf(&b, "## Goal\n\n\"%s\"\n\n", oneLine(sc.Goal))
	if len(sc.StartPages) > 0 {
		b.WriteString("## Pages to start from\n\nThese pages exist on the site. Use them to find your way around. You are not limited to them.\n\n")
		for _, u := range sc.StartPages {
			fmt.Fprintf(&b, "- %s\n", u)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "## Steps %s would take\n\n", name)
	n := 0
	for _, s := range sc.Steps {
		n++
		fmt.Fprintf(&b, "%d. %s\n", n, oneLine(s))
	}
	fmt.Fprintf(&b, "%d. Run the accessibility check once, on the page that matters most.\n", n+1)
	fmt.Fprintf(&b, "%d. Finish with what worked and what did not.\n\n", n+2)
	b.WriteString("## Things to look for\n\nOn every page you read, note any of these. Report each one you saw as a finding, citing the step that shows it and naming the control or quoting the words on the page. Leave out anything you did not see.\n\n")
	for _, l := range lookFor {
		fmt.Fprintf(&b, "- %s\n", l)
	}
	if finds != "" {
		fmt.Fprintf(&b, "- What %s notices most: %s\n", name, oneLine(finds))
	}
	b.WriteString("\n## Do not\n\n")
	doNot := append([]string{}, sc.DoNot...)
	for _, must := range mustNot {
		if !slices.ContainsFunc(doNot, must.re.MatchString) {
			doNot = append(doNot, must.text)
		}
	}
	for _, d := range doNot {
		fmt.Fprintf(&b, "- %s\n", oneLine(d))
	}
	b.WriteString("\n## Success looks like\n\n")
	for _, s := range sc.Success {
		fmt.Fprintf(&b, "- %s\n", oneLine(s))
	}
	if len(sc.Success) == 0 {
		fmt.Fprintf(&b, "- %s did what they came to do.\n", name)
	}
	b.WriteString("\n## What to report\n\n")
	fmt.Fprintf(&b, "%s's one-sentence guess at what the site is and whether the site proved it right. For each goal, whether it worked. Anything missing, broken or confusing, with step numbers and the control or the words on the page.\n", name)
	return b.String()
}

// mustNot are the rules every scenario carries. A rule is left out only when the plan already says it.
var mustNot = []struct {
	re   *regexp.Regexp
	text string
}{
	{regexp.MustCompile(`(?i)\b(?:do not|don't|never)\s+(?:send|submit)\b`), "Do not send any form or message."},
	{regexp.MustCompile(`(?i)\b(?:do not|don't|never)\s+(?:sign in|log in|create an account)\b`), "Do not sign in or create an account."},
	{regexp.MustCompile(`(?i)\b(?:do not|don't|never)\s+(?:pay|buy)\b`), "Do not pay for or book anything."},
}

// settingLine matches the start of a line that the cast loader reads as a setting.
var settingLine = regexp.MustCompile(`(?i)^(device|zoom|text scale|min steps|budget|sign in as|site)\s*:`)

// oneLine keeps model text on one line, so it cannot add a heading or a "Min steps" line of its own.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(strings.TrimLeft(s, "#"))
	return settingLine.ReplaceAllString(s, "$1 -")
}
