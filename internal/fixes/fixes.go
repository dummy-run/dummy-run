// Package fixes holds how to fix each rule Dummy Run's own checks report: the steps, who usually takes them, an
// example and the guidance the advice follows. The report joins an entry to every finding with the same rule ID.
package fixes

import (
	_ "embed"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"
)

//go:embed fixes.yaml
var data []byte

// Owners are the people who usually make a fix.
var Owners = []string{"content", "design", "development"}

// Fix says how to fix one rule.
type Fix struct {
	Rule    string   `yaml:"rule" json:"-"`
	Owner   string   `yaml:"owner" json:"owner"`
	Summary string   `yaml:"summary" json:"summary"`
	Steps   []string `yaml:"steps" json:"steps"`
	Example *Example `yaml:"example,omitempty" json:"example,omitempty"`
	Sources []string `yaml:"sources,omitempty" json:"sources,omitempty"`
}

// Example is markup or CSS before and after the fix.
type Example struct {
	Before string `yaml:"before" json:"before"`
	After  string `yaml:"after" json:"after"`
}

var byRule map[string]*Fix

func init() {
	all, err := load(data)
	if err != nil {
		panic(err)
	}
	byRule = all
}

func load(b []byte) (map[string]*Fix, error) {
	var list []*Fix
	if err := yaml.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("fixes: %w", err)
	}
	out := map[string]*Fix{}
	for _, f := range list {
		if out[f.Rule] != nil {
			return nil, fmt.Errorf("fixes: %s is listed twice", f.Rule)
		}
		out[f.Rule] = f
	}
	return out, nil
}

// For returns how to fix a rule, or nil when the library has nothing for it. A rule named after a WCAG criterion,
// such as dummyrun-engines-1.1.1, gets the entry for its prefix.
func For(rule string) *Fix {
	if f := byRule[rule]; f != nil {
		return f
	}
	for prefix, f := range byRule {
		if strings.HasSuffix(prefix, "-") && strings.HasPrefix(rule, prefix) {
			return f
		}
	}
	return nil
}

// Rules lists the rule IDs the library covers, prefixes included.
func Rules() []string {
	out := make([]string, 0, len(byRule))
	for r := range byRule {
		out = append(out, r)
	}
	return out
}
