// Package identity gives a finding its problem identity: a normalised route, a locus, a symptom from a
// closed taxonomy and the fingerprint computed from the three. The engine computes it, never the model.
//
// The functions reproduce the reference implementation in the coverage research
// (analysis/coverage_lib.py). testdata/fingerprint_vectors.json holds its golden vectors.
package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TaxonomyVersion is the version of the closed symptom list. Changing the base list, the route rules or
// the fingerprint definition is a version bump.
const TaxonomyVersion = 1

// Symptoms is the closed list of base symptoms. missing-information and a11y take a parameter.
var Symptoms = []string{
	"not-reachable", "off-screen", "obscured", "not-operable", "wrong-outcome", "missing-information",
	"misleading-copy", "unclear-label", "layout-broken", "slow", "error-shown", "dead-link", "focus-problem",
	"access-wrongly-allowed", "access-wrongly-denied", "content-inconsistent", "a11y",
}

// SymptomMeaning says in one line what each base symptom means, for the persona's tool description.
var SymptomMeaning = map[string]string{
	"not-reachable":          "the needed page or control cannot be reached by any route you tried",
	"off-screen":             "the control exists but sits outside the viewport and cannot be scrolled to",
	"obscured":               "covered by another element (keyboard, banner, overlay, sticky bar)",
	"not-operable":           "visible but does not respond to the input mode (tap, key, voice)",
	"wrong-outcome":          "the action runs and gives a result other than the one the page promises",
	"missing-information":    "content you needed is not stated (give a topic)",
	"misleading-copy":        "text that points the reader to a wrong conclusion",
	"unclear-label":          "a control or heading whose name does not say what it does",
	"layout-broken":          "overlap, clipping or reflow failure that does not hide a control",
	"slow":                   "response or render time beyond what a person would wait",
	"error-shown":            "an error page, console error or failed request you saw",
	"dead-link":              "a link target returns an error status",
	"focus-problem":          "focus order, focus loss or a missing focus indicator",
	"access-wrongly-allowed": "a role did or saw what the policy denies",
	"access-wrongly-denied":  "a role was stopped from what the policy allows",
	"content-inconsistent":   "two places on the app say different things",
	"a11y":                   "an automated accessibility rule failure (give the axe rule id)",
}

var (
	slugRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+){0,3}\z`)
	ruleRE = regexp.MustCompile(`^[a-z0-9-]+\z`)
	hexRE  = regexp.MustCompile(`^(?i:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9a-f]{12,})\z`)
)

// isSpace is Python's str.isspace set, so the Go and Python references split text the same way.
func isSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

func fields(s string) []string { return strings.FieldsFunc(s, isSpace) }

// isID reports whether a path segment is all digits (any Unicode decimal digit), a UUID or 12 or more hex characters.
func isID(seg string) bool {
	if seg == "" {
		return false
	}
	allDigits := true
	for _, r := range seg {
		if !unicode.IsDigit(r) {
			allDigits = false
			break
		}
	}
	return allDigits || hexRE.MatchString(seg)
}

// split follows Python's urllib.parse.urlsplit closely enough for routes: scheme, authority, path, query, fragment.
func split(raw string) (path, query string) {
	raw = strings.TrimLeftFunc(raw, func(r rune) bool { return r <= 0x20 })
	raw = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(raw)
	if i := strings.IndexByte(raw, ':'); i > 0 && isASCIIAlpha(raw[0]) {
		ok := true
		for j := 0; j < i; j++ {
			c := raw[j]
			if !isASCIIAlpha(c) && (c < '0' || c > '9') && c != '+' && c != '-' && c != '.' {
				ok = false
				break
			}
		}
		if ok {
			raw = raw[i+1:]
		}
	}
	if strings.HasPrefix(raw, "//") {
		rest := raw[2:]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			raw = rest[j:]
		} else {
			raw = ""
		}
	}
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i], raw[i+1:]
	}
	return raw, ""
}

func isASCIIAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

// unquote decodes percent escapes and '+' the way Python's parse_qsl does: bad escapes stay as written
// and bytes that are not UTF-8 become U+FFFD.
func unquote(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	if !strings.Contains(s, "%") {
		return strings.ToValidUTF8(s, "�")
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			d, err := hex.DecodeString(s[i+1 : i+3])
			if err == nil {
				b = append(b, d[0])
				i += 2
				continue
			}
		}
		b = append(b, s[i])
	}
	if !utf8.Valid(b) {
		return strings.ToValidUTF8(string(b), "�")
	}
	return string(b)
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// NormaliseRoute keeps the path only. The scheme, host and fragment and a trailing slash are dropped;
// numeric, UUID and long hex path segments become ":id"; the query becomes the sorted set of parameter
// names. The path keeps its case.
func NormaliseRoute(urlOrPath string) string {
	p, q := split(urlOrPath)
	var segs []string
	for _, s := range strings.Split(p, "/") {
		if s == "" {
			continue
		}
		if isID(s) {
			s = ":id"
		}
		segs = append(segs, s)
	}
	path := "/" + strings.Join(segs, "/")
	names := map[string]bool{}
	for _, pair := range strings.Split(q, "&") {
		if pair == "" {
			continue
		}
		k, _, _ := strings.Cut(pair, "=")
		names[unquote(k)] = true
	}
	if len(names) == 0 {
		return path
	}
	list := make([]string, 0, len(names))
	for k := range names {
		list = append(list, k)
	}
	sort.Strings(list)
	return path + "?" + strings.Join(list, "&")
}

func clean(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(fields(b.String()), " ")
}

// NormaliseLocus names where the symptom is: "role:accessible name" for a control, "region:name" for a
// landmark or heading, or "page".
func NormaliseLocus(role, name, region string) string {
	switch {
	case role != "" && name != "":
		return clean(role) + ":" + clean(name)
	case region != "":
		return "region:" + clean(region)
	}
	return "page"
}

// ValidSymptom reports whether s is in the closed taxonomy.
func ValidSymptom(s string) bool {
	base, param, hasParam := strings.Cut(s, ":")
	found := false
	for _, b := range Symptoms {
		if b == base {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	switch base {
	case "missing-information":
		return hasParam && slugRE.MatchString(param)
	case "a11y":
		return hasParam && ruleRE.MatchString(param)
	}
	return !hasParam
}

// Slug turns words into a topic slug of at most four lower-case words joined by hyphens, or "" if nothing is left.
func Slug(s string) string {
	var words []string
	for _, w := range fields(clean(s)) {
		words = append(words, w)
		if len(words) == 4 {
			break
		}
	}
	return strings.Join(words, "-")
}

// Fingerprint is the first 16 hex characters of SHA-256 over "route|locus|symptom", with route and locus
// normalised first. It fails for a symptom outside the closed taxonomy.
func Fingerprint(route, locus, symptom string) (string, error) {
	if !ValidSymptom(symptom) {
		return "", fmt.Errorf("symptom not in the closed taxonomy: %q", symptom)
	}
	key := NormaliseRoute(route) + "|" + strings.Join(fields(strings.ToLower(locus)), " ") + "|" + symptom
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:16], nil
}
