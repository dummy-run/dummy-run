package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/tempo"
)

// Run shows one run picked by its ID: the trace, its steps, its findings and its logs.
func (c Config) Run() *dashboard.DashboardBuilder {
	ids := &ids{}
	b := c.base(uidRun, "Dummy Run: run",
		"One run: its trace, steps, findings and logs. Pick the run at the top or click one in the table.")

	// The run picker reads span attribute values from Tempo for the selected time range.
	runQuery := map[string]any{"type": 1, "label": "dummyrun.run.id", "refId": "TempoDatasourceVariableQueryEditor-VariableQuery"}
	b.WithVariable(dashboard.NewQueryVariableBuilder("run").
		Label("Run").
		Datasource(c.tempo()).
		Query(dashboard.StringOrMap{Map: runQuery}).
		Definition("dummyrun.run.id").
		Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
		Sort(dashboard.VariableSortAlphabeticalDesc).
		Multi(false).
		IncludeAll(false))

	b.WithRow(dashboard.NewRowBuilder("Pick a run").Id(ids.id()))
	b.WithPanel(c.runsTable(ids.id(), "Runs matching the filters",
		"Runs for the selected environment, persona and scenario. Click a run ID to show it below.",
		lokiRunEnd+lokiRunSel, 24, 7))

	b.WithRow(dashboard.NewRowBuilder("Run $run").Id(ids.id()))
	b.WithPanel(c.attributesTable(ids.id(), "Run", "From the run end log line.",
		lokiRunEnd+` | dummyrun_run_id="$run"`, 24, 4,
		"dummyrun_outcome", "Outcome",
		"dummyrun_persona_id", "Persona",
		"dummyrun_persona_role", "Role",
		"dummyrun_persona_device", "Device",
		"dummyrun_scenario_id", "Scenario",
		"dummyrun_target_environment", "Environment",
		"browser_name", "Browser",
		"gen_ai_request_model", "Model",
	).OverrideByName("Outcome", []dashboard.DynamicConfigValue{
		{Id: "mappings", Value: valueMappings(outcomeColours, outcomeOrder)},
		{Id: "custom.cellOptions", Value: map[string]any{"type": "color-text"}},
		{Id: "custom.width", Value: 130},
	}).
		OverrideByName("Role", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 100}}).
		OverrideByName("Device", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 90}}).
		OverrideByName("Environment", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 110}}).
		OverrideByName("Browser", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 90}}))

	// Grafana's traces panel needs a trace ID, and the contract keys runs by run ID. So this is a
	// search result with a link to the trace view.
	b.WithPanel(table.NewPanelBuilder().
		Id(ids.id()).
		Title("Trace").
		Description("The run's trace in Tempo. Click the trace ID to open the trace view, where the root span is the run and each child span is a step.").
		Datasource(c.tempo()).
		Span(24).Height(4).
		WithTarget(tempoSearch(c.tempo(), `{ span.dummyrun.run.id = "$run" }`, tempo.SearchTableTypeTraces, 1, 1)).
		WithTransformation(organize(
			"Trace ID", "Trace ID",
			"Start time", "Started",
			"Duration", "Duration",
			"Service", "Service",
		)).
		CellHeight(common.TableCellHeightSm).
		OverrideByName("Trace ID", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 320}}).
		OverrideByName("Started", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 200}}).
		OverrideByName("Duration", []dashboard.DynamicConfigValue{{Id: "unit", Value: "ms"}, {Id: "custom.width", Value: 120}}))

	b.WithPanel(table.NewPanelBuilder().
		Id(ids.id()).
		Title("Steps").
		Description("The step spans of the run, from Tempo. Errors is the number of console errors and A11y the number of accessibility violations.").
		Datasource(c.tempo()).
		Span(14).Height(12).
		WithTarget(tempoSearch(c.tempo(),
			`{ span.dummyrun.run.id = "$run" } > { span.dummyrun.step.n != nil } | select(span.dummyrun.step.n, span.dummyrun.step.action, span.dummyrun.step.target, span.http.response.status_code, span.dummyrun.step.console_errors, span.dummyrun.step.a11y_violations)`,
			tempo.SearchTableTypeSpans, 1, 100)).
		WithTransformation(organize(
			"dummyrun.step.n", "Step",
			"dummyrun.step.action", "Action",
			"dummyrun.step.target", "Target",
			"Duration", "Duration",
			"http.response.status_code", "Status",
			"dummyrun.step.console_errors", "Errors",
			"dummyrun.step.a11y_violations", "A11y",
			"Start time", "Started",
		)).
		WithTransformation(sortBy("Step", false)).
		CellHeight(common.TableCellHeightSm).
		OverrideByName("Step", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 55}}).
		OverrideByName("Action", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 75}}).
		OverrideByName("Target", []dashboard.DynamicConfigValue{wrap("auto")}).
		OverrideByName("Duration", []dashboard.DynamicConfigValue{{Id: "unit", Value: "ns"}, {Id: "custom.width", Value: 85}}).
		OverrideByName("Status", []dashboard.DynamicConfigValue{
			{Id: "thresholds", Value: map[string]any{"mode": "absolute", "steps": []map[string]any{{"color": colourInk, "value": nil}, {"color": colourBlocker, "value": 500}}}},
			{Id: "custom.cellOptions", Value: map[string]any{"type": "color-text"}},
			{Id: "custom.width", Value: 70},
		}).
		OverrideByName("Errors", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 65}}).
		OverrideByName("A11y", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 55}}).
		OverrideByName("Started", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 170}}))

	b.WithPanel(table.NewPanelBuilder().
		Id(ids.id()).
		Title("Findings").
		Description("Findings raised in this run, confirmed or not.").
		Datasource(c.loki()).
		Span(10).Height(12).
		WithTarget(lokiLogs(c.loki(), lokiFindings+` | dummyrun_run_id="$run"`, 100)).
		WithTransformation(extractLabels()).
		WithTransformation(organize(
			"dummyrun_finding_severity", "Severity",
			"dummyrun_finding_kind", "Kind",
			"dummyrun_finding_verdict", "Verdict",
			"dummyrun_finding_steps", "Steps",
			"dummyrun_finding_title", "Title",
		)).
		CellHeight(common.TableCellHeightSm).
		OverrideByName("Severity", []dashboard.DynamicConfigValue{
			{Id: "mappings", Value: valueMappings(severityColours, severityOrder)},
			{Id: "custom.cellOptions", Value: map[string]any{"type": "color-text"}},
			{Id: "custom.width", Value: 90},
		}).
		OverrideByName("Kind", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 95}}).
		OverrideByName("Verdict", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 90}}).
		OverrideByName("Steps", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 55}}).
		OverrideByName("Title", []dashboard.DynamicConfigValue{wrap("auto")}))

	b.WithPanel(logs.NewPanelBuilder().
		Id(ids.id()).
		Title("Logs").
		Description("Every log line for this run: the run end line and one line per finding.").
		Datasource(c.loki()).
		Span(24).Height(10).
		WithTarget(lokiLogs(c.loki(), lokiStream+` | dummyrun_run_id="$run"`, 200)).
		ShowTime(true).
		ShowLabels(false).
		ShowCommonLabels(false).
		WrapLogMessage(true).
		EnableLogDetails(true).
		SortOrder(common.LogsSortOrderDescending).
		DedupStrategy(common.LogsDedupStrategyNone))
	return b
}
