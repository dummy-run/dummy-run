// Package record defines the run record, validates it against the schema in
// schema/run-record.schema.json and writes it to disk.
package record

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:generate cp ../../schema/run-record.schema.json run-record.schema.json
//go:generate cp ../../schema/run-record.v1.schema.json run-record.v1.schema.json

//go:embed run-record.schema.json
var schemaJSON []byte

//go:embed run-record.v1.schema.json
var schemaV1JSON []byte

// SchemaVersion is the version the engine writes. Version 1 records stay readable.
const SchemaVersion = 2

// SchemaJSON returns the embedded version 2 schema, for a CI check against the repository copy.
func SchemaJSON() []byte { return schemaJSON }

// SchemaV1JSON returns the embedded version 1 schema, kept so older records still validate.
func SchemaV1JSON() []byte { return schemaV1JSON }

// Record is one persona working through one scenario against one target.
type Record struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Batch         string `json:"batch,omitempty"`
	// RepeatIndex counts earlier runs in the batch with the same persona, scenario and model.
	RepeatIndex int `json:"repeat_index"`
	// Seed is the sampling seed sent to the model, or nil.
	Seed      *int64    `json:"seed"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Persona   Persona   `json:"persona"`
	Scenario  Scenario  `json:"scenario"`
	Target    Target    `json:"target"`
	Provider  Provider  `json:"provider"`
	Browser   Browser   `json:"browser"`
	// Signin says how the engine signed the persona in before the run. Nil when it did not.
	Signin   *Signin   `json:"signin,omitempty"`
	Outcome  string    `json:"outcome"`
	Steps    []Step    `json:"steps"`
	Findings []Finding `json:"findings"`
	Usage    Usage     `json:"usage"`
	// Events are things the engine saw that a reviewer should know about. Left out when there are none.
	Events []Event `json:"events,omitempty"`
}

// Event kinds.
const (
	EventSuspectedInjection = "suspected_injection"
	EventOffOriginBlocked   = "off_origin_blocked"
	EventUncitedFinding     = "uncited_finding"
	EventProviderFiltered   = "provider_filtered"
)

// Event is one thing the engine noticed. For a suspected injection Detail is text from the page, so a
// reader that passes it to a model must treat it as data.
type Event struct {
	Kind    string `json:"kind"`
	Step    int    `json:"step,omitempty"`
	Source  string `json:"source,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Detail  string `json:"detail"`
}

// Persona identifies the persona and how it browsed in this run. Device and
// Zoom are the values the run used: a scenario can replace the persona's own.
type Persona struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Device string `json:"device"`
	// Zoom is the page zoom in percent, 100 to 500. Zero leaves it out, meaning 100.
	Zoom int `json:"zoom,omitempty"`
	// Dimensions are the persona's dimensions for diversity coverage. Nil in a version 1 record.
	Dimensions *Dimensions `json:"dimensions,omitempty"`
}

// Viewport is the size of the page in CSS pixels.
type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Dimensions are what varied between personas. A nil value is written as null: not varied or unknown.
type Dimensions struct {
	Viewport   *Viewport `json:"viewport"`
	Locale     *string   `json:"locale"`
	AT         *string   `json:"at"`
	Connection *string   `json:"connection"`
	Zoom       *float64  `json:"zoom"`
	Motion     *string   `json:"motion"`
}

// Scenario identifies the scenario.
type Scenario struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// GoalMet is yes, partly or no. Empty in a version 1 record.
	GoalMet string `json:"goal_met,omitempty"`
	// BlockedBy is the fingerprint of the finding that blocked the goal.
	BlockedBy string `json:"blocked_by,omitempty"`
}

// RouteUniverseEntry is a route that exists, and where the engine learned of it.
type RouteUniverseEntry struct {
	Route  string `json:"route"`
	Source string `json:"source"`
}

// Target describes the preview under test.
type Target struct {
	Adapter     string `json:"adapter"`
	Environment string `json:"environment"`
	BaseURL     string `json:"base_url"`
	Commit      string `json:"commit,omitempty"`
	// BuildHash identifies the deployed build when there is no commit.
	BuildHash     string               `json:"build_hash,omitempty"`
	RouteUniverse []RouteUniverseEntry `json:"route_universe,omitempty"`
}

// Provider describes the model that played the persona.
type Provider struct {
	Kind   string `json:"kind"`
	Model  string `json:"model"`
	Region string `json:"region"`
}

// Browser describes the browser.
type Browser struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Signin records the sign-in the engine did before the persona started. It never holds a credential.
type Signin struct {
	// Method is form or session.
	Method string `json:"method"`
	// Role is the role name from the target file's credentials.
	Role string `json:"role"`
	OK   bool   `json:"ok"`
	// AllowedRequest is the one request the read-only policy let through during sign-in. Nil when
	// none was sent, as with a session file.
	AllowedRequest *SigninRequest `json:"allowed_request"`
	DurationMs     int            `json:"duration_ms"`
	// Error says why the sign-in failed. Credentials are removed from it.
	Error string `json:"error,omitempty"`
}

// SigninRequest is a request method and address, without the query.
type SigninRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

// Step is one action.
type Step struct {
	N              int       `json:"n"`
	At             time.Time `json:"at"`
	Action         string    `json:"action"`
	Target         string    `json:"target,omitempty"`
	URL            string    `json:"url,omitempty"`
	Route          string    `json:"route,omitempty"`
	Status         *int      `json:"status,omitempty"`
	DurationMs     int       `json:"duration_ms"`
	Screenshot     string    `json:"screenshot,omitempty"`
	ConsoleErrors  *int      `json:"console_errors,omitempty"`
	A11yViolations *int      `json:"a11y_violations,omitempty"`
	Note           string    `json:"note,omitempty"`
}

// Finding is a claim with evidence. The reviewer sets the verdict later.
type Finding struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Steps    []int  `json:"steps"`
	Evidence string `json:"evidence"`
	Verdict  string `json:"verdict"`
	// Class is problem or observation. Only problems enter incidence statistics.
	Class string `json:"class,omitempty"`
	// Route is the normalised route where the problem appears.
	Route string `json:"route,omitempty"`
	// State is an optional precondition for the problem.
	State string `json:"state,omitempty"`
	// Locus is role:name of the control, region:name, or page.
	Locus string `json:"locus,omitempty"`
	// Symptom is from the closed taxonomy.
	Symptom         string `json:"symptom,omitempty"`
	TaxonomyVersion int    `json:"taxonomy_version,omitempty"`
	// Fingerprint is computed by the engine from route, locus and symptom.
	Fingerprint    string   `json:"fingerprint,omitempty"`
	ClusterID      string   `json:"cluster_id,omitempty"`
	SeededDefectID string   `json:"seeded_defect_id,omitempty"`
	ReviewedBy     []string `json:"reviewed_by,omitempty"`
	// Reviewer, ReviewedAt and Reason are set by dummyrun review when a person confirms or rejects the finding.
	Reviewer   string     `json:"reviewer,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	Reason     string     `json:"reason,omitempty"`
}

// Usage is the model usage for the run.
type Usage struct {
	ModelCalls   int `json:"model_calls"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	// CachedInputTokens is the part of InputTokens served from the prompt cache.
	CachedInputTokens int `json:"cached_input_tokens,omitempty"`
	// CostUSD comes from the price table; nil is written as null when the model is not in it.
	CostUSD *float64 `json:"cost_usd"`
}

// Outcomes and other enumerations from the schema.
const (
	OutcomeCompleted        = "completed"
	OutcomeBlocked          = "blocked"
	OutcomeErrored          = "errored"
	OutcomeBudgetExhausted  = "budget_exhausted"
	OutcomeProviderFiltered = "provider_filtered"
	VerdictPending          = "pending"
	ClassProblem            = "problem"
	ClassObservation        = "observation"
)

// Actions lists the thirteen step actions.
var Actions = []string{"look", "visit", "click", "fill", "type", "select", "press", "back", "scroll", "wait", "a11y", "screenshot", "finish"}

// Kinds lists the finding kinds.
var Kinds = []string{"bug", "permission", "friction", "accessibility", "question", "security"}

// Severities lists the finding severities.
var Severities = []string{"blocker", "major", "minor", "info"}

// Evidence lists the evidence tags.
var Evidence = []string{"measured", "code", "sourced", "inferred"}

// Int returns a pointer to v, for the optional integer fields.
func Int(v int) *int { return &v }

// schemas compiles the embedded schemas once, keyed by schema_version; concurrent callers share the result.
var schemas = sync.OnceValues(func() (map[int]*jsonschema.Schema, error) {
	out := map[int]*jsonschema.Schema{}
	for v, data := range map[int][]byte{1: schemaV1JSON, 2: schemaJSON} {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("record: embedded schema %d is not JSON: %w", v, err)
		}
		c := jsonschema.NewCompiler()
		c.AssertFormat()
		name := fmt.Sprintf("run-record-v%d.schema.json", v)
		if err := c.AddResource(name, doc); err != nil {
			return nil, err
		}
		s, err := c.Compile(name)
		if err != nil {
			return nil, fmt.Errorf("record: compile schema %d: %w", v, err)
		}
		out[v] = s
	}
	return out, nil
})

// ErrInvalid marks a record that fails the schema.
var ErrInvalid = errors.New("record: does not match the schema")

// Marshal encodes the record and validates the result against its schema.
func Marshal(r *Record) ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := ValidateJSON(data); err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ValidateJSON checks encoded JSON against the schema for its schema_version, 1 or 2.
func ValidateJSON(data []byte) error {
	all, err := schemas()
	if err != nil {
		return err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	version := 0
	if m, ok := doc.(map[string]any); ok {
		switch v := m["schema_version"].(type) {
		case json.Number:
			if n, err := v.Int64(); err == nil {
				version = int(n)
			}
		case float64:
			version = int(v)
		}
	}
	s, ok := all[version]
	if !ok {
		return fmt.Errorf("%w: schema_version must be 1 or 2", ErrInvalid)
	}
	if err := s.Validate(doc); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}

// Write validates the record and writes it to <dir>/<id>.json. The record's
// screenshots live under <dir>/<id>/, so paths in steps are relative to dir.
func Write(dir string, r *Record) (string, error) {
	data, err := Marshal(r)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, r.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// MaxRecordBytes bounds how much of one file Read takes, so a stray large file cannot exhaust memory.
const MaxRecordBytes = 64 << 20

// Parse validates JSON bytes against the schema for their schema_version and decodes them. Version 1
// and version 2 records are both accepted; a version 1 record has none of the version 2 fields.
func Parse(data []byte) (*Record, error) {
	if err := ValidateJSON(data); err != nil {
		return nil, err
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Read loads and validates a record of either version.
func Read(path string) (*Record, error) {
	f, err := os.Open(path) //nolint:gosec // the path comes from the command line on purpose
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxRecordBytes {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, MaxRecordBytes)
	}
	r, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// FindingID builds the stable fingerprint for a finding: kind plus a slug of the title.
func FindingID(kind, title string) string {
	slug := strings.ToLower(title)
	var b strings.Builder
	lastDash := true
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		s = "untitled"
	}
	return kind + "-" + s
}
