package report

import (
	"encoding/json"
	"io"
	"sort"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string         `json:"id"`
	ShortDescription sarifText      `json:"shortDescription"`
	HelpURI          string         `json:"helpUri,omitempty"`
	Properties       map[string]any `json:"properties,omitempty"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             sarifText         `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
	Properties          map[string]any    `json:"properties,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           sarifRegion   `json:"region"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

func sarifLevel(sev string) string {
	switch sev {
	case "blocker", "major", "critical", "serious":
		return "error"
	case "minor", "moderate":
		return "warning"
	}
	return "note"
}

func joinURL(base, route string) string {
	if base == "" {
		return route
	}
	if route == "" {
		return base
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	if route[0] != '/' {
		route = "/" + route
	}
	return base + route
}

// renderSARIF writes SARIF 2.1.0. Each distinct problem is one result and each axe element is one
// result. The locations are page addresses with line 1, since a sampled problem has no source line.
func renderSARIF(w io.Writer, rep *Report) error {
	base := ""
	if len(rep.Scope.Targets) == 1 {
		base = rep.Scope.Targets[0].BaseURL
	}
	rules := map[string]sarifRule{}
	results := []sarifResult{}
	for _, f := range rep.Findings {
		id := "dummyrun/" + orStr(f.Symptom, f.Kind)
		if _, ok := rules[id]; !ok {
			r := sarifRule{ID: id, ShortDescription: sarifText{Text: orStr(f.Symptom, f.Kind)}, HelpURI: f.HelpURL}
			rules[id] = r
		}
		msg := f.Title
		if f.Status != StatusConfirmed {
			msg += " (not reviewed)"
		}
		props := map[string]any{"severity": f.Severity, "status": f.Status, "kind": f.Kind, "runs": f.Runs}
		if len(f.WCAG) > 0 {
			props["wcag"] = f.WCAG
		}
		if f.Baseline != "" {
			props["baseline"] = f.Baseline
		}
		res := sarifResult{RuleID: id, Level: sarifLevel(f.Severity), Message: sarifText{Text: msg},
			Locations: []sarifLocation{loc(joinURL(base, f.Route))}, Properties: props}
		if f.Fingerprint != "" {
			res.PartialFingerprints = map[string]string{"dummyrun/v1": f.Fingerprint}
		}
		results = append(results, res)
	}
	if rep.Axe != nil {
		for _, r := range rep.Axe.Rules {
			id := "axe/" + r.Rule
			if _, ok := rules[id]; !ok {
				rules[id] = sarifRule{ID: id, ShortDescription: sarifText{Text: r.Help}, HelpURI: r.HelpURL}
			}
			for _, pg := range r.Pages {
				for _, t := range pg.Targets {
					props := map[string]any{"impact": r.Impact, "element": t}
					if len(r.WCAG) > 0 {
						props["wcag"] = r.WCAG
					}
					res := sarifResult{RuleID: id, Level: sarifLevel(r.Impact), Message: sarifText{Text: r.Help + ": " + t},
						Locations: []sarifLocation{loc(pg.URL)}, Properties: props,
						PartialFingerprints: map[string]string{"dummyrun/v1": r.Fingerprint + "/" + shortHash(pg.URL+"|"+t)}}
					results = append(results, res)
				}
			}
		}
	}
	ids := make([]string, 0, len(rules))
	for id := range rules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := make([]sarifRule, 0, len(ids))
	for _, id := range ids {
		list = append(list, rules[id])
	}
	log := sarifLog{Schema: "https://json.schemastore.org/sarif-2.1.0.json", Version: "2.1.0", Runs: []sarifRun{{
		Tool:    sarifTool{Driver: sarifDriver{Name: "dummyrun", Version: rep.ToolVersion, InformationURI: "https://dummyrun.dev", Rules: list}},
		Results: results,
	}}}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

func loc(uri string) sarifLocation {
	return sarifLocation{PhysicalLocation: sarifPhysical{ArtifactLocation: sarifArtifact{URI: uri}, Region: sarifRegion{StartLine: 1}}}
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
