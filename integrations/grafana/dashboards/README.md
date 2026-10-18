# Grafana dashboards

Four dashboards for Dummy Run telemetry, plus a home page for the local stack. A Go program builds
them with the Grafana Foundation SDK and writes the JSON into `json/`. The JSON is committed, and CI
fails when it differs from what the program writes (code: `go run . -check` in
`.github/workflows/dashboards.yml`).

The dashboards read the metrics, logs and traces described in [docs/TELEMETRY.md](../../../docs/TELEMETRY.md).
Screenshots are in [docs/images/grafana/](../../../docs/images/grafana/).

| Dashboard | File | What it is for |
|---|---|---|
| Dummy Run: overview | `json/overview.json` | What happened across all runs in the time range |
| Dummy Run: personas | `json/personas.json` | One row per persona, to see each user's runs |
| Dummy Run: findings | `json/findings.json` | What the personas found, permission findings first |
| Dummy Run: run | `json/run.json` | One run picked by its ID |
| Dummy Run: home | `json/home.json` | The start page of the local stack |

The overview shows runs by outcome and the pass rate. It shows confirmed findings by kind and by
severity. It shows step duration percentiles and the slowest actions. It shows tokens and cost by
model. It ends with the latest runs and links to their traces. A persona row shows that persona's
runs and outcomes. It lists the findings it surfaced and its slowest steps. It shows its role and
device and links to its traces. The findings dashboard starts with confirmed permission findings and
then blockers. It lists every distinct finding with first seen and last seen. It shows the run count
and the personas affected. The run dashboard shows the trace of one run. It lists the run's steps
and findings and shows its logs.

Every dashboard has variables for environment, persona and scenario. The run dashboard adds a run
picker fed by Tempo. Each dashboard also carries a "Dummy Run runs" annotation that marks when
personas were active. It is off by default, because a fortnight of runs covers every chart in
stripes (measured: 300 seeded runs over 14 days in the local stack). The toggle at the top of the
dashboard switches it on.

## Regenerate

```sh
cd integrations/grafana/dashboards
go run .
go test ./...
```

`go run . -check` compares the generator output with `json/` and exits 1 on a difference. CI runs it.

## Import into an existing Grafana or Grafana Cloud

The JSON refers to data sources by UID: `prometheus`, `loki` and `tempo`. Grafana Cloud and an
existing install assign their own UIDs, so generate the JSON with yours:

```sh
go run . -out /tmp/dummyrun -prometheus-uid grafanacloud-prom -loki-uid grafanacloud-logs -tempo-uid grafanacloud-traces
```

Find a data source UID under Connections, Data sources, in the URL of the data source page. Then
import each file from Dashboards, New, Import. The Loki data source needs a derived field named
`trace_id` that opens the Tempo data source, as in
[../stack/grafana/provisioning/datasources/dummyrun.yaml](../stack/grafana/provisioning/datasources/dummyrun.yaml),
for the log panels to link to traces.

## Where each number comes from

Metric data points carry the time the exporter sent them, and each exporter process starts its
counters at zero (sourced: docs/TELEMETRY.md, Metrics). So every total and every chart over time
reads the log records in Loki: run end lines for runs, tokens and cost, and finding lines for
findings (code: `countOver` and `sumOver` in `overview.go`). The two histogram panels, step duration
percentiles and slowest actions, read Prometheus (code: `stepPercentiles` and `slowestActions`).
Cost comes from the `dummyrun.cost.usd` attribute on the run end log line (code:
`sumOver(runs, "dummyrun_cost_usd", ...)` in `overview.go`).

## Metric and label names

Prometheus renames OpenTelemetry metrics on the way in. The queries use the names below (measured:
Prometheus 3.15.0 in the local stack, `grafana/otel-lgtm` 0.35.0, after seeding 300 synthetic runs).

| Metric in docs/TELEMETRY.md | Name in Prometheus |
|---|---|
| `dummyrun.runs` | `dummyrun_runs_total` |
| `dummyrun.run.duration` | `dummyrun_run_duration_seconds_bucket`, `_sum` and `_count` |
| `dummyrun.steps` | `dummyrun_steps_total` |
| `dummyrun.step.duration` | `dummyrun_step_duration_milliseconds_bucket`, `_sum` and `_count` |
| `dummyrun.findings` | `dummyrun_findings_total` |
| `dummyrun.tokens` | `dummyrun_tokens_total` |
| `dummyrun.cost.usd` | `dummyrun_cost_usd_total` |

Attribute dots become underscores, so `dummyrun.persona.id` is the label `dummyrun_persona_id`. Loki
stores every log attribute as structured metadata with the same renaming. The stream label is
`service_name="dummyrun"` (measured: Loki 3.7.8 in the local stack after seeding 300 synthetic runs).

## Colours

Pass and blocker use the dark theme colours from [brand/README.md](../../../brand/README.md), Colour
(code: `colourPass` and `colourBlocker` in `common.go`, checked by `TestBrandColours`). Grafana's
default theme is dark (sourced: Grafana's `defaults.ini`, `default_theme = dark`). Amber marks
budget-exhausted runs and major findings.
