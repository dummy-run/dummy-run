// Package record defines the run record, validates it against the schema in
// schema/run-record.schema.json and writes it to disk.
package record

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed run-record.schema.json
var schemaJSON []byte

// SchemaJSON returns the embedded schema, for a CI check against the repository copy.
func SchemaJSON() []byte { return schemaJSON }

// Record is one persona working through one scenario against one target.
type Record struct {
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

// Persona identifies the persona.
type Persona struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Device string `json:"device"`
}

// Scenario identifies the scenario.
type Scenario struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Target describes the preview under test.
type Target struct {
	Adapter     string `json:"adapter"`
	Environment string `json:"environment"`
	BaseURL     string `json:"base_url"`
	Commit      string `json:"commit,omitempty"`
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

// Step is one action.
type Step struct {
	N              int       `json:"n"`
	At             time.Time `json:"at"`
	Action         string    `json:"action"`
	Target         string    `json:"target,omitempty"`
	URL            string    `json:"url,omitempty"`
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
}

// Usage is the model usage for the run.
type Usage struct {
	ModelCalls   int `json:"model_calls"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	// CachedInputTokens is the part of InputTokens served from the prompt cache.
	CachedInputTokens int      `json:"cached_input_tokens,omitempty"`
	CostUSD           *float64 `json:"cost_usd,omitempty"`
}

// Outcomes and other enumerations from the schema.
const (
	OutcomeCompleted       = "completed"
	OutcomeBlocked         = "blocked"
	OutcomeErrored         = "errored"
	OutcomeBudgetExhausted = "budget_exhausted"
	VerdictPending         = "pending"
)

// Actions lists the thirteen step actions.
var Actions = []string{"look", "visit", "click", "fill", "type", "select", "press", "back", "scroll", "wait", "a11y", "screenshot", "finish"}

// Kinds lists the finding kinds.
var Kinds = []string{"bug", "permission", "friction", "accessibility", "question"}

// Severities lists the finding severities.
var Severities = []string{"blocker", "major", "minor", "info"}

// Evidence lists the evidence tags.
var Evidence = []string{"measured", "code", "sourced", "inferred"}

// Int returns a pointer to v, for the optional integer fields.
func Int(v int) *int { return &v }

// schema compiles the embedded schema once; concurrent callers share the result.
var schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("record: embedded schema is not JSON: %w", err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("run-record.schema.json", doc); err != nil {
		return nil, err
	}
	s, err := c.Compile("run-record.schema.json")
	if err != nil {
		return nil, fmt.Errorf("record: compile schema: %w", err)
	}
	return s, nil
})

// ErrInvalid marks a record that fails the schema.
var ErrInvalid = errors.New("record: does not match the schema")

// Marshal encodes the record and validates the result against the schema.
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

// ValidateJSON checks encoded JSON against the schema.
func ValidateJSON(data []byte) error {
	s, err := schema()
	if err != nil {
		return err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
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

// Read loads and validates a record.
func Read(path string) (*Record, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the path comes from the command line on purpose
	if err != nil {
		return nil, err
	}
	if err := ValidateJSON(data); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
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
