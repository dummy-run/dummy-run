// Package wcag lists the WCAG 2.2 success criteria, so the engine keeps only criteria that exist and the
// report can name them.
package wcag

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Criterion is one success criterion.
type Criterion struct {
	Number string
	Name   string
	Level  string
}

// criteria are the WCAG 2.2 success criteria. 4.1.1 Parsing is obsolete in 2.2 and is left out.
var criteria = []Criterion{
	{"1.1.1", "Non-text Content", "A"},
	{"1.2.1", "Audio-only and Video-only (Prerecorded)", "A"},
	{"1.2.2", "Captions (Prerecorded)", "A"},
	{"1.2.3", "Audio Description or Media Alternative (Prerecorded)", "A"},
	{"1.2.4", "Captions (Live)", "AA"},
	{"1.2.5", "Audio Description (Prerecorded)", "AA"},
	{"1.2.6", "Sign Language (Prerecorded)", "AAA"},
	{"1.2.7", "Extended Audio Description (Prerecorded)", "AAA"},
	{"1.2.8", "Media Alternative (Prerecorded)", "AAA"},
	{"1.2.9", "Audio-only (Live)", "AAA"},
	{"1.3.1", "Info and Relationships", "A"},
	{"1.3.2", "Meaningful Sequence", "A"},
	{"1.3.3", "Sensory Characteristics", "A"},
	{"1.3.4", "Orientation", "AA"},
	{"1.3.5", "Identify Input Purpose", "AA"},
	{"1.3.6", "Identify Purpose", "AAA"},
	{"1.4.1", "Use of Color", "A"},
	{"1.4.2", "Audio Control", "A"},
	{"1.4.3", "Contrast (Minimum)", "AA"},
	{"1.4.4", "Resize Text", "AA"},
	{"1.4.5", "Images of Text", "AA"},
	{"1.4.6", "Contrast (Enhanced)", "AAA"},
	{"1.4.7", "Low or No Background Audio", "AAA"},
	{"1.4.8", "Visual Presentation", "AAA"},
	{"1.4.9", "Images of Text (No Exception)", "AAA"},
	{"1.4.10", "Reflow", "AA"},
	{"1.4.11", "Non-text Contrast", "AA"},
	{"1.4.12", "Text Spacing", "AA"},
	{"1.4.13", "Content on Hover or Focus", "AA"},
	{"2.1.1", "Keyboard", "A"},
	{"2.1.2", "No Keyboard Trap", "A"},
	{"2.1.3", "Keyboard (No Exception)", "AAA"},
	{"2.1.4", "Character Key Shortcuts", "A"},
	{"2.2.1", "Timing Adjustable", "A"},
	{"2.2.2", "Pause, Stop, Hide", "A"},
	{"2.2.3", "No Timing", "AAA"},
	{"2.2.4", "Interruptions", "AAA"},
	{"2.2.5", "Re-authenticating", "AAA"},
	{"2.2.6", "Timeouts", "AAA"},
	{"2.3.1", "Three Flashes or Below Threshold", "A"},
	{"2.3.2", "Three Flashes", "AAA"},
	{"2.3.3", "Animation from Interactions", "AAA"},
	{"2.4.1", "Bypass Blocks", "A"},
	{"2.4.2", "Page Titled", "A"},
	{"2.4.3", "Focus Order", "A"},
	{"2.4.4", "Link Purpose (In Context)", "A"},
	{"2.4.5", "Multiple Ways", "AA"},
	{"2.4.6", "Headings and Labels", "AA"},
	{"2.4.7", "Focus Visible", "AA"},
	{"2.4.8", "Location", "AAA"},
	{"2.4.9", "Link Purpose (Link Only)", "AAA"},
	{"2.4.10", "Section Headings", "AAA"},
	{"2.4.11", "Focus Not Obscured (Minimum)", "AA"},
	{"2.4.12", "Focus Not Obscured (Enhanced)", "AAA"},
	{"2.4.13", "Focus Appearance", "AAA"},
	{"2.5.1", "Pointer Gestures", "A"},
	{"2.5.2", "Pointer Cancellation", "A"},
	{"2.5.3", "Label in Name", "A"},
	{"2.5.4", "Motion Actuation", "A"},
	{"2.5.5", "Target Size (Enhanced)", "AAA"},
	{"2.5.6", "Concurrent Input Mechanisms", "AAA"},
	{"2.5.7", "Dragging Movements", "AA"},
	{"2.5.8", "Target Size (Minimum)", "AA"},
	{"3.1.1", "Language of Page", "A"},
	{"3.1.2", "Language of Parts", "AA"},
	{"3.1.3", "Unusual Words", "AAA"},
	{"3.1.4", "Abbreviations", "AAA"},
	{"3.1.5", "Reading Level", "AAA"},
	{"3.1.6", "Pronunciation", "AAA"},
	{"3.2.1", "On Focus", "A"},
	{"3.2.2", "On Input", "A"},
	{"3.2.3", "Consistent Navigation", "AA"},
	{"3.2.4", "Consistent Identification", "AA"},
	{"3.2.5", "Change on Request", "AAA"},
	{"3.2.6", "Consistent Help", "A"},
	{"3.3.1", "Error Identification", "A"},
	{"3.3.2", "Labels or Instructions", "A"},
	{"3.3.3", "Error Suggestion", "AA"},
	{"3.3.4", "Error Prevention (Legal, Financial, Data)", "AA"},
	{"3.3.5", "Help", "AAA"},
	{"3.3.6", "Error Prevention (All)", "AAA"},
	{"3.3.7", "Redundant Entry", "A"},
	{"3.3.8", "Accessible Authentication (Minimum)", "AA"},
	{"3.3.9", "Accessible Authentication (Enhanced)", "AAA"},
	{"4.1.2", "Name, Role, Value", "A"},
	{"4.1.3", "Status Messages", "AA"},
}

var byNumber = func() map[string]Criterion {
	m := make(map[string]Criterion, len(criteria))
	for _, c := range criteria {
		m[c.Number] = c
	}
	return m
}()

var numberRE = regexp.MustCompile(`[1-4]\.[0-9]{1,2}\.[0-9]{1,2}`)

// Lookup returns the criterion with this number, such as 2.4.4.
func Lookup(number string) (Criterion, bool) {
	c, ok := byNumber[number]
	return c, ok
}

// Clean keeps the criteria that exist from what a model wrote ("2.4.4", "SC 1.1.1 Non-text Content",
// "WCAG 2.4.6"), in number order without repeats.
func Clean(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		for _, n := range numberRE.FindAllString(v, -1) {
			if _, ok := byNumber[n]; ok && !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}

func less(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range pa {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return false
}

// Label is "2.4.4 Link Purpose (In Context)", or the number alone when it is not known.
func Label(number string) string {
	if c, ok := byNumber[number]; ok {
		return c.Number + " " + c.Name
	}
	return number
}
