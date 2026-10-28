package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// AccessGuruCase is one violation from the AccessGuru dataset (MIT): the rule axe-core reported and the markup of the
// element it reported. The set comes from real pages. It holds no page around the element, so a case stands in for
// the element and not for the page.
type AccessGuruCase struct {
	ID     string `json:"id"`
	Rule   string `json:"rule"`
	Source string `json:"source"`
	HTML   string `json:"html"`
}

// LoadAccessGuru reads the cases file.
func LoadAccessGuru(path string) ([]AccessGuruCase, error) {
	b, err := os.ReadFile(path) //nolint:gosec // a fixture path chosen by the test
	if err != nil {
		return nil, err
	}
	var cs []AccessGuruCase
	if err := json.Unmarshal(b, &cs); err != nil {
		return nil, fmt.Errorf("benchmark: %s: %w", path, err)
	}
	for _, c := range cs {
		if c.ID == "" || c.Rule == "" || strings.TrimSpace(c.HTML) == "" {
			return nil, fmt.Errorf("benchmark: %s: case %q is missing its id, rule or markup", path, c.ID)
		}
	}
	return cs, nil
}

// Page puts the element in a small, otherwise clean page, so a rule that fires is firing on the element.
func (c AccessGuruCase) Page() string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<title>` + c.ID + `</title></head><body><main><h1>Case</h1>` + c.HTML + `</main></body></html>`
}

// Outcome turns the rules a sweep reported on the page into an outcome. The element is the whole page's only
// content, so any report of the rule counts.
func (c AccessGuruCase) Outcome(reported []string) Outcome {
	return Outcome{Page: "accessguru", Defect: c.ID, Rules: []string{c.Rule}, After: reported}
}
