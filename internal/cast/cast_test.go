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
		"scenarios/01-first-look.md": "# First look at the home page\n\nBudget: 30 steps\n\nRead the home page.\n",
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
	if p.Scenarios[0].Title != "First look at the home page" || p.Scenarios[0].Budget != 30 {
		t.Errorf("scenario %+v", p.Scenarios[0])
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
