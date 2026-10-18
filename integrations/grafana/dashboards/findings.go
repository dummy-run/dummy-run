package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/table"
)

// Findings puts confirmed permission findings first, then blockers, then every distinct finding.
func (c Config) Findings() *dashboard.DashboardBuilder {
	ids := &ids{}
	b := c.base(uidFindings, "Dummy Run: findings",
		"Confirmed permission findings first, then blockers, then every distinct finding with when it was seen and who hit it. Everything here comes from the finding log lines.")

	personaOnly := lokiFindSel

	b.WithRow(dashboard.NewRowBuilder("Permission findings").Id(ids.id()))
	b.WithPanel(c.lokiStat(ids.id(), "Permission findings",
		"A role saw or did something beyond its rights, and the reviewer confirmed it.",
		countOver(lokiFindings+personaOnly+` | dummyrun_finding_kind="permission" | dummyrun_finding_verdict="confirmed"`, "$__range"),
		"short", 0, thresholds(colourPass, 1.0, colourBlocker), 6, 7))
	b.WithPanel(c.findingsSurfaced(ids.id(), "Confirmed permission findings",
		"Distinct confirmed permission findings in the time range, with the number of runs that hit each one.",
		lokiFindings+personaOnly+` | dummyrun_finding_kind="permission" | dummyrun_finding_verdict="confirmed"`, 18, 7))

	b.WithRow(dashboard.NewRowBuilder("Blockers").Id(ids.id()))
	b.WithPanel(c.lokiStat(ids.id(), "Confirmed blockers",
		"Confirmed findings with blocker severity, of any kind.",
		countOver(lokiFindings+personaOnly+` | dummyrun_finding_severity="blocker" | dummyrun_finding_verdict="confirmed"`, "$__range"),
		"short", 0, thresholds(colourPass, 1.0, colourBlocker), 6, 7))
	b.WithPanel(c.findingsSurfaced(ids.id(), "Confirmed blockers",
		"Distinct confirmed blockers in the time range, with the number of runs that hit each one.",
		lokiFindings+personaOnly+` | dummyrun_finding_severity="blocker" | dummyrun_finding_verdict="confirmed"`, 18, 7))

	b.WithRow(dashboard.NewRowBuilder("All findings").Id(ids.id()))
	b.WithPanel(c.distinctFindings(ids.id(), lokiFindings+personaOnly, 24, 14))
	b.WithPanel(c.findingsOverTime(ids.id(), "Confirmed findings by kind over time", "Confirmed findings in each interval, stacked by kind.",
		"dummyrun_finding_kind", nil, nil, 12, 8))
	b.WithPanel(c.findingsOverTime(ids.id(), "Confirmed findings by severity over time", "Confirmed findings in each interval, stacked by severity.",
		"dummyrun_finding_severity", severityColours, severityOrder, 12, 8))

	b.WithRow(dashboard.NewRowBuilder("Finding logs").Id(ids.id()))
	b.WithPanel(logs.NewPanelBuilder().
		Id(ids.id()).
		Title("Finding logs").
		Description("One line per finding per run. Open a line to see its attributes and the link to the run's trace.").
		Datasource(c.loki()).
		Span(24).Height(14).
		WithTarget(lokiLogs(c.loki(), lokiFindings+personaOnly, 500)).
		ShowTime(true).
		ShowLabels(false).
		ShowCommonLabels(false).
		WrapLogMessage(true).
		EnableLogDetails(true).
		SortOrder(common.LogsSortOrderDescending).
		DedupStrategy(common.LogsDedupStrategyNone))
	return b
}

// distinctFindings groups finding log lines by finding ID with first seen, last seen, run count and personas.
func (c Config) distinctFindings(id uint32, expr string, w, h uint32) *table.PanelBuilder {
	group := map[string]any{
		"dummyrun_finding_id":       map[string]any{"operation": "groupby"},
		"Time":                      map[string]any{"operation": "aggregate", "aggregations": []string{"min", "max"}},
		"dummyrun_run_id":           map[string]any{"operation": "aggregate", "aggregations": []string{"distinctCount"}},
		"dummyrun_persona_id":       map[string]any{"operation": "aggregate", "aggregations": []string{"uniqueValues"}},
		"dummyrun_finding_kind":     map[string]any{"operation": "aggregate", "aggregations": []string{"lastNotNull"}},
		"dummyrun_finding_severity": map[string]any{"operation": "aggregate", "aggregations": []string{"lastNotNull"}},
		"dummyrun_finding_verdict":  map[string]any{"operation": "aggregate", "aggregations": []string{"uniqueValues"}},
		"dummyrun_finding_title":    map[string]any{"operation": "aggregate", "aggregations": []string{"lastNotNull"}},
	}
	return table.NewPanelBuilder().
		Id(id).
		Title("Distinct findings").
		Description("Every finding ID in the time range, confirmed or not. First seen and last seen are the times of the runs that hit it.").
		Datasource(c.loki()).
		Span(w).Height(h).
		WithTarget(lokiLogs(c.loki(), expr, 5000)).
		WithTransformation(extractLabels()).
		WithTransformation(transform("groupBy", map[string]any{"fields": group})).
		WithTransformation(organize(
			"dummyrun_finding_id", "Finding",
			"dummyrun_finding_severity (lastNotNull)", "Severity",
			"dummyrun_finding_kind (lastNotNull)", "Kind",
			"dummyrun_finding_title (lastNotNull)", "Title",
			"Time (min)", "First seen",
			"Time (max)", "Last seen",
			"dummyrun_run_id (distinctCount)", "Runs",
			"dummyrun_persona_id (uniqueValues)", "Personas",
			"dummyrun_finding_verdict (uniqueValues)", "Verdicts",
		)).
		WithTransformation(sortBy("Runs", true)).
		CellHeight(common.TableCellHeightSm).
		OverrideByName("Finding", []dashboard.DynamicConfigValue{
			fieldLink("Show these findings in Explore", exploreLokiURL(c.LokiUID, lokiFindings+` | dummyrun_finding_id="__DR_VALUE__"`)),
			{Id: "custom.width", Value: 180},
			wrap("auto"),
		}).
		OverrideByName("Severity", []dashboard.DynamicConfigValue{
			{Id: "mappings", Value: valueMappings(severityColours, severityOrder)},
			{Id: "custom.cellOptions", Value: map[string]any{"type": "color-text"}},
			{Id: "custom.width", Value: 80},
		}).
		OverrideByName("Kind", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 100}}).
		OverrideByName("Title", []dashboard.DynamicConfigValue{wrap("auto")}).
		OverrideByName("Runs", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 60}}).
		OverrideByName("Personas", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 130}, wrap("auto")}).
		OverrideByName("Verdicts", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 130}, wrap("auto")}).
		OverrideByName("First seen", []dashboard.DynamicConfigValue{{Id: "unit", Value: "dateTimeAsIso"}, {Id: "custom.width", Value: 150}}).
		OverrideByName("Last seen", []dashboard.DynamicConfigValue{{Id: "unit", Value: "dateTimeAsIso"}, {Id: "custom.width", Value: 150}})
}
