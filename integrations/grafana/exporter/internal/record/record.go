// Package record reads Dummy Run run records and checks them against the schema.
//
// The schema is embedded from schema/run-record.schema.json at the repository root. A test checks
// that the embedded copy matches the file in the repository.
package record

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:generate cp ../../../../../schema/run-record.schema.json run-record.schema.json
//go:generate cp ../../../../../schema/run-record.v1.schema.json run-record.v1.schema.json

// SchemaJSON is the embedded copy of schema/run-record.schema.json, version 2.
//
//go:embed run-record.schema.json
var SchemaJSON []byte

// SchemaV1JSON is the embedded copy of schema/run-record.v1.schema.json. Version 1 records stay readable.
//
//go:embed run-record.v1.schema.json
var SchemaV1JSON []byte

// SchemaID is the $id of the current run record schema.
const SchemaID = "https://dummyrun.dev/schema/run-record/v2.json"

// SchemaIDV1 is the $id of the version 1 schema.
const SchemaIDV1 = "https://dummyrun.dev/schema/run-record/v1.json"

// Run is one persona working through one scenario against one preview.
type Run struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Batch         string    `json:"batch,omitempty"`
	RepeatIndex   int       `json:"repeat_index,omitempty"`
	Seed          *int64    `json:"seed,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at"`
	Persona       Persona   `json:"persona"`
	Scenario      Scenario  `json:"scenario"`
	Target        Target    `json:"target"`
	Provider      Provider  `json:"provider"`
	Browser       Browser   `json:"browser"`
	Outcome       string    `json:"outcome"`
	Steps         []Step    `json:"steps"`
	Findings      []Finding `json:"findings"`
	Usage         Usage     `json:"usage"`
}

// Persona is the fictional user who ran the scenario.
type Persona struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Device string `json:"device"`
	// Zoom is the page zoom in percent, or 0 when the record has none (100).
	Zoom int `json:"zoom,omitempty"`
	// Dimensions are set in version 2 records.
	Dimensions *Dimensions `json:"dimensions,omitempty"`
}

// Dimensions are what varied between personas. A nil field is null in the record.
type Dimensions struct {
	Viewport   *Viewport `json:"viewport"`
	Locale     *string   `json:"locale"`
	AT         *string   `json:"at"`
	Connection *string   `json:"connection"`
	Zoom       *float64  `json:"zoom"`
	Motion     *string   `json:"motion"`
}

// Viewport is the page size in CSS pixels.
type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Scenario is the job the persona tried to do.
type Scenario struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// GoalMet is yes, partly or no in version 2 records.
	GoalMet   string `json:"goal_met,omitempty"`
	BlockedBy string `json:"blocked_by,omitempty"`
}

// RouteUniverseEntry is a route that exists, and where it was learned.
type RouteUniverseEntry struct {
	Route  string `json:"route"`
	Source string `json:"source"`
}

// Target is the preview the persona used.
type Target struct {
	Adapter     string `json:"adapter"`
	Environment string `json:"environment"`
	BaseURL     string `json:"base_url"`
	Commit      string `json:"commit,omitempty"`
	BuildHash   string `json:"build_hash,omitempty"`
	// RouteUniverse is set in version 2 records that state the routes that exist.
	RouteUniverse []RouteUniverseEntry `json:"route_universe,omitempty"`
}

// Provider is the model that drove the persona.
type Provider struct {
	Kind   string `json:"kind"`
	Model  string `json:"model"`
	Region string `json:"region"`
}

// Browser is the browser the persona used.
type Browser struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Step is one action the persona took.
type Step struct {
	N              int       `json:"n"`
	At             time.Time `json:"at"`
	Action         string    `json:"action"`
	Target         *string   `json:"target,omitempty"`
	URL            *string   `json:"url,omitempty"`
	Route          *string   `json:"route,omitempty"`
	Status         *int      `json:"status,omitempty"`
	DurationMS     int64     `json:"duration_ms"`
	Screenshot     *string   `json:"screenshot,omitempty"`
	ConsoleErrors  *int      `json:"console_errors,omitempty"`
	A11yViolations *int      `json:"a11y_violations,omitempty"`
	Note           *string   `json:"note,omitempty"`
}

// Finding is one problem the persona reported and the reviewer judged.
type Finding struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Steps    []int  `json:"steps"`
	Evidence string `json:"evidence"`
	Verdict  string `json:"verdict"`
	// The fields below are set in version 2 records.
	Class           string   `json:"class,omitempty"`
	Route           string   `json:"route,omitempty"`
	State           string   `json:"state,omitempty"`
	Locus           string   `json:"locus,omitempty"`
	Symptom         string   `json:"symptom,omitempty"`
	TaxonomyVersion int      `json:"taxonomy_version,omitempty"`
	Fingerprint     string   `json:"fingerprint,omitempty"`
	ClusterID       string   `json:"cluster_id,omitempty"`
	SeededDefectID  string   `json:"seeded_defect_id,omitempty"`
	ReviewedBy      []string `json:"reviewed_by,omitempty"`
	// Reviewer, ReviewedAt and Reason are set by dummyrun review when a person confirms or rejects the finding.
	Reviewer   string     `json:"reviewer,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	Reason     string     `json:"reason,omitempty"`
}

// Usage is what the run cost in model calls, tokens and money.
type Usage struct {
	ModelCalls   int   `json:"model_calls"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	// CachedInputTokens is the part of InputTokens the service served from its cache.
	CachedInputTokens int64    `json:"cached_input_tokens,omitempty"`
	CostUSD           *float64 `json:"cost_usd,omitempty"`
}

// MaxDurationMS is the longest step duration a span can represent: time.Duration counts nanoseconds
// in an int64, so a year of milliseconds is a safe bound well below the overflow.
const MaxDurationMS = 365 * 24 * 60 * 60 * 1000

// Step returns the step numbered n, or nil when the run has no such step.
func (r *Run) Step(n int) *Step {
	for i := range r.Steps {
		if r.Steps[i].N == n {
			return &r.Steps[i]
		}
	}
	return nil
}

// FirstStep returns the lowest numbered step a finding refers to.
func (r *Run) FirstStep(f Finding) *Step {
	var first *Step
	for _, n := range f.Steps {
		s := r.Step(n)
		if s != nil && (first == nil || s.N < first.N) {
			first = s
		}
	}
	return first
}

// ConfirmedFindings counts the findings the reviewer confirmed.
func (r *Run) ConfirmedFindings() int {
	n := 0
	for _, f := range r.Findings {
		if f.Verdict == "confirmed" {
			n++
		}
	}
	return n
}

// Validator checks run records against the embedded schemas, version 1 and version 2.
type Validator struct {
	schemas map[int]*jsonschema.Schema
}

// NewValidator compiles the embedded schemas.
func NewValidator() (*Validator, error) {
	v := &Validator{schemas: map[int]*jsonschema.Schema{}}
	for version, src := range map[int]struct {
		id   string
		data []byte
	}{1: {SchemaIDV1, SchemaV1JSON}, 2: {SchemaID, SchemaJSON}} {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(src.data))
		if err != nil {
			return nil, fmt.Errorf("parse embedded schema %d: %w", version, err)
		}
		c := jsonschema.NewCompiler()
		c.AssertFormat()
		if err := c.AddResource(src.id, doc); err != nil {
			return nil, fmt.Errorf("add embedded schema %d: %w", version, err)
		}
		s, err := c.Compile(src.id)
		if err != nil {
			return nil, fmt.Errorf("compile embedded schema %d: %w", version, err)
		}
		v.schemas[version] = s
	}
	return v, nil
}

// Parse validates JSON bytes against the schema and decodes them into a Run.
//
// Beyond the schema it checks that every finding refers to steps that exist, because the
// finding's event time comes from its first step.
func (v *Validator) Parse(data []byte) (*Run, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("not JSON: %w", err)
	}
	version := 0
	if m, ok := doc.(map[string]any); ok {
		switch n := m["schema_version"].(type) {
		case json.Number:
			if i, err := n.Int64(); err == nil {
				version = int(i)
			}
		case float64:
			version = int(n)
		}
	}
	schema, ok := v.schemas[version]
	if !ok {
		return nil, fmt.Errorf("does not match the run record schema: schema_version must be 1 or 2")
	}
	if err := schema.Validate(doc); err != nil {
		return nil, fmt.Errorf("does not match the run record schema: %w", err)
	}
	var run Run
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&run); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if run.EndedAt.Before(run.StartedAt) {
		return nil, fmt.Errorf("ended_at %s is before started_at %s", run.EndedAt.Format(time.RFC3339Nano), run.StartedAt.Format(time.RFC3339Nano))
	}
	seen := map[int]bool{}
	for _, s := range run.Steps {
		if seen[s.N] {
			return nil, fmt.Errorf("step %d appears twice", s.N)
		}
		seen[s.N] = true
		if s.DurationMS > MaxDurationMS {
			return nil, fmt.Errorf("step %d lasts %d ms, longer than the %d ms a span can hold", s.N, s.DurationMS, MaxDurationMS)
		}
	}
	for _, f := range run.Findings {
		for _, n := range f.Steps {
			if !seen[n] {
				return nil, fmt.Errorf("finding %s refers to step %d, which the run does not have", f.ID, n)
			}
		}
	}
	return &run, nil
}

// File is a run record together with the path it was read from.
type File struct {
	Path string
	Run  *Run
}

// Load reads every run record named by paths. A directory contributes each of its *.json files,
// in name order. The first invalid or unreadable file stops the load and the error names it.
func (v *Validator) Load(paths ...string) ([]File, error) {
	var files []File
	for _, p := range paths {
		names, err := expand(p)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			run, err := v.LoadFile(name)
			if err != nil {
				return nil, err
			}
			files = append(files, File{Path: name, Run: run})
		}
	}
	return files, nil
}

// LoadFile reads and validates one run record. The error names the file.
func (v *Validator) LoadFile(name string) (*Run, error) {
	data, err := readLimited(name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	run, err := v.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid run record: %w", name, err)
	}
	return run, nil
}

// maxRecordBytes bounds how much of one file is read, so a stray large file cannot exhaust memory.
const maxRecordBytes = 64 << 20

func readLimited(name string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRecordBytes {
		return nil, fmt.Errorf("file is larger than %d bytes", maxRecordBytes)
	}
	return data, nil
}

// expand turns a path into the list of record files it names.
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, filepath.Join(p, e.Name()))
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: no .json files in directory", p)
	}
	return names, nil
}
