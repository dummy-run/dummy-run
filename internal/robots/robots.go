// Package robots reads a robots.txt file as RFC 9309 describes and answers whether a path may be fetched
// by a crawler with a given product token.
package robots

import (
	"bufio"
	"bytes"
	"strings"
	"unicode/utf8"
)

// MaxBytes is how much of a robots.txt file is read. RFC 9309 asks crawlers to parse at least 500 KiB.
const MaxBytes = 500 << 10

type rule struct {
	allow   bool
	pattern string
}

// Rules are the rules that apply to one product token.
type Rules struct {
	rules []rule
	// Group is the user-agent value of the group that matched: the token, "*" or "" when nothing matched.
	Group string
	// denyAll is set when the file could not be fetched and RFC 9309 says to assume everything is disallowed.
	denyAll bool
}

// AllowAll are the rules for a site with no robots.txt.
func AllowAll() *Rules { return &Rules{} }

// DenyAll are the rules for a site whose robots.txt could not be reached because the server failed.
func DenyAll() *Rules { return &Rules{denyAll: true} }

// Parse returns the rules that apply to token. A group that names the token wins over the "*" group. The
// token matches case-insensitively and a group's rules are merged when the file names the token twice.
// Lines it does not understand are ignored.
func Parse(data []byte, token string) *Rules {
	if len(data) > MaxBytes {
		data = data[:MaxBytes]
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	token = strings.ToLower(token)

	var named, star []rule
	var sawNamed, sawStar bool
	// agents are the user-agent lines of the group being read; open is false once a rule line ends the agent list.
	var agents []string
	open := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), MaxBytes)
	for sc.Scan() {
		line := sc.Text()
		if !utf8.ValidString(line) {
			line = strings.ToValidUTF8(line, "")
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if !open {
				agents = agents[:0]
				open = true
			}
			agents = append(agents, strings.ToLower(value))
			if strings.ToLower(value) == token {
				sawNamed = true
			}
			if value == "*" {
				sawStar = true
			}
		case "allow", "disallow":
			open = false
			// An empty Disallow allows everything and an empty Allow adds nothing, so neither is kept.
			if value == "" {
				continue
			}
			r := rule{allow: key == "allow", pattern: value}
			for _, a := range agents {
				switch a {
				case token:
					named = append(named, r)
				case "*":
					star = append(star, r)
				}
			}
		default:
			// Sitemap, Crawl-delay and unknown lines do not end a group.
		}
	}
	switch {
	case sawNamed:
		return &Rules{rules: named, Group: token}
	case sawStar:
		return &Rules{rules: star, Group: "*"}
	}
	return &Rules{}
}

// Allowed reports whether a path, with its query, may be fetched. The longest matching pattern decides and
// Allow wins a tie. The path "/robots.txt" is always allowed.
func (r *Rules) Allowed(pathAndQuery string) bool {
	if pathAndQuery == "" || pathAndQuery[0] != '/' {
		pathAndQuery = "/" + pathAndQuery
	}
	if pathAndQuery == "/robots.txt" {
		return true
	}
	if r.denyAll {
		return false
	}
	best, allowed := -1, true
	for _, ru := range r.rules {
		if !matches(ru.pattern, pathAndQuery) {
			continue
		}
		n := len(ru.pattern)
		if n > best || (n == best && ru.allow) {
			best, allowed = n, ru.allow
		}
	}
	return allowed
}

// DeniesAll reports whether nothing may be fetched: the server failed, or no Allow rule exists and a
// Disallow rule covers the whole site.
func (r *Rules) DeniesAll() bool {
	if r.denyAll {
		return true
	}
	for _, ru := range r.rules {
		if ru.allow {
			return false
		}
	}
	return !r.Allowed("/")
}

// matches reports whether pattern matches the start of s. "*" matches any run of characters and a final
// "$" anchors the pattern to the end.
func matches(pattern, s string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	if anchored {
		pattern = strings.TrimSuffix(pattern, "$")
	}
	parts := strings.Split(pattern, "*")
	// The first part must match at the start.
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	pos := len(parts[0])
	for i := 1; i < len(parts); i++ {
		p := parts[i]
		if i == len(parts)-1 && anchored {
			// The last part must end the string, after the earlier parts.
			return len(s)-pos >= len(p) && strings.HasSuffix(s, p)
		}
		idx := strings.Index(s[pos:], p)
		if idx < 0 {
			return false
		}
		pos += idx + len(p)
	}
	if anchored {
		return pos == len(s)
	}
	return true
}
