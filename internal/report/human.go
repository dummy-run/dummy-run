package report

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/dummy-run/dummy-run/internal/wcag"
)

// symptomWords says each symptom of the closed taxonomy in plain words, as a short sentence fragment.
var symptomWords = map[string]string{
	"not-reachable":          "Could not be reached",
	"off-screen":             "Sat off the screen and could not be scrolled to",
	"obscured":               "Covered by something else",
	"not-operable":           "Visible but did not respond",
	"wrong-outcome":          "Did something other than what the page promised",
	"missing-information":    "Information that was needed is missing",
	"misleading-copy":        "The wording points to a wrong conclusion",
	"unclear-label":          "The name does not say what it does",
	"layout-broken":          "The layout overlaps, is clipped or does not reflow",
	"slow":                   "Slower than a person would wait",
	"error-shown":            "An error appeared",
	"dead-link":              "A link leads to an error",
	"focus-problem":          "Keyboard focus is lost, out of order or hard to see",
	"access-wrongly-allowed": "Let in when it should have been blocked",
	"access-wrongly-denied":  "Blocked when it should have been allowed",
	"content-inconsistent":   "Two places say different things",
	"a11y":                   "An automated accessibility check failed",
}

// symptomText turns a symptom such as access-wrongly-allowed, or missing-information:rates, into words.
func symptomText(s string) string {
	base, param, _ := strings.Cut(s, ":")
	w, ok := symptomWords[base]
	if !ok {
		return s
	}
	if param == "" {
		return w
	}
	if base == "a11y" {
		return w + " (" + param + ")"
	}
	return w + " (" + strings.ReplaceAll(param, "-", " ") + ")"
}

func upperFirst(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+len(string(r)):]
	}
	return s
}

// pageName turns a route such as /payments into "the Payments page".
func pageName(route string) string {
	var words []string
	for _, seg := range strings.Split(route, "/") {
		if seg == "" || strings.HasPrefix(seg, ":") {
			continue
		}
		words = append(words, strings.NewReplacer("-", " ", "_", " ").Replace(seg))
	}
	if len(words) == 0 {
		return "the Home page"
	}
	return "the " + upperFirst(strings.Join(words, " ")) + " page"
}

var locusRoles = map[string]string{
	"link": "link", "button": "button", "textbox": "text field", "combobox": "drop-down", "checkbox": "checkbox",
	"radio": "option", "heading": "heading", "tab": "tab", "menuitem": "menu item", "region": "area", "switch": "switch",
}

// locusText turns role:name, such as link:rates, into "the Rates link". A locus of page, or none, gives "".
func locusText(locus string) string {
	role, name, ok := strings.Cut(locus, ":")
	if !ok || strings.TrimSpace(name) == "" {
		return ""
	}
	word := locusRoles[role]
	if word == "" {
		word = role
	}
	return "the " + upperFirst(strings.TrimSpace(name)) + " " + word
}

// placeText says where a finding is as a person would: "the Payments page, the Rates link".
func placeText(route, locus string) string {
	var parts []string
	if route != "" {
		parts = append(parts, pageName(route))
	}
	if l := locusText(locus); l != "" {
		parts = append(parts, l)
	}
	return strings.Join(parts, ", ")
}

// withArticle puts "a" or "an" before a word by how it sounds. It looks at the first letter, with
// exceptions for words that start with a vowel but sound like "you" or "wun", such as user and one.
func withArticle(word string) string {
	lower := strings.ToLower(word)
	if lower != "" && strings.ContainsRune("aeiou", rune(lower[0])) && !soundsConsonant(lower) {
		return "an " + word
	}
	return "a " + word
}

func soundsConsonant(lower string) bool {
	for _, prefix := range []string{"user", "uni", "use", "eu", "one", "ubiq", "util"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// personaName is the first part of a persona ID, as a name: lin-contributor gives Lin.
func personaName(id string) string {
	first, _, _ := strings.Cut(id, "-")
	return upperFirst(first)
}

// whoText names the personas who hit a finding and how often: "Lin, a contributor, in 1 of 4 runs".
func whoText(rep *Report, f *Finding) string {
	roles := map[string]string{}
	names := map[string]string{}
	for _, p := range rep.Scope.Personas {
		roles[p.ID] = p.Role
		names[p.ID] = p.Name
	}
	var who []string
	for _, id := range f.Personas {
		s := orWord(names[id], personaName(id))
		if r := roles[id]; r != "" {
			s += ", " + withArticle(r) + ","
		}
		who = append(who, s)
	}
	text := strings.Join(who, " and ")
	text = strings.TrimSuffix(text, ",")
	return fmt.Sprintf("%s, in %d of %d runs", text, f.Runs, f.OfRuns)
}

func browserName(b string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(b), " ")
	return upperFirst(name)
}

func deviceText(d string) string {
	switch d {
	case "desktop", "laptop":
		return "a " + d + " computer"
	case "":
		return "a browser"
	}
	return withArticle(d)
}

// reproSteps writes the steps in the words of a person following them, from the persona's setup and the cited steps.
func reproSteps(f *Finding) []string {
	r := f.Reproduction
	var out []string
	if r.Role != "" && (r.SignedIn || !signedOutRole[strings.ToLower(r.Role)]) {
		out = append(out, "Sign in as "+withArticle(r.Role)+".")
	}
	use := "Use " + deviceText(r.Device)
	if r.Zoom > 0 && r.Zoom != 100 {
		use += fmt.Sprintf(" at %d%% zoom", r.Zoom)
	}
	if b := browserName(r.Browser); b != "" {
		use += " in " + b
	}
	out = append(out, use+".")
	for _, st := range r.Steps {
		page := ""
		if st.Route != "" {
			page = pageName(st.Route)
		}
		target := st.Target
		if st.Element != "" {
			target = elementWords(st.Element)
		} else if strings.HasPrefix(target, "@") {
			// An @number means something only inside the run, so it is no use to a reader.
			target = ""
		}
		var s string
		switch st.Action {
		case "visit":
			s = "Open " + orWord(page, "the page")
		case "look":
			s = "On " + orWord(page, "the page") + ", look at what is shown"
		case "click":
			s = "Click " + orWord(target, "the control")
		case "fill", "type":
			s = "Type into " + orWord(target, "the field")
		case "select":
			s = "Choose an option in " + orWord(target, "the list")
		case "press":
			s = "Press " + orWord(target, "a key")
		case "back":
			s = "Go back"
		case "scroll":
			s = "Scroll " + orWord(page, "the page")
		case "wait":
			s = "Wait for the page"
		case "a11y":
			s = "Run an accessibility check on " + orWord(page, "the page")
		default:
			s = cap1(describeStepText(st))
		}
		s += "."
		out = append(out, s)
	}
	return out
}

// signedOutRole lists roles that mean nobody is signed in, so the steps do not tell the reader to sign in.
var signedOutRole = map[string]bool{"visitor": true, "guest": true, "anonymous": true, "public": true, "signed-out": true}

// elementWords turns an element as the engine names it, such as link "Read more", into words a reader follows:
// the "Read more" link.
func elementWords(el string) string {
	role, name, ok := strings.Cut(el, " ")
	if !ok {
		return "the " + el
	}
	if name == "with no name" {
		return withArticle(roleWord(role)) + " with no name"
	}
	if role == "text" {
		return "the words " + name
	}
	return "the " + name + " " + roleWord(role)
}

func roleWord(role string) string {
	if w := locusRoles[role]; w != "" {
		return w
	}
	return role
}

// whereText says where the evidence is: the element and the page address.
func whereText(f *Finding) string {
	var parts []string
	if f.Element != "" {
		parts = append(parts, upperFirst(elementWords(f.Element)))
	}
	if f.URL != "" {
		if len(parts) > 0 {
			return parts[0] + " on " + f.URL
		}
		return f.URL
	}
	return strings.Join(parts, "")
}

// wcagText names each criterion: "2.4.4 Link Purpose (In Context)".
func wcagText(nums []string) string {
	out := make([]string, len(nums))
	for i, n := range nums {
		out[i] = wcag.Label(n)
	}
	return strings.Join(out, "; ")
}

func describeStepText(st StepRef) string {
	return strings.TrimSpace(st.Action + " " + st.Target)
}

func orWord(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// shotCaption says what a screenshot shows: "The Payments page as Lin saw it".
func shotCaption(f *Finding, s Screenshot) string {
	page := "the page"
	for _, st := range f.Reproduction.Steps {
		if st.N == s.Step && st.Route != "" {
			page = pageName(st.Route)
		}
	}
	return upperFirst(page) + " as " + personaName(f.Reproduction.Persona) + " saw it"
}

// lookAt tells the reader what to look at in the screenshot of a permission or security finding.
func lookAt(f *Finding) string {
	if f.Kind != "permission" && f.Kind != "security" {
		return ""
	}
	if l := locusText(f.Locus); l != "" {
		return "What to look at: " + l + "."
	}
	return ""
}
