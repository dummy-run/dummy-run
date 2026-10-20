package report

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// Badge colours. White text on each has a contrast ratio of at least 4.5, except amber, which takes dark text.
const (
	badgeBlocker = "c0392b"
	badgeMajor   = "b85c00"
	badgeMinor   = "fbad18"
	badgeNone    = "4f6d8a"
	badgeLabel   = "555555"
)

const badgeName = "Dummy Run"

// badgeSeverityColour maps the highest open severity to a colour. No findings and info take the grey-blue.
func badgeSeverityColour(sev string) string {
	switch sev {
	case "blocker":
		return badgeBlocker
	case "major":
		return badgeMajor
	case "minor":
		return badgeMinor
	}
	return badgeNone
}

// badgeMessage is the right half of the badge: the open count and the date of the newest run.
func badgeMessage(rep *Report) string {
	n := len(rep.Findings)
	what := "open"
	if rep.Basis != BasisConfirmed {
		what = "open unreviewed"
	}
	return fmt.Sprintf("%d %s · %s", n, what, badgeDate(rep))
}

// badgeDate is the end of the newest run in UTC as D Mon YYYY. A report with no runs uses the report date.
func badgeDate(rep *Report) string {
	for _, s := range []string{rep.Scope.EndedAt, rep.Date} {
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UTC().Format("2 Jan 2006")
			}
		}
	}
	return "no date"
}

func badgeColour(rep *Report) string {
	for _, sev := range Severities {
		for _, f := range rep.Findings {
			if f.Severity == sev {
				return badgeSeverityColour(sev)
			}
		}
	}
	return badgeNone
}

// badgeWidths holds the advance of a character in 11 px Verdana, rounded to a tenth. There is no font loading,
// so a badge has the same widths on every machine. The SVG pins each text to its width with textLength.
var badgeWidths = func() map[rune]float64 {
	w := map[rune]float64{}
	set := func(chars string, v float64) {
		for _, c := range chars {
			w[c] = v
		}
	}
	set("0123456789", 7.0)
	set("abcdeghknopqsuvxyz", 6.6)
	set("fjrt", 4.4)
	set("il", 3.1)
	set("mw", 10.0)
	set("ABCDEGHKNOQRSUVXYZ", 7.8)
	set("FLPTJ", 6.6)
	set("IM", 4.4)
	set("W", 10.4)
	set(" .,:;'!|", 3.8)
	set("·", 4.0)
	set("-", 4.5)
	return w
}()

func badgeTextWidth(s string) int {
	var sum float64
	for _, c := range s {
		v, ok := badgeWidths[c]
		if !ok {
			v = 7.0
		}
		sum += v
	}
	return int(math.Ceil(sum))
}

// RenderBadge writes a flat two-part SVG badge. It is small, has a title for screen readers and uses no script or font.
func RenderBadge(w io.Writer, rep *Report) error {
	msg := badgeMessage(rep)
	colour := badgeColour(rep)
	ink := "fff"
	if colour == badgeMinor {
		ink = "1d1d1b"
	}
	const icon, pad = 14, 6
	lw := pad + icon + 4 + badgeTextWidth(badgeName) + pad
	mw := pad + badgeTextWidth(msg) + pad
	total := lw + mw
	title := fmt.Sprintf("%s: %s", badgeName, msg)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s">`, total, title)
	fmt.Fprintf(&b, `<title>%s</title>`, title)
	fmt.Fprintf(&b, `<clipPath id="r"><rect width="%d" height="20" rx="3"/></clipPath>`, total)
	fmt.Fprintf(&b, `<g clip-path="url(#r)"><rect width="%d" height="20" fill="#%s"/><rect x="%d" width="%d" height="20" fill="#%s"/></g>`, lw, badgeLabel, lw, mw, colour)
	// The viewfinder mark from brand/logo/mark.svg: a disc and four corner brackets.
	fmt.Fprintf(&b, `<g transform="translate(%d 3)scale(.21875)"><circle cx="32" cy="32" r="18" fill="#fbad18"/><path fill="none" stroke="#fff" stroke-width="7" d="M3 15V3H15M49 3H61V15M61 49V61H49M15 61H3V49"/></g>`, pad)
	fmt.Fprintf(&b, `<g font-family="Verdana,DejaVu Sans,sans-serif" font-size="11" fill="#fff"><text x="%d" y="14" textLength="%d">%s</text>`, pad+icon+4, badgeTextWidth(badgeName), badgeName)
	fmt.Fprintf(&b, `<text x="%d" y="14" fill="#%s" textLength="%d">%s</text></g></svg>`, lw+pad, ink, badgeTextWidth(msg), msg)
	b.WriteByte('\n')
	_, err := io.WriteString(w, b.String())
	return err
}

// shieldsEndpoint is the JSON that img.shields.io/endpoint reads.
type shieldsEndpoint struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Tint          string `json:"color"` //nolint:misspell // the shields.io key has this spelling
	NamedLogo     string `json:"namedLogo"`
}

// RenderShields writes the shields.io endpoint JSON with the same message as the SVG badge.
func RenderShields(w io.Writer, rep *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(shieldsEndpoint{SchemaVersion: 1, Label: badgeName, Message: badgeMessage(rep), Tint: badgeColour(rep), NamedLogo: "none"})
}
