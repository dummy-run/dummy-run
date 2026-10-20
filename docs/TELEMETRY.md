# Telemetry

Dummy Run describes every run in OpenTelemetry. Any OpenTelemetry backend can store it, and the Grafana
integration in `integrations/grafana/` uses Tempo, Loki and Prometheus or Mimir. This page is the contract
between the runner, the exporter and the dashboards. Change it here first.

The source of truth is the run record, defined by `schema/run-record.schema.json` (schema version 2).
Synthetic examples are in `examples/runs/`. The exporter also reads version 1 records, which have none of
the version 2 fields, so the attributes marked "version 2" are left out for them.
`dummyrun.cost.usd` is present when the model is in the price table, and the record has `cost_usd: null`
otherwise.

## Resource

| Attribute | Value |
|---|---|
| `service.name` | `dummyrun` |
| `service.version` | The Dummy Run version |
| `service.namespace` | Optional. Set it to the target app's name to group runs by app |

## Traces

Each run is one trace. The root span is named `dummyrun.run` and lasts from `started_at` to `ended_at`.
The trace ID is the first 16 bytes of SHA-256 of `dummyrun.run:<id>`. The root span ID is the first 8
bytes of SHA-256 of `dummyrun.span:<id>:root`. The span ID of step `n` is the first 8 bytes of SHA-256
of `dummyrun.span:<id>:step:<n>`. Each request during a step carries a `traceparent` with the run's trace
ID and that step's span ID, so the app's spans become children of the step. Exporting a run twice writes
the same trace.
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
| `dummyrun.persona.zoom` | `persona.zoom`, when present |
| `dummyrun.persona.at` | `persona.dimensions.at`, when it is not null (version 2 records) |
| `dummyrun.scenario.id` | `scenario.id` |
| `dummyrun.scenario.goal_met` | `scenario.goal_met`, when present (version 2 records) |
| `dummyrun.repeat_index` | `repeat_index` (version 2 records) |
| `dummyrun.target.adapter` | `target.adapter` |
| `dummyrun.target.environment` | `target.environment` |
| `dummyrun.target.commit` | `target.commit`, when present |
| `dummyrun.target.build_hash` | `target.build_hash`, when present |
| `url.full` | `target.base_url` |
| `gen_ai.system` | `provider.kind` |
| `gen_ai.request.model` | `provider.model` |
| `cloud.region` | `provider.region` |
| `browser.name` | `browser.name` |
| `dummyrun.outcome` | `outcome` |
| `gen_ai.usage.input_tokens` | `usage.input_tokens` |
| `gen_ai.usage.output_tokens` | `usage.output_tokens` |
| `dummyrun.cost.usd` | `usage.cost_usd`, when present |
| `dummyrun.steps.count` | The number of entries in `steps` |

The root span status is `ERROR` when the outcome is `errored` or `blocked`, and `OK` otherwise.

Step span attributes are `dummyrun.step.n`, `dummyrun.step.action` and `dummyrun.step.target`. When
present they add `url.full`, `http.response.status_code`, `dummyrun.step.console_errors` and
`dummyrun.step.a11y_violations`. Version 2 records add `dummyrun.step.route`, the normalised route. A step with a status of 500 or more has status `ERROR`.

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
| `dummyrun.finding.class` | `class`, when present (version 2 records) |
| `dummyrun.finding.route` | `route`, when present |
| `dummyrun.finding.symptom` | `symptom`, when present |
| `dummyrun.finding.fingerprint` | `fingerprint`, when present. The same problem has the same fingerprint in every run |

The verdict is the one the record holds at the moment of the export. `dummyrun review` changes it, so review first and export after. It adds `reviewer`, `reviewed_at` and `reason` to the record, and the exporter reads them without sending them. Sending a reviewed record again does not update what Grafana holds. It adds a second set of spans, lines and metric points, as the exporter guide says. Panels that count confirmed findings are then right only for runs exported after review.

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
a new series. Scenario IDs come from the cast's files, so `scenario.id` is a bounded label on
`dummyrun.runs` and `dummyrun.run.duration`. Findings carry it only in their log record.

Metric data points carry the time the exporter sent them. Each exporter
process starts its counters at zero. Metrics therefore describe runs as they are exported. Use
`dummyrun-otel watch` for live charts. For history, use the log records. Each run-end record holds
the run's outcome, step count, tokens and cost. Each finding record holds one finding. Step durations come from the
step spans in traces.

| Histogram | Bucket boundaries |
|---|---|
| `dummyrun.run.duration` | 15 30 60 120 180 300 600 900 1800 3600 (seconds) |
| `dummyrun.step.duration` | 100 250 500 1000 2000 5000 10000 30000 60000 (milliseconds) |

## Logs

The exporter writes one log record when a run ends and one for each finding. Both carry the root span's
trace ID and span ID, so Grafana can jump from a log line to the trace.

| Record | Severity | Body | Attributes |
|---|---|---|---|
| Run end | `INFO`, or `ERROR` when errored | `run <id> <outcome>: <persona.id> / <scenario.id>` | The root span attributes |
| Finding | `WARN` for blocker and major, `INFO` otherwise | The finding title | The finding event attributes plus `dummyrun.run.id`, `dummyrun.persona.id`, `dummyrun.scenario.id` and `dummyrun.target.environment` |

Loki stores `dummyrun.finding.steps` as JSON text, such as `[2,3]`. Query it as a string.

Loki rejects a line more than an hour older than the newest line already in its stream. The exporter
sorts every line it sends by its own time. Before sending, it checks that its oldest line is no more
than an hour older than the newest line already sent to that stream, and it fails if not.

## Grafana annotations

Each run becomes one region annotation from `started_at` to `ended_at`. Its text is
`<persona.id> / <scenario.id>: <outcome>`, followed by the number of confirmed findings. The exporter posts with a Grafana service account token that has the Editor role. It reads the
token from `GRAFANA_TOKEN` and sends it as `Authorization: Bearer <token>`. Before posting, it lists
annotations with the same tags and times. It skips the run if one exists. The text is
`<persona.id> / <scenario.id>: <outcome>, <n> confirmed finding(s)`. Its tags are:

- `dummyrun`
- `persona:<persona.id>`
- `scenario:<scenario.id>`
- `env:<target.environment>`
- `outcome:<outcome>`
- `findings` when the run has at least one confirmed finding

Dashboards for the app under test can show these annotations to mark when personas were active.

Grafana can answer HTTP 429 when annotations come too fast. The exporter waits half a second between
calls. It retries a call that gets a 429 up to five times and honours `Retry-After`. A lookup also retries on a 5xx, but a create does not, because Grafana may have stored the annotation. One export spends at most ten minutes on annotations (code: `integrations/grafana/exporter/internal/cli/cli.go`). If an
annotation still fails, the exporter goes on with the telemetry of every run and flushes it.
It then lists the failed annotations and exits with code 3. Code 3 means the telemetry arrived and some
annotations did not. Code 1 means the telemetry failed. `--signals annotations` sends only the
annotations, so a second export fills the gaps without repeating spans, metrics or logs.

## Linking to the app's own telemetry

The runner adds two things to every request it sends to the target's hosts. The first is a W3C `baggage`
header carrying `dummyrun.run.id` and `dummyrun.persona.id`. The second is a `traceparent` header for
the step span. An app instrumented with OpenTelemetry then joins the persona's trace, and its spans
carry the baggage. The browser's user agent also ends with `DummyRun/<version>`, so frontend tools such
as Grafana Faro can filter persona sessions.

The page's own instrumentation may already set a `traceparent`, for example Faro or OpenTelemetry JS
on a `fetch` call. The runner then keeps that header and adds only the baggage. Faro's client span stays
the parent of the server span, and the app's spans still carry the run and persona IDs through baggage.
