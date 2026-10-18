package main

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/table"
)

// Personas shows one row per persona: its runs, outcomes, findings, slowest steps and traces.
func (c Config) Personas() *dashboard.DashboardBuilder {
	ids := &ids{}
	prom := c.prom()
	b := c.base(uidPersonas, "Dummy Run: personas",
		"One row per persona. Each row shows the persona's runs, what it found, where it was slow and its traces.")

	// Inside a repeated row, $persona holds one value.
	one := `dummyrun_persona_id=~"$persona"`
	runs := lokiRunEnd + ` | dummyrun_target_environment=~"$environment" | ` + one + ` | dummyrun_scenario_id=~"$scenario"`
	findings := lokiFindings + ` | dummyrun_target_environment=~"$environment" | ` + one + ` | dummyrun_scenario_id=~"$scenario"`

	b.WithRow(dashboard.NewRowBuilder("$persona").Id(ids.id()).Repeat("persona"))
	b.WithPanel(c.lokiStat(ids.id(), "Runs", "Runs by this persona that ended in the time range.",
		countOver(runs, "$__range"), "short", 0, thresholds(colourInk), 4, 4))
	b.WithPanel(c.lokiStat(ids.id(), "Pass rate", "Share of this persona's runs where it finished its job.",
		passRate(runs, "$__range"), "percentunit", 0, thresholds(colourBlocker, 0.8, colourAmber, 0.9, colourPass), 4, 4))
	b.WithPanel(c.lokiStat(ids.id(), "Findings", "Findings this persona surfaced that the reviewer confirmed.",
		countOver(findings+` | dummyrun_finding_verdict="confirmed"`, "$__range"), "short", 0, thresholds(colourInk, 1.0, colourAmber), 4, 4))
	b.WithPanel(c.lokiStat(ids.id(), "Blockers", "Confirmed blocker findings from this persona.",
		countOver(findings+` | dummyrun_finding_severity="blocker" | dummyrun_finding_verdict="confirmed"`, "$__range"),
		"short", 0, thresholds(colourPass, 1.0, colourBlocker), 4, 4))
	b.WithPanel(c.attributesTable(ids.id(), "Role, device, browser and model", "From the persona's latest run end log line.",
		lokiRunEnd+` | `+one, 8, 4,
		"dummyrun_persona_role", "Role",
		"dummyrun_persona_device", "Device",
		"browser_name", "Browser",
		"gen_ai_request_model", "Model",
	).
		OverrideByName("Role", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 95}}).
		OverrideByName("Device", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 75}}).
		OverrideByName("Browser", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 75}}).
		OverrideByName("Model", []dashboard.DynamicConfigValue{wrap("auto")}))

	b.WithPanel(c.outcomesOverTime(ids.id(), "Outcomes", "This persona's runs that ended in each interval, stacked by outcome.", runs, 9, 8))
	b.WithPanel(c.findingsSurfaced(ids.id(), "Findings surfaced",
		"Confirmed findings from this persona in the time range, with the number of runs that hit each one.",
		findings+` | dummyrun_finding_verdict="confirmed"`, 9, 8))
	b.WithPanel(slowestActions(prom, ids.id(), one, 6, 8))

	b.WithPanel(c.runsTable(ids.id(), "Recent runs",
		"This persona's recent runs. Click a run ID to open the run dashboard, or the trace ID to open the trace.",
		runs, 24, 8))
	return b
}

// findingsSurfaced lists distinct findings from finding log lines, counted per run.
func (c Config) findingsSurfaced(id uint32, title, description, selector string, w, h uint32) *table.PanelBuilder {
	expr := fmt.Sprintf(`sort_desc(sum by (dummyrun_finding_id, dummyrun_finding_kind, dummyrun_finding_severity, dummyrun_finding_title) (count_over_time(%s [$__range])))`, selector)
	return table.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		Datasource(c.loki()).
		Span(w).Height(h).
		WithTarget(lokiInstant(c.loki(), expr)).
		WithTransformation(transform("labelsToFields", map[string]any{"mode": "columns"})).
		WithTransformation(transform("merge", map[string]any{})).
		WithTransformation(organize(
			"dummyrun_finding_id", "Finding",
			"dummyrun_finding_severity", "Severity",
			"Value", "Runs",
			"dummyrun_finding_title", "Title",
		)).
		WithTransformation(sortBy("Runs", true)).
		CellHeight(common.TableCellHeightSm).
		OverrideByName("Finding", []dashboard.DynamicConfigValue{
			fieldLink("Show these findings in Explore", exploreLokiURL(c.LokiUID, lokiFindings+` | dummyrun_finding_id="__DR_VALUE__"`)),
			{Id: "custom.width", Value: 170},
			wrap("auto"),
		}).
		OverrideByName("Severity", []dashboard.DynamicConfigValue{
			{Id: "mappings", Value: valueMappings(severityColours, severityOrder)},
			{Id: "custom.cellOptions", Value: map[string]any{"type": "color-text"}},
			{Id: "custom.width", Value: 75},
		}).
		OverrideByName("Title", []dashboard.DynamicConfigValue{wrap("auto")}).
		OverrideByName("Runs", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 55}, {Id: "decimals", Value: 0}})
}
