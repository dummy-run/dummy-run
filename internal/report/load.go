package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/record"
)

// Review is what a reviewer wrote about one finding. Every field is optional.
type Review struct {
	Reviewer   string
	ReviewedAt string
	Reason     string
}

// Run is a loaded run record with the folder it came from, so screenshots can be found.
type Run struct {
	Rec     *record.Record
	Dir     string
	Reviews map[string]Review // by finding ID
}

// maxFileBytes bounds how much of one input file is read.
const maxFileBytes = record.MaxRecordBytes

// LoadRuns reads run records from files and directories. A directory contributes its own *.json
// files in name order. The fields a reviewer adds (reviewer, reviewed_at and reason on a finding)
// are read when present and are not required.
func LoadRuns(paths ...string) ([]*Run, error) {
	var out []*Run
	seen := map[string]bool{}
	for _, p := range paths {
		names, err := expand(p)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			r, err := loadRun(n)
			if err != nil {
				return nil, err
			}
			if seen[r.Rec.ID] {
				return nil, fmt.Errorf("%s: run %s appears twice", n, r.Rec.ID)
			}
			seen[r.Rec.ID] = true
			out = append(out, r)
		}
	}
	return out, nil
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

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the path comes from the command line on purpose
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxFileBytes)
	}
	return data, nil
}

func loadRun(path string) (*Run, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	reviews, stripped, err := splitReviews(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	rec, err := record.Parse(stripped)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Run{Rec: rec, Dir: filepath.Dir(path), Reviews: reviews}, nil
}

// splitReviews takes the optional reviewer fields off each finding before the record is checked
// against the schema, so a record written by `dummyrun review` loads whether or not the schema
// in this build lists those fields.
func splitReviews(data []byte) (map[string]Review, []byte, error) {
	var doc map[string]any
	// UseNumber keeps every integer exact when the record is written back, such as a seed above 2^53.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, nil, err
	}
	reviews := map[string]Review{}
	list, _ := doc["findings"].([]any)
	changed := false
	for _, it := range list {
		f, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id, _ := f["id"].(string)
		var rv Review
		if v, ok := f["reviewer"]; ok {
			rv.Reviewer = text(v)
			delete(f, "reviewer")
			changed = true
		}
		if v, ok := f["reviewed_at"]; ok {
			rv.ReviewedAt = text(v)
			delete(f, "reviewed_at")
			changed = true
		}
		if v, ok := f["reason"]; ok {
			rv.Reason = text(v)
			delete(f, "reason")
			changed = true
		}
		if rv != (Review{}) && id != "" {
			reviews[id] = rv
		}
	}
	if !changed {
		return reviews, data, nil
	}
	out, err := json.Marshal(doc)
	return reviews, out, err
}

func text(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var parts []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// LoadA11y reads a `dummyrun a11y --json` file.
func LoadA11y(path string) (*a11y.Report, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var rep a11y.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(rep.Results) == 0 && rep.Summary.Pages == 0 {
		return nil, fmt.Errorf("%s: not a dummyrun a11y report (no results)", path)
	}
	return &rep, nil
}

// LoadBaseline reads an earlier report in JSON form.
func LoadBaseline(path string) (*Report, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if rep.ReportVersion != ReportVersion {
		return nil, fmt.Errorf("%s: not a dummyrun report JSON of version %d", path, ReportVersion)
	}
	return &rep, nil
}
