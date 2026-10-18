# Grafana dashboards

Four dashboards for Dummy Run telemetry, plus a home page for the local stack. A Go program builds
them with the Grafana Foundation SDK and writes the JSON into `json/`. The JSON is committed, and CI
fails when it differs from what the program writes (code: `go run . -check` in
`.github/workflows/dashboards.yml`).

The dashboards read the metrics, logs and traces described in [docs/TELEMETRY.md](../../../docs/TELEMETRY.md).
Screenshots are in [docs/images/grafana/](../../../docs/images/grafana/).

| Dashboard | File | What it is for |
|---|---|---|
| Dummy Run: overview | `json/overview.json` | Runs by outcome, pass rate, confirmed findings by kind and severity, step duration percentiles, tokens and cost by model, and the latest runs with links to their traces |
| Dummy Run: personas | `json/personas.json` | One row per persona: its runs, outcomes, the findings it surfaced, its slowest steps, its device and links to its traces |
| Dummy Run: findings | `json/findings.json` | Confirmed permission findings first, then blockers, then every distinct finding with first seen, last seen, run count and personas affected |
| Dummy Run: run | `json/run.json` | One run picked by its ID: the trace, its steps, its findings and its logs |
| Dummy Run: home | `json/home.json` | The start page of the local stack, with the logo and links to the four dashboards |

Every dashboard has variables for environment, persona and scenario. The run dashboard adds a run
picker fed by Tempo. Each dashboard also carries a "Dummy Run runs" annotation that marks when
personas were active. It is off by default, because a fortnight of runs covers every chart in
stripes. The toggle at the top of the dashboard switches it on.

## Regenerate

```sh
cd integrations/grafana/dashboards
go run .
go test ./...
```

`go run . -check` compares the generator output with `json/` and exits 1 on a difference. CI runs it.

## Import into an existing Grafana or Grafana Cloud

The JSON refers to data sources by UID: `prometheus`, `loki` and `tempo`. Grafana Cloud and most
existing installs use other UIDs, so generate the JSON with yours:

```sh
go run . -out /tmp/dummyrun -prometheus-uid grafanacloud-prom -loki-uid grafanacloud-logs -tempo-uid grafanacloud-traces
```

Find a data source UID under Connections, Data sources, in the URL of the data source page. Then
import each file from Dashboards, New, Import. The Loki data source needs a derived field named
`trace_id` that opens the Tempo data source, as in
[../stack/grafana/provisioning/datasources/dummyrun.yaml](../stack/grafana/provisioning/datasources/dummyrun.yaml),
for the log panels to link to traces.

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
stores every log attribute as structured metadata with the same renaming, and the stream label is
`service_name="dummyrun"`.

## Colours

Pass and blocker use the dark theme colours from [brand/README.md](../../../brand/README.md), since
Grafana's default theme is dark. Amber marks budget-exhausted runs and major findings.
