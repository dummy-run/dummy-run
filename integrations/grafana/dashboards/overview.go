package main

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Overview shows runs, findings, step timing and model cost for the selected filters. Totals and
// series come from the log records, because metric data points carry the export time and each
// exporter process starts its counters at zero (docs/TELEMETRY.md, Metrics). Prometheus serves the
// histogram panels. Those read each series' highest value in the window, because a one-shot export
// writes one sample per series and increase() needs two.
func (c Config) Overview() *dashboard.DashboardBuilder {
	ids := &ids{}
	prom := c.prom()
	b := c.base(uidOverview, "Dummy Run: overview",
		"Runs by outcome, pass rate, confirmed findings, step timing and model cost. Pick an environment, persona or scenario at the top.")

	runs := lokiRunEnd + lokiFilters
	findings := lokiFindings + lokiFilters

	b.WithRow(dashboard.NewRowBuilder("Runs").Id(ids.id()))
	b.WithPanel(c.lokiStat(ids.id(), "Runs", "Runs that ended in the time range, counted from run end log lines.",
		countOver(runs, "$__range"), "short", 0, thresholds(colourInk), 4, 4))
	b.WithPanel(c.lokiStat(ids.id(), "Pass rate", "Share of runs where the persona finished its job.",
		passRate(runs, "$__range"), "percentunit", 0, thresholds(colourBlocker, 0.8, colourAmber, 0.9, colourPass), 4, 4))
	b.WithPanel(c.lokiStat(ids.id(), "Findings", "Findings the reviewer confirmed, counted from finding log lines.",
		countOver(findings+` | dummyrun_finding_verdict="confirmed"`, "$__range"), "short", 0, thresholds(colourInk, 1.0, colourAmber), 4, 4))
	b.WithPanel(c.lokiStat(ids.id(), "Blockers", "Confirmed findings with blocker severity.",
		countOver(findings+` | dummyrun_finding_severity="blocker" | dummyrun_finding_verdict="confirmed"`, "$__range"),
		"short", 0, thresholds(colourPass, 1.0, colourBlocker), 4, 4))
	b.WithPanel(c.lokiStat(ids.id(), "Tokens", "Model tokens in and out, summed from run end log lines.",
		tokensOver(runs, "$__range"), "short", 1, thresholds(colourInk), 4, 4))
	b.WithPanel(c.lokiStat(ids.id(), "Cost", "Model cost in US dollars, summed from run end log lines.",
		sumOver(runs, "dummyrun_cost_usd", "$__range"), "currencyUSD", 2, thresholds(colourInk), 4, 4))

	b.WithPanel(c.outcomesOverTime(ids.id(), "Runs by outcome", "Runs that ended in each interval, stacked by outcome.", runs, 16, 9))
	b.WithPanel(c.lokiSeries(ids.id(), "Pass rate", "Completed runs divided by all runs that ended in each interval.", 8, 9,
		series{passRate(runs, "$__interval"), "pass rate"}).
		Unit("percentunit").
		Decimals(0).
		Min(0).Max(1).
		ColorScheme(fixedColour(colourPass)).
		DrawStyle(common.GraphDrawStyleLine).
		FillOpacity(15).
		LineWidth(2).
		SpanNulls(common.BoolOrFloat64{Bool: ptr(true)}).
		Thresholds(thresholds(colourBlocker, 0.9, colourPass)).
		ThresholdsStyle(common.NewGraphThresholdsStyleConfigBuilder().Mode(common.GraphThresholdsStyleModeDashed)).
		Legend(common.NewVizLegendOptionsBuilder().ShowLegend(false)))

	b.WithRow(dashboard.NewRowBuilder("Findings").Id(ids.id()))
	b.WithPanel(c.findingsBarGauge(ids.id(), "Confirmed findings by kind", "Confirmed findings in the time range, by kind.",
		"dummyrun_finding_kind", nil, nil, 8, 8))
	b.WithPanel(c.findingsBarGauge(ids.id(), "Confirmed findings by severity", "Confirmed findings in the time range, by severity.",
		"dummyrun_finding_severity", severityColours, severityOrder, 8, 8))
	b.WithPanel(c.findingsOverTime(ids.id(), "Confirmed findings by severity over time", "Confirmed findings in each interval, stacked by severity.",
		"dummyrun_finding_severity", severityColours, severityOrder, 8, 8))

	b.WithRow(dashboard.NewRowBuilder("Steps").Id(ids.id()))
	b.WithPanel(stepPercentiles(prom, ids.id(), selPersona, 12, 8))
	b.WithPanel(slowestActions(prom, ids.id(), selPersona, 12, 8))

	b.WithRow(dashboard.NewRowBuilder("Model usage").Id(ids.id()))
	b.WithPanel(c.lokiSeries(ids.id(), "Tokens by model", "Input and output tokens of the runs that ended in each interval, by model.", 12, 8,
		series{sumBy(runs, "gen_ai_request_model", "gen_ai_usage_input_tokens", "$__interval"), "{{gen_ai_request_model}} input"},
		series{sumBy(runs, "gen_ai_request_model", "gen_ai_usage_output_tokens", "$__interval"), "{{gen_ai_request_model}} output"},
	).Unit("short").Decimals(0))
	b.WithPanel(c.lokiSeries(ids.id(), "Cost by model", "Model cost in US dollars of the runs that ended in each interval, by model.", 12, 8,
		series{sumBy(runs, "gen_ai_request_model", "dummyrun_cost_usd", "$__interval"), "{{gen_ai_request_model}}"},
	).Unit("currencyUSD").Decimals(2))

	b.WithRow(dashboard.NewRowBuilder("Latest runs").Id(ids.id()))
	b.WithPanel(c.runsTable(ids.id(), "Latest runs",
		"Run end log lines from Loki. Click a run ID to open the run dashboard, or the trace ID to open the trace.",
		runs, 24, 12))
	return b
}

// LogQL builders. A missing series becomes zero, so a stat never shows "No data".

func countOver(selector, window string) string {
	return fmt.Sprintf(`sum(count_over_time(%s [%s])) or vector(0)`, selector, window)
}

func sumOver(selector, field, window string) string {
	return fmt.Sprintf(`sum(sum_over_time(%s | unwrap %s [%s])) or vector(0)`, selector, field, window)
}

func tokensOver(selector, window string) string {
	return fmt.Sprintf(`(sum(sum_over_time(%s | unwrap gen_ai_usage_input_tokens [%s])) + sum(sum_over_time(%s | unwrap gen_ai_usage_output_tokens [%s]))) or vector(0)`,
		selector, window, selector, window)
}

func passRate(selector, window string) string {
	return fmt.Sprintf(`sum(count_over_time(%s | dummyrun_outcome="completed" [%s])) / sum(count_over_time(%s [%s]))`,
		selector, window, selector, window)
}

func countBy(selector, label, window string) string {
	return fmt.Sprintf(`sum by (%s) (count_over_time(%s [%s]))`, label, selector, window)
}

func sumBy(selector, label, field, window string) string {
	return fmt.Sprintf(`sum by (%s) (sum_over_time(%s | unwrap %s [%s]))`, label, selector, field, window)
}

// findingsBarGauge ranks confirmed findings by one label over the time range. Loki returns one
// frame per label value, so the gauge shows every value and names each bar after its label.
func (c Config) findingsBarGauge(id uint32, title, description, label string, colours map[string]string, order []string, w, h uint32) *bargauge.PanelBuilder {
	expr := fmt.Sprintf(`sort_desc(%s)`, countBy(lokiFindings+lokiFilters+` | dummyrun_finding_verdict="confirmed"`, label, "$__range"))
	p := bargauge.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		Datasource(c.loki()).
		Span(w).Height(h).
		WithTarget(lokiInstant(c.loki(), expr)).
		Unit("short").
		Decimals(0).
		Min(0).
		ColorScheme(fixedColour(colourAmber)).
		Orientation(common.VizOrientationHorizontal).
		DisplayMode(common.BarGaugeDisplayModeBasic).
		ValueMode(common.BarGaugeValueModeColor).
		NamePlacement(common.BarGaugeNamePlacementLeft).
		ShowUnfilled(true).
		ReduceOptions(common.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"}).Fields("").Values(true))
	for _, k := range order {
		p.WithOverride(colourOverride(k, colours[k]))
	}
	return p
}

// findingsOverTime stacks confirmed findings per interval by one label.
func (c Config) findingsOverTime(id uint32, title, description, label string, colours map[string]string, order []string, w, h uint32) *timeseries.PanelBuilder {
	p := c.lokiSeries(id, title, description, w, h,
		series{countBy(lokiFindings+lokiFilters+` | dummyrun_finding_verdict="confirmed"`, label, "$__interval"), "{{" + label + "}}"}).
		Unit("short").
		Decimals(0)
	if colours == nil {
		p.ColorScheme(paletteColour())
	}
	for _, k := range order {
		p.WithOverride(colourOverride(k, colours[k]))
	}
	return p
}

// stepPercentiles shows p50, p90 and p99 step duration over the time range from the step histogram.
// It is an instant query at the end of the range, so a backlog exported a moment ago counts too.
func stepPercentiles(ds common.DataSourceRef, id uint32, selector string, w, h uint32) *bargauge.PanelBuilder {
	p := bargauge.NewPanelBuilder().
		Id(id).
		Title("Step duration percentiles").
		Description("How long a browser step took, from the step duration histogram of the runs exported in the time range. p99 is the slowest 1 in 100 steps.").
		Datasource(ds).
		Span(w).Height(h).
		Unit("ms").
		Decimals(0).
		Min(0).
		ColorScheme(fixedColour(colourAmber)).
		Orientation(common.VizOrientationHorizontal).
		DisplayMode(common.BarGaugeDisplayModeBasic).
		ValueMode(common.BarGaugeValueModeColor).
		NamePlacement(common.BarGaugeNamePlacementLeft).
		ShowUnfilled(true).
		ReduceOptions(common.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"}).Fields("").Values(false))
	for i, quantile := range []string{"0.5", "0.9", "0.99"} {
		legend := []string{"p50", "p90", "p99"}[i]
		p.WithTarget(promInstant(ds, fmt.Sprintf(`histogram_quantile(%s, sum by (le) (max_over_time(%s{%s}[$__range])))`, quantile, metricStepBucket, selector), legend).RefId(string(rune('A' + i))))
	}
	return p
}

// slowestActions ranks step actions by their p90 duration over the time range.
func slowestActions(ds common.DataSourceRef, id uint32, selector string, w, h uint32) *bargauge.PanelBuilder {
	return bargauge.NewPanelBuilder().
		Id(id).
		Title("Slowest actions, p90").
		Description("Step actions ranked by the duration 9 in 10 steps stay under, from the step duration histogram.").
		Datasource(ds).
		Span(w).Height(h).
		WithTarget(promInstant(ds, fmt.Sprintf(
			`sort_desc(histogram_quantile(0.9, sum by (le, dummyrun_action) (max_over_time(%s{%s}[$__range]))))`, metricStepBucket, selector),
			"{{dummyrun_action}}")).
		Unit("ms").
		Decimals(0).
		Min(0).
		ColorScheme(fixedColour(colourAmber)).
		Orientation(common.VizOrientationHorizontal).
		DisplayMode(common.BarGaugeDisplayModeBasic).
		ValueMode(common.BarGaugeValueModeColor).
		NamePlacement(common.BarGaugeNamePlacementLeft).
		ShowUnfilled(true).
		ReduceOptions(common.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"}).Fields("").Values(false))
}
