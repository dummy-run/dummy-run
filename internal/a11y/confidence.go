package a11y

import (
	"github.com/dummy-run/dummy-run/internal/confidence"
	"github.com/dummy-run/dummy-run/internal/record"
)

// rate is the confidence band for an element a check failed: the kind of evidence, the rule and the engines that
// agree, looked up in the confidence table. A standalone sweep has no journey and no repeated runs, so those
// inputs are left out; the report adds them when it has persona runs beside the sweep.
func rate(evidence, rule string, engines []string) string {
	return confidence.Default().Rate(record.FindingInputs{EvidenceKind: evidence, Check: rule, Engines: engines}).Band
}
