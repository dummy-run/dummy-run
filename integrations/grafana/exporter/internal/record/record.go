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

// SchemaJSON is the embedded copy of schema/run-record.schema.json.
//
//go:embed run-record.schema.json
var SchemaJSON []byte

// SchemaID is the $id of the run record schema.
const SchemaID = "https://dummyrun.dev/schema/run-record/v1.json"

// Run is one persona working through one scenario against one preview.
type Run struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Batch         string    `json:"batch,omitempty"`
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
}

// Scenario is the job the persona tried to do.
type Scenario struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Target is the preview the persona used.
type Target struct {
	Adapter     string `json:"adapter"`
	Environment string `json:"environment"`
	BaseURL     string `json:"base_url"`
	Commit      string `json:"commit,omitempty"`
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
}

// Usage is what the run cost in model calls, tokens and money.
type Usage struct {
	ModelCalls   int      `json:"model_calls"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
}

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

// Validator checks run records against the embedded schema.
type Validator struct {
	schema *jsonschema.Schema
}

// NewValidator compiles the embedded schema.
func NewValidator() (*Validator, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(SchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("parse embedded schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource(SchemaID, doc); err != nil {
		return nil, fmt.Errorf("add embedded schema: %w", err)
	}
	s, err := c.Compile(SchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile embedded schema: %w", err)
	}
	return &Validator{schema: s}, nil
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
	if err := v.schema.Validate(doc); err != nil {
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
