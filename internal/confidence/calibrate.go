package confidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dummy-run/dummy-run/internal/record"
)

// Labelled is one finding with its inputs and whether it turned out to be true. It is the input of
// dummyrun calibrate.
type Labelled struct {
	ID     string               `json:"id,omitempty"`
	Inputs record.FindingInputs `json:"inputs"`
	True   bool                 `json:"true"`
	// Source says who labelled it and how.
	Source string `json:"source,omitempty"`
}

// ReadLabels reads a JSON array of labelled findings.
func ReadLabels(path string) ([]Labelled, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Labelled
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, l := range out {
		if !contains(record.EvidenceKinds, l.Inputs.Evidence()) {
			return nil, fmt.Errorf("%s: label %d: evidence_kind %q is not one of %s", path, i+1, l.Inputs.EvidenceKind, strings.Join(record.EvidenceKinds, ", "))
		}
	}
	return out, nil
}

// Calibrate builds a table from labelled findings. Every finding counts towards each bucket it belongs to:
// its exact key and every generalisation of it, with the check or the reproduction replaced by Any. The engine
// set is never generalised, since Rate never pools engines. Buckets with fewer samples than minSamples are still
// written, so the table shows what data is still needed, and Rate skips them.
func Calibrate(labels []Labelled, minSamples int) *Table {
	if minSamples <= 0 {
		minSamples = DefaultMinSamples
	}
	type acc struct {
		correct, total int
		sources        map[string]bool
	}
	by := map[Key]*acc{}
	for _, l := range labels {
		for _, k := range generalisations(KeyOf(l.Inputs)) {
			a := by[k]
			if a == nil {
				a = &acc{sources: map[string]bool{}}
				by[k] = a
			}
			a.total++
			if l.True {
				a.correct++
			}
			if l.Source != "" {
				a.sources[l.Source] = true
			}
		}
	}
	t := &Table{MinSamples: minSamples,
		Note: "Measured accuracy of findings by their inputs. Built by dummyrun calibrate from labelled findings; a bucket with fewer than min_samples findings is not used."}
	for k, a := range by {
		low, high := Wilson(a.correct, a.total)
		src := make([]string, 0, len(a.sources))
		for s := range a.sources {
			src = append(src, s)
		}
		sort.Strings(src)
		t.Buckets = append(t.Buckets, Bucket{Key: k, Correct: a.correct, Total: a.total,
			Accuracy: round3(float64(a.correct) / float64(a.total)), Low: round3(low), High: round3(high), Source: strings.Join(src, "; ")})
	}
	sort.Slice(t.Buckets, func(i, j int) bool { return less(t.Buckets[i].Key, t.Buckets[j].Key) })
	return t
}

// generalisations lists the exact key and every key with its check, its reproduction or both set to Any.
func generalisations(k Key) []Key {
	var out []Key
	for mask := 0; mask < 4; mask++ {
		g := k
		if mask&1 != 0 {
			g.Check = Any
		}
		if mask&2 != 0 {
			g.Repro = Any
		}
		out = append(out, g)
	}
	return out
}

func round3(v float64) float64 {
	return float64(int(v*1000+0.5)) / 1000
}

// Calibration says how a table's bands compare with what the labels show.
type Calibration struct {
	Labels int `json:"labels"`
	// Buckets are the buckets Rate used for at least one label, with the band they state and the accuracy the
	// labels that fell into them show.
	Buckets []BucketCheck `json:"buckets"`
	// Bands sum the labels by the band they were given.
	Bands []BandCheck `json:"bands"`
	// Unknown counts the labels no bucket could rate.
	Unknown int `json:"unknown"`
	// Discriminates is true when each band that has labels is more often right than the next band down.
	Discriminates bool `json:"discriminates"`
	// Separation is the accuracy of the high band less that of the low band, when both have labels.
	Separation *float64 `json:"separation"`
}

// BucketCheck is one bucket's stated band against the accuracy of the labels it rated.
type BucketCheck struct {
	Bucket   string  `json:"bucket"`
	Band     string  `json:"band"`
	Stated   float64 `json:"stated_accuracy"`
	Labels   int     `json:"labels"`
	Correct  int     `json:"correct"`
	Observed float64 `json:"observed_accuracy"`
}

// BandCheck is one band's labels and how many were true.
type BandCheck struct {
	Band     string  `json:"band"`
	Labels   int     `json:"labels"`
	Correct  int     `json:"correct"`
	Observed float64 `json:"observed_accuracy"`
}

// Evaluate rates every label with the table and compares the bands with the labels. A table checked against
// the labels it was built from looks better than it is, so pass held-out labels when you have them.
func Evaluate(t *Table, labels []Labelled) Calibration {
	c := Calibration{Labels: len(labels)}
	type acc struct{ correct, total int }
	byBucket := map[string]*acc{}
	bandOfBucket := map[string]string{}
	byBand := map[string]*acc{}
	for _, l := range labels {
		r := t.Rate(l.Inputs)
		b := byBand[r.Band]
		if b == nil {
			b = &acc{}
			byBand[r.Band] = b
		}
		b.total++
		if l.True {
			b.correct++
		}
		if r.Bucket == "" {
			c.Unknown++
			continue
		}
		a := byBucket[r.Bucket]
		if a == nil {
			a = &acc{}
			byBucket[r.Bucket] = a
		}
		a.total++
		if l.True {
			a.correct++
		}
		bandOfBucket[r.Bucket] = r.Band
	}
	for key, a := range byBucket {
		var stated float64
		if b := t.Find(parseKey(key)); b != nil {
			stated = b.Accuracy
		}
		c.Buckets = append(c.Buckets, BucketCheck{Bucket: key, Band: bandOfBucket[key], Stated: stated, Labels: a.total, Correct: a.correct,
			Observed: round3(float64(a.correct) / float64(a.total))})
	}
	sort.Slice(c.Buckets, func(i, j int) bool {
		if c.Buckets[i].Band != c.Buckets[j].Band {
			return bandRank(c.Buckets[i].Band) < bandRank(c.Buckets[j].Band)
		}
		return c.Buckets[i].Bucket < c.Buckets[j].Bucket
	})
	c.Discriminates = true
	var prev *BandCheck
	var high, low *BandCheck
	for _, band := range Bands {
		a := byBand[band]
		if a == nil {
			continue
		}
		bc := BandCheck{Band: band, Labels: a.total, Correct: a.correct, Observed: round3(float64(a.correct) / float64(a.total))}
		c.Bands = append(c.Bands, bc)
		if band != BandUnknown {
			if prev != nil && bc.Observed >= prev.Observed {
				c.Discriminates = false
			}
			prev = &c.Bands[len(c.Bands)-1]
		}
		switch band {
		case BandHigh:
			high = &c.Bands[len(c.Bands)-1]
		case BandLow:
			low = &c.Bands[len(c.Bands)-1]
		}
	}
	if high != nil && low != nil {
		sep := round3(high.Observed - low.Observed)
		c.Separation = &sep
	}
	return c
}

func bandRank(b string) int {
	for i, x := range Bands {
		if x == b {
			return i
		}
	}
	return len(Bands)
}

func parseKey(s string) Key {
	p := strings.SplitN(s, "|", 4)
	if len(p) != 4 {
		return Key{}
	}
	return Key{Evidence: p[0], Check: p[1], Engines: p[2], Repro: p[3]}
}

// Markdown writes the calibration report.
func (c Calibration) Markdown() string {
	var b strings.Builder
	b.WriteString("# Calibration report\n\n")
	fmt.Fprintf(&b, "%d labelled findings. %d could not be rated: no bucket with enough samples fits them.\n\n", c.Labels, c.Unknown)
	b.WriteString("## Bands\n\n| Band | Findings | Right | Observed accuracy |\n|---|---:|---:|---:|\n")
	for _, bc := range c.Bands {
		fmt.Fprintf(&b, "| %s | %d | %d | %.3f |\n", bc.Band, bc.Labels, bc.Correct, bc.Observed)
	}
	b.WriteString("\n")
	switch {
	case c.Discriminates && c.Separation != nil:
		fmt.Fprintf(&b, "The bands discriminate: each band is more often right than the one below it, and high is %.3f more often right than low.\n\n", *c.Separation)
	case c.Discriminates:
		b.WriteString("Each band that has findings is more often right than the one below it. High and low were not both seen, so there is no separation figure.\n\n")
	default:
		b.WriteString("The bands do not discriminate: a lower band is at least as often right as the one above it. Look at the buckets below before shipping this table.\n\n")
	}
	b.WriteString("## Buckets\n\nStated accuracy is the table's figure. Observed accuracy is what the labels rated by that bucket show. Measured on the same labels the table was built from, they match by construction, so hold labels out to test the table.\n\n")
	b.WriteString("| Bucket | Band | Stated | Findings | Right | Observed |\n|---|---|---:|---:|---:|---:|\n")
	for _, bc := range c.Buckets {
		fmt.Fprintf(&b, "| `%s` | %s | %.3f | %d | %d | %.3f |\n", bc.Bucket, bc.Band, bc.Stated, bc.Labels, bc.Correct, bc.Observed)
	}
	return b.String()
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
