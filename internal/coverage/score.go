package coverage

import "fmt"

// SeverityOrder lists severities from low to high.
var SeverityOrder = []string{"info", "minor", "major", "blocker"}

func severityRank(s string) int {
	for i, v := range SeverityOrder {
		if v == s {
			return i
		}
	}
	return 0
}

// EvidenceRank orders the evidence tags. Measured and code outrank sourced, which outranks inferred.
func EvidenceRank(e string) int {
	switch e {
	case "measured", "code":
		return 3
	case "sourced":
		return 2
	}
	return 1
}

// FindingVector is what a band is computed from.
type FindingVector struct {
	Severity string
	Evidence string
	Verdict  string
	K        int // exposed runs in which the problem was seen
	N        int // runs that reached the route
}

// Band is the band of a reviewer-confirmed finding: its confirmed severity, one step lower only when the
// finding is both unreplicated (k < 2) and inferred. Info stays info. A finding that is not confirmed gets
// no band. There is no multiplication and no weight.
func Band(v FindingVector) (string, bool) {
	if v.Verdict != "confirmed" {
		return "", false
	}
	i := severityRank(v.Severity)
	if v.K < 2 && v.Evidence == "inferred" {
		i = max(0, i-1)
	}
	return SeverityOrder[i], true
}

// SortKey orders findings: band rank, then evidence rank, then the Wilson lower bound of k/n. It is a
// sort order and is never shown as a number. A finding without a band sorts last.
func SortKey(v FindingVector) [3]float64 {
	b, ok := Band(v)
	if !ok {
		return [3]float64{-1, 0, 0}
	}
	lo := 0.0
	if v.N > 0 {
		lo, _ = Wilson(v.K, v.N)
	}
	return [3]float64{float64(severityRank(b)), float64(EvidenceRank(v.Evidence)), lo}
}

// Less reports whether a sorts before b in descending order of SortKey.
func Less(a, b FindingVector) bool {
	ka, kb := SortKey(a), SortKey(b)
	for i := range ka {
		if ka[i] != kb[i] {
			return ka[i] > kb[i]
		}
	}
	return false
}

// Display is the line shown beside every band.
func Display(v FindingVector) string {
	b, ok := Band(v)
	if !ok {
		return fmt.Sprintf("unscored (%s); %s, %s, %d of %d exposed runs", v.Verdict, v.Severity, v.Evidence, v.K, v.N)
	}
	flag := ""
	if v.K < 2 {
		flag = "; unreplicated"
	}
	return fmt.Sprintf("%s (reviewer severity %s); seen in %d of %d exposed runs; evidence %s%s", b, v.Severity, v.K, v.N, v.Evidence, flag)
}
