package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Name    string       `xml:"name,attr"`
	Tests   int          `xml:"tests,attr"`
	Failure int          `xml:"failures,attr"`
	Skipped int          `xml:"skipped,attr"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name    string      `xml:"name,attr"`
	Tests   int         `xml:"tests,attr"`
	Failure int         `xml:"failures,attr"`
	Skipped int         `xml:"skipped,attr"`
	Cases   []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

// renderJUnit writes one test case for each distinct problem and each failing axe rule. Blocker,
// major and minor problems fail. Info problems are reported as skipped, so a job can stay green
// on them. Findings that are not reviewed say so in the message.
func renderJUnit(w io.Writer, rep *Report) error {
	persona := junitSuite{Name: "dummyrun personas"}
	for _, f := range rep.Findings {
		c := junitCase{Name: f.Title, Classname: "dummyrun." + orStr(f.Symptom, f.Kind)}
		msg := f.Severity + ": " + f.Title
		if f.Status != StatusConfirmed {
			msg += " (not reviewed)"
		}
		if f.Severity == "info" {
			c.Skipped = &junitSkipped{Message: msg}
			persona.Skipped++
		} else {
			text := fmt.Sprintf("Route %s. Found by %s in %d of %d runs. Fingerprint %s.", f.Route, joinList(f.Personas), f.Runs, f.OfRuns, f.Fingerprint)
			c.Failure = &junitFailure{Message: msg, Type: f.Severity, Text: text}
			persona.Failure++
		}
		persona.Cases = append(persona.Cases, c)
	}
	persona.Tests = len(persona.Cases)
	suites := []junitSuite{persona}
	if rep.Axe != nil {
		ax := junitSuite{Name: "dummyrun axe sweep"}
		for _, r := range rep.Axe.Rules {
			text := ""
			for _, pg := range r.Pages {
				text += fmt.Sprintf("%s: %s\n", pg.URL, joinList(pg.Targets))
			}
			ax.Cases = append(ax.Cases, junitCase{Name: r.Help, Classname: "axe." + r.Rule,
				Failure: &junitFailure{Message: r.Impact + ": " + r.Help, Type: r.Impact, Text: text}})
			ax.Failure++
		}
		ax.Tests = len(ax.Cases)
		suites = append(suites, ax)
	}
	all := junitSuites{Name: rep.Title, Suites: suites}
	for _, s := range suites {
		all.Tests += s.Tests
		all.Failure += s.Failure
		all.Skipped += s.Skipped
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(all); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func joinList(l []string) string { return strings.Join(l, ", ") }
