package tech

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// webappanalyzer's category IDs that map to the categories here. The rest are Other.
var waCategories = map[int]string{
	6: Shop, 108: Shop,
	29: Search,
	72: Booking, 93: Booking, 104: Booking,
	110: Forms,
	41:  Payments, 91: Payments,
	67: Consent,
	1:  CMS, 11: CMS,
	51: SiteBuilder,
	12: Framework, 18: Framework, 66: Framework,
}

// FromEnv reads the webappanalyzer fingerprints from the directory DUMMYRUN_WEBAPPANALYZER names, when it is set.
// It returns nothing when the variable is empty.
func FromEnv() ([]Fingerprint, error) {
	dir := os.Getenv("DUMMYRUN_WEBAPPANALYZER")
	if dir == "" {
		return nil, nil
	}
	return LoadWebappanalyzer(dir)
}

// LoadWebappanalyzer reads a copy of webappanalyzer (https://github.com/enthec/webappanalyzer, GPL-3.0): the
// repository or its src directory. Its fingerprints are turned into ours at run time and never written anywhere.
// Only the markers the page itself shows are used: js, scriptSrc, meta, dom, html and cookies. Headers, URLs and
// XHR patterns are left out, and a js marker counts when the global exists, whatever its value.
func LoadWebappanalyzer(dir string) ([]Fingerprint, error) {
	if _, err := os.Stat(filepath.Join(dir, "src", "technologies")); err == nil { //nolint:gosec // a directory the user chose
		dir = filepath.Join(dir, "src")
	}
	files, err := filepath.Glob(filepath.Join(dir, "technologies", "*.json"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("tech: no technologies/*.json under %s; point DUMMYRUN_WEBAPPANALYZER at a webappanalyzer checkout", dir)
	}
	var out []Fingerprint
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // a directory the user chose
		if err != nil {
			return nil, err
		}
		var techs map[string]waTech
		if err := json.Unmarshal(b, &techs); err != nil {
			return nil, fmt.Errorf("tech: %s: %w", filepath.Base(f), err)
		}
		for name, t := range techs {
			if fp, ok := t.fingerprint(name); ok {
				out = append(out, fp)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type waTech struct {
	Cats      []int             `json:"cats"`
	JS        map[string]string `json:"js"`
	ScriptSrc json.RawMessage   `json:"scriptSrc"`
	HTML      json.RawMessage   `json:"html"`
	Meta      map[string]any    `json:"meta"`
	Cookies   map[string]string `json:"cookies"`
	DOM       json.RawMessage   `json:"dom"`
}

// pattern drops webappanalyzer's \;version: and \;confidence: tags, which are not part of the regular expression.
func pattern(p string) string {
	if i := strings.Index(p, `\;`); i >= 0 {
		return p[:i]
	}
	return p
}

// strs reads a field that is either one string or a list of them.
func strs(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	return nil
}

// dom reads webappanalyzer's dom field: a selector, a list of them, or an object keyed by selector. In the object
// form, an entry with exists is a plain selector, and one with attributes or text needs them to match too. An
// entry that checks only properties is left out, since a property can't be read from markup.
func dom(raw json.RawMessage) (plain []string, checks []DOMCheck) {
	if s := strs(raw); s != nil {
		return s, nil
	}
	var obj map[string]struct {
		Exists     *string           `json:"exists"`
		Attributes map[string]string `json:"attributes"`
		Text       *string           `json:"text"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return nil, nil
	}
	for sel, c := range obj {
		switch {
		case c.Exists != nil:
			plain = append(plain, sel)
		case len(c.Attributes) > 0 || c.Text != nil:
			d := DOMCheck{Selector: sel, Attributes: map[string]string{}}
			for a, p := range c.Attributes {
				d.Attributes[a] = pattern(p)
			}
			if c.Text != nil {
				d.Text = pattern(*c.Text)
				if d.Text == "" {
					d.Text = "\\S"
				}
			}
			checks = append(checks, d)
		}
	}
	sort.Strings(plain)
	sort.Slice(checks, func(i, j int) bool { return checks[i].Selector < checks[j].Selector })
	return plain, checks
}

func (t waTech) fingerprint(name string) (Fingerprint, bool) {
	fp := Fingerprint{Name: name, Category: Other, Source: "webappanalyzer", Meta: map[string]string{}}
	for _, c := range t.Cats {
		if cat, ok := waCategories[c]; ok {
			fp.Category = cat
			break
		}
	}
	for g := range t.JS {
		fp.Globals = append(fp.Globals, g)
	}
	for _, s := range strs(t.ScriptSrc) {
		fp.Scripts = append(fp.Scripts, pattern(s))
	}
	for _, h := range strs(t.HTML) {
		fp.HTML = append(fp.HTML, pattern(h))
	}
	for k, v := range t.Meta {
		switch v := v.(type) {
		case string:
			fp.Meta[k] = pattern(v)
		case []any:
			if len(v) > 0 {
				if s, ok := v[0].(string); ok {
					fp.Meta[k] = pattern(s)
				}
			}
		}
	}
	for c := range t.Cookies {
		fp.Cookies = append(fp.Cookies, "^"+regexp.QuoteMeta(c)+"$")
	}
	fp.Selectors, fp.DOM = dom(t.DOM)
	sort.Strings(fp.Globals)
	sort.Strings(fp.Cookies)
	if len(fp.Globals)+len(fp.Scripts)+len(fp.HTML)+len(fp.Meta)+len(fp.Cookies)+len(fp.Selectors)+len(fp.DOM) == 0 {
		return fp, false
	}
	return fp, true
}
