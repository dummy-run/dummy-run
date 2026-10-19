package runner

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dummy-run/dummy-run/internal/bidi"
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
			"report": str("Your report in Markdown: what you tried, what happened, what you would tell the team. Cite steps as 'step 4' or 'steps 6-8'."),
			"findings": map[string]any{"type": "array", "description": "Each distinct problem or question, with evidence.",
				"items": object(map[string]any{
					"kind":     map[string]any{"type": "string", "enum": record.Kinds},
					"severity": map[string]any{"type": "string", "enum": record.Severities},
					"title":    str("One sentence, who cannot do what"),
					"steps":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "minItems": 1},
					"evidence": map[string]any{"type": "string", "enum": []string{"measured", "inferred"},
						"description": "measured: you saw it in a step. inferred: your reasoning, labelled as such."},
				}, "kind", "severity", "title", "steps", "evidence")},
		}, "verdict", "report", "findings")},
}

// Finish is what the finish tool receives.
type Finish struct {
	Verdict  string          `json:"verdict"`
	Report   string          `json:"report"`
	Findings []FinishFinding `json:"findings"`
}

// FinishFinding is one finding as the persona reported it.
type FinishFinding struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Steps    []int  `json:"steps"`
	Evidence string `json:"evidence"`
}

// ParseFinish decodes the finish tool's arguments and repairs what it can:
// unknown enumerations fall back to the safest value and step numbers outside
// the run are dropped.
func ParseFinish(args json.RawMessage, maxStep int) (Finish, error) {
	var f Finish
	if err := json.Unmarshal(args, &f); err != nil {
		return Finish{}, fmt.Errorf("finish arguments: %w", err)
	}
	if !contains([]string{"pass", "friction", "fail", "blocked"}, f.Verdict) {
		f.Verdict = "blocked"
	}
	kept := make([]FinishFinding, 0, len(f.Findings))
	for _, ff := range f.Findings {
		if !contains(record.Kinds, ff.Kind) {
			ff.Kind = "question"
		}
		if !contains(record.Severities, ff.Severity) {
			ff.Severity = "info"
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
			continue
		}
		ff.Steps = steps
		ff.Title = strings.TrimSpace(ff.Title)
		kept = append(kept, ff)
	}
	f.Findings = kept
	return f, nil
}

// RecordFindings converts the persona's findings to run record findings with a
// pending verdict. The reviewer is a later stage.
func RecordFindings(f Finish) []record.Finding {
	out := make([]record.Finding, 0, len(f.Findings))
	seen := map[string]bool{}
	for _, ff := range f.Findings {
		id := record.FindingID(ff.Kind, ff.Title)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, record.Finding{ID: id, Kind: ff.Kind, Severity: ff.Severity, Title: ff.Title,
			Steps: ff.Steps, Evidence: ff.Evidence, Verdict: record.VerdictPending})
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
