package runner

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/identity"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/record"
)

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func object(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

// Tools are the thirteen tools a persona can call.
var Tools = []provider.Tool{
	{Name: "look", Description: "Look at the current page again: title, headings, numbered controls (@1, @2...) and text."},
	{Name: "visit", Description: "Go to an address on the target site. Only when the scenario says to type an address.",
		Parameters: object(map[string]any{"url": str("Absolute URL, or a path such as /docs")}, "url")},
	{Name: "click", Description: "Click a control: '@12' from the last look, or the words on it.",
		Parameters: object(map[string]any{"target": str("@ref from the last look, the control's text, or css:selector")}, "target")},
	{Name: "fill", Description: "Put text into a field in one go: target is '@12' or its label; value is the text.",
		Parameters: object(map[string]any{"target": str("@ref or the field's label"), "value": str("The text")}, "target", "value")},
	{Name: "type", Description: "Type into a field key by key, as a person would, for search boxes and pickers that react as you type. It replaces what the field holds.",
		Parameters: object(map[string]any{"target": str("@ref or the field's label"), "text": str("The text")}, "target", "text")},
	{Name: "select", Description: "Choose an option in a select by its visible text.",
		Parameters: object(map[string]any{"target": str("@ref or the field's label"), "option": str("The option's text")}, "target", "option")},
	{Name: "press", Description: "Press a key: " + strings.Join(bidi.KeyNames, ", ") + ".",
		Parameters: object(map[string]any{"key": str("The key name")}, "key")},
	{Name: "back", Description: "Go back one page."},
	{Name: "scroll", Description: "Scroll the page by most of a screen.",
		Parameters: object(map[string]any{"direction": map[string]any{"type": "string", "enum": []string{"up", "down"}}}, "direction")},
	{Name: "wait", Description: "Wait for the page, up to 15 seconds.",
		Parameters: object(map[string]any{"seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 15}}, "seconds")},
	{Name: "a11y", Description: "Run the accessibility check (axe-core, WCAG 2.2 AA) on this page. Once per run, on the page that matters most."},
	{Name: "screenshot", Description: "See a screenshot of the page as it is now, when the text is not enough."},
	{Name: "finish", Description: "End the run with your verdict, your report and your findings. Every finding cites step numbers from the results.",
		Parameters: object(map[string]any{
			"verdict": map[string]any{"type": "string", "enum": []string{"pass", "friction", "fail", "blocked"},
				"description": "pass: you did the job. friction: you did it but something slowed or confused you. fail: the site stopped you. blocked: the set-up or the site's state stopped you before you could start."},
			"goal_met": map[string]any{"type": "string", "enum": []string{"yes", "partly", "no"},
				"description": "Did you do what you came to do? yes: all of it. partly: some of it or by a workaround. no: not at all."},
			"report": str("Your report in Markdown: what you tried, what happened, what you would tell the team. Cite steps as 'step 4' or 'steps 6-8'."),
			"findings": map[string]any{"type": "array", "description": "Each distinct problem or question, with evidence.",
				"items": object(map[string]any{
					"kind":     map[string]any{"type": "string", "enum": record.Kinds},
					"severity": map[string]any{"type": "string", "enum": record.Severities},
					"class": map[string]any{"type": "string", "enum": []string{record.ClassProblem, record.ClassObservation},
						"description": "problem: something wrong or in the way. observation: a positive or neutral note. Leave out for a problem."},
					"symptom": map[string]any{"type": "string", "enum": identity.Symptoms,
						"description": "What is wrong, in the closest of these words. " + symptomHelp()},
					"topic":   str("Only with symptom missing-information: what the missing content is about, at most four words, such as 'uk data residency'."),
					"rule":    str("Only with symptom a11y: the axe rule id, such as image-alt."),
					"control": str("The control the finding is about, as @ref from the results (for example @12). Leave out when it is about the page."),
					"title":   str("One sentence, who cannot do what"),
					"steps":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "minItems": 1},
					"evidence": map[string]any{"type": "string", "enum": []string{"measured", "inferred"},
						"description": "measured: you saw it in a step. inferred: your reasoning, labelled as such."},
				}, "kind", "severity", "symptom", "title", "steps", "evidence")},
		}, "verdict", "goal_met", "report", "findings")},
}

// Finish is what the finish tool receives.
type Finish struct {
	Verdict  string          `json:"verdict"`
	GoalMet  string          `json:"goal_met"`
	Report   string          `json:"report"`
	Findings []FinishFinding `json:"findings"`
	// Uncited holds the titles of findings that ParseFinish dropped because they cited no step in the run.
	Uncited []string `json:"-"`
}

// FinishFinding is one finding as the persona reported it.
type FinishFinding struct {
	Class    string `json:"class"`
	Symptom  string `json:"symptom"`
	Topic    string `json:"topic"`
	Rule     string `json:"rule"`
	Control  string `json:"control"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Steps    []int  `json:"steps"`
	Evidence string `json:"evidence"`
}

// symptomHelp lists the symptoms with their meaning, for the finish tool.
func symptomHelp() string {
	parts := make([]string, 0, len(identity.Symptoms))
	for _, name := range identity.Symptoms {
		parts = append(parts, name+": "+identity.SymptomMeaning[name])
	}
	return strings.Join(parts, "; ") + "."
}

// FullSymptom builds the symptom string from the persona's fields: the base value, or
// missing-information:<topic slug>, or a11y:<rule id>. The error says what to fix.
func (ff FinishFinding) FullSymptom() (string, error) {
	sym := ff.Symptom
	switch ff.Symptom {
	case "missing-information":
		slug := identity.Slug(ff.Topic)
		if slug == "" {
			return "", fmt.Errorf("finding %q: symptom missing-information needs a topic of one to four words", ff.Title)
		}
		sym += ":" + slug
	case "a11y":
		sym += ":" + strings.ToLower(strings.TrimSpace(ff.Rule))
	}
	if !identity.ValidSymptom(sym) {
		return "", fmt.Errorf("finding %q: symptom %q is not one of %s (a11y needs a rule id such as image-alt)",
			ff.Title, ff.Symptom, strings.Join(identity.Symptoms, ", "))
	}
	return sym, nil
}

// ParseFinish decodes the finish tool's arguments and repairs what it can:
// unknown enumerations fall back to the safest value and step numbers outside
// the run are dropped. A symptom that is not in the closed list is not repaired:
// Problems reports it so the runner can ask the model again.
func ParseFinish(args json.RawMessage, maxStep int) (Finish, error) {
	var f Finish
	if err := json.Unmarshal(args, &f); err != nil {
		return Finish{}, fmt.Errorf("finish arguments: %w", err)
	}
	if !contains([]string{"pass", "friction", "fail", "blocked"}, f.Verdict) {
		f.Verdict = "blocked"
	}
	if !contains([]string{"yes", "partly", "no"}, f.GoalMet) {
		f.GoalMet = map[string]string{"pass": "yes", "friction": "partly"}[f.Verdict]
		if f.GoalMet == "" {
			f.GoalMet = "no"
		}
	}
	kept := make([]FinishFinding, 0, len(f.Findings))
	for _, ff := range f.Findings {
		if !contains(record.Kinds, ff.Kind) {
			ff.Kind = "question"
		}
		if !contains(record.Severities, ff.Severity) {
			ff.Severity = "info"
		}
		if ff.Class != record.ClassObservation {
			ff.Class = record.ClassProblem
		}
		if ff.Evidence != "measured" {
			ff.Evidence = "inferred"
		}
		steps := make([]int, 0, len(ff.Steps))
		for _, s := range ff.Steps {
			if s >= 1 && s <= maxStep && !containsInt(steps, s) {
				steps = append(steps, s)
			}
		}
		if len(steps) == 0 || strings.TrimSpace(ff.Title) == "" {
			// A finding with no step to point at is not evidence. Say so, so a reviewer can see it was claimed.
			if t := strings.TrimSpace(ff.Title); t != "" {
				f.Uncited = append(f.Uncited, t)
			}
			continue
		}
		ff.Steps = steps
		ff.Title = strings.TrimSpace(ff.Title)
		kept = append(kept, ff)
	}
	f.Findings = kept
	return f, nil
}

// Problems lists what is wrong with the findings' symptoms, one message each.
func (f Finish) Problems() []string {
	var out []string
	for _, ff := range f.Findings {
		if _, err := ff.FullSymptom(); err != nil {
			out = append(out, err.Error())
		}
	}
	return out
}

// DropInvalid removes the findings whose symptom is not valid and returns how many it removed.
func (f *Finish) DropInvalid() int {
	kept := f.Findings[:0:0]
	for _, ff := range f.Findings {
		if _, err := ff.FullSymptom(); err == nil {
			kept = append(kept, ff)
		}
	}
	n := len(f.Findings) - len(kept)
	f.Findings = kept
	return n
}

// RecordFindings converts the persona's findings to run record findings with a
// pending verdict. The reviewer is a later stage. The engine, not the model, sets the
// route, the locus and the fingerprint. Findings with an invalid symptom are left out.
func RecordFindings(f Finish, loc *Locator) []record.Finding {
	out := make([]record.Finding, 0, len(f.Findings))
	seen := map[string]bool{}
	for _, ff := range f.Findings {
		symptom, err := ff.FullSymptom()
		if err != nil {
			continue
		}
		id := record.FindingID(ff.Kind, ff.Title)
		if seen[id] {
			continue
		}
		seen[id] = true
		route := loc.Route(ff.Steps)
		locus := loc.Locus(ff.Steps, ff.Control)
		fp, err := identity.Fingerprint(route, locus, symptom)
		if err != nil {
			continue
		}
		out = append(out, record.Finding{ID: id, Kind: ff.Kind, Severity: ff.Severity, Title: ff.Title,
			Steps: ff.Steps, Evidence: ff.Evidence, Verdict: record.VerdictPending, Class: ff.Class,
			Route: route, Locus: locus, Symptom: symptom, TaxonomyVersion: identity.TaxonomyVersion, Fingerprint: fp})
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// toolNames lists the tools in the order the persona is shown them.
func toolNames() []string {
	names := make([]string, len(Tools))
	for i, t := range Tools {
		names[i] = t.Name
	}
	return names
}
