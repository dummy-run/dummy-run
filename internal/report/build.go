package report

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/confidence"
	"github.com/dummy-run/dummy-run/internal/coverage"
	"github.com/dummy-run/dummy-run/internal/fixes"
	"github.com/dummy-run/dummy-run/internal/identity"
	"github.com/dummy-run/dummy-run/internal/record"
)

// Options choose what a report holds.
type Options struct {
	Title  string
	Client string
	// IncludePending adds findings no reviewer has confirmed, marked as not reviewed. Rejected
	// findings never appear.
	IncludePending bool
	// Build selects records by commit or build hash (at least 7 characters).
	Build string
	// Date is the report date. The zero value means now.
	Date time.Time
	// ToolVersion is the dummyrun version.
	ToolVersion string
	A11y        []*a11y.Report
	Baseline    *Report
	// Thresholds are passed to the coverage model. The zero value means the defaults.
	Thresholds coverage.Thresholds
}

// defaultAxeVersion is the axe-core minor version used to link rule help when a sweep names none.
const defaultAxeVersion = "4.13"

// Limits is the statement of what the report is not. Every format prints it.
var Limits = []string{
	"This report is a sample. Personas played a set of scenarios on the builds listed under scope. A problem they did not meet is not in it.",
	"It is not a WCAG conformance claim, a certificate or a statement about the law.",
	"Automated checks find only some accessibility problems. The sweep covers the pages it lists and nothing else.",
	"It does not replace testing with disabled people.",
	"A finding marked not reviewed is a claim by a model that no person has checked.",
	"Parts of the app that no run visited are not covered, whether or not the report has findings.",
}

var severityRank = map[string]int{"blocker": 0, "major": 1, "minor": 2, "info": 3}
var impactRank = map[string]int{"critical": 0, "serious": 1, "moderate": 2, "minor": 3, "": 4}

// Severities lists persona finding severities, worst first.
var Severities = []string{"blocker", "major", "minor", "info"}

// Build assembles the report.
func Build(runs []*Run, opt Options) (*Report, error) {
	if opt.Build != "" {
		if len(opt.Build) < 7 {
			return nil, errors.New("--build needs at least 7 characters of the commit or build hash")
		}
		var keep []*Run
		for _, r := range runs {
			if matchesBuild(buildOf(r.Rec), opt.Build) {
				keep = append(keep, r)
			}
		}
		runs = keep
	}
	if len(runs) == 0 && len(opt.A11y) == 0 {
		return nil, errors.New("no run records and no accessibility results to report on")
	}
	date := opt.Date
	if date.IsZero() {
		date = time.Now()
	}
	title := opt.Title
	if title == "" {
		title = "Accessibility and usability audit"
	}
	rep := &Report{
		ReportVersion: ReportVersion,
		Title:         title,
		Client:        opt.Client,
		Date:          date.UTC().Format("2006-01-02"),
		Tool:          "dummyrun",
		ToolVersion:   opt.ToolVersion,
		Basis:         BasisConfirmed,
		Limits:        Limits,
		Warnings:      []string{},
	}
	if opt.IncludePending {
		rep.Basis = "confirmed findings and findings not reviewed"
	}
	rep.Scope = buildScope(runs)
	// A report from a sweep alone takes its times from the sweep.
	if rep.Scope.StartedAt == "" {
		rep.Scope.StartedAt, rep.Scope.EndedAt = sweepTimes(opt.A11y)
	}
	rep.Axe = buildAxe(opt.A11y, routesVisited(runs))
	var axeRules map[string]AxeRule
	if rep.Axe != nil {
		axeRules = map[string]AxeRule{}
		for _, r := range append(append([]AxeRule{}, rep.Axe.Rules...), rep.Axe.NeedsReview...) {
			axeRules[r.Rule] = r
		}
	}
	axeVersion := defaultAxeVersion
	if rep.Axe != nil && rep.Axe.AxeVersion != "" {
		parts := strings.Split(rep.Axe.AxeVersion, ".")
		if len(parts) >= 2 {
			axeVersion = parts[0] + "." + parts[1]
		}
	}
	var omitted []string
	rep.Findings, omitted = buildFindings(runs, opt.IncludePending, axeRules, axeVersion)
	rep.Summary.PendingOmitted = len(omitted)
	rep.Worked = buildWorked(runs)
	rep.Coverage = buildCoverage(runs, opt)
	if opt.Baseline != nil {
		applyBaseline(rep, opt.Baseline, omitted)
	}
	addEN301549(rep)
	summarise(rep)
	return rep, nil
}

// buildWorked collects the observations of the runs in the report. The same note from several runs is one
// entry. Observations are not findings, so they never enter the counts.
func buildWorked(runs []*Run) []WorkedNote {
	type acc struct {
		runs     map[string]bool
		personas map[string]bool
	}
	byText := map[string]*acc{}
	var order []string
	for _, run := range runs {
		for _, o := range run.Rec.Observations {
			text := strings.TrimSpace(o.Text)
			// A problem claim that did not become a finding is not something that worked, and has no evidence to show.
			if text == "" || o.Kind != "" {
				continue
			}
			a := byText[text]
			if a == nil {
				a = &acc{runs: map[string]bool{}, personas: map[string]bool{}}
				byText[text] = a
				order = append(order, text)
			}
			a.runs[run.Rec.ID] = true
			a.personas[run.Rec.Persona.ID] = true
		}
	}
	var out []WorkedNote
	for _, text := range order {
		a := byText[text]
		out = append(out, WorkedNote{Text: text, Runs: len(a.runs), Personas: sortedKeys(a.personas)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Runs > out[j].Runs })
	return out
}

func buildOf(r *record.Record) string {
	if r.Target.Commit != "" {
		return r.Target.Commit
	}
	if r.Target.BuildHash != "" {
		return r.Target.BuildHash
	}
	return "unknown"
}

func matchesBuild(build, want string) bool {
	return build != "unknown" && (strings.HasPrefix(build, want) || strings.HasPrefix(want, build))
}

func buildScope(runs []*Run) Scope {
	sc := Scope{Targets: []TargetUse{}, Browsers: []BrowserUse{}, Models: []ModelUse{}, Personas: []PersonaUse{},
		Scenarios: []ScenarioUse{}, Routes: []string{}}
	if len(runs) == 0 {
		return sc
	}
	targets := map[string]*TargetUse{}
	browsers := map[string]*BrowserUse{}
	models := map[string]*ModelUse{}
	scenarios := map[string]*ScenarioUse{}
	type pAgg struct {
		use     *PersonaUse
		devices map[string]bool
		dims    map[string]bool
	}
	personas := map[string]*pAgg{}
	routes := map[string]bool{}
	var start, end time.Time
	for _, run := range runs {
		r := run.Rec
		if start.IsZero() || r.StartedAt.Before(start) {
			start = r.StartedAt
		}
		if r.EndedAt.After(end) {
			end = r.EndedAt
		}
		b := buildOf(r)
		kind := "commit"
		if r.Target.Commit == "" {
			kind = "build hash"
		}
		tk := r.Target.BaseURL + "|" + r.Target.Environment + "|" + b
		if targets[tk] == nil {
			targets[tk] = &TargetUse{BaseURL: r.Target.BaseURL, Environment: r.Target.Environment, Adapter: r.Target.Adapter, Build: b, BuildKind: kind}
		}
		targets[tk].Runs++
		bk := r.Browser.Name + "|" + r.Browser.Version
		if browsers[bk] == nil {
			browsers[bk] = &BrowserUse{Name: r.Browser.Name, Version: r.Browser.Version}
		}
		browsers[bk].Runs++
		mk := r.Provider.Kind + "|" + r.Provider.Model + "|" + r.Provider.Region
		if models[mk] == nil {
			models[mk] = &ModelUse{Kind: r.Provider.Kind, Model: r.Provider.Model, Region: r.Provider.Region}
		}
		models[mk].Runs++
		sk := r.Scenario.ID
		if scenarios[sk] == nil {
			scenarios[sk] = &ScenarioUse{ID: r.Scenario.ID, Title: r.Scenario.Title}
		}
		s := scenarios[sk]
		s.Runs++
		switch r.Scenario.GoalMet {
		case "yes":
			s.GoalMet++
		case "partly":
			s.GoalPartly++
		case "no":
			s.GoalNotMet++
		default:
			s.GoalUnknown++
		}
		p := personas[r.Persona.ID]
		if p == nil {
			p = &pAgg{use: &PersonaUse{ID: r.Persona.ID, Name: r.Persona.Name, Role: r.Persona.Role}, devices: map[string]bool{}, dims: map[string]bool{}}
			personas[r.Persona.ID] = p
		}
		p.use.Runs++
		dev := r.Persona.Device
		if r.Persona.Zoom > 0 && r.Persona.Zoom != 100 {
			dev += fmt.Sprintf(" at %d percent zoom", r.Persona.Zoom)
		}
		if r.Persona.TextScale > 100 {
			dev += fmt.Sprintf(" with text at %d percent", r.Persona.TextScale)
		}
		p.devices[dev] = true
		if d := r.Persona.Dimensions; d != nil {
			if d.Viewport != nil {
				p.dims[fmt.Sprintf("viewport %dx%d", d.Viewport.Width, d.Viewport.Height)] = true
			}
			if d.Locale != nil {
				p.dims["locale "+*d.Locale] = true
			}
			if d.AT != nil && *d.AT != "none" {
				p.dims["assistive technology "+*d.AT] = true
			}
			if d.Connection != nil && *d.Connection != "unthrottled" {
				p.dims["connection "+*d.Connection] = true
			}
			if d.Motion != nil && *d.Motion != "no-preference" {
				p.dims["motion "+*d.Motion] = true
			}
		}
		sc.Runs.Total++
		switch r.Outcome {
		case record.OutcomeCompleted:
			sc.Runs.Completed++
		case record.OutcomeBlocked:
			sc.Runs.Blocked++
		case record.OutcomeErrored:
			sc.Runs.Errored++
		case record.OutcomeBudgetExhausted:
			sc.Runs.BudgetExhausted++
		}
		if r.Usage.CostUSD != nil {
			sc.Cost.USD += *r.Usage.CostUSD
		} else {
			sc.Cost.UnpricedRuns++
		}
		sc.Cost.ModelCalls += r.Usage.ModelCalls
		sc.Cost.InputTokens += r.Usage.InputTokens
		sc.Cost.OutputTokens += r.Usage.OutputTokens
		for _, st := range r.Steps {
			if st.Route != "" {
				routes[st.Route] = true
			}
		}
	}
	sc.Cost.USD = math.Round(sc.Cost.USD*10000) / 10000
	sc.StartedAt = start.UTC().Format(time.RFC3339)
	sc.EndedAt = end.UTC().Format(time.RFC3339)
	for _, v := range targets {
		sc.Targets = append(sc.Targets, *v)
	}
	sort.Slice(sc.Targets, func(i, j int) bool {
		a, b := sc.Targets[i], sc.Targets[j]
		return a.BaseURL+a.Build < b.BaseURL+b.Build
	})
	for _, v := range browsers {
		sc.Browsers = append(sc.Browsers, *v)
	}
	sort.Slice(sc.Browsers, func(i, j int) bool {
		return sc.Browsers[i].Name+sc.Browsers[i].Version < sc.Browsers[j].Name+sc.Browsers[j].Version
	})
	for _, v := range models {
		sc.Models = append(sc.Models, *v)
	}
	sort.Slice(sc.Models, func(i, j int) bool {
		return sc.Models[i].Model+sc.Models[i].Region < sc.Models[j].Model+sc.Models[j].Region
	})
	for _, v := range personas {
		v.use.Devices = sortedKeys(v.devices)
		v.use.Dimensions = sortedKeys(v.dims)
		sc.Personas = append(sc.Personas, *v.use)
	}
	sort.Slice(sc.Personas, func(i, j int) bool { return sc.Personas[i].ID < sc.Personas[j].ID })
	for _, v := range scenarios {
		sc.Scenarios = append(sc.Scenarios, *v)
	}
	sort.Slice(sc.Scenarios, func(i, j int) bool { return sc.Scenarios[i].ID < sc.Scenarios[j].ID })
	sc.Routes = sortedKeys(routes)
	return sc
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// hit is one run's sighting of a problem.
type hit struct {
	run    *Run
	f      record.Finding
	status string
}

// visits counts the runs that visited each route, by the role the persona played. A finding one role met is
// only expected of runs in the same role, so another role's runs never count against it.
type visits map[string]map[string]int

// routesVisited counts, for each role and route, the runs in that role that visited the route.
func routesVisited(runs []*Run) visits {
	out := visits{}
	for _, run := range runs {
		role := run.Rec.Persona.Role
		if out[role] == nil {
			out[role] = map[string]int{}
		}
		seen := map[string]bool{}
		for _, st := range run.Rec.Steps {
			if st.Route != "" && !seen[st.Route] {
				seen[st.Route] = true
				out[role][st.Route]++
			}
		}
	}
	return out
}

// any reports whether a run in any role visited the route.
func (v visits) any(route string) bool {
	for _, byRoute := range v {
		if byRoute[route] > 0 {
			return true
		}
	}
	return false
}

// eligible counts the runs in the given roles that visited the route.
func (v visits) eligible(roles map[string]bool, route string) int {
	n := 0
	for role := range roles {
		n += v[role][route]
	}
	return n
}

func buildFindings(runs []*Run, includePending bool, axe map[string]AxeRule, axeVersion string) ([]Finding, []string) {
	groups := map[string][]hit{}
	var order []string
	visited := routesVisited(runs)
	for _, run := range runs {
		for _, f := range run.Rec.Findings {
			if f.Class == record.ClassObservation || f.Verdict == "rejected" {
				continue
			}
			status := StatusConfirmed
			if f.Verdict != "confirmed" {
				status = StatusNotReviewed
			}
			key := f.Fingerprint
			if key == "" {
				key = "unfingerprinted-" + run.Rec.ID + "-" + f.ID
			}
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], hit{run: run, f: f, status: status})
		}
	}
	total := len(runs)
	out := []Finding{}
	var omitted []string
	for _, key := range order {
		hits := groups[key]
		var confirmed, pending []hit
		for _, h := range hits {
			if h.status == StatusConfirmed {
				confirmed = append(confirmed, h)
			} else {
				pending = append(pending, h)
			}
		}
		use := confirmed
		status := StatusConfirmed
		if len(confirmed) == 0 {
			if !includePending {
				omitted = append(omitted, key)
				continue
			}
			use, status = pending, StatusNotReviewed
		} else if includePending {
			use = hits
		}
		out = append(out, makeFinding(key, use, status, total, axe, axeVersion, visited))
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] < severityRank[b.Severity]
		}
		if a.Runs != b.Runs {
			return a.Runs > b.Runs
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.Fingerprint < b.Fingerprint
	})
	return out, omitted
}

func makeFinding(key string, hits []hit, status string, total int, axe map[string]AxeRule, axeVersion string, visited visits) Finding {
	// The lead sighting gives the title: the worst severity, then the earliest run. A confirmed finding
	// takes its lead and its reproduction from the confirmed sightings, so a model's claim that no person
	// checked cannot set the severity beside the badge that says a person confirmed it.
	pool := hits
	if status == StatusConfirmed {
		var confirmed []hit
		for _, h := range hits {
			if h.status == StatusConfirmed {
				confirmed = append(confirmed, h)
			}
		}
		if len(confirmed) > 0 {
			pool = confirmed
		}
	}
	lead := pool[0]
	for _, h := range pool[1:] {
		if severityRank[h.f.Severity] < severityRank[lead.f.Severity] {
			lead = h
		}
	}
	f := Finding{
		Fingerprint: key, Title: lead.f.Title, Severity: lead.f.Severity, Kind: lead.f.Kind,
		Symptom: lead.f.Symptom, Route: lead.f.Route, Locus: lead.f.Locus, State: lead.f.State,
		Evidence: lead.f.Evidence, Status: status, Personas: []string{}, Instances: []Instance{},
		OfRuns: total, Body: lead.f.Body, FixText: lead.f.Fix, Element: lead.f.Element, Quote: lead.f.Quote, URL: lead.f.URL,
		WCAG: lead.f.WCAG,
	}
	if strings.HasPrefix(key, "unfingerprinted-") {
		f.Fingerprint = ""
	}
	// Review details come from the confirmed sightings.
	reviewers := map[string]bool{}
	personas := map[string]bool{}
	roles := map[string]bool{}
	runSet := map[string]bool{}
	for _, h := range hits {
		personas[h.run.Rec.Persona.ID] = true
		roles[h.run.Rec.Persona.Role] = true
		runSet[h.run.Rec.ID] = true
		f.Instances = append(f.Instances, Instance{Run: h.run.Rec.ID, FindingID: h.f.ID, Persona: h.run.Rec.Persona.ID,
			Device: h.run.Rec.Persona.Device, Browser: h.run.Rec.Browser.Name, Steps: append([]int{}, h.f.Steps...), Status: h.status})
		if h.status != StatusConfirmed {
			continue
		}
		for _, who := range h.f.ReviewedBy {
			reviewers[who] = true
		}
		if rv, ok := h.run.Reviews[h.f.ID]; ok {
			if rv.Reviewer != "" {
				reviewers[rv.Reviewer] = true
			}
			if rv.ReviewedAt > f.ReviewedAt {
				f.ReviewedAt = rv.ReviewedAt
			}
			if f.Reason == "" {
				f.Reason = rv.Reason
			}
		}
	}
	f.Reviewers = sortedKeys(reviewers)
	f.Personas = sortedKeys(personas)
	f.Runs = len(runSet)
	sort.Slice(f.Instances, func(i, j int) bool { return f.Instances[i].Run < f.Instances[j].Run })

	// The reproduction is the first sighting that has a screenshot, or else the first sighting.
	rep := pool[0]
	for _, h := range pool {
		if len(screenshotsFor(h)) > 0 {
			rep = h
			break
		}
	}
	for _, h := range pool {
		if h.run.Rec.ID < rep.run.Rec.ID && len(screenshotsFor(rep)) == 0 && len(screenshotsFor(h)) == 0 {
			rep = h
		}
	}
	r := rep.run.Rec
	f.Reproduction = Reproduction{Run: r.ID, Persona: r.Persona.ID, Role: r.Persona.Role, SignedIn: r.Signin != nil && r.Signin.OK, Device: r.Persona.Device,
		Zoom: orHundred(r.Persona.Zoom), Browser: strings.TrimSpace(r.Browser.Name + " " + r.Browser.Version), Steps: []StepRef{}}
	cited := map[int]bool{}
	for _, n := range rep.f.Steps {
		cited[n] = true
	}
	for _, st := range r.Steps {
		if cited[st.N] {
			f.Reproduction.Steps = append(f.Reproduction.Steps, StepRef{N: st.N, Action: st.Action, Target: st.Target,
				Element: st.Element, Route: st.Route, URL: st.URL, Note: st.Note, Evidence: st.N == rep.f.EvidenceStep})
		}
	}
	f.Screenshots = screenshotsFor(rep)
	// The screenshot of the step where the engine found the evidence comes first.
	sort.SliceStable(f.Screenshots, func(i, j int) bool {
		return f.Screenshots[i].Step == rep.f.EvidenceStep && f.Screenshots[j].Step != rep.f.EvidenceStep
	})

	if rule, ok := strings.CutPrefix(f.Symptom, "a11y:"); ok {
		if ax, ok := axe[rule]; ok && len(ax.WCAG) > 0 {
			f.WCAG = ax.WCAG
			f.HelpURL = ax.HelpURL
		}
		// Deque has a page for each axe rule, but none for Dummy Run's own rules.
		if f.HelpURL == "" && !strings.HasPrefix(rule, "dummyrun-") {
			f.HelpURL = "https://dequeuniversity.com/rules/axe/" + axeVersion + "/" + rule
		}
		f.Fix = fixes.For(rule)
	}
	// Reproduction counts the runs that could have met the problem: those in a role that found it, which visited
	// its route. Runs in other roles may not see the same page, so they never dilute the count.
	in := deriveInputs(lead.f)
	if of := visited.eligible(roles, f.Route); of >= 2 || f.Runs >= 2 {
		in.Reproduced = &record.Reproduced{Runs: f.Runs, Of: max(of, f.Runs)}
	}
	rating := confidence.Default().Rate(in)
	f.Inputs, f.Confidence, f.ConfidenceReason = &in, rating.Band, rating.Reason
	return f
}

// deriveInputs is a finding's inputs: the record's when it has them, else what can be read from the finding
// alone. A record from before inputs existed gives no way to tell what a cited accessibility check reported,
// so its finding counts as the model's own report.
func deriveInputs(f record.Finding) record.FindingInputs {
	if f.Inputs != nil {
		in := *f.Inputs
		in.Engines = append([]string{}, in.Engines...)
		return in
	}
	base, param, _ := strings.Cut(f.Symptom, ":")
	check := base
	if base == "a11y" && param != "" {
		check = param
	}
	if check == "" {
		check = f.Kind
	}
	return record.FindingInputs{EvidenceKind: record.EvidenceModelOnly, Engines: []string{}, Check: check, OnJourney: true}
}

func orHundred(z int) int {
	if z == 0 {
		return 100
	}
	return z
}

func screenshotsFor(h hit) []Screenshot {
	var out []Screenshot
	r := h.run.Rec
	for _, n := range h.f.Steps {
		for _, st := range r.Steps {
			if st.N != n || st.Screenshot == "" {
				continue
			}
			s := Screenshot{Run: r.ID, Step: n, Path: st.Screenshot,
				Alt: fmt.Sprintf("Screenshot from run %s, step %d: %s", r.ID, n, describeStep(st))}
			if !filepath.IsLocal(st.Screenshot) {
				// A path that is absolute or climbs out of the run folder is never read.
				out = append(out, s)
				continue
			}
			for _, cand := range []string{filepath.Join(h.run.Dir, st.Screenshot), filepath.Join(h.run.Dir, r.ID, st.Screenshot)} {
				if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
					s.File = cand
					break
				}
			}
			out = append(out, s)
		}
	}
	return out
}

func describeStep(st record.Step) string {
	s := st.Action
	if st.Target != "" {
		s += " " + st.Target
	}
	if st.Route != "" {
		s += " on " + st.Route
	}
	return s
}

func axeFingerprint(rule string) string {
	sum := sha256.Sum256([]byte("axe|" + rule))
	return hex.EncodeToString(sum[:])[:16]
}

func buildAxe(reps []*a11y.Report, visited visits) *AxeSection {
	if len(reps) == 0 {
		return nil
	}
	sec := &AxeSection{Tool: "dummyrun a11y", Browsers: []string{}, Tags: []string{}, Rules: []AxeRule{}, NeedsReview: []AxeRule{}}
	pages := map[string]bool{}
	browsers := map[string]bool{}
	tags := map[string]bool{}
	adviceTags := map[string]bool{}
	type acc struct {
		rule  AxeRule
		pages map[string]*AxePage
		nodes map[string]bool
		// evidence is the rule's evidence kind, and engines the engines that agree on every failing element.
		evidence string
		engines  []string
		journey  bool
		// colours are the failing colour pairs, and colourNodes the elements already counted for each.
		colours     map[string]*ColourPair
		colourNodes map[string]bool
	}
	collect := func(into map[string]*acc, f a11y.Finding, p a11y.PageResult, countAllowed bool) {
		a := into[f.Rule]
		if a == nil {
			a = &acc{rule: AxeRule{Fingerprint: axeFingerprint(f.Rule), Rule: f.Rule, Impact: f.Impact, WCAG: f.WCAG, Help: f.Help, HelpURL: f.HelpURL, Fix: fixes.For(f.Rule)},
				pages: map[string]*AxePage{}, nodes: map[string]bool{}, evidence: evidenceOf(f)}
			into[f.Rule] = a
		}
		setting := p.Setting()
		for _, n := range f.Nodes {
			if n.Allowed {
				if countAllowed {
					sec.AllowedNodes++
				}
				continue
			}
			// Only an element that still fails puts the rule on a journey. An allow-listed one does not count.
			if visited.any(identity.NormaliseRoute(p.URL)) {
				a.journey = true
			}
			a.engines = agreeing(a.engines, nodeEngines(f, n), len(a.nodes) == 0)
			pg := a.pages[p.URL]
			if pg == nil {
				pg = &AxePage{URL: p.URL, Targets: []string{}, Settings: []string{}}
				a.pages[p.URL] = pg
			}
			if !contains(pg.Targets, n.Target) {
				pg.Targets = append(pg.Targets, n.Target)
			}
			if !contains(pg.Settings, setting) {
				pg.Settings = append(pg.Settings, setting)
			}
			// An element counts once for each pair of colours it has, so one in light and dark mode can count twice.
			if c := n.Colours; c != nil {
				key := fmt.Sprintf("%s|%s|%g", c.Text, c.Background, c.Need)
				if a.colours == nil {
					a.colours, a.colourNodes = map[string]*ColourPair{}, map[string]bool{}
				}
				cp := a.colours[key]
				if cp == nil {
					cp = &ColourPair{Text: c.Text, Background: c.Background, Ratio: c.Ratio, Need: c.Need, TextFix: c.TextFix, BackgroundFix: c.BackgroundFix}
					a.colours[key] = cp
				}
				if node := key + "|" + p.URL + "|" + n.Target; !a.colourNodes[node] {
					a.colourNodes[node] = true
					cp.Elements++
				}
			}
			a.nodes[p.URL+"|"+n.Target] = true
		}
	}
	fail := map[string]*acc{}
	review := map[string]*acc{}
	advice := map[string]*acc{}
	for _, rep := range reps {
		if rep.Browser != "" {
			b := rep.Browser
			if rep.BrowserVersion != "" {
				b += " " + rep.BrowserVersion
			}
			browsers[b] = true
		}
		if sec.AxeVersion == "" {
			sec.AxeVersion = rep.AxeVersion
		}
		for _, t := range rep.Tags {
			tags[t] = true
		}
		for _, t := range rep.AdviceTags {
			adviceTags[t] = true
		}
		for _, p := range rep.Results {
			pages[p.URL] = true
			sec.Runs++
			if p.Error != "" {
				sec.PageErrors++
				sec.Errors = append(sec.Errors, AxeError{URL: p.URL, Setting: p.Setting(), Message: p.Error})
				continue
			}
			for _, f := range p.Violations {
				collect(fail, f, p, true)
			}
			for _, f := range p.NeedsReview {
				collect(review, f, p, false)
			}
			for _, f := range p.Advice {
				collect(advice, f, p, false)
			}
		}
	}
	sec.Pages = len(pages)
	sec.Browsers = sortedKeys(browsers)
	sec.Tags = sortedKeys(tags)
	if len(adviceTags) > 0 {
		sec.AdviceTags = sortedKeys(adviceTags)
	}
	flatten := func(m map[string]*acc, countFailing bool) []AxeRule {
		out := []AxeRule{}
		for _, a := range m {
			if len(a.pages) == 0 {
				continue
			}
			a.rule.Elements = len(a.nodes)
			if countFailing {
				sec.FailingNodes += len(a.nodes)
				in := record.FindingInputs{EvidenceKind: a.evidence, Engines: append([]string{}, a.engines...), Check: a.rule.Rule, OnJourney: a.journey}
				rating := confidence.Default().Rate(in)
				a.rule.Severity = confidence.Severity(confidence.SeverityInputs{Evidence: a.evidence, Impact: a.rule.Impact, Level: a11y.Level(a.rule.WCAG), OnJourney: a.journey, Pages: len(a.pages)})
				a.rule.Inputs, a.rule.Confidence, a.rule.ConfidenceReason = &in, rating.Band, rating.Reason
			}
			for _, pg := range a.pages {
				sort.Strings(pg.Targets)
				sort.Strings(pg.Settings)
				a.rule.Pages = append(a.rule.Pages, *pg)
			}
			sort.Slice(a.rule.Pages, func(i, j int) bool { return a.rule.Pages[i].URL < a.rule.Pages[j].URL })
			for _, cp := range a.colours {
				a.rule.Colours = append(a.rule.Colours, *cp)
			}
			// The pairs on the most elements come first.
			sort.Slice(a.rule.Colours, func(i, j int) bool {
				ci, cj := a.rule.Colours[i], a.rule.Colours[j]
				if ci.Elements != cj.Elements {
					return ci.Elements > cj.Elements
				}
				return ci.Text+ci.Background < cj.Text+cj.Background
			})
			out = append(out, a.rule)
		}
		sort.Slice(out, func(i, j int) bool {
			if impactRank[out[i].Impact] != impactRank[out[j].Impact] {
				return impactRank[out[i].Impact] < impactRank[out[j].Impact]
			}
			return out[i].Rule < out[j].Rule
		})
		return out
	}
	sec.Rules = flatten(fail, true)
	sec.NeedsReview = flatten(review, false)
	if a := flatten(advice, false); len(a) > 0 {
		sec.Advice = a
	}
	return sec
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// evidenceOf is a sweep finding's evidence kind. A sweep from before the field existed is read from the rule ID:
// the layout and keyboard checks measure, the markup checks read the accessibility tree, and the rest are rule
// engines.
func evidenceOf(f a11y.Finding) string {
	switch {
	case f.Evidence != "":
		return f.Evidence
	case a11y.IsLayoutRule(f.Rule), a11y.IsKeyboardRule(f.Rule):
		return record.EvidenceMeasurement
	case a11y.IsMarkupRule(f.Rule):
		return record.EvidenceAxFact
	}
	return record.EvidenceEngineRule
}

// nodeEngines is the engines that report one failing element: the ones the sweep recorded, or axe alone for an
// axe rule from a sweep that ran no other engine.
func nodeEngines(f a11y.Finding, n a11y.Node) []string {
	if len(n.Engines) > 0 {
		return n.Engines
	}
	if evidenceOf(f) == record.EvidenceEngineRule && !a11y.IsEnginesRule(f.Rule) {
		return []string{"axe"}
	}
	return nil
}

// agreeing keeps the engines that report every element so far: the first element's engines, then the
// intersection with each later one.
func agreeing(sofar, next []string, first bool) []string {
	if first {
		return append([]string{}, next...)
	}
	var out []string
	for _, e := range sofar {
		if contains(next, e) {
			out = append(out, e)
		}
	}
	return out
}

func buildCoverage(runs []*Run, opt Options) CoverageSection {
	cs := CoverageSection{}
	if len(runs) == 0 {
		cs.Unavailable = "No run records were given, so there is nothing to measure coverage on."
		return cs
	}
	recs := make([]*record.Record, len(runs))
	for i, r := range runs {
		recs[i] = r.Rec
	}
	rep, err := coverage.Build(recs, coverage.Options{Strata: "device", ConfirmedOnly: !opt.IncludePending, Thresholds: opt.Thresholds})
	if err != nil {
		var ref *coverage.Refusal
		if errors.As(err, &ref) {
			cs.Unavailable = ref.Msg
			return cs
		}
		cs.Unavailable = err.Error()
		return cs
	}
	cs.Available = true
	cs.Basis = BasisConfirmed
	if opt.IncludePending {
		cs.Basis = "findings that no person has confirmed, so a demonstration of the method and not an estimate"
	}
	cs.Detail = rep
	cs.Observed = rep.Completeness.SObs
	cs.CompletedRuns = rep.Completeness.T
	cs.Label = rep.Completeness.Label
	cs.StoppingRule = rep.Stopping.Summary
	cs.Warnings = []string{}
	for _, w := range rep.Warnings {
		if strings.Contains(w, "--confirmed-only") {
			w = "Findings that no person has confirmed are counted. Read these numbers as a demonstration of the method and not as an estimate."
		}
		cs.Warnings = append(cs.Warnings, strings.TrimSuffix(cap1(w), ".")+".")
	}
	if len(rep.Completeness.IdentityBand) > 0 {
		head := rep.Completeness.IdentityBand[0]
		cs.SampleCoverage = head.Coverage
		cs.NewPerExtraRun = head.NewPerExtraRun
		if head.Chao2 != nil && head.Chao2.Form != coverage.FormNotEstimable {
			v := head.Chao2.AtLeast
			cs.TotalAtLeast = &v
			cs.Form = head.Chao2.Form
			u := math.Max(0, math.Round((v-float64(head.SObs))*10)/10)
			cs.UnfoundAtLeast = &u
		}
	}
	if rc := rep.Reach.RouteCoverage; rc != nil {
		cs.RoutesVisited, cs.RouteUniverse = rc.Visited, rc.Universe
	}
	return cs
}

// applyBaseline marks each finding new or still present and lists what the baseline had and this report
// does not. A fingerprint that the runs saw again but left out as not reviewed is not fixed, so it counts as seen.
func applyBaseline(rep *Report, base *Report, omitted []string) {
	rep.Baseline = &BaselineInfo{Title: base.Title, Date: base.Date}
	prev := map[string]bool{}
	prevInfo := map[string]FixedFinding{}
	for _, f := range base.Findings {
		if f.Fingerprint != "" {
			prev[f.Fingerprint] = true
			prevInfo[f.Fingerprint] = FixedFinding{Fingerprint: f.Fingerprint, Title: f.Title, Severity: f.Severity, Source: "persona"}
		}
	}
	if base.Axe != nil {
		for _, r := range base.Axe.Rules {
			prev[r.Fingerprint] = true
			prevInfo[r.Fingerprint] = FixedFinding{Fingerprint: r.Fingerprint, Title: r.Help, Severity: r.Impact, Source: "axe"}
		}
	}
	seen := map[string]bool{}
	for _, fp := range omitted {
		seen[fp] = true
	}
	for i := range rep.Findings {
		f := &rep.Findings[i]
		if f.Fingerprint == "" {
			continue
		}
		seen[f.Fingerprint] = true
		f.Baseline = BaselineNew
		if prev[f.Fingerprint] {
			f.Baseline = BaselineStillPresent
		}
	}
	if rep.Axe != nil {
		for i := range rep.Axe.Rules {
			r := &rep.Axe.Rules[i]
			seen[r.Fingerprint] = true
			r.Baseline = BaselineNew
			if prev[r.Fingerprint] {
				r.Baseline = BaselineStillPresent
			}
		}
	}
	rep.Fixed = []FixedFinding{}
	for fp, info := range prevInfo {
		// A report can only say a problem was not seen again when it looked for that kind of problem.
		// Without runs nothing looked for persona findings, and without a sweep nothing looked for axe rules.
		// A sweep with pages that could not be tested may have missed the rule there, so it clears nothing.
		if info.Source == "axe" && (rep.Axe == nil || rep.Axe.PageErrors > 0) || info.Source != "axe" && rep.Scope.Runs.Total == 0 {
			continue
		}
		if !seen[fp] {
			rep.Fixed = append(rep.Fixed, info)
		}
	}
	sort.Slice(rep.Fixed, func(i, j int) bool {
		a, b := rep.Fixed[i], rep.Fixed[j]
		ra, rb := rank(a), rank(b)
		if ra != rb {
			return ra < rb
		}
		return a.Title+a.Fingerprint < b.Title+b.Fingerprint
	})
}

func rank(f FixedFinding) int {
	if f.Source == "axe" {
		return impactRank[f.Severity]
	}
	return severityRank[f.Severity]
}

func summarise(rep *Report) {
	s := &rep.Summary
	s.BySeverity = map[string]int{"blocker": 0, "major": 0, "minor": 0, "info": 0}
	for _, f := range rep.Findings {
		s.FindingsTotal++
		s.BySeverity[f.Severity]++
		if f.Status == StatusConfirmed {
			s.Confirmed++
		} else {
			s.NotReviewed++
		}
		switch f.Baseline {
		case BaselineNew:
			s.New++
		case BaselineStillPresent:
			s.StillPresent++
		}
	}
	if rep.Axe != nil {
		s.AxeRules = len(rep.Axe.Rules)
		s.AxeNodes = rep.Axe.FailingNodes
		for _, r := range rep.Axe.Rules {
			switch r.Baseline {
			case BaselineNew:
				s.New++
			case BaselineStillPresent:
				s.StillPresent++
			}
		}
	}
	s.Fixed = len(rep.Fixed)
	s.Text = summaryText(rep)
}

// sweepTimes is the earliest start and the latest end of the sweeps, or empty when they don't say. The times are
// compared as instants, so sweeps written with different time zone offsets order correctly. A time that isn't RFC
// 3339 is left out.
func sweepTimes(reps []*a11y.Report) (string, string) {
	var start, end string
	var first, last time.Time
	for _, r := range reps {
		if t, err := time.Parse(time.RFC3339Nano, r.StartedAt); err == nil && (start == "" || t.Before(first)) {
			start, first = r.StartedAt, t
		}
		if t, err := time.Parse(time.RFC3339Nano, r.EndedAt); err == nil && (end == "" || t.After(last)) {
			end, last = r.EndedAt, t
		}
	}
	return start, end
}

// addEN301549 fills in the EN 301 549 clauses from the WCAG criteria of every finding and axe rule.
func addEN301549(rep *Report) {
	for i := range rep.Findings {
		rep.Findings[i].EN301549 = a11y.EN301549(rep.Findings[i].WCAG)
	}
	if rep.Axe == nil {
		return
	}
	for _, rules := range [][]AxeRule{rep.Axe.Rules, rep.Axe.NeedsReview, rep.Axe.Advice} {
		for i := range rules {
			rules[i].EN301549 = a11y.EN301549(rules[i].WCAG)
		}
	}
}
