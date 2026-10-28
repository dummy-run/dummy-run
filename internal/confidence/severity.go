// Package confidence decides a finding's severity and confidence from observable inputs, never from a
// model's opinion.
//
// Severity is a rule over the check's impact class, its WCAG level, whether the finding sits on a journey and
// how many pages it affects. Confidence is a lookup in a table of measured accuracy, keyed by the inputs the
// engine records on every finding: the kind of evidence, which engines agree, whether it reproduced across runs,
// and the check. The table ships with base rates and dummyrun calibrate rebuilds it from labelled findings.
package confidence

import "github.com/dummy-run/dummy-run/internal/record"

// Severities, worst first, as the run record names them.
const (
	SeverityBlocker = "blocker"
	SeverityMajor   = "major"
	SeverityMinor   = "minor"
	SeverityInfo    = "info"
)

// ManyPages is the number of affected pages from which a problem counts as site-wide.
const ManyPages = 3

// SeverityInputs are the facts the severity rule reads.
type SeverityInputs struct {
	// Evidence is the finding's evidence kind. A task-verdict finding, where the engine tried the task itself
	// and could not complete it, is a blocker whatever its impact class.
	Evidence string
	// Impact is the check's impact class: critical, serious, moderate or minor. Empty when the check has none.
	Impact string
	// Level is the WCAG level of the criterion the check maps to: A, AA, or empty when it maps to none or the
	// level is not known.
	Level string
	// OnJourney is true when the finding sits on a journey a persona was trying to complete.
	OnJourney bool
	// Pages is how many pages the problem was found on. Zero counts as one.
	Pages int
}

var impactRank = map[string]int{"critical": 3, "serious": 2, "moderate": 1, "minor": 0, "": 0}

var severityByRank = []string{SeverityInfo, SeverityMinor, SeverityMajor, SeverityBlocker}

// Severity applies the rule:
//
//  1. The impact class sets the base: critical is a blocker, serious is major, moderate is minor, and minor or
//     no impact is info.
//  2. A problem on a journey, or on three or more pages, moves up one step, to major at most. A problem on a
//     journey stands between someone and what they came to do, and a site-wide pattern is worth fixing once.
//     Neither makes a blocker on its own.
//  3. A WCAG level A failure is never below minor.
//  4. A task the engine tried itself and could not complete is a blocker.
//
// Nothing else moves up more than one step, and only a critical impact or a failed task makes a blocker.
func Severity(in SeverityInputs) string {
	if in.Evidence == record.EvidenceTaskVerdict {
		return SeverityBlocker
	}
	r := impactRank[in.Impact]
	if (in.OnJourney || in.Pages >= ManyPages) && r < impactRank["serious"] {
		r++
	}
	if in.Level == "A" && r < 1 {
		r = 1
	}
	return severityByRank[r]
}

// SymptomImpact is the impact class of each persona symptom when no check gives one: how much the problem
// matters to the person who meets it, if it is real. The a11y symptom takes the impact of the axe rule it
// names, so it is not here.
var SymptomImpact = map[string]string{
	"not-reachable":          "serious",
	"not-operable":           "serious",
	"access-wrongly-allowed": "serious",
	"access-wrongly-denied":  "serious",
	"obscured":               "serious",
	"off-screen":             "serious",
	"wrong-outcome":          "moderate",
	"error-shown":            "moderate",
	"dead-link":              "moderate",
	"focus-problem":          "moderate",
	"layout-broken":          "moderate",
	"unclear-label":          "moderate",
	"misleading-copy":        "moderate",
	"content-inconsistent":   "moderate",
	"slow":                   "moderate",
	"missing-information":    "minor",
	"a11y":                   "moderate",
}

// ImpactOfSymptom returns the impact class for a persona symptom, with its parameter removed. A symptom that
// is not in the list is moderate.
func ImpactOfSymptom(symptom string) string {
	base := symptom
	for i := 0; i < len(symptom); i++ {
		if symptom[i] == ':' {
			base = symptom[:i]
			break
		}
	}
	if v, ok := SymptomImpact[base]; ok {
		return v
	}
	return "moderate"
}
