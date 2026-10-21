// Package report turns run records and accessibility sweep results into an audit report.
//
// A report is built once as a Report value. The renderers then write it as HTML, Markdown, JSON,
// SARIF or JUnit XML. The wording is in templates/, so the renderers differ only in markup.
package report

import "github.com/dummy-run/dummy-run/internal/coverage"

// ReportVersion is the version of the report's JSON shape. schema/report.schema.json describes it.
const ReportVersion = 1

// Finding statuses.
const (
	StatusConfirmed   = "confirmed"
	StatusNotReviewed = "not-reviewed"
)

// Baseline statuses, set only when a baseline report is given.
const (
	BaselineNew          = "new"
	BaselineStillPresent = "still-present"
	BaselineFixed        = "fixed"
)

// Report is everything a rendered report says.
type Report struct {
	ReportVersion int    `json:"report_version"`
	Title         string `json:"title"`
	Client        string `json:"client,omitempty"`
	Date          string `json:"date"`
	Tool          string `json:"tool"`
	ToolVersion   string `json:"tool_version"`
	// Basis says which findings are in the report: "confirmed" or "confirmed and not reviewed".
	Basis    string          `json:"basis"`
	Summary  Summary         `json:"summary"`
	Scope    Scope           `json:"scope"`
	Coverage CoverageSection `json:"coverage"`
	Findings []Finding       `json:"findings"`
	// Worked lists what the personas reported as working. A person has not checked these notes.
	Worked   []WorkedNote   `json:"worked,omitempty"`
	Fixed    []FixedFinding `json:"fixed,omitempty"`
	Axe      *AxeSection    `json:"axe,omitempty"`
	Baseline *BaselineInfo  `json:"baseline,omitempty"`
	Limits   []string       `json:"limits"`
	Warnings []string       `json:"warnings"`
}

// WorkedNote is something personas reported as working, with how many runs said so.
type WorkedNote struct {
	Text     string   `json:"text"`
	Runs     int      `json:"runs"`
	Personas []string `json:"personas"`
}

// Summary holds the counts and the plain-language paragraphs at the top of the report.
type Summary struct {
	Text           []string       `json:"text"`
	FindingsTotal  int            `json:"findings_total"`
	BySeverity     map[string]int `json:"by_severity"`
	Confirmed      int            `json:"confirmed"`
	NotReviewed    int            `json:"not_reviewed"`
	PendingOmitted int            `json:"pending_omitted"`
	AxeRules       int            `json:"axe_failing_rules"`
	AxeNodes       int            `json:"axe_failing_elements"`
	New            int            `json:"new,omitempty"`
	StillPresent   int            `json:"still_present,omitempty"`
	Fixed          int            `json:"fixed,omitempty"`
}

// Scope says what was tested, how and with what.
type Scope struct {
	Targets   []TargetUse   `json:"targets"`
	StartedAt string        `json:"started_at"`
	EndedAt   string        `json:"ended_at"`
	Browsers  []BrowserUse  `json:"browsers"`
	Models    []ModelUse    `json:"models"`
	Personas  []PersonaUse  `json:"personas"`
	Scenarios []ScenarioUse `json:"scenarios"`
	Runs      RunCounts     `json:"runs"`
	Cost      Cost          `json:"cost"`
	Routes    []string      `json:"routes_visited"`
}

// TargetUse is one build of one preview.
type TargetUse struct {
	BaseURL     string `json:"base_url"`
	Environment string `json:"environment"`
	Adapter     string `json:"adapter"`
	Build       string `json:"build"`
	BuildKind   string `json:"build_kind"`
	Runs        int    `json:"runs"`
}

// BrowserUse counts runs in one browser.
type BrowserUse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Runs    int    `json:"runs"`
}

// ModelUse counts runs on one model.
type ModelUse struct {
	Kind   string `json:"kind"`
	Model  string `json:"model"`
	Region string `json:"region"`
	Runs   int    `json:"runs"`
}

// PersonaUse describes one persona across its runs.
type PersonaUse struct {
	ID         string   `json:"id"`
	Role       string   `json:"role"`
	Devices    []string `json:"devices"`
	Dimensions []string `json:"dimensions"`
	Runs       int      `json:"runs"`
}

// ScenarioUse counts runs of one scenario and whether the persona did what it came to do.
type ScenarioUse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Runs        int    `json:"runs"`
	GoalMet     int    `json:"goal_met"`
	GoalPartly  int    `json:"goal_partly"`
	GoalNotMet  int    `json:"goal_not_met"`
	GoalUnknown int    `json:"goal_unknown"`
}

// RunCounts counts runs by outcome.
type RunCounts struct {
	Total           int `json:"total"`
	Completed       int `json:"completed"`
	Blocked         int `json:"blocked"`
	Errored         int `json:"errored"`
	BudgetExhausted int `json:"budget_exhausted"`
}

// Cost is what the runs cost in model calls. UnpricedRuns counts runs whose model has no price.
type Cost struct {
	USD          float64 `json:"usd"`
	UnpricedRuns int     `json:"unpriced_runs"`
	ModelCalls   int     `json:"model_calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
}

// BasisConfirmed is the basis of a report that counts only findings a person confirmed.
const BasisConfirmed = "confirmed findings"

// CoverageSection reuses the numbers of `dummyrun coverage`.
type CoverageSection struct {
	Available   bool   `json:"available"`
	Unavailable string `json:"unavailable,omitempty"`
	Basis       string `json:"basis,omitempty"`
	// Observed is the number of distinct problems seen.
	Observed int `json:"observed"`
	// TotalAtLeast is the Chao2 lower bound on all distinct problems, null when not estimable.
	TotalAtLeast *float64 `json:"total_at_least"`
	Form         string   `json:"chao2_form,omitempty"`
	// UnfoundAtLeast is TotalAtLeast less Observed, never below zero.
	UnfoundAtLeast *float64         `json:"unfound_at_least"`
	SampleCoverage *float64         `json:"sample_coverage"`
	NewPerExtraRun *float64         `json:"new_per_extra_run"`
	CompletedRuns  int              `json:"completed_runs"`
	Label          string           `json:"label,omitempty"`
	StoppingRule   string           `json:"stopping_rule,omitempty"`
	RoutesVisited  int              `json:"routes_visited"`
	RouteUniverse  int              `json:"route_universe"`
	Warnings       []string         `json:"warnings,omitempty"`
	Detail         *coverage.Report `json:"detail,omitempty"`
}

// Finding is one problem, with every run that found it folded in by fingerprint.
type Finding struct {
	Fingerprint string `json:"fingerprint"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	Symptom     string `json:"symptom,omitempty"`
	Route       string `json:"route,omitempty"`
	Locus       string `json:"locus,omitempty"`
	State       string `json:"state,omitempty"`
	Evidence    string `json:"evidence"`
	// Status is "confirmed" or "not-reviewed".
	Status     string   `json:"status"`
	Reviewers  []string `json:"reviewers,omitempty"`
	ReviewedAt string   `json:"reviewed_at,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	// WCAG lists success criteria where known. Persona findings often have none.
	WCAG []string `json:"wcag,omitempty"`
	// HelpURL points to fix guidance where there is some.
	HelpURL string `json:"help_url,omitempty"`
	// Baseline is new or still-present when a baseline report was given.
	Baseline     string       `json:"baseline,omitempty"`
	Personas     []string     `json:"personas"`
	Runs         int          `json:"runs"`
	OfRuns       int          `json:"of_runs"`
	Reproduction Reproduction `json:"reproduction"`
	Screenshots  []Screenshot `json:"screenshots,omitempty"`
	Instances    []Instance   `json:"instances"`
}

// Reproduction is the context and cited steps of one run that found the problem.
type Reproduction struct {
	Run     string    `json:"run"`
	Persona string    `json:"persona"`
	Role    string    `json:"role"`
	Device  string    `json:"device"`
	Zoom    int       `json:"zoom_percent"`
	Browser string    `json:"browser"`
	Steps   []StepRef `json:"steps"`
}

// StepRef is a cited step.
type StepRef struct {
	N      int    `json:"n"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Route  string `json:"route,omitempty"`
	URL    string `json:"url,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Screenshot refers to a screenshot of a cited step. File is where it was found, and is empty when
// the file does not exist. Renderers embed it in HTML.
type Screenshot struct {
	Run  string `json:"run"`
	Step int    `json:"step"`
	Path string `json:"path"`
	Alt  string `json:"alt"`
	File string `json:"-"`
}

// Instance is one run's sighting of the problem.
type Instance struct {
	Run       string `json:"run"`
	FindingID string `json:"finding_id"`
	Persona   string `json:"persona"`
	Device    string `json:"device"`
	Browser   string `json:"browser"`
	Steps     []int  `json:"steps"`
	Status    string `json:"status"`
}

// FixedFinding is a problem in the baseline report that this report did not find again.
type FixedFinding struct {
	Fingerprint string `json:"fingerprint"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Source      string `json:"source"`
}

// BaselineInfo says which earlier report this one was compared with.
type BaselineInfo struct {
	Title string `json:"title"`
	Date  string `json:"date"`
}

// AxeSection is the accessibility sweep. Impact follows axe: critical, serious, moderate and minor.
type AxeSection struct {
	Tool         string    `json:"tool"`
	Browsers     []string  `json:"browsers"`
	AxeVersion   string    `json:"axe_version,omitempty"`
	Tags         []string  `json:"tags"`
	Pages        int       `json:"pages"`
	Runs         int       `json:"runs"`
	FailingNodes int       `json:"failing_elements"`
	AllowedNodes int       `json:"allowed_elements"`
	PageErrors   int       `json:"page_errors"`
	Rules        []AxeRule `json:"rules"`
	NeedsReview  []AxeRule `json:"needs_review"`
	// Advice holds the axe best-practice rules that failed. They are not WCAG failures, never count as failing
	// elements and are shown apart from them.
	Advice []AxeRule  `json:"advice,omitempty"`
	Errors []AxeError `json:"errors,omitempty"`
}

// AxeRule is one rule that failed (or needs a person to look), folded across pages.
type AxeRule struct {
	Fingerprint string    `json:"fingerprint"`
	Rule        string    `json:"rule"`
	Impact      string    `json:"impact"`
	WCAG        []string  `json:"wcag,omitempty"`
	Help        string    `json:"help"`
	HelpURL     string    `json:"help_url,omitempty"`
	Elements    int       `json:"elements"`
	Baseline    string    `json:"baseline,omitempty"`
	Pages       []AxePage `json:"pages"`
}

// AxePage lists the elements of one page that failed a rule.
type AxePage struct {
	URL      string   `json:"url"`
	Targets  []string `json:"targets"`
	Settings []string `json:"settings"`
}

// AxeError is a page the sweep could not test.
type AxeError struct {
	URL     string `json:"url"`
	Setting string `json:"setting"`
	Message string `json:"message"`
}
