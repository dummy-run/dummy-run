---
title: Grafana
weight: 40
summary: Four dashboards and a local stack for the telemetry a run emits
lead: Every run is one trace, one annotation and a handful of metrics and log lines. Four dashboards read them.
description: The Grafana integration for Dummy Run. Four dashboards read the traces, metrics and logs that runs emit. A local stack and an exporter feed them.
---

The [telemetry](../../../docs/TELEMETRY.md) contract gives each run one OpenTelemetry trace with a span per step and an event per finding. It adds a log record per run and per finding. Counters and histograms cover runs and steps. Others cover findings and token spend. Any OpenTelemetry backend can store that. The Grafana integration in `integrations/grafana/` uses Tempo for traces, Loki for logs and Prometheus or Mimir for metrics.

## Get started

`integrations/grafana/stack` starts Grafana, Tempo, Loki and Prometheus on your machine with one command. `dummyrun-otel` in `integrations/grafana/exporter` sends run records to it, or to Grafana Cloud. The [Grafana guide](../grafana-guide/) walks through both, then shows how to link runs to your app's own traces and set up alerts. The screenshots below show the dashboards fed 300 synthetic runs against the fictional app Inkwell.

## The dashboards

| Dashboard | What it is for |
|---|---|
| Overview | Runs by outcome, pass rate, confirmed findings by kind and severity, step duration percentiles, tokens and cost by model, and the latest runs with links to their traces |
| Personas | One row per persona. It shows the runs by outcome and the findings that persona surfaced. It lists the slowest steps, the device and links to the traces |
| Findings | Confirmed permission findings first and blockers second. Then every distinct finding with when it was first and last seen, how many runs hit it and which personas |
| Run | One run picked by its ID. The trace as a waterfall above tables of steps and findings, with the log lines below |

Every dashboard can be filtered by environment and by persona. A scenario variable narrows it further. Each one carries a "Dummy Run runs" annotation that marks when personas were active. It is off by default, because a fortnight of runs covers every chart in stripes.

{{< shot-grid >}}
{{< grafana-shot name="overview" alt="The overview dashboard. Stat panels for runs, pass rate, confirmed findings and cost sit above time series of runs by outcome and findings by kind, with a table of the latest runs." >}}The overview dashboard. Each run in the table links to its trace in Tempo.{{< /grafana-shot >}}

{{< grafana-shot name="personas" alt="The personas dashboard. One row per persona shows its runs by outcome, its findings and its slowest steps." >}}The personas dashboard. Four personas from the example cast, one row each.{{< /grafana-shot >}}

{{< grafana-shot name="findings" alt="The findings dashboard. A table of confirmed permission findings sits above blockers and then every distinct finding with first seen, last seen and run count." >}}The findings dashboard. Permission findings come first because they are the ones a role should never have reached.{{< /grafana-shot >}}

{{< grafana-shot name="run" alt="The run dashboard for one run. The trace is shown as a waterfall of step spans above tables of the steps and findings and then the log lines." >}}The run dashboard shows one run, picked by its ID from a list that Tempo feeds.{{< /grafana-shot >}}
{{< /shot-grid >}}

## Names in Prometheus

Prometheus renames OpenTelemetry metrics on the way in. `dummyrun.runs` becomes `dummyrun_runs_total` and `dummyrun.run.duration` becomes `dummyrun_run_duration_seconds_bucket` with its `_sum` and `_count`. Attributes become labels with the same prefix, so `persona.id` is `dummyrun_persona_id`. Run IDs and finding IDs are never labels, because each value would create a new series.

## Your app's own traces

The runner adds a `traceparent` header and a `baggage` header to every request it sends to your hosts. An app instrumented with OpenTelemetry joins the persona's trace. The baggage carries `dummyrun.run.id` and `dummyrun.persona.id`, and a span processor in your SDK copies them onto your spans. The [Grafana guide](../grafana-guide/) names the processor for six SDKs and gives the TraceQL to find the spans afterwards.
