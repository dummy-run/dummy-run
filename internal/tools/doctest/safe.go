package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Only commands that cannot change anything outside the working tree run. A block with any other line
// is an error, so nobody can mark a block that calls a model or a backend.

var (
	shellMeta = regexp.MustCompile("[;&|<>`$()\\\\]")
	localURL  = regexp.MustCompile(`^https?://(127\.0\.0\.1|localhost)(:[0-9]+)?(/|$)`)
	headTail  = regexp.MustCompile(`^(.*\S)\s+\|\s+head -n [0-9]+$`)
)

// commands joins backslash continuations and returns the command lines of a block. Blank lines and
// comment lines are dropped.
func commands(code string) []string {
	var out []string
	var cur strings.Builder
	for _, raw := range strings.Split(code, "\n") {
		line := strings.TrimSpace(raw)
		if cur.Len() == 0 && (line == "" || strings.HasPrefix(line, "#")) {
			continue
		}
		if strings.HasSuffix(line, "\\") {
			cur.WriteString(strings.TrimSpace(strings.TrimSuffix(line, "\\")) + " ")
			continue
		}
		cur.WriteString(line)
		out = append(out, cur.String())
		cur.Reset()
	}
	if cur.Len() > 0 {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

// checkSafe returns nil when the command may run. A single "| head -n N" at the end is allowed, to cut long output.
func checkSafe(line string) error {
	if m := headTail.FindStringSubmatch(line); m != nil {
		line = m[1]
	}
	if shellMeta.MatchString(line) {
		return fmt.Errorf("%q: pipes, redirects, substitutions and command lists are not allowed", line)
	}
	words := strings.Fields(line)
	if len(words) < 2 {
		return fmt.Errorf("%q: not a dummyrun or dummyrun-otel command", line)
	}
	args := words[1:]
	switch words[0] {
	case "dummyrun", "dummyrun-otel":
	default:
		return fmt.Errorf("%q: only dummyrun and dummyrun-otel run in a doctest block", line)
	}
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "-help" {
			return nil
		}
	}
	switch {
	case args[0] == "version" && len(args) == 1:
		return nil
	case words[0] == "dummyrun-otel" && args[0] == "export" && contains(args, "--dry-run"):
		return nil
	case words[0] == "dummyrun" && args[0] == "a11y":
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "http") && !localURL.MatchString(a) {
				return fmt.Errorf("%q: a11y may only test a site served on this machine", line)
			}
		}
		return nil
	}
	return fmt.Errorf("%q: only version, help, export --dry-run and a11y on a local site may run", line)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
