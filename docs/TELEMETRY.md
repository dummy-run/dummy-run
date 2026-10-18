# Telemetry

Dummy Run describes every run in OpenTelemetry. Any OpenTelemetry backend can store it, and the Grafana
integration in `integrations/grafana/` uses Tempo, Loki and Prometheus or Mimir. This page is the contract
between the runner, the exporter and the dashboards. Change it here first.

The source of truth is the run record, defined by `schema/run-record.schema.json`. Synthetic examples
are in `examples/runs/`.

## Resource

| Attribute | Value |
|---|---|
| `service.name` | `dummyrun` |
| `service.version` | The Dummy Run version |
| `service.namespace` | Optional. Set it to the target app's name to group runs by app |

## Traces

Each run is one trace. The root span is named `dummyrun.run` and lasts from `started_at` to `ended_at`.
Each step is a child span named `dummyrun.step <action>`, such as `dummyrun.step click`. A step span
starts at its `at` time and lasts `duration_ms`.

Root span attributes:

| Attribute | From the run record |
|---|---|
| `dummyrun.run.id` | `id` |
| `dummyrun.batch.id` | `batch`, when present |
| `dummyrun.persona.id` | `persona.id` |
| `dummyrun.persona.role` | `persona.role` |
| `dummyrun.persona.device` | `persona.device` |
| `dummyrun.scenario.id` | `scenario.id` |
| `dummyrun.target.adapter` | `target.adapter` |
| `dummyrun.target.environment` | `target.environment` |
| `dummyrun.target.commit` | `target.commit`, when present |
| `url.full` | `target.base_url` |
| `gen_ai.system` | `provider.kind` |
| `gen_ai.request.model` | `provider.model` |
| `cloud.region` | `provider.region` |
| `browser.name` | `browser.name` |
| `dummyrun.outcome` | `outcome` |
| `gen_ai.usage.input_tokens` | `usage.input_tokens` |
| `gen_ai.usage.output_tokens` | `usage.output_tokens` |

The root span status is `ERROR` when the outcome is `errored` or `blocked`, and `OK` otherwise.

Step span attributes are `dummyrun.step.n`, `dummyrun.step.action` and `dummyrun.step.target`. When
present they add `url.full`, `http.response.status_code`, `dummyrun.step.console_errors` and
`dummyrun.step.a11y_violations`. A step with a status of 500 or more has status `ERROR`.

Each finding is a span event named `dummyrun.finding` on the root span. The event time is the `at`
time of the finding's first step.

| Event attribute | From the finding |
|---|---|
| `dummyrun.finding.id` | `id` |
| `dummyrun.finding.kind` | `kind` |
| `dummyrun.finding.severity` | `severity` |
| `dummyrun.finding.title` | `title` |
| `dummyrun.finding.evidence` | `evidence` |
| `dummyrun.finding.verdict` | `verdict` |
| `dummyrun.finding.steps` | `steps`, as an integer array |

## Metrics

| Metric | Type | Unit | Attributes |
|---|---|---|---|
| `dummyrun.runs` | counter | `{run}` | `persona.id`, `scenario.id`, `target.environment`, `outcome` |
| `dummyrun.run.duration` | histogram | `s` | `persona.id`, `scenario.id`, `outcome` |
| `dummyrun.steps` | counter | `{step}` | `persona.id`, `action` |
| `dummyrun.step.duration` | histogram | `ms` | `persona.id`, `action` |
| `dummyrun.findings` | counter | `{finding}` | `persona.id`, `target.environment`, `kind`, `severity`, `verdict` |
| `dummyrun.tokens` | counter | `{token}` | `persona.id`, `gen_ai.system`, `gen_ai.request.model`, `direction` (`input` or `output`) |
| `dummyrun.cost.usd` | counter | `{USD}` | `persona.id`, `gen_ai.system`, `gen_ai.request.model` |

Every metric attribute without a namespace above is prefixed with `dummyrun.`, as in
`dummyrun.persona.id`. In Prometheus these become names such as `dummyrun_runs_total` and labels such as
`dummyrun_persona_id`. The cost unit is written in braces so the translation drops it, giving
`dummyrun_cost_usd_total`. Run IDs and finding IDs are never metric labels, because each value would create
a new series.

## Logs

The exporter writes one log record when a run ends and one for each finding. Both carry the root span's
trace ID and span ID, so Grafana can jump from a log line to the trace.

| Record | Severity | Body | Attributes |
|---|---|---|---|
| Run end | `INFO`, or `ERROR` when errored | `run <id> <outcome>: <persona.id> / <scenario.id>` | The root span attributes |
| Finding | `WARN` for blocker and major, `INFO` otherwise | The finding title | The finding event attributes plus `dummyrun.run.id`, `dummyrun.persona.id` and `dummyrun.target.environment` |

## Grafana annotations

Each run becomes one region annotation from `started_at` to `ended_at`. Its text is
`<persona.id> / <scenario.id>: <outcome>`, followed by the number of confirmed findings. Its tags are:

- `dummyrun`
- `persona:<persona.id>`
- `scenario:<scenario.id>`
- `env:<target.environment>`
- `outcome:<outcome>`
- `findings` when the run has at least one confirmed finding

Dashboards for the app under test can show these annotations to mark when personas were active.

## Linking to the app's own telemetry

The runner adds two things to every request it sends to the target's hosts. The first is a W3C `baggage`
header carrying `dummyrun.run.id` and `dummyrun.persona.id`. The second is a `traceparent` header for
the step span. An app instrumented with OpenTelemetry then joins the persona's trace, and its spans
carry the baggage. The browser's user agent also ends with `DummyRun/<version>`, so frontend tools such
as Grafana Faro can filter persona sessions.

The page's own instrumentation may already set a `traceparent`, for example Faro or OpenTelemetry JS
on a `fetch` call. The runner then keeps that header and adds only the baggage. Faro's client span stays
the parent of the server span, and the app's spans still carry the run and persona IDs through baggage.
