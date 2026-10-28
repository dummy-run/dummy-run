package cast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCast(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "qa-lead")
	if err := os.MkdirAll(filepath.Join(dir, "scenarios"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"profile.md":                 "---\nid: qa-lead\nname: Priya\nrole: visitor\ndevice: desktop\n---\n\n# Priya, QA lead\n\nShe tests things.\n",
		"prompt.md":                  "You are Priya.\n",
		"scenarios/01-first-look.md": "# First look at the home page\n\nBudget: 30 steps\nMin steps: 8\n\nRead the home page.\n",
		"scenarios/02-find-docs.md":  "# Find the documentation\n\nLook for docs.\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLoad(t *testing.T) {
	root := writeCast(t)
	p, err := Load(root, "qa-lead")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "qa-lead" || p.Name != "Priya" || p.Role != "visitor" || p.Device != "desktop" {
		t.Errorf("persona %+v", p)
	}
	if !strings.HasPrefix(p.Profile, "# Priya, QA lead") {
		t.Errorf("profile body %q", p.Profile)
	}
	if p.Prompt != "You are Priya.\n" {
		t.Errorf("prompt %q", p.Prompt)
	}
	if len(p.Scenarios) != 2 || p.Scenarios[0].ID != "first-look" || p.Scenarios[1].ID != "find-docs" {
		t.Fatalf("scenarios %+v", p.Scenarios)
	}
	if p.Scenarios[0].Title != "First look at the home page" || p.Scenarios[0].Budget != 30 || p.Scenarios[0].MinSteps != 8 {
		t.Errorf("scenario %+v", p.Scenarios[0])
	}
	if p.Scenarios[1].MinSteps != 0 {
		t.Errorf("scenario without a minimum got %d", p.Scenarios[1].MinSteps)
	}
	if p.Scenarios[1].Budget != 0 {
		t.Errorf("scenario without a budget line got %d", p.Scenarios[1].Budget)
	}
	s, err := p.Scenario("find-docs")
	if err != nil || s.Title != "Find the documentation" {
		t.Errorf("Scenario: %+v %v", s, err)
	}
	if _, err := p.Scenario("nope"); err == nil || !strings.Contains(err.Error(), "first-look, find-docs") {
		t.Errorf("missing scenario error %v", err)
	}
	ids, err := List(root)
	if err != nil || len(ids) != 1 || ids[0] != "qa-lead" {
		t.Errorf("List %v %v", ids, err)
	}
}

func TestLoadRejectsBadPersonas(t *testing.T) {
	root := writeCast(t)
	if _, err := Load(root, "Bad ID"); err == nil {
		t.Error("accepted a bad ID")
	}
	if _, err := Load(root, "missing"); err == nil {
		t.Error("accepted a missing persona")
	}
	profile := filepath.Join(root, "qa-lead", "profile.md")
	for _, bad := range []string{
		"---\nid: other\nrole: visitor\ndevice: desktop\n---\n",
		"---\nid: qa-lead\ndevice: desktop\n---\n",
		"---\nid: qa-lead\nrole: visitor\ndevice: watch\n---\n",
		"id: qa-lead\n",
		"---\nid: qa-lead\nrole visitor\n---\n",
		"---\nid: qa-lead\n",
	} {
		if err := os.WriteFile(profile, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root, "qa-lead"); err == nil {
			t.Errorf("accepted profile %q", bad)
		}
	}
}

func TestLoadScenarioRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	noHeading := filepath.Join(dir, "01-x.md")
	if err := os.WriteFile(noHeading, []byte("no heading\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadScenario(noHeading); err == nil {
		t.Error("accepted a scenario without a heading")
	}
	badName := filepath.Join(dir, "Bad Name.md")
	if err := os.WriteFile(badName, []byte("# T\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadScenario(badName); err == nil {
		t.Error("accepted a badly named scenario")
	}
}

func TestFrontMatter(t *testing.T) {
	meta, body, err := FrontMatter("---\na: 1\nb: \"two words\"\n---\nBody\n")
	if err != nil || meta["a"] != "1" || meta["b"] != "two words" || body != "Body\n" {
		t.Errorf("%v %q %v", meta, body, err)
	}
}

func writePersona(t *testing.T, profile, scenario string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "kit")
	if err := os.MkdirAll(filepath.Join(dir, "scenarios"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"profile.md":                "---\nid: kit\nrole: visitor\ndevice: desktop\n" + profile + "---\n# Kit\n",
		"prompt.md":                 "You are Kit.\n",
		"scenarios/01-read-docs.md": "# Read\n\n" + scenario + "\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestToolLimits(t *testing.T) {
	t.Run("denied", func(t *testing.T) {
		p, err := Load(writePersona(t, "tools-denied: click, Fill\n", "Go."), "kit")
		if err != nil {
			t.Fatal(err)
		}
		for tool, want := range map[string]bool{"click": false, "fill": false, "press": true, "look": true, "finish": true} {
			if got := p.Allows(tool); got != want {
				t.Errorf("Allows(%s) = %v, want %v", tool, got, want)
			}
		}
		if got := p.Refused([]string{"look", "click", "fill"}); strings.Join(got, ",") != "click,fill" {
			t.Errorf("Refused gave %v", got)
		}
	})
	t.Run("allowed", func(t *testing.T) {
		p, err := Load(writePersona(t, "tools-allowed: press look a11y\ntools-denied: a11y\n", "Go."), "kit")
		if err != nil {
			t.Fatal(err)
		}
		for tool, want := range map[string]bool{"press": true, "look": true, "a11y": false, "click": false, "visit": false, "finish": true} {
			if got := p.Allows(tool); got != want {
				t.Errorf("Allows(%s) = %v, want %v", tool, got, want)
			}
		}
	})
	t.Run("no limits", func(t *testing.T) {
		p, err := Load(writePersona(t, "", "Go."), "kit")
		if err != nil {
			t.Fatal(err)
		}
		if !p.Allows("click") || len(p.ToolsAllowed) != 0 || len(p.ToolsDenied) != 0 {
			t.Errorf("a persona without limits was limited: %+v", p)
		}
	})
	t.Run("mistakes are errors", func(t *testing.T) {
		for profile, want := range map[string]string{
			"tools-denied: smash\n":       `"smash" is not a tool`,
			"tools-allowed: scroll tap\n": `"tap" is not a tool`,
			"tools-denied: finish\n":      "finish cannot be denied",
		} {
			if _, err := Load(writePersona(t, profile, "Go."), "kit"); err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%q: error %v, want %q", profile, err, want)
			}
		}
	})
}

func TestDisplay(t *testing.T) {
	cases := []struct {
		name, profile, scenario string
		device                  string
		zoom, cssW, cssH        int
	}{
		{"defaults", "", "Go.", "desktop", 100, 1400, 1000},
		{"persona zoom", "zoom: 200\n", "Go.", "desktop", 200, 700, 500},
		{"persona zoom with a percent sign", "zoom: 150%\n", "Go.", "desktop", 150, 933, 667},
		{"scenario device", "", "Device: phone. Start at the home page.", "phone", 100, 390, 844},
		{"scenario overrides persona zoom", "zoom: 200\n", "Zoom: 400%", "desktop", 400, 350, 250},
		{"mixed-case percent", "", "Zoom: 250 PERCENT", "desktop", 250, 560, 400},
		{"device with prose after it", "", "Device: tablet. Start at the home page.", "tablet", 100, 820, 1180},
		{"scenario sets both", "device: tablet\n", "Device: Tablet\nZoom: 200 percent", "tablet", 200, 410, 590},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Load(writePersona(t, c.profile, c.scenario), "kit")
			if err != nil {
				t.Fatal(err)
			}
			d := p.DisplayFor(p.Scenarios[0])
			if d.Device != c.device || d.Zoom != c.zoom || d.CSSWidth != c.cssW || d.CSSHeight != c.cssH {
				t.Errorf("DisplayFor = %+v, want device %s zoom %d css %dx%d", d, c.device, c.zoom, c.cssW, c.cssH)
			}
		})
	}
	for scenario, want := range map[string]string{
		"Device: watch":     `device "watch" must be desktop, tablet or phone`,
		"Zoom: 600%":        "outside 100 to 500",
		"Zoom: 50":          "outside 100 to 500",
		"Device: 4k-tv":     `device "4k-tv" must be desktop, tablet or phone`,
		"Device:":           `device "" must be desktop, tablet or phone`,
		"Zoom: two hundred": "not a percentage",
		"Zoom: x2":          "not a percentage",
		"Zoom: 200% please": "not a percentage",
	} {
		if _, err := Load(writePersona(t, "", scenario), "kit"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want %q", scenario, err, want)
		}
	}
	if _, err := Load(writePersona(t, "zoom: big\n", "Go."), "kit"); err == nil || !strings.Contains(err.Error(), "not a percentage") {
		t.Errorf("zoom: big gave %v", err)
	}
}

func TestATFollowsTheToolLimits(t *testing.T) {
	for _, c := range []struct {
		p    Persona
		want string
	}{
		{Persona{}, "none"},
		{Persona{ToolsDenied: []string{"click"}}, "keyboard-only"},
		{Persona{ToolsAllowed: []string{"press", "look"}}, "keyboard-only"},
		{Persona{ToolsDenied: []string{"screenshot"}}, "screen-reader-view"},
		{Persona{ToolsDenied: []string{"scroll"}}, "none"},
	} {
		if got := c.p.AT(); got != c.want {
			t.Errorf("%+v: AT %q, want %q", c.p, got, c.want)
		}
	}
}

func TestSigninRole(t *testing.T) {
	p := &Persona{ID: "x", Role: "Admin"}
	if r, explicit := p.SigninRole(Scenario{}); r != "admin" || explicit {
		t.Errorf("default %q %v", r, explicit)
	}
	p.Signin = "editor"
	if r, explicit := p.SigninRole(Scenario{}); r != "editor" || !explicit {
		t.Errorf("persona key %q %v", r, explicit)
	}
	if r, explicit := p.SigninRole(Scenario{Signin: "viewer"}); r != "viewer" || !explicit {
		t.Errorf("scenario line %q %v", r, explicit)
	}
	if r, explicit := p.SigninRole(Scenario{Signin: "none"}); r != "" || !explicit {
		t.Errorf("none %q %v", r, explicit)
	}
}

func TestScenarioSigninLine(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	s, err := LoadScenario(write("01-a.md", "# A\n\nSign in as: Editor\n\nDo it.\n"))
	if err != nil || s.Signin != "editor" {
		t.Fatalf("%+v %v", s, err)
	}
	s, err = LoadScenario(write("02-b.md", "# B\n\nSign in: none.\n"))
	if err != nil || s.Signin != "none" {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := LoadScenario(write("03-c.md", "# C\n\nSign in as:\n")); err == nil {
		t.Error("a Sign in line with no role was accepted")
	}
}

func TestTextScale(t *testing.T) {
	cases := []struct {
		name, profile, scenario string
		want, zoom              int
	}{
		{"defaults", "", "Go.", 100, 100},
		{"persona text scale", "text_scale: 200\n", "Go.", 200, 100},
		{"with a percent sign", "text_scale: 150%\n", "Go.", 150, 100},
		{"scenario replaces the persona's", "text_scale: 200\n", "Text scale: 300%", 300, 100},
		{"scenario can put it back to normal", "text_scale: 200\n", "Text scale: 100", 100, 100},
		{"words", "", "Text scale: 250 percent", 250, 100},
		{"separate from zoom", "zoom: 200\n", "Text scale: 200%", 200, 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Load(writePersona(t, c.profile, c.scenario), "kit")
			if err != nil {
				t.Fatal(err)
			}
			d := p.DisplayFor(p.Scenarios[0])
			if d.TextScale != c.want || d.Zoom != c.zoom {
				t.Errorf("DisplayFor = %+v, want text scale %d zoom %d", d, c.want, c.zoom)
			}
			if d.CSSWidth != (d.Width*100+d.Zoom/2)/d.Zoom {
				t.Errorf("text scale must not change the viewport: %+v", d)
			}
		})
	}
	for scenario, want := range map[string]string{
		"Text scale: 400%":       "outside 100 to 300",
		"Text scale: 50":         "outside 100 to 300",
		"Text scale: big":        "not a percentage",
		"Text scale: 200% large": "not a percentage",
	} {
		if _, err := Load(writePersona(t, "", scenario), "kit"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want %q", scenario, err, want)
		}
	}
	for profile, want := range map[string]string{"text_scale: 301\n": "outside 100 to 300", "text_scale: huge\n": "not a percentage"} {
		if _, err := Load(writePersona(t, profile, "Go."), "kit"); err == nil || !strings.Contains(err.Error(), "text_scale") || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want text_scale and %q", profile, err, want)
		}
	}
}

func TestLoadScenarioRejectsAnOversizedMinSteps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-big.md")
	if err := os.WriteFile(path, []byte("# Big\n\nMin steps: 99999999999999999999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadScenario(path); err == nil || !strings.Contains(err.Error(), "min steps") {
		t.Errorf("want a min steps error, got %v", err)
	}
}

func TestLoadScenarioRejectsAMalformedMinSteps(t *testing.T) {
	for value, want := range map[string]int{"8": 8, "8 steps": 8, "8\r": 8, "nope": -1, "8 or so": -1, "": -1, "-3": -1} {
		path := filepath.Join(t.TempDir(), "01-min.md")
		if err := os.WriteFile(path, []byte("# Min\n\nMin steps: "+value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := LoadScenario(path)
		if want < 0 {
			if err == nil {
				t.Errorf("Min steps: %q should be an error", value)
			}
			continue
		}
		if err != nil || s.MinSteps != want {
			t.Errorf("Min steps: %q gave %d, %v, want %d", value, s.MinSteps, err, want)
		}
	}
}

func TestLoadScenarioReadsTheSiteLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-site.md")
	body := "# Who is this\n\nSite:  personal site - people who want to know who the author is  \nDevice: desktop.\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadScenario(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Site != "personal site - people who want to know who the author is" {
		t.Errorf("Site = %q", s.Site)
	}
	long := "# Long\n\nSite: " + strings.Repeat("x", 400) + "\n"
	if err := os.WriteFile(path, []byte(long), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err = LoadScenario(path); err != nil || len([]rune(s.Site)) != maxSiteRunes {
		t.Errorf("a long Site line should be cut to %d runes, got %d (%v)", maxSiteRunes, len([]rune(s.Site)), err)
	}
}
