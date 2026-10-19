// Package a11y runs a deterministic accessibility sweep. It loads pages in a real browser over
// WebDriver BiDi, runs the bundled axe-core rules for WCAG 2.2 A and AA, and reports what failed.
// It makes no model calls.
package a11y

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// AllowEntry is one accepted violation. It says which rule is accepted, on which pages, on which
// elements, and why.
type AllowEntry struct {
	Rule     string
	Page     string // a path pattern; "*" matches any run of characters
	Selector string // a pattern matched against the element's selector; "*" matches any run
	Reason   string
	Line     int
	used     bool
}

// AllowList is the set of accepted violations.
type AllowList struct {
	Entries []*AllowEntry
}

// ParseAllowList reads an allow-list. One entry per line, four fields separated by " | ":
//
//	rule | page | selector | reason
//
// The page is matched against the URL path, the selector against the element's selector. In both,
// "*" matches any run of characters and everything else must match exactly, except that the
// selector may also match as a plain substring when it has no "*". Blank lines and lines that
// start with "#" are ignored. A reason is required: an accepted violation with no reason is an
// error, so the list cannot fill up with unexplained exceptions.
func ParseAllowList(r io.Reader) (*AllowList, error) {
	var list AllowList
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimRight(sc.Text(), " \t\r")
		if t := strings.TrimSpace(line); t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		parts := strings.Split(line, " | ")
		if len(parts) < 4 {
			return nil, fmt.Errorf("allow-list line %d: want \"rule | page | selector | reason\", got %d field(s)", n, len(parts))
		}
		// A reason may itself contain " | ".
		e := &AllowEntry{
			Rule:     strings.TrimSpace(parts[0]),
			Page:     strings.TrimSpace(parts[1]),
			Selector: strings.TrimSpace(parts[2]),
			Reason:   strings.TrimSpace(strings.Join(parts[3:], " | ")),
			Line:     n,
		}
		switch {
		case e.Rule == "":
			return nil, fmt.Errorf("allow-list line %d: the rule is empty", n)
		case strings.ContainsAny(e.Rule, "* \t"):
			return nil, fmt.Errorf("allow-list line %d: the rule must be one axe rule ID, not %q", n, e.Rule)
		case e.Page == "":
			return nil, fmt.Errorf("allow-list line %d: the page is empty (use * for every page)", n)
		case e.Selector == "":
			return nil, fmt.Errorf("allow-list line %d: the selector is empty (use * for every element)", n)
		case e.Reason == "":
			return nil, fmt.Errorf("allow-list line %d: a reason is required", n)
		}
		list.Entries = append(list.Entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("allow-list: %w", err)
	}
	return &list, nil
}

// Match returns the entry that accepts this violation, or nil. A nil list accepts nothing.
func (l *AllowList) Match(rule, path, selector string) *AllowEntry {
	if l == nil {
		return nil
	}
	for _, e := range l.Entries {
		if e.Rule != rule || !globMatch(e.Page, path) {
			continue
		}
		if strings.Contains(e.Selector, "*") {
			if !globMatch(e.Selector, selector) {
				continue
			}
		} else if !strings.Contains(selector, e.Selector) {
			continue
		}
		e.used = true
		return e
	}
	return nil
}

// Unused lists entries that matched nothing, so stale exceptions can be removed.
func (l *AllowList) Unused() []*AllowEntry {
	if l == nil {
		return nil
	}
	var out []*AllowEntry
	for _, e := range l.Entries {
		if !e.used {
			out = append(out, e)
		}
	}
	return out
}

// globMatch reports whether s matches pattern, where "*" matches any run of characters
// (including none) and every other character matches itself.
func globMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(s, p)
		if i < 0 {
			return false
		}
		s = s[i+len(p):]
	}
	return strings.HasSuffix(s, last) && len(s) >= len(last)
}
