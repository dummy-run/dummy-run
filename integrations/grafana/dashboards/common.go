// Package main generates the Dummy Run Grafana dashboards as JSON with the Grafana Foundation SDK.
// The telemetry contract they read is docs/TELEMETRY.md.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/cog/variants"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/tempo"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Colours from brand/README.md, dark theme column, because Grafana's default theme is dark.
const (
	colourPass    = "#57d273"
	colourBlocker = "#ff7b72"
	colourAmber   = "#fbad18"
	colourInk     = "#f3f1ec"
	// Named Grafana colours for values the brand palette does not cover.
	colourMinor    = "blue"
	colourNeutral  = "text"
	colourFiltered = "purple"
)

// Prometheus names after the OTLP translation, verified against grafana/otel-lgtm 0.35.0
// (Prometheus 3.15). The translation adds unit suffixes and _total.
const (
	metricRuns        = "dummyrun_runs_total"
	metricRunDuration = "dummyrun_run_duration_seconds"
	metricSteps       = "dummyrun_steps_total"
	metricStepBucket  = "dummyrun_step_duration_milliseconds_bucket"
	metricFindings    = "dummyrun_findings_total"
	metricTokens      = "dummyrun_tokens_total"
	metricCost        = "dummyrun_cost_usd_total"
)

const (
	uidOverview = "dummyrun-overview"
	uidPersonas = "dummyrun-personas"
	uidFindings = "dummyrun-findings"
	uidRun      = "dummyrun-run"
	uidHome     = "dummyrun-home"
	tagDummyRun = "dummyrun"
)

// Config holds the data source UIDs the generated dashboards refer to.
type Config struct {
	PrometheusUID string
	LokiUID       string
	TempoUID      string
	// LogoSVG is the SVG source of the logo shown on the home dashboard. Empty means no logo.
	LogoSVG string
}

func ptr[T any](v T) *T { return &v }

func (c Config) prom() common.DataSourceRef {
	return common.DataSourceRef{Type: ptr("prometheus"), Uid: ptr(c.PrometheusUID)}
}

func (c Config) loki() common.DataSourceRef {
	return common.DataSourceRef{Type: ptr("loki"), Uid: ptr(c.LokiUID)}
}

func (c Config) tempo() common.DataSourceRef {
	return common.DataSourceRef{Type: ptr("tempo"), Uid: ptr(c.TempoUID)}
}

func grafanaDS() common.DataSourceRef {
	return common.DataSourceRef{Type: ptr("grafana"), Uid: ptr("-- Grafana --")}
}

// Label selectors for the Prometheus queries. The runs metric carries all three labels and the
// histograms only carry the persona, as docs/TELEMETRY.md defines them.
const (
	selEnv      = `dummyrun_target_environment=~"$environment"`
	selPersona  = `dummyrun_persona_id=~"$persona"`
	selScenario = `dummyrun_scenario_id=~"$scenario"`
	selRuns     = selEnv + ", " + selPersona + ", " + selScenario
)

// Loki stream and structured metadata filters. The OTLP endpoint turns attribute dots into
// underscores and stores every attribute as structured metadata.
const (
	lokiStream   = `{service_name="dummyrun"}`
	lokiRunEnd   = lokiStream + ` | dummyrun_outcome != ""`
	lokiFindings = lokiStream + ` | dummyrun_finding_id != ""`
	// Both record kinds carry environment, persona and scenario, so one filter serves both.
	lokiFilters = ` | dummyrun_target_environment=~"$environment" | dummyrun_persona_id=~"$persona" | dummyrun_scenario_id=~"$scenario"`
)

// rawQuery is a query the SDK has no typed builder for, such as the built-in annotation target.
type rawQuery map[string]any

func (r rawQuery) Build() (variants.Dataquery, error) { return variants.UnknownDataquery(r), nil }

// ids hands out panel IDs, which Grafana needs to be unique within a dashboard.
type ids struct{ next uint32 }

func (i *ids) id() uint32 {
	i.next++
	return i.next
}

// base returns a dashboard builder with the settings every Dummy Run dashboard shares.
func (c Config) base(uid, title, description string) *dashboard.DashboardBuilder {
	b := dashboard.NewDashboardBuilder(title).
		Uid(uid).
		Description(description).
		Tags([]string{tagDummyRun}).
		Editable().
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		Time("now-14d", "now").
		Timezone(common.TimeZoneBrowser).
		Refresh("").
		Link(dashboard.NewDashboardLinkBuilder("Dummy Run dashboards").
			Type(dashboard.DashboardLinkTypeDashboards).
			Tags([]string{tagDummyRun}).
			AsDropdown(true).
			IncludeVars(true).
			KeepTime(true)).
		Annotation(dashboard.NewAnnotationQueryBuilder().
			Name("Annotations & Alerts").
			Datasource(grafanaDS()).
			BuiltIn(1).
			Hide(true).
			IconColor("rgba(0, 211, 255, 1)").
			Enable(true).
			Type("dashboard")).
		// Off by default: a fortnight of runs covers every chart in stripes. The toggle at the top
		// of the dashboard switches it on.
		Annotation(dashboard.NewAnnotationQueryBuilder().
			Name("Dummy Run runs").
			Datasource(grafanaDS()).
			Enable(false).
			IconColor(colourAmber).
			Type("tags").
			Target(rawQuery{"type": "tags", "tags": []string{tagDummyRun}, "matchAny": false, "limit": 500}))
	for _, v := range c.filterVariables() {
		b.WithVariable(v)
	}
	return b
}

// filterVariables are the environment, persona and scenario pickers, from the runs metric.
func (c Config) filterVariables() []cog.Builder[dashboard.VariableModel] {
	mk := func(name, label, query string) cog.Builder[dashboard.VariableModel] {
		return dashboard.NewQueryVariableBuilder(name).
			Label(label).
			Datasource(c.prom()).
			Query(dashboard.StringOrMap{String: ptr(query)}).
			Definition(query).
			Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
			Sort(dashboard.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			AllValue(".*").
			Current(dashboard.VariableOption{
				Selected: ptr(true),
				Text:     dashboard.StringOrArrayOfString{ArrayOfString: []string{"All"}},
				Value:    dashboard.StringOrArrayOfString{ArrayOfString: []string{"$__all"}},
			})
	}
	return []cog.Builder[dashboard.VariableModel]{
		mk("environment", "Environment", fmt.Sprintf(`label_values(%s, dummyrun_target_environment)`, metricRuns)),
		mk("persona", "Persona", fmt.Sprintf(`label_values(%s{%s}, dummyrun_persona_id)`, metricRuns, selEnv)),
		mk("scenario", "Scenario", fmt.Sprintf(`label_values(%s{%s, %s}, dummyrun_scenario_id)`, metricRuns, selEnv, selPersona)),
	}
}

// Query helpers.

func promRange(ds common.DataSourceRef, expr, legend string) *prometheus.DataqueryBuilder {
	return prometheus.NewDataqueryBuilder().Datasource(ds).Expr(expr).LegendFormat(legend).Range().RefId("A")
}

func promInstant(ds common.DataSourceRef, expr, legend string) *prometheus.DataqueryBuilder {
	return prometheus.NewDataqueryBuilder().Datasource(ds).Expr(expr).LegendFormat(legend).Instant().RefId("A")
}

func lokiLogs(ds common.DataSourceRef, expr string, maxLines int64) *loki.DataqueryBuilder {
	return loki.NewDataqueryBuilder().Datasource(ds).Expr(expr).QueryType("range").Range(true).MaxLines(maxLines).RefId("A")
}

func lokiInstant(ds common.DataSourceRef, expr string) *loki.DataqueryBuilder {
	return loki.NewDataqueryBuilder().Datasource(ds).Expr(expr).QueryType("instant").Instant(true).RefId("A")
}

func lokiRange(ds common.DataSourceRef, expr, legend string) *loki.DataqueryBuilder {
	return loki.NewDataqueryBuilder().Datasource(ds).Expr(expr).LegendFormat(legend).QueryType("range").Range(true).RefId("A")
}

func tempoSearch(ds common.DataSourceRef, query string, tableType tempo.SearchTableType, limit, spss int64) *tempo.DataqueryBuilder {
	return tempo.NewDataqueryBuilder().Datasource(ds).QueryType("traceql").Query(query).TableType(tableType).Limit(limit).Spss(spss).RefId("A")
}

// Field config helpers.

func fixedColour(hex string) *dashboard.FieldColorBuilder {
	return dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdFixed).FixedColor(hex)
}

func paletteColour() *dashboard.FieldColorBuilder {
	return dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)
}

// thresholds builds absolute thresholds. The first colour is the base, the rest apply from each value.
func thresholds(base string, steps ...any) *dashboard.ThresholdsConfigBuilder {
	out := []dashboard.Threshold{{Value: nil, Color: base}}
	for i := 0; i+1 < len(steps); i += 2 {
		v := steps[i].(float64)
		out = append(out, dashboard.Threshold{Value: ptr(v), Color: steps[i+1].(string)})
	}
	return dashboard.NewThresholdsConfigBuilder().Mode(dashboard.ThresholdsModeAbsolute).Steps(out)
}

// colourOverride gives one series or field a fixed colour, matched by its display name.
func colourOverride(name, hex string) (dashboard.MatcherConfig, []dashboard.DynamicConfigValue) {
	return dashboard.MatcherConfig{Id: "byName", Options: name}, []dashboard.DynamicConfigValue{
		{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": hex}},
	}
}

// outcomeColours maps the run outcomes to colours.
var outcomeColours = map[string]string{
	"completed":         colourPass,
	"blocked":           colourBlocker,
	"budget_exhausted":  colourAmber,
	"errored":           colourNeutral,
	"provider_filtered": colourFiltered,
}

var severityColours = map[string]string{
	"blocker": colourBlocker,
	"major":   colourAmber,
	"minor":   colourMinor,
	"info":    colourNeutral,
}

// Ordered keys, so the generated JSON is stable.
var outcomeOrder = []string{"completed", "blocked", "budget_exhausted", "errored", "provider_filtered"}
var severityOrder = []string{"blocker", "major", "minor", "info"}

func valueMappings(colours map[string]string, order []string) []dashboard.ValueMapping {
	options := map[string]dashboard.ValueMappingResult{}
	for i, k := range order {
		options[k] = dashboard.ValueMappingResult{Color: ptr(colours[k]), Index: ptr(int32(i))}
	}
	return []dashboard.ValueMapping{{ValueMap: &dashboard.ValueMap{Type: dashboard.MappingTypeValueToText, Options: options}}}
}

// Transformations.

func transform(id string, options map[string]any) dashboard.DataTransformerConfig {
	return dashboard.DataTransformerConfig{Id: id, Options: options}
}

// extractLabels turns the Loki labels field (stream labels plus structured metadata) into columns.
func extractLabels() dashboard.DataTransformerConfig {
	return transform("extractFields", map[string]any{"source": "labels", "format": "auto", "replace": false, "keepTime": false})
}

// organize keeps the named fields in order and renames them. Pairs are old name, new name.
func organize(pairs ...string) dashboard.DataTransformerConfig {
	include := map[string]any{}
	index := map[string]any{}
	rename := map[string]any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		include[pairs[i]] = true
		index[pairs[i]] = i / 2
		if pairs[i+1] != pairs[i] {
			rename[pairs[i]] = pairs[i+1]
		}
	}
	return transform("organize", map[string]any{"includeByName": include, "indexByName": index, "renameByName": rename})
}

func sortBy(field string, desc bool) dashboard.DataTransformerConfig {
	return transform("sortBy", map[string]any{"sort": []map[string]any{{"field": field, "desc": desc}}})
}

// Links.

// wrap lets a text column break over several lines instead of being cut off.
func wrap(cellType string) dashboard.DynamicConfigValue {
	return dashboard.DynamicConfigValue{Id: "custom.cellOptions", Value: map[string]any{"type": cellType, "wrapText": true}}
}

// fieldLink is a data link on a table column. Grafana reads title and url from the field config.
func fieldLink(title, u string) dashboard.DynamicConfigValue {
	return dashboard.DynamicConfigValue{Id: "links", Value: []map[string]any{{"title": title, "url": u}}}
}

// exploreTempoURL opens a trace ID in Explore. The trace ID comes from the cell value.
func exploreTempoURL(tempoUID string) string {
	pane := map[string]any{
		"a": map[string]any{
			"datasource": tempoUID,
			"queries": []map[string]any{{
				"refId":      "A",
				"datasource": map[string]string{"type": "tempo", "uid": tempoUID},
				"queryType":  "traceql",
				"query":      "__DR_TRACEID__",
			}},
			"range": map[string]string{"from": "__DR_FROM__", "to": "__DR_TO__"},
		},
	}
	raw, _ := json.Marshal(pane)
	encoded := url.QueryEscape(string(raw))
	encoded = strings.ReplaceAll(encoded, "__DR_TRACEID__", "${__value.raw}")
	encoded = strings.ReplaceAll(encoded, "__DR_FROM__", "${__from}")
	encoded = strings.ReplaceAll(encoded, "__DR_TO__", "${__to}")
	return "/explore?schemaVersion=1&panes=" + encoded
}

// exploreLokiURL opens a LogQL query in Explore, with the cell value inserted where __DR_VALUE__
// appears. The sentinels cannot occur in a data source UID or a query.
func exploreLokiURL(lokiUID, expr string) string {
	pane := map[string]any{
		"a": map[string]any{
			"datasource": lokiUID,
			"queries": []map[string]any{{
				"refId":      "A",
				"datasource": map[string]string{"type": "loki", "uid": lokiUID},
				"expr":       expr,
			}},
			"range": map[string]string{"from": "__DR_FROM__", "to": "__DR_TO__"},
		},
	}
	raw, _ := json.Marshal(pane)
	encoded := url.QueryEscape(string(raw))
	encoded = strings.ReplaceAll(encoded, "__DR_VALUE__", "${__value.raw}")
	encoded = strings.ReplaceAll(encoded, "__DR_FROM__", "${__from}")
	encoded = strings.ReplaceAll(encoded, "__DR_TO__", "${__to}")
	return "/explore?schemaVersion=1&panes=" + encoded
}

// runDashboardURL opens the run dashboard for the run ID in the cell, keeping the filters and time.
func runDashboardURL() string {
	return "/d/" + uidRun + "/dummy-run-run?${environment:queryparam}&${persona:queryparam}&${scenario:queryparam}&var-run=${__value.raw}&${__url_time_range}"
}

// Shared panels.

// lokiStat is a single number from a LogQL instant query, with no graph.
func (c Config) lokiStat(id uint32, title, description, expr, unit string, decimals float64, steps *dashboard.ThresholdsConfigBuilder, w, h uint32) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		Datasource(c.loki()).
		Span(w).Height(h).
		WithTarget(lokiInstant(c.loki(), expr)).
		Unit(unit).
		Decimals(decimals).
		Thresholds(steps).
		ColorMode(common.BigValueColorModeValue).
		GraphMode(common.BigValueGraphModeNone).
		TextMode(common.BigValueTextModeValue).
		ReduceOptions(common.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"}).Fields("").Values(false))
}

// series is one LogQL range query and its legend.
type series struct{ expr, legend string }

// lokiSeries is a stacked bar chart of LogQL range queries, one bar per interval.
func (c Config) lokiSeries(id uint32, title, description string, w, h uint32, queries ...series) *timeseries.PanelBuilder {
	p := timeseries.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		Datasource(c.loki()).
		Span(w).Height(h).
		Interval("1h").
		MaxDataPoints(60).
		Min(0).
		DrawStyle(common.GraphDrawStyleBars).
		FillOpacity(80).
		LineWidth(1).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Legend(common.NewVizLegendOptionsBuilder().DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom).ShowLegend(true)).
		Tooltip(common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti).Sort(common.SortOrderDescending))
	for i, q := range queries {
		p.WithTarget(lokiRange(c.loki(), q.expr, q.legend).RefId(string(rune('A' + i))))
	}
	return p
}

// outcomesOverTime is a stacked bar chart of runs by outcome, from run end log lines.
func (c Config) outcomesOverTime(id uint32, title, description, selector string, w, h uint32) *timeseries.PanelBuilder {
	p := c.lokiSeries(id, title, description, w, h, series{countBy(selector, "dummyrun_outcome", "$__interval"), "{{dummyrun_outcome}}"}).
		Unit("short").
		Decimals(0)
	for _, k := range outcomeOrder {
		p.WithOverride(colourOverride(k, outcomeColours[k]))
	}
	return p
}

// attributesTable shows attributes of one log line as a one row table. Pairs are label, column title.
func (c Config) attributesTable(id uint32, title, description, expr string, w, h uint32, pairs ...string) *table.PanelBuilder {
	return table.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		Datasource(c.loki()).
		Span(w).Height(h).
		WithTarget(lokiLogs(c.loki(), expr, 1)).
		WithTransformation(extractLabels()).
		WithTransformation(organize(pairs...)).
		CellHeight(common.TableCellHeightSm)
}

// runsTable lists run end log lines as a table with links to the run dashboard and the trace.
func (c Config) runsTable(id uint32, title, description, expr string, w, h uint32) *table.PanelBuilder {
	return table.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		Datasource(c.loki()).
		Span(w).Height(h).
		WithTarget(lokiLogs(c.loki(), expr, 200)).
		WithTransformation(extractLabels()).
		WithTransformation(organize(
			"Time", "Ended",
			"dummyrun_run_id", "Run",
			"dummyrun_outcome", "Outcome",
			"dummyrun_persona_id", "Persona",
			"dummyrun_persona_device", "Device",
			"dummyrun_scenario_id", "Scenario",
			"dummyrun_target_environment", "Environment",
			"gen_ai_request_model", "Model",
			"trace_id", "Trace",
		)).
		WithTransformation(sortBy("Ended", true)).
		Filterable(true).
		CellHeight(common.TableCellHeightSm).
		OverrideByName("Run", []dashboard.DynamicConfigValue{
			fieldLink("Open run", runDashboardURL()),
			{Id: "custom.width", Value: 250},
		}).
		OverrideByName("Trace", []dashboard.DynamicConfigValue{
			fieldLink("Open trace", exploreTempoURL(c.TempoUID)),
			{Id: "custom.width", Value: 150},
		}).
		OverrideByName("Outcome", []dashboard.DynamicConfigValue{
			{Id: "mappings", Value: valueMappings(outcomeColours, outcomeOrder)},
			{Id: "custom.cellOptions", Value: map[string]any{"type": "color-text"}},
			{Id: "custom.width", Value: 140},
		}).
		OverrideByName("Ended", []dashboard.DynamicConfigValue{{Id: "custom.width", Value: 180}})
}

// outcomesOverTime is a stacked bar chart of runs by outcome.
func outcomesOverTime(ds common.DataSourceRef, id uint32, title, description, selector string, w, h uint32) *timeseries.PanelBuilder {
	p := timeseries.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		Datasource(ds).
		Span(w).Height(h).
		Interval("1h").
		MaxDataPoints(60).
		WithTarget(promRange(ds, fmt.Sprintf(`sum by (dummyrun_outcome) (increase(%s{%s}[$__interval]))`, metricRuns, selector), "{{dummyrun_outcome}}")).
		Unit("short").
		Decimals(0).
		Min(0).
		DrawStyle(common.GraphDrawStyleBars).
		FillOpacity(80).
		LineWidth(1).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Legend(common.NewVizLegendOptionsBuilder().DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom).ShowLegend(true)).
		Tooltip(common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti).Sort(common.SortOrderDescending))
	for _, k := range outcomeOrder {
		p.WithOverride(colourOverride(k, outcomeColours[k]))
	}
	return p
}
