package report

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PlanSection is "What we tested and why": the kind of site, its key journeys and why each persona was sent.
// It comes from the plan.json that dummyrun plan writes.
type PlanSection struct {
	SiteType string        `json:"site_type"`
	Site     string        `json:"site"`
	Audience string        `json:"audience,omitempty"`
	Summary  string        `json:"summary,omitempty"`
	Journeys []PlanJourney `json:"journeys"`
	Personas []PlanPersona `json:"personas"`
	// Intro is the opening sentence, ready to print.
	Intro string `json:"intro"`
}

// PlanJourney is one thing the site's visitors come to do.
type PlanJourney struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Why  string `json:"why,omitempty"`
}

// PlanPersona is one persona that was sent, and why.
type PlanPersona struct {
	ID       string   `json:"id"`
	Name     string   `json:"name,omitempty"`
	Label    string   `json:"label,omitempty"`
	Why      string   `json:"why,omitempty"`
	Journeys []string `json:"journeys"`
	// Text is the sentence about this persona, ready to print.
	Text string `json:"text"`
}

// PlanUsage is what planning cost. It is added to the report's cost.
type PlanUsage struct {
	ModelCalls   int      `json:"model_calls"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd"`
}

var planSiteWords = map[string]string{
	"personal": "personal site", "blog": "blog", "shop": "shop", "saas": "software product site", "docs": "documentation site",
	"public-service": "public service", "organisation": "organisation's site", "charity": "charity's site", "news": "news site",
	"local-business": "local business site", "education": "school or university site", "open-source": "open-source project site",
}

// LoadPlan reads a plan.json written by dummyrun plan.
func LoadPlan(path string) (*PlanSection, PlanUsage, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the path comes from the command line on purpose
	if err != nil {
		return nil, PlanUsage{}, err
	}
	var raw struct {
		Version  int    `json:"plan_version"`
		SiteType string `json:"site_type"`
		Audience string `json:"audience"`
		Summary  string `json:"summary"`
		Journeys []struct {
			ID, Name, Why string
		} `json:"journeys"`
		Personas []struct {
			ID       string   `json:"id"`
			Name     string   `json:"name"`
			Label    string   `json:"label"`
			Why      string   `json:"why"`
			Journeys []string `json:"journeys"`
		} `json:"personas"`
		Usage PlanUsage `json:"usage"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, PlanUsage{}, fmt.Errorf("%s: %w", path, err)
	}
	if raw.Version != 1 {
		return nil, PlanUsage{}, fmt.Errorf("%s: not a plan.json from dummyrun plan (plan_version %d)", path, raw.Version)
	}
	// The lists are never nil, so the JSON report has arrays where the schema says arrays.
	ps := &PlanSection{SiteType: raw.SiteType, Audience: raw.Audience, Summary: raw.Summary, Journeys: []PlanJourney{}, Personas: []PlanPersona{}}
	for _, j := range raw.Journeys {
		ps.Journeys = append(ps.Journeys, PlanJourney{ID: j.ID, Name: j.Name, Why: j.Why})
	}
	for _, p := range raw.Personas {
		if p.Journeys == nil {
			p.Journeys = []string{}
		}
		ps.Personas = append(ps.Personas, PlanPersona{ID: p.ID, Name: p.Name, Label: p.Label, Why: p.Why, Journeys: p.Journeys})
	}
	ps.write()
	return ps, raw.Usage, nil
}

// write fills in the sentences the templates print.
func (ps *PlanSection) write() {
	ps.Site = planSiteWords[ps.SiteType]
	if ps.Site == "" {
		ps.Site = "website"
	}
	ps.Intro = "Your site looks like " + article(ps.Site) + " " + ps.Site
	if ps.Audience != "" {
		ps.Intro += " for " + strings.TrimSuffix(ps.Audience, ".")
	}
	ps.Intro += ", so we tried the things its visitors come to do."
	names := map[string]string{}
	for _, j := range ps.Journeys {
		names[j.ID] = lowerFirst(j.Name)
	}
	for i, p := range ps.Personas {
		who := p.Name
		if p.Label != "" {
			if who != "" {
				who += ", " + article(p.Label) + " " + lowerFirst(p.Label) + ","
			} else {
				who = article(p.Label) + " " + lowerFirst(p.Label)
			}
		}
		if who == "" {
			who = p.ID
		}
		var tried []string
		for _, id := range p.Journeys {
			if n := names[id]; n != "" {
				tried = append(tried, n)
			}
		}
		text := "We sent " + who
		if len(tried) > 0 {
			text += " to " + joinAnd(tried)
		}
		text += "."
		if p.Why != "" {
			text += " " + strings.TrimSpace(p.Why)
			if !strings.HasSuffix(text, ".") {
				text += "."
			}
		}
		ps.Personas[i].Text = text
	}
}

func article(s string) string {
	if s == "" {
		return "a"
	}
	switch strings.ToLower(s[:1]) {
	case "a", "e", "i", "o", "u":
		return "an"
	}
	return "a"
}

// lowerFirst lowers the first letter unless the first word looks like a name or an acronym, that is, unless an
// upper-case letter is followed by a lower-case one. It works on runes, so a name in any script is left whole.
func lowerFirst(s string) string {
	first, n := utf8.DecodeRuneInString(s)
	second, _ := utf8.DecodeRuneInString(s[n:])
	if first == utf8.RuneError || !unicode.IsUpper(first) || !unicode.IsLower(second) {
		return s
	}
	return string(unicode.ToLower(first)) + s[n:]
}

func joinAnd(l []string) string {
	switch len(l) {
	case 0:
		return ""
	case 1:
		return l[0]
	}
	return strings.Join(l[:len(l)-1], ", ") + " and " + l[len(l)-1]
}

// addPlanCost adds what planning cost to the report's cost. A planning call whose model has no price counts
// as unpriced, like a run would, so the report says the total is a floor. A fallback plan made no call and
// adds nothing.
func addPlanCost(c *Cost, u PlanUsage) {
	c.ModelCalls += u.ModelCalls
	c.InputTokens += u.InputTokens
	c.OutputTokens += u.OutputTokens
	switch {
	case u.CostUSD != nil:
		c.USD += *u.CostUSD
	case u.ModelCalls > 0:
		c.UnpricedRuns++
	}
}
