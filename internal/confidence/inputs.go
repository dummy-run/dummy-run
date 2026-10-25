package confidence

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dummy-run/dummy-run/internal/record"
)

// Any is the wildcard in a bucket key: the bucket pools every value of that field.
const Any = "*"

// None is the key value for a field that does not apply: no engines, or no repeated runs.
const None = "-"

// Key names a bucket of the confidence table. Each field is an exact value, None or Any.
type Key struct {
	Evidence string `json:"evidence_kind"`
	Check    string `json:"check"`
	Engines  string `json:"engines"`
	Repro    string `json:"reproduced"`
}

// String writes the key as evidence|check|engines|reproduced.
func (k Key) String() string {
	return k.Evidence + "|" + k.Check + "|" + k.Engines + "|" + k.Repro
}

// Reproduction classes.
const (
	ReproAll  = "all"  // found in every run that visited where it is
	ReproSome = "some" // found in more than one of them, but not all
	ReproOnce = "once" // found in one of several
)

// EngineNames are the names a report shows for each engine's short name.
var EngineNames = map[string]string{
	"axe":    "axe-core",
	"ibm":    "IBM Equal Access",
	"htmlcs": "HTML_CodeSniffer",
	"alfa":   "Siteimprove Alfa",
}

// EngineName returns the display name of an engine, or the short name when it has none.
func EngineName(short string) string {
	if n, ok := EngineNames[short]; ok {
		return n
	}
	return short
}

// EnginesClass is the key value for a set of engines: their short names sorted and joined with "+", or None.
func EnginesClass(engines []string) string {
	if len(engines) == 0 {
		return None
	}
	s := slices.Clone(engines)
	slices.Sort(s)
	return strings.Join(slices.Compact(s), "+")
}

// ReproClass is the key value for a reproduction count: None when there were no repeated runs.
func ReproClass(r *record.Reproduced) string {
	if r == nil || r.Of < 2 {
		return None
	}
	switch {
	case r.Runs >= r.Of:
		return ReproAll
	case r.Runs <= 1:
		return ReproOnce
	}
	return ReproSome
}

// KeyOf is the exact key of a finding's inputs.
func KeyOf(in record.FindingInputs) Key {
	check := in.Check
	if check == "" {
		check = None
	}
	return Key{Evidence: in.Evidence(), Check: check, Engines: EnginesClass(in.Engines), Repro: ReproClass(in.Reproduced)}
}

// Candidates lists the keys to look up for a finding, most specific first. The check is tried exact and
// then as Any. The engine set is tried exact, then each subset of it (largest first): more engines agreeing is
// never weaker evidence than fewer, so the accuracy measured for a subset is a floor. It is never Any, so an
// engine with no record of its own borrows nothing from the others. Reproduction is tried exact and then as Any.
func Candidates(in record.FindingInputs) []Key {
	exact := KeyOf(in)
	engines := []string{exact.Engines}
	if exact.Engines != None {
		engines = append(engines, subsets(strings.Split(exact.Engines, "+"))...)
	}
	var out []Key
	seen := map[Key]bool{}
	for _, check := range []string{exact.Check, Any} {
		for _, eng := range engines {
			for _, repro := range []string{exact.Repro, Any} {
				k := Key{Evidence: exact.Evidence, Check: check, Engines: eng, Repro: repro}
				if !seen[k] {
					seen[k] = true
					out = append(out, k)
				}
			}
		}
	}
	return out
}

// subsets lists the proper, non-empty subsets of a sorted engine list as key values, largest first.
func subsets(names []string) []string {
	n := len(names)
	var out []string
	for size := n - 1; size >= 1; size-- {
		for mask := (1 << n) - 1; mask > 0; mask-- {
			if countBits(mask) != size {
				continue
			}
			var part []string
			for i, name := range names {
				if mask&(1<<i) != 0 {
					part = append(part, name)
				}
			}
			out = append(out, strings.Join(part, "+"))
		}
	}
	return out
}

func countBits(v int) int {
	n := 0
	for ; v > 0; v >>= 1 {
		n += v & 1
	}
	return n
}

// Reason says in one line what a finding rests on: the evidence, the engines that agree and whether it
// reproduced across runs. It is built from the inputs alone.
func Reason(in record.FindingInputs) string {
	var s string
	switch in.Evidence() {
	case record.EvidenceMeasurement:
		s = "Measured in the browser"
	case record.EvidenceAxFact:
		s = "Read from the page's accessibility tree"
	case record.EvidenceTaskVerdict:
		s = "The engine tried the task itself and could not complete it"
	case record.EvidenceEngineRule:
		s = enginesText(in.Engines)
	default:
		s = "The persona's own report, with no check behind it"
	}
	if r := in.Reproduced; r != nil && r.Of >= 2 {
		switch {
		case r.Runs >= r.Of:
			s += fmt.Sprintf(", and it happened in all %d runs", r.Of)
		default:
			s += fmt.Sprintf(", and it happened in %d of %d runs", r.Runs, r.Of)
		}
	}
	return s + "."
}

func enginesText(engines []string) string {
	s := slices.Clone(engines)
	slices.Sort(s)
	s = slices.Compact(s)
	names := make([]string, len(s))
	for i, e := range s {
		names[i] = EngineName(e)
	}
	slices.SortFunc(names, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	switch len(names) {
	case 0:
		return "A rule engine reports it"
	case 1:
		return names[0] + " reports it, and no other engine was asked"
	case 2:
		return "Two engines agree (" + strings.Join(names, " and ") + ")"
	}
	return fmt.Sprintf("%s engines agree (%s)", countWord(len(names)), strings.Join(names, ", "))
}

func countWord(n int) string {
	words := []string{"No", "One", "Two", "Three", "Four", "Five"}
	if n < len(words) {
		return words[n]
	}
	return fmt.Sprint(n)
}
