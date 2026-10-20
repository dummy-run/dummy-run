package main

import (
	"fmt"
	"regexp"
	"strings"
)

// A block is a shell code block that a marker comment put under test.
type block struct {
	File string
	Line int // the line of the marker
	Site bool
	Code string
}

var (
	markerRE = regexp.MustCompile(`^\s*<!--\s*doctest(\s+site)?\s*-->\s*$`)
	fenceRE  = regexp.MustCompile("^\\s*(```+|~~~+)\\s*([a-z]*)\\s*$")
)

// extract finds the marked blocks in one Markdown file. A marker must be followed by a fenced block in
// sh, bash, shell or console, so a stale marker is an error and not silently ignored.
func extract(file, text string) ([]block, error) {
	lines := strings.Split(text, "\n")
	var out []block
	for i := 0; i < len(lines); i++ {
		m := markerRE.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		start := i + 1
		for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
			start++
		}
		if start >= len(lines) {
			return nil, fmt.Errorf("%s:%d: the doctest marker has no code block after it", file, i+1)
		}
		f := fenceRE.FindStringSubmatch(lines[start])
		if f == nil {
			return nil, fmt.Errorf("%s:%d: the doctest marker is not followed by a fenced code block", file, i+1)
		}
		switch f[2] {
		case "sh", "bash", "shell":
		default:
			return nil, fmt.Errorf("%s:%d: a doctest block must be sh, bash or shell, not %q", file, i+1, f[2])
		}
		end := start + 1
		for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), f[1]) {
			end++
		}
		if end >= len(lines) {
			return nil, fmt.Errorf("%s:%d: the code block is not closed", file, i+1)
		}
		out = append(out, block{File: file, Line: i + 1, Site: m[1] != "", Code: strings.Join(lines[start+1:end], "\n")})
		i = end
	}
	return out, nil
}
