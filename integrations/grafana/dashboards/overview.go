package main

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Overview shows runs, findings, step timing and model cost for the selected filters.
func (c Config) Overview() *dashboard.DashboardBuilder {
	ids := &ids{}
	prom := c.prom()
	b := c.base(uidOverview, "Dummy Run: overview",
		"Runs by outcome, confirmed findings, step timing and model cost. Pick an environment, persona or scenario at the top.")

	runsAll := fmt.Sprintf(`sum(increase(%s{%s}[$__range]))`, metricRuns, selRuns)
	runsCompleted := fmt.Sprintf(`sum(increase(%s{%s, dummyrun_outcome="completed"}[$__range]))`, metricRuns, selRuns)

	b.WithRow(dashboard.NewRowBuilder("Runs").Id(ids.id()))
	b.WithPanel(statNumber(prom, ids.id(), "Runs", "Runs that ended in the time range.",
		runsAll, "short", 0, thresholds(colourInk), 4, 4))
	b.WithPanel(statNumber(prom, ids.id(), "Pass rate", "Share of runs where the persona finished its job.",
		fmt.Sprintf(`%s / %s`, runsCompleted, runsAll), "percentunit", 0,
		thresholds(colourBlocker, 0.8, colourAmber, 0.9, colourPass), 4, 4))
	b.WithPanel(statNumber(prom, ids.id(), "Findings", "Findings the reviewer confirmed, for the selected environments and personas.",
		fmt.Sprintf(`sum(increase(%s{%s, dummyrun_verdict="confirmed"}[$__range]))`, metricFindings, selFindings),
		"short", 0, thresholds(colourInk, 1.0, colourAmber), 4, 4))
	b.WithPanel(statNumber(prom, ids.id(), "Blockers", "Confirmed findings with blocker severity.",
		fmt.Sprintf(`sum(increase(%s{%s, dummyrun_severity="blocker", dummyrun_verdict="confirmed"}[$__range]))`, metricFindings, selFindings),
		"short", 0, thresholds(colourPass, 1.0, colourBlocker), 4, 4))
	b.WithPanel(statNumber(prom, ids.id(), "Tokens", "Model tokens in and out.",
		fmt.Sprintf(`sum(increase(%s{%s}[$__range]))`, metricTokens, selPersona), "short", 1, thresholds(colourInk), 4, 4))
	b.WithPanel(statNumber(prom, ids.id(), "Cost", "Model cost in US dollars.",
		fmt.Sprintf(`sum(increase(%s{%s}[$__range]))`, metricCost, selPersona), "currencyUSD", 2, thresholds(colourInk), 4, 4))

	b.WithPanel(outcomesOverTime(prom, ids.id(), "Runs by outcome", "Runs in each interval, stacked by outcome.", selRuns, 16, 9))
	b.WithPanel(timeseries.NewPanelBuilder().
		Id(ids.id()).
		Title("Pass rate").
		Description("Completed runs divided by all runs in each interval.").
		Datasource(prom).
		Span(8).Height(9).
		Interval("1h").
		MaxDataPoints(60).
		WithTarget(promRange(prom, fmt.Sprintf(
			`sum(increase(%s{%s, dummyrun_outcome="completed"}[$__interval])) / sum(increase(%s{%s}[$__interval]))`,
			metricRuns, selRuns, metricRuns, selRuns), "pass rate")).
		Unit("percentunit").
		Decimals(0).
		Min(0).Max(1).
		ColorScheme(fixedColour(colourPass)).
		FillOpacity(15).
		LineWidth(2).
		SpanNulls(common.BoolOrFloat64{Bool: ptr(true)}).
		Thresholds(thresholds(colourBlocker, 0.9, colourPass)).
		ThresholdsStyle(common.NewGraphThresholdsStyleConfigBuilder().Mode(common.GraphThresholdsStyleModeDashed)).
		Legend(common.NewVizLegendOptionsBuilder().ShowLegend(false)))

	b.WithRow(dashboard.NewRowBuilder("Findings").Id(ids.id()))
	b.WithPanel(findingsBarGauge(prom, ids.id(), "Confirmed findings by kind", "Confirmed findings in the time range, by kind.",
		"dummyrun_kind", nil, nil, 8, 8))
	b.WithPanel(findingsBarGauge(prom, ids.id(), "Confirmed findings by severity", "Confirmed findings in the time range, by severity.",
		"dummyrun_severity", severityColours, severityOrder, 8, 8))
	b.WithPanel(findingsOverTime(prom, ids.id(), "Confirmed findings by severity over time", "Confirmed findings in each interval, stacked by severity.",
		"dummyrun_severity", severityColours, severityOrder, 8, 8))

	b.WithRow(dashboard.NewRowBuilder("Steps").Id(ids.id()))
	b.WithPanel(stepPercentiles(prom, ids.id(), selPersona, 12, 8))
	b.WithPanel(slowestActions(prom, ids.id(), selPersona, 12, 8))

	b.WithRow(dashboard.NewRowBuilder("Model usage").Id(ids.id()))
	b.WithPanel(timeseries.NewPanelBuilder().
		Id(ids.id()).
		Title("Tokens by model").
		Description("Input and output tokens in each interval, by model.").
		Datasource(prom).
		Span(12).Height(8).
		Interval("1h").
		MaxDataPoints(60).
		WithTarget(promRange(prom, fmt.Sprintf(
			`sum by (gen_ai_request_model, dummyrun_direction) (increase(%s{%s}[$__interval]))`, metricTokens, selPersona),
			"{{gen_ai_request_model}} {{dummyrun_direction}}")).
		Unit("short").
		Decimals(0).
		Min(0).
		DrawStyle(common.GraphDrawStyleBars).
		FillOpacity(80).
		LineWidth(1).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Legend(common.NewVizLegendOptionsBuilder().DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom).ShowLegend(true)).
		Tooltip(common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti).Sort(common.SortOrderDescending)))
	b.WithPanel(timeseries.NewPanelBuilder().
		Id(ids.id()).
		Title("Cost by model").
		Description("Model cost in US dollars in each interval, by model.").
		Datasource(prom).
		Span(12).Height(8).
		Interval("1h").
		MaxDataPoints(60).
		WithTarget(promRange(prom, fmt.Sprintf(
			`sum by (gen_ai_request_model) (increase(%s{%s}[$__interval]))`, metricCost, selPersona), "{{gen_ai_request_model}}")).
		Unit("currencyUSD").
		Decimals(2).
		Min(0).
		DrawStyle(common.GraphDrawStyleBars).
		FillOpacity(80).
		LineWidth(1).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Legend(common.NewVizLegendOptionsBuilder().DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom).ShowLegend(true)).
		Tooltip(common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti).Sort(common.SortOrderDescending)))

	b.WithRow(dashboard.NewRowBuilder("Latest runs").Id(ids.id()))
	b.WithPanel(c.runsTable(ids.id(), "Latest runs",
		"Run end log lines from Loki. Click a run ID to open the run dashboard, or the trace ID to open the trace.",
		lokiRunEnd+lokiRunSel, 24, 12))
	return b
}

// findingsBarGauge ranks confirmed findings by one label over the time range.
func findingsBarGauge(ds common.DataSourceRef, id uint32, title, description, label string, colours map[string]string, order []string, w, h uint32) *bargauge.PanelBuilder {
	p := bargauge.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		Datasource(ds).
		Span(w).Height(h).
		WithTarget(promInstant(ds, fmt.Sprintf(
			`sort_desc(sum by (%s) (increase(%s{%s, dummyrun_verdict="confirmed"}[$__range])))`, label, metricFindings, selFindings),
			"{{"+label+"}}")).
		Unit("short").
		Decimals(0).
		Min(0).
		Orientation(common.VizOrientationHorizontal).
		DisplayMode(common.BarGaugeDisplayModeBasic).
		ValueMode(common.BarGaugeValueModeColor).
		NamePlacement(common.BarGaugeNamePlacementLeft).
		ShowUnfilled(true).
		ReduceOptions(common.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"}).Fields("").Values(false))
	if colours == nil {
		p.ColorScheme(fixedColour(colourAmber))
		return p
	}
	p.ColorScheme(fixedColour(colourAmber))
	for _, k := range order {
		p.WithOverride(colourOverride(k, colours[k]))
	}
	return p
}

// findingsOverTime stacks confirmed findings per interval by one label.
func findingsOverTime(ds common.DataSourceRef, id uint32, title, description, label string, colours map[string]string, order []string, w, h uint32) *timeseries.PanelBuilder {
	p := timeseries.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		Datasource(ds).
		Span(w).Height(h).
		Interval("1h").
		MaxDataPoints(60).
		WithTarget(promRange(ds, fmt.Sprintf(
			`sum by (%s) (increase(%s{%s, dummyrun_verdict="confirmed"}[$__interval]))`, label, metricFindings, selFindings),
			"{{"+label+"}}")).
		Unit("short").
		Decimals(0).
		Min(0).
		DrawStyle(common.GraphDrawStyleBars).
		FillOpacity(80).
		LineWidth(1).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Legend(common.NewVizLegendOptionsBuilder().DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom).ShowLegend(true)).
		Tooltip(common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti).Sort(common.SortOrderDescending))
	for _, k := range order {
		p.WithOverride(colourOverride(k, colours[k]))
	}
	return p
}

// stepPercentiles plots p50, p90 and p99 step duration from the step histogram.
func stepPercentiles(ds common.DataSourceRef, id uint32, selector string, w, h uint32) *timeseries.PanelBuilder {
	q := func(quantile, legend string) string {
		return fmt.Sprintf(`histogram_quantile(%s, sum by (le) (increase(%s{%s}[$__interval])))`, quantile, metricStepBucket, selector) + legend
	}
	_ = q
	p := timeseries.NewPanelBuilder().
		Id(id).
		Title("Step duration percentiles").
		Description("How long a browser step took, from the step duration histogram. p99 is the slowest 1 in 100 steps.").
		Datasource(ds).
		Span(w).Height(h).
		Interval("1h").
		MaxDataPoints(60).
		Unit("ms").
		Decimals(0).
		Min(0).
		LineWidth(2).
		SpanNulls(common.BoolOrFloat64{Bool: ptr(true)}).
		ColorScheme(paletteColour()).
		Legend(common.NewVizLegendOptionsBuilder().DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom).ShowLegend(true)).
		Tooltip(common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti).Sort(common.SortOrderDescending))
	for i, quantile := range []string{"0.5", "0.9", "0.99"} {
		legend := []string{"p50", "p90", "p99"}[i]
		p.WithTarget(promRange(ds, fmt.Sprintf(`histogram_quantile(%s, sum by (le) (increase(%s{%s}[$__interval])))`, quantile, metricStepBucket, selector), legend).RefId(string(rune('A' + i))))
	}
	return p
}

// slowestActions ranks step actions by their p90 duration over the time range.
func slowestActions(ds common.DataSourceRef, id uint32, selector string, w, h uint32) *bargauge.PanelBuilder {
	return bargauge.NewPanelBuilder().
		Id(id).
		Title("Slowest actions, p90").
		Description("Step actions ranked by the duration 9 in 10 steps stay under.").
		Datasource(ds).
		Span(w).Height(h).
		WithTarget(promInstant(ds, fmt.Sprintf(
			`sort_desc(histogram_quantile(0.9, sum by (le, dummyrun_action) (increase(%s{%s}[$__range]))))`, metricStepBucket, selector),
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
