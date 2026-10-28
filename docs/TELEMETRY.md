# Telemetry

Every run can be sent to OpenTelemetry as traces, metrics and logs, so any OpenTelemetry backend can
store it. The Grafana integration in `integrations/grafana/` uses Tempo, Loki and Prometheus or Mimir.
This page lists every signal the exporter writes, so you can build your own queries and dashboards
on them.

Everything here comes from the run record, which `schema/run-record.schema.json` defines (schema version
2). You'll find made-up examples in `examples/runs/`. The exporter also reads version 1 records. Those
have none of the version 2 fields, so the attributes marked "version 2" are missing for them.
`dummyrun.cost.usd` appears when the model is in the price table. Otherwise the record has
`cost_usd: null` and the attribute is left out.

## What leaves the network

Dummy Run sends data only where you tell it to. That's the model provider and your OpenTelemetry
backend, plus Grafana if you ask. You choose the provider and the Grafana address, and the OpenTelemetry endpoints come
from your environment.

- The model provider you pick with `--provider` gets screenshots and page text, because the persona is a model that reads what the browser shows. [Ethics and limits](ETHICS.md) covers what providers keep.
- The OpenTelemetry backend you configure gets the telemetry on this page. The exporter sends it to `OTEL_EXPORTER_OTLP_ENDPOINT`, or to a per-signal variable such as `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` when you set one. With neither set, it uses the local collector address. It sends telemetry nowhere else.
- Grafana gets annotations at `--grafana-url` when you pass that flag. These go through the Grafana HTTP API, outside OTLP.

Telemetry never holds screenshots or a copy of the persona's report. What it does hold:

- run IDs and outcomes
- each step's action, page URL and target (the element reference, selector or label the action named)
- the model, its region, the token counts and the cost
- each finding's title and evidence label (measured, code, sourced or inferred)

The persona writes the titles of its own findings, and the engine writes the title of a security
finding. A title can quote a few words from the page, and a URL can hold personal data if your app puts
it there. The run record and its screenshots stay on the machine where you run Dummy Run.

Telemetry is stored wherever your backend is. Choosing a model region doesn't move it, so if you need it
to stay in one market, host the backend there.

## Resource

| Attribute | Value |
|---|---|
| `service.name` | `dummyrun` |
| `service.version` | The Dummy Run version |
| `service.namespace` | Optional. Set it to the target app's name to group runs by app |

The optional record fields `target.identify`, `target.respect_robots`, `target.tech`, `target.journeys`,
`target.tech_error` and `outcome_reason` aren't exported.

## Traces

Each run is one trace. The root span is named `dummyrun.run` and lasts from `started_at` to `ended_at`.
Each step is a child span named `dummyrun.step <action>`, such as `dummyrun.step click`, which starts at
the step's `at` time and lasts `duration_ms`.

The IDs are derived from the run ID, so exporting a run twice writes the same trace:

- The trace ID is the first 16 bytes of SHA-256 of `dummyrun.run:<id>`.
- The root span ID is the first 8 bytes of SHA-256 of `dummyrun.span:<id>:root`.
- The span ID of step `n` is the first 8 bytes of SHA-256 of `dummyrun.span:<id>:step:<n>`.

The engine adds a `traceparent` to each request the browser sends to the target's own origins during a step,
when the request doesn't already have one. It carries the run's trace ID and that step's span ID, so your app's
spans become children of the step. If the page's own instrumentation has already set a `traceparent`, the
engine keeps it, so that request carries the page's trace and not the run's, and only the baggage is added.
The `baggage` header goes to the same requests. Requests to other hosts, such as a CDN's images, get neither.
The one request the sign-in gate lets through also gets them, and it always goes to the target's own sign-in
form.

Root span attributes:

| Attribute | From the run record |
|---|---|
| `dummyrun.run.id` | `id` |
| `dummyrun.batch.id` | `batch`, when present |
| `dummyrun.persona.id` | `persona.id` |
| `dummyrun.persona.role` | `persona.role` |
| `dummyrun.persona.device` | `persona.device` |
| `dummyrun.persona.zoom` | `persona.zoom`, when present |
| `dummyrun.persona.text_scale` | `persona.text_scale`, when present |
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

The root span's status is `ERROR` when the outcome is `errored`, `blocked` or `provider_filtered`, and
`OK` otherwise. A record's `events` list isn't exported, including the `harness_limit_finding` events for
findings the engine dropped because they showed only the test's own limits. Records written before that kind
existed stay valid. A reader built from an earlier copy of the schema rejects a record that has one.

Every step span has `dummyrun.step.n`, `dummyrun.step.action` and `dummyrun.step.target`. When the step
has them, it also gets `url.full` and `http.response.status_code`, plus
`dummyrun.step.console_errors` and `dummyrun.step.a11y_violations`. Version 2 records add `dummyrun.step.route`, the normalised route. A
step that got a status of 500 or more has status `ERROR`.

Each finding is a span event named `dummyrun.finding` on the root span, timed at the `at` of the
finding's first step, or at the run's start when the finding cites no step. Security findings show up with `security` as `dummyrun.finding.kind`.

| Event attribute | From the finding |
|---|---|
| `dummyrun.finding.id` | `id` |
| `dummyrun.finding.kind` | `kind` |
| `dummyrun.finding.severity` | `severity` |
| `dummyrun.finding.title` | `title` |
| `dummyrun.finding.evidence` | `evidence` |
| `dummyrun.finding.verdict` | `verdict` |
| `dummyrun.finding.steps` | `steps`, as an integer array |
| `dummyrun.finding.class` | `class`, when present (version 2 records). Always `problem` in records from the current engine |
| `dummyrun.finding.route` | `route`, when present |
| `dummyrun.finding.symptom` | `symptom`, when present |
| `dummyrun.finding.fingerprint` | `fingerprint`, when present. The same problem has the same fingerprint in every run |

The verdict is whatever the record holds when you export it, so review with `dummyrun review` first
and export afterwards. Review also adds `reviewer`, `reviewed_at` and `reason` to the record. The
exporter reads those fields but doesn't send them. The same goes for each finding's `inputs` block (its
evidence kind, engines, check, journey and reproduction), which the engine writes and a report turns into a
confidence band. The block is optional in the schema, so records from before it existed still load.

Exporting a reviewed record again won't update what Grafana already holds. The trace IDs stay the same,
but Loki keeps the repeated log lines and Prometheus counts the run again, as the exporter guide explains. Panels that count confirmed findings
are only right for runs you exported after review.

A record can also hold a `preview_access` block, which names the Basic auth or header protection the run got past. The exporter accepts it and sends nothing for it, and it never holds a secret.

A record can also hold an `observations` list of things that worked. The exporter sends nothing for it,
and the findings metric doesn't count it.

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

Each attribute above without a namespace gets a `dummyrun.` prefix, as in `dummyrun.persona.id`. In
Prometheus you'll see names such as `dummyrun_runs_total` and labels such as `dummyrun_persona_id`. The
cost unit is in braces so the translation drops it, which gives `dummyrun_cost_usd_total`.

Run IDs and finding IDs are never metric labels, since every value would start a new series.
`scenario.id` is a label on `dummyrun.runs` and `dummyrun.run.duration` because scenario IDs come from
the scenario file names in your cast. Keep their number small so the series stay few. Findings carry the scenario only in their log record.

Metric data points carry the time the exporter sent them, and each exporter process starts its counters
at zero. So metrics describe runs as you export them. Use `dummyrun-otel watch` for live charts, and the
log records for history. Each run-end record holds the run's outcome, step count and tokens, plus the cost when the record has one.
Each finding record holds one finding. For step durations, use the step spans in traces.

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

Loki rejects a line that's more than an hour older than the newest line already in its stream. The
exporter sorts the lines it sends by time. Before sending, it checks that its oldest line is within an
hour of the newest line already sent to that stream, and stops with an error if it isn't.

## Grafana annotations

Each run becomes one region annotation from `started_at` to `ended_at`, with the text
`<persona.id> / <scenario.id>: <outcome>, <n> confirmed finding(s)`. Its tags are:

- `dummyrun`
- `persona:<persona.id>`
- `scenario:<scenario.id>`
- `env:<target.environment>`
- `outcome:<outcome>`
- `findings` when the run has at least one confirmed finding

Your app's dashboards can show these annotations to mark when personas were active.

The exporter posts with a Grafana service account token that has the Editor role. It reads the token
from `GRAFANA_TOKEN` and sends it as `Authorization: Bearer <token>`. Before posting, it looks for an
annotation with the same tags and times, and skips the run if there is one.

Grafana can answer HTTP 429 when annotations arrive too fast. The exporter waits at least half a second
between calls, and retries a call that gets a 429 up to five times, honouring `Retry-After`. A lookup
also retries on a 5xx. The request that creates an annotation doesn't retry on a 5xx, because Grafana may already have stored it. One
export spends at most ten minutes on annotations.

If an annotation still fails, the exporter carries on and sends the telemetry for every run. Then it
lists the failed annotations and exits with code 3. The exit codes mean:

- 3 means some annotations couldn't be created. Any telemetry you selected was sent first, and with `--signals annotations` there is none.
- 1 means the telemetry failed.

To fill the gaps, export again with `--signals annotations`. That sends only the annotations, without
repeating spans, metrics or logs.

## Linking to the app's own telemetry

The runner adds two headers to every request it sends to the target's hosts:

- a W3C `baggage` header carrying `dummyrun.run.id` and `dummyrun.persona.id`
- a `traceparent` header for the step span

If your app uses OpenTelemetry, its spans join the persona's trace and carry the baggage. The browser's
user agent also ends in `DummyRun/<version>`, so frontend tools such as Grafana Faro can filter persona
sessions. Match the `DummyRun/<version>` token anywhere in the string, since with `--identify` the user
agent ends with `DummyRun/<version> (+https://dummyrun.dev/scanner/)`.

Your page's own instrumentation may already set a `traceparent`, for example Faro or OpenTelemetry JS
on a `fetch` call. The runner keeps that header and adds only the baggage. Faro's client span stays the
parent of the server span, and your app's spans still carry the run and persona IDs through baggage.
