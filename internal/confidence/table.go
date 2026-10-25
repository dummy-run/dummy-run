package confidence

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/dummy-run/dummy-run/internal/record"
)

// Bands, surest first. Unknown means the table has too little data for this kind of finding.
const (
	BandHigh    = "high"
	BandMedium  = "medium"
	BandLow     = "low"
	BandUnknown = "unknown"
)

// Bands lists the bands a report can show.
var Bands = []string{BandHigh, BandMedium, BandLow, BandUnknown}

// Band thresholds on the lower bound of the 95 percent Wilson interval of a bucket's accuracy.
const (
	HighFloor   = 0.80
	MediumFloor = 0.50
)

// DefaultMinSamples is the fewest labelled findings a bucket needs before its accuracy counts.
const DefaultMinSamples = 10

// Bucket is one group of findings with the same inputs, and how often findings in it were right.
type Bucket struct {
	Key
	// Correct is the number of labelled findings in the bucket that were true. Total is all of them.
	Correct int `json:"correct"`
	Total   int `json:"total"`
	// Accuracy is Correct over Total. Low and High bound it with a 95 percent Wilson interval.
	Accuracy float64 `json:"accuracy"`
	Low      float64 `json:"low"`
	High     float64 `json:"high"`
	// Source says where the labels came from.
	Source string `json:"source"`
}

// Band is the band the bucket's lower bound gives it, or unknown when it has too few samples.
func (b Bucket) Band(minSamples int) string {
	if b.Total < minSamples {
		return BandUnknown
	}
	return bandOf(b.Low)
}

func bandOf(low float64) string {
	switch {
	case low >= HighFloor:
		return BandHigh
	case low >= MediumFloor:
		return BandMedium
	}
	return BandLow
}

// Table is the confidence table: measured accuracy by bucket.
type Table struct {
	// Note says how the table was made and when it should be rebuilt.
	Note string `json:"note,omitempty"`
	// MinSamples is the fewest labelled findings a bucket needs before it is used. Zero means the default.
	MinSamples int      `json:"min_samples"`
	Buckets    []Bucket `json:"buckets"`
}

//go:embed table.json
var defaultJSON []byte

var defaultTable = sync.OnceValues(func() (*Table, error) {
	return ParseTable(defaultJSON)
})

// Default returns the table that ships with the engine. It panics only if the embedded file is broken, which
// a test prevents.
func Default() *Table {
	t, err := defaultTable()
	if err != nil {
		panic(err)
	}
	return t
}

// ParseTable reads a table from JSON.
func ParseTable(data []byte) (*Table, error) {
	var t Table
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("confidence table: %w", err)
	}
	if t.MinSamples <= 0 {
		t.MinSamples = DefaultMinSamples
	}
	return &t, nil
}

// ReadTable reads a table from a file.
func ReadTable(path string) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseTable(data)
}

// Marshal writes the table as indented JSON with the buckets in a stable order.
func (t *Table) Marshal() ([]byte, error) {
	sort.Slice(t.Buckets, func(i, j int) bool { return less(t.Buckets[i].Key, t.Buckets[j].Key) })
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// less orders keys by evidence, then from the most specific to the least, then alphabetically.
func less(a, b Key) bool {
	if a.Evidence != b.Evidence {
		return a.Evidence < b.Evidence
	}
	if wa, wb := wildcards(a), wildcards(b); wa != wb {
		return wa < wb
	}
	return a.String() < b.String()
}

func wildcards(k Key) int {
	n := 0
	for _, f := range []string{k.Check, k.Engines, k.Repro} {
		if f == Any {
			n++
		}
	}
	return n
}

// Find returns the bucket with the key, or nil.
func (t *Table) Find(k Key) *Bucket {
	for i := range t.Buckets {
		if t.Buckets[i].Key == k {
			return &t.Buckets[i]
		}
	}
	return nil
}

// Rating is what a report shows for a finding's confidence.
type Rating struct {
	// Band is high, medium, low or unknown.
	Band string `json:"band"`
	// Reason is one line saying what the finding rests on.
	Reason string `json:"reason"`
	// Bucket is the key of the bucket that set the band. Empty when no bucket had enough samples.
	Bucket string `json:"bucket,omitempty"`
	// Accuracy and Samples are the bucket's measured accuracy and size. Zero when no bucket was used.
	Accuracy float64 `json:"accuracy,omitempty"`
	Samples  int     `json:"samples,omitempty"`
}

// Rate looks a finding's inputs up in the table. It tries the candidate keys most specific first and uses
// the first bucket with enough samples. With none, the band is unknown, except that a finding with nothing but
// the model's word behind it is low: an unsupported claim is never shown as better than low until data says
// otherwise.
func (t *Table) Rate(in record.FindingInputs) Rating {
	r := Rating{Band: BandUnknown, Reason: Reason(in)}
	for _, k := range Candidates(in) {
		b := t.Find(k)
		if b == nil || b.Total < t.MinSamples {
			continue
		}
		r.Band, r.Bucket, r.Accuracy, r.Samples = b.Band(t.MinSamples), k.String(), b.Accuracy, b.Total
		if k.Check == Any && KeyOf(in).Check != Any {
			r.Reason += fmt.Sprintf(" This check has too few checked findings of its own, so the band comes from all %s together (%d checked).",
				population(k), b.Total)
		}
		return r
	}
	if in.Evidence() == record.EvidenceModelOnly {
		r.Band = BandLow
		return r
	}
	r.Reason += " How often this kind of check is right has not been measured yet."
	return r
}

// population names the findings a pooled bucket holds: its evidence kind, the engines it is scoped to and, when
// it is scoped to one, its reproduction class.
func population(k Key) string {
	var s string
	switch k.Evidence {
	case record.EvidenceEngineRule:
		if k.Engines == None || k.Engines == Any {
			s = "rule engine findings"
		} else {
			names := strings.Split(k.Engines, "+")
			for i, n := range names {
				names[i] = EngineName(n)
			}
			s = "findings " + strings.Join(names, " and ") + " report"
			if len(names) == 1 {
				s += "s"
			}
		}
	case record.EvidenceMeasurement:
		s = "measured findings"
	case record.EvidenceAxFact:
		s = "accessibility tree findings"
	case record.EvidenceTaskVerdict:
		s = "task findings"
	default:
		s = "persona-only findings"
	}
	switch k.Repro {
	case ReproAll:
		s += " seen in every run"
	case ReproSome:
		s += " seen in some runs"
	case ReproOnce:
		s += " seen in one run"
	}
	return s
}

// Wilson returns the 95 percent Wilson score interval for k successes in n trials. For n of zero it is 0 to 1.
func Wilson(k, n int) (low, high float64) {
	if n <= 0 {
		return 0, 1
	}
	const z = 1.959964
	p := float64(k) / float64(n)
	nn := float64(n)
	denom := 1 + z*z/nn
	centre := (p + z*z/(2*nn)) / denom
	half := z * math.Sqrt(p*(1-p)/nn+z*z/(4*nn*nn)) / denom
	return math.Max(0, centre-half), math.Min(1, centre+half)
}
