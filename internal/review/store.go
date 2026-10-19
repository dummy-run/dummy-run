// Package review is the human review stage. A finding starts with the verdict pending. A person
// confirms or rejects it here, and only confirmed findings reach a paid report.
//
// Findings with the same fingerprint are one problem seen in several runs. The package groups them
// so a person judges the problem once and the verdict reaches every pending finding in the group.
// A decision rewrites each affected record in place through a temporary file and a rename, checks
// the result against the schema first, and adds one line to review-log.jsonl next to the record.
package review

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/record"
)

// LogName is the audit log, written next to the records it describes.
const LogName = "review-log.jsonl"

// Verdicts a person can set.
const (
	Confirmed = "confirmed"
	Rejected  = "rejected"
	Pending   = record.VerdictPending
)

// Occurrence is one finding in one run record.
type Occurrence struct {
	Path    string
	Record  *record.Record
	Finding record.Finding
}

// Group is every finding that shares one fingerprint.
type Group struct {
	Fingerprint string
	Occurrences []Occurrence
}

// Pending returns the occurrences that nobody has reviewed.
func (g *Group) Pending() []Occurrence {
	var out []Occurrence
	for _, o := range g.Occurrences {
		if o.Finding.Verdict == Pending {
			out = append(out, o)
		}
	}
	return out
}

// Lead is the occurrence shown to the reviewer: the first pending one with the highest severity,
// or the first one when none is pending.
func (g *Group) Lead() Occurrence {
	best := g.Occurrences[0]
	bestRank := 99
	for _, o := range g.Occurrences {
		r := severityRank(o.Finding.Severity)
		if o.Finding.Verdict != Pending {
			r += 10
		}
		if r < bestRank {
			best, bestRank = o, r
		}
	}
	return best
}

// Counts returns how many findings in the group hold each verdict.
func (g *Group) Counts() map[string]int {
	c := map[string]int{}
	for _, o := range g.Occurrences {
		c[o.Finding.Verdict]++
	}
	return c
}

func severityRank(s string) int {
	for i, v := range record.Severities {
		if v == s {
			return i
		}
	}
	return len(record.Severities)
}

// Set is the records a review works on.
type Set struct {
	Paths []string
	// Skipped names records that review cannot edit, with the reason.
	Skipped []string
	groups  []*Group
}

// Load reads run records from files and directories. A directory contributes its own *.json files.
// Version 1 records have no fingerprint and no review fields, so they are skipped and named in Skipped.
func Load(paths ...string) (*Set, error) {
	s := &Set{}
	byFP := map[string]*Group{}
	seenRun := map[string]string{}
	for _, p := range paths {
		names, err := expand(p)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			rec, err := record.Read(n)
			if err != nil {
				return nil, err
			}
			if prev, dup := seenRun[rec.ID]; dup {
				return nil, fmt.Errorf("%s: run %s is also in %s", n, rec.ID, prev)
			}
			seenRun[rec.ID] = n
			s.Paths = append(s.Paths, n)
			if rec.SchemaVersion < 2 {
				s.Skipped = append(s.Skipped, fmt.Sprintf("%s: schema version %d records cannot be reviewed", n, rec.SchemaVersion))
				continue
			}
			for _, f := range rec.Findings {
				g := byFP[f.Fingerprint]
				if g == nil {
					g = &Group{Fingerprint: f.Fingerprint}
					byFP[f.Fingerprint] = g
					s.groups = append(s.groups, g)
				}
				g.Occurrences = append(g.Occurrences, Occurrence{Path: n, Record: rec, Finding: f})
			}
		}
	}
	return s, nil
}

func expand(p string) ([]string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{p}, nil
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, filepath.Join(p, e.Name()))
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: no .json run records", p)
	}
	return names, nil
}

// Groups returns the groups that hold at least one finding with the verdict, most severe first. An
// empty verdict returns every group.
func (s *Set) Groups(verdict string) []*Group {
	var out []*Group
	for _, g := range s.groups {
		if verdict == "" || g.Counts()[verdict] > 0 {
			out = append(out, g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := severityRank(a.Lead().Finding.Severity), severityRank(b.Lead().Finding.Severity); ra != rb {
			return ra < rb
		}
		if len(a.Occurrences) != len(b.Occurrences) {
			return len(a.Occurrences) > len(b.Occurrences)
		}
		return a.Fingerprint < b.Fingerprint
	})
	return out
}

// ErrNoMatch and ErrAmbiguous are the two ways a selector can fail.
var (
	ErrNoMatch   = errors.New("no pending finding matches")
	ErrAmbiguous = errors.New("matches more than one problem")
)

// MinPrefix is the shortest fingerprint prefix a selector may use.
const MinPrefix = 6

// Find resolves a selector to one group that has a pending finding. A selector is a fingerprint,
// a fingerprint prefix of at least six characters, run/finding, or a finding id that only one
// problem uses.
func (s *Set) Find(sel string) (*Group, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" {
		return nil, fmt.Errorf("an empty selector: %w", ErrNoMatch)
	}
	var hits []*Group
	for _, g := range s.groups {
		if len(g.Pending()) > 0 && matches(g, sel) {
			hits = append(hits, g)
		}
	}
	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("%q: %w", sel, ErrNoMatch)
	case 1:
		return hits[0], nil
	}
	fps := make([]string, len(hits))
	for i, g := range hits {
		fps[i] = g.Fingerprint
	}
	return nil, fmt.Errorf("%q %w (%s): give a fingerprint or run/finding", sel, ErrAmbiguous, strings.Join(fps, ", "))
}

func matches(g *Group, sel string) bool {
	if g.Fingerprint == sel || (len(sel) >= MinPrefix && strings.HasPrefix(g.Fingerprint, strings.ToLower(sel))) {
		return true
	}
	run, fid, qualified := strings.Cut(sel, "/")
	for _, o := range g.Pending() {
		if qualified && o.Record.ID == run && o.Finding.ID == fid {
			return true
		}
		if !qualified && o.Finding.ID == sel {
			return true
		}
	}
	return false
}

// Decision is what a person decided about a group.
type Decision struct {
	Verdict  string
	Reason   string
	Reviewer string
	At       time.Time
	// Severity and Title replace the finding's own when not empty.
	Severity string
	Title    string
}

// Applied is one finding that a decision changed.
type Applied struct {
	File string `json:"file"`
	Run  string `json:"run"`
	ID   string `json:"finding"`
	// The values before the decision.
	FromSeverity string `json:"from_severity"`
	FromTitle    string `json:"from_title"`
}

// Check reports why a decision is not allowed.
func (d Decision) Check() error {
	switch d.Verdict {
	case Confirmed:
	case Rejected:
		if strings.TrimSpace(d.Reason) == "" {
			return errors.New("a rejection needs a reason")
		}
	default:
		return fmt.Errorf("verdict %q: want confirmed or rejected", d.Verdict)
	}
	if strings.TrimSpace(d.Reviewer) == "" {
		return errors.New("no reviewer: pass --reviewer or set git config user.name")
	}
	if d.Severity != "" && !contains(record.Severities, d.Severity) {
		return fmt.Errorf("severity %q: want one of %s", d.Severity, strings.Join(record.Severities, ", "))
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Apply sets the verdict on every pending finding in the group and adds one line per directory to
// the review log. Each record is rewritten whole and validated before it replaces the old file, so
// a failure leaves that file as it was. The findings already changed are returned with any error.
func Apply(g *Group, d Decision) ([]Applied, error) {
	if err := d.Check(); err != nil {
		return nil, err
	}
	if d.At.IsZero() {
		d.At = time.Now()
	}
	d.At = d.At.UTC().Truncate(time.Second)
	var applied []Applied
	var firstErr error
	for _, path := range uniquePaths(g.Pending()) {
		got, err := editFile(path, g.Fingerprint, d)
		applied = append(applied, got...)
		if err != nil {
			firstErr = fmt.Errorf("%s: %w", path, err)
			break
		}
	}
	if err := writeLog(g.Fingerprint, d, applied); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("the records were updated but the log was not: %w", err)
	}
	return applied, firstErr
}

func uniquePaths(occ []Occurrence) []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range occ {
		if !seen[o.Path] {
			seen[o.Path] = true
			out = append(out, o.Path)
		}
	}
	return out
}

// editFile rereads the record, so a change made since Load is kept, and sets the verdict on its
// pending findings with the fingerprint.
func editFile(path, fingerprint string, d Decision) ([]Applied, error) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	// Two reviewers in one folder take turns: the lock covers the read, the edit, the check and the
	// rename. Where there is no lock, the record is read once more before the rename, and if it
	// changed the edit starts again from the new text.
	unlock, err := lockDir(filepath.Dir(target))
	if err != nil {
		return nil, err
	}
	defer unlock()
	for attempt := 0; attempt < casAttempts; attempt++ {
		applied, retry, err := editOnce(path, target, fingerprint, d)
		if !retry {
			return applied, err
		}
	}
	return nil, errors.New("the record kept changing while it was being reviewed: try again")
}

// beforeSwap runs just before editOnce checks that the record is unchanged. A test uses it to change the record at that moment.
var beforeSwap = func() {}

// casAttempts is how many times editFile starts over when the record changes under it.
const casAttempts = 5

func editOnce(path, target, fingerprint string, d Decision) (applied []Applied, retry bool, err error) {
	data, err := readLimited(target)
	if err != nil {
		return nil, false, err
	}
	doc, err := parseOrdered(data)
	if err != nil {
		return nil, false, err
	}
	if v, _ := doc.get("schema_version"); fmt.Sprint(v) != "2" {
		return nil, false, errors.New("only schema version 2 records can be reviewed")
	}
	list, _ := doc.vals["findings"].([]any)
	runID := doc.str("id")
	for _, item := range list {
		f, ok := item.(*omap)
		if !ok || f.str("fingerprint") != fingerprint || f.str("verdict") != Pending {
			continue
		}
		applied = append(applied, Applied{File: path, Run: runID, ID: f.str("id"),
			FromSeverity: f.str("severity"), FromTitle: f.str("title")})
		f.set("verdict", d.Verdict)
		f.set("reviewer", d.Reviewer)
		f.set("reviewed_at", d.At.Format(time.RFC3339))
		if strings.TrimSpace(d.Reason) != "" {
			f.set("reason", strings.TrimSpace(d.Reason))
		}
		if d.Severity != "" {
			f.set("severity", d.Severity)
		}
		if d.Title != "" {
			f.set("title", d.Title)
		}
	}
	if len(applied) == 0 {
		return nil, false, nil
	}
	out, err := formatOrdered(doc)
	if err != nil {
		return nil, false, err
	}
	if err := record.ValidateJSON(out); err != nil {
		return nil, false, err
	}
	beforeSwap()
	if now, err := readLimited(target); err != nil {
		return nil, false, err
	} else if !bytes.Equal(now, data) {
		return nil, true, nil
	}
	if err := writeAtomic(target, out); err != nil {
		return nil, false, err
	}
	return applied, false, nil
}

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the path comes from the command line on purpose
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, record.MaxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > record.MaxRecordBytes {
		return nil, fmt.Errorf("larger than %d bytes", record.MaxRecordBytes)
	}
	return data, nil
}

// writeAtomic writes data to a temporary file in the same directory, syncs it and renames it over
// path, so a reader sees the old record or the new one and never half of either.
func writeAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".review-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(name) // our own temporary file, never a record
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		return fail(err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name) // our own temporary file, never a record
		return err
	}
	if dir, err := os.Open(filepath.Dir(path)); err == nil { //nolint:gosec // the directory of a record the user named
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
