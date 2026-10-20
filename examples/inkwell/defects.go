package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// switches lists every behaviour a manifest can turn on. Each one changes a single
// place in the app, named after the thing it breaks. With no manifest the app has
// none of them and behaves as documented on /help.
var switches = map[string]string{
	// Permissions: access given or taken away wrongly.
	"perm-viewer-edit":         "a viewer can open and save the edit form for any draft",
	"perm-editor-self-publish": "an editor can publish a draft she owns",
	"perm-reviewer-payments":   "a reviewer can read the payments page",
	"perm-author-edit-others":  "an author can edit another author's draft by address",
	"perm-editor-users":        "an editor can read the user list",
	"perm-owner-self-review":   "an author can approve a draft she owns",
	"perm-editor-settings":     "an editor can open and save the settings form",
	"perm-editor-delete":       "an editor is offered Delete and can delete any draft",
	"perm-reviewer-denied":     "a reviewer is refused the reviews page",
	// Broken flows.
	"bug-signin-silent":     "a wrong password returns to the home page with no message",
	"bug-edit-title-lost":   "saving a draft keeps the body but drops the new title",
	"bug-issue-past-500":    "scheduling an issue in the past returns a server error",
	"bug-submit-noop":       "Submit for review says it worked and changes nothing",
	"bug-search-ignored":    "the draft search box ignores what is typed",
	"bug-dead-link":         "the help page links to a style guide that does not exist",
	"bug-page-next-loop":    "the Next link on the draft list goes back to page 1",
	"bug-payments-total":    "the payments total leaves out the last row",
	"bug-list-stale-status": "a published draft shows as approved in the list",
	// Accessibility faults.
	"a11y-edit-title-nolabel": "the draft title field has no label",
	"a11y-signin-pw-nolabel":  "the password field has no label",
	"a11y-status-contrast":    "status badges are pale grey on white",
	"a11y-delete-noname":      "the delete button is an icon with no name",
	"a11y-issue-img-noalt":    "issue cover images have no alt attribute",
	"a11y-help-dialog-trap":   "the keyboard help dialog cannot be left with the keyboard",
	"a11y-payments-nozoom":    "the payments page stops the user zooming the page",
	"a11y-reviews-notitle":    "the reviews page has an empty title",
	"a11y-focus-hidden":       "links lose their focus outline",
	// Friction.
	"fric-feedback-covers-save":  "on a phone a fixed Feedback bar covers the Save button",
	"fric-banner-covers-publish": "on a phone a fixed notice covers the Publish button",
	"fric-error-vague":           "an empty title says only Invalid",
	"fric-date-format":           "the issue date needs DD-MM-YYYY and the field gives no hint",
	"fric-signout-hover":         "Sign out only appears while the mouse hovers over the menu",
	"fric-jargon-slots":          "the issues page has an unexplained Embargo slots column",
	"fric-reviews-slow":          "the reviews page takes three seconds",
	"fric-session-short":         "the session ends after two minutes with no warning",
	"fric-payments-wide":         "on a phone the payments table runs off the screen with no scroll",
}

// Defect is one entry of a manifest. The app reads only Switch. The other fields
// are the ground truth that a scorer reads, so the app ignores them.
type Defect struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Severity    string `json:"severity,omitempty"`
	Route       string `json:"route"`
	State       string `json:"state,omitempty"`
	Locus       string `json:"locus"`
	Symptom     string `json:"symptom"`
	WCAG        string `json:"wcag,omitempty"`
	Condition   string `json:"condition,omitempty"`
	Depth       int    `json:"depth,omitempty"`
	Switch      string `json:"switch"`
	Description string `json:"description,omitempty"`
}

// Manifest is the file given to -defects.
type Manifest struct {
	Version   int      `json:"version"`
	Name      string   `json:"name,omitempty"`
	Injection bool     `json:"injection,omitempty"`
	Defects   []Defect `json:"defects"`
}

// LoadManifest reads and checks a manifest. It rejects unknown fields, duplicate IDs
// and switches the app does not have, so a typo cannot silently plant nothing.
func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseManifest(raw)
}

// ParseManifest is LoadManifest for bytes.
func ParseManifest(raw []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(bytesReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	// A second JSON value after the manifest would be dropped without a word, so it is an error.
	if dec.More() {
		return nil, fmt.Errorf("manifest: unexpected data after the manifest")
	}
	if m.Version != 1 {
		return nil, fmt.Errorf("manifest: version must be 1, got %d", m.Version)
	}
	seen := map[string]bool{}
	seenSwitch := map[string]bool{}
	for _, d := range m.Defects {
		if d.ID == "" {
			return nil, fmt.Errorf("manifest: a defect has no id")
		}
		if seen[d.ID] {
			return nil, fmt.Errorf("manifest: duplicate id %s", d.ID)
		}
		seen[d.ID] = true
		if _, ok := switches[d.Switch]; !ok {
			return nil, fmt.Errorf("manifest: %s names unknown switch %q", d.ID, d.Switch)
		}
		if seenSwitch[d.Switch] {
			return nil, fmt.Errorf("manifest: switch %q used twice", d.Switch)
		}
		seenSwitch[d.Switch] = true
	}
	return &m, nil
}

// Enabled returns the set of switches the manifest turns on.
func (m *Manifest) Enabled() map[string]bool {
	out := map[string]bool{}
	if m == nil {
		return out
	}
	for _, d := range m.Defects {
		out[d.Switch] = true
	}
	return out
}

// SwitchNames lists every known switch in order.
func SwitchNames() []string {
	names := make([]string, 0, len(switches))
	for k := range switches {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
