# dummyrun-otel

`dummyrun-otel` reads Dummy Run run records and sends them to Grafana as OpenTelemetry traces, metrics
and logs, plus Grafana annotations. It follows [docs/TELEMETRY.md](../../../docs/TELEMETRY.md) and
works against a local `grafana/otel-lgtm` container and against Grafana Cloud.

## Install

```sh
go install github.com/dummy-run/dummy-run/integrations/grafana/exporter/cmd/dummyrun-otel@latest
```

Go 1.27 or later builds it. There are no other runtime dependencies.

## Quickstart with a local stack

Start the all-in-one stack, then export the example records:

```sh
docker run --rm -p 4318:4318 -p 3000:3000 grafana/otel-lgtm:0.35.0
dummyrun-otel export examples/runs
```

With no `OTEL_EXPORTER_OTLP_ENDPOINT` set, the exporter sends to `http://localhost:4318`. Open
Grafana at `http://localhost:3000` (admin / admin), pick Explore and query Loki for
`{service_name="dummyrun"}` over the last 7 days. Each line links to its trace. A TraceQL search such as
`{ span.dummyrun.run.id = "2026-10-16-ada-editor-003" }` needs the Tempo setting described under
historical timestamps.

To add annotations, create a service account with the Editor role in Grafana, make a token for it
and pass the Grafana URL:

```sh
export GRAFANA_TOKEN=glsa_...
dummyrun-otel export --grafana-url http://localhost:3000 examples/runs
```

## Quickstart with Grafana Cloud

Open the OpenTelemetry tile in your Grafana Cloud stack. It gives you three variables. Set them,
then run the same command:

```sh
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp-gateway-prod-eu-west-2.grafana.net/otlp
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic <base64 of instanceID:token>"
export GRAFANA_TOKEN=glsa_...
dummyrun-otel export --grafana-url https://yourstack.grafana.net examples/runs
```

The OTLP token needs the `metrics:write`, `logs:write` and `traces:write` scopes. `GRAFANA_TOKEN` is a
separate service account token on the Grafana instance, because annotations use Grafana's HTTP API
rather than OTLP.

## Commands

`dummyrun-otel export [flags] FILE|DIR...` reads every file named, or every `*.json` file in each
directory, validates all of them against `schema/run-record.schema.json` and then sends them. One
invalid record stops the export before anything is sent, and the error names the file. The exit code
is 0 on success, 1 when a record is invalid or a backend rejects the data, and 2 for a usage error.

`dummyrun-otel watch [flags] DIR` polls a directory and sends each new record once its size and
modification time have stayed the same across two polls. Files already in the directory at start are
skipped unless you pass `--existing`. It runs until SIGINT or SIGTERM, then flushes and exits 1 if any
file failed. Run it next to the runner that writes records.

`dummyrun-otel version` prints the version.

## Flags

| Flag | Default | What it does |
|---|---|---|
| `--dry-run` | off | Print every signal as JSON on stdout and send nothing |
| `--protocol` | `OTEL_EXPORTER_OTLP_PROTOCOL` or `http/protobuf` | OTLP transport, `http/protobuf` or `grpc` |
| `--metrics-temporality` | `cumulative` | `cumulative` for Prometheus and Mimir, `delta` for a backend that converts deltas |
| `--grafana-url` | unset | Grafana base URL for annotations. The token comes from `GRAFANA_TOKEN` |
| `--verify-loki` | unset | Loki base URL. Before the export, refuse lines Loki would drop as too old for its stream. After it, count the lines back per run and exit 1 naming any run that is short |
| `--logs-max-age` | `168h` | Skip log records older than this, because Loki rejects them. The local stack in `integrations/grafana/stack` accepts `720h`. `0` sends every record |
| `--timeout` | `30s` | How long the final flush may take |
| `--interval` (watch) | `5s` | How often to look for new files |
| `--existing` (watch) | off | Also send the records already in the directory |

## Environment variables

The exporter uses the OpenTelemetry Go SDK, so the standard variables apply.

| Variable | What it does |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Base URL of the OTLP receiver. Default `http://localhost:4318` |
| `OTEL_EXPORTER_OTLP_HEADERS` | Headers for every request, such as `Authorization=Basic ...` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` or `grpc` |
| `OTEL_EXPORTER_OTLP_TIMEOUT`, `OTEL_EXPORTER_OTLP_INSECURE` | Request timeout and plain HTTP |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` and the metrics and logs variants | A full URL per signal when the signals go to different places |
| `OTEL_METRIC_EXPORT_INTERVAL` | How often `watch` pushes metrics, in milliseconds |
| `OTEL_RESOURCE_ATTRIBUTES` | Extra resource attributes |
| `GRAFANA_TOKEN` | Grafana service account token for annotations |
| `LOKI_BASIC_AUTH` | `user:password` for `--verify-loki` against Grafana Cloud Loki |

Set `service.namespace` in the resource attributes to group runs by the app under test:

```sh
export OTEL_RESOURCE_ATTRIBUTES=service.namespace=inkwell
```

The exporter does not accept `GRAFANA_TOKEN` or `LOKI_BASIC_AUTH` as flags, so neither secret appears in
a process list. Set them from your secret store or CI secrets rather than typing them, because an
interactive shell records typed commands in its history. Both secrets travel only over HTTPS or to
localhost. A plain HTTP URL on another host is refused, and so is a redirect from HTTPS to HTTP.

## What gets sent

One trace per run. The trace ID is a hash of the run ID and the span IDs are hashes of the run ID and
the step number, so exporting the same record twice updates one trace rather than making two. The
root span `dummyrun.run` carries the run's attributes and one `dummyrun.finding` event per finding.
Each step is a child span named `dummyrun.step <action>`.

Seven metrics, named in the contract. Prometheus renames them when it stores them.

| Contract name | Prometheus name |
|---|---|
| `dummyrun.runs` | `dummyrun_runs_total` |
| `dummyrun.run.duration` | `dummyrun_run_duration_seconds_bucket`, `_sum` and `_count` |
| `dummyrun.steps` | `dummyrun_steps_total` |
| `dummyrun.step.duration` | `dummyrun_step_duration_milliseconds_bucket`, `_sum` and `_count` |
| `dummyrun.findings` | `dummyrun_findings_total` |
| `dummyrun.tokens` | `dummyrun_tokens_total` |
| `dummyrun.cost.usd` | `dummyrun_cost_usd_total` |

The two histograms use explicit buckets: 15 s to 3600 s for runs and 100 ms to 60 s for steps.

Each exporter process starts its counters at zero, and every data point carries the time it was sent.
So `export` describes runs as they are exported, and two exports of the same records add nothing to
the totals. For live charts run `dummyrun-otel watch` next to the runner. For history and totals query
the log records, which keep the run's own times.

One log record when a run ends and one per finding, each with the root span's trace and span IDs so
Grafana can jump from the line to the trace.

One region annotation per run from `started_at` to `ended_at`, with the tags the contract lists. Before
posting, the exporter lists annotations with the same tags in the same time window. It skips the run
when one already has the same `time` and `timeEnd`, so re-running an export is safe.

## Historical timestamps

Run records describe the past. The exporter keeps the record's own timestamps wherever the backend
stores them, and stamps the rest at export time. Each claim below is tagged the way docs/STYLE.md asks:
sourced from the backend's documentation, measured by the end-to-end test, or code in this module.

| Signal | Timestamp sent | What the backend does with it |
|---|---|---|
| Spans | The run's `started_at`, each step's `at` and `duration_ms` (code: `internal/telemetry/plan.go`) | Tempo stores the span times as given, so the trace view and a lookup by trace ID show the run as it happened (measured: `e2e/e2e_test.go`) |
| Log records | `ended_at` for the run line, the first step's `at` for a finding (code) | Loki stores the record timestamp, so `count_over_time` and the log panel show history (measured). Two limits apply, described below |
| Metric points | Export time (code: the OpenTelemetry SDK stamps each point when it collects) | Prometheus and Mimir reject a sample older than the head block, the in-memory block that holds the newest two hours, unless `out_of_order_time_window` allows more (sourced). `grafana/otel-lgtm` allows 10 minutes (sourced: its `prometheus.yaml`). Each process starts its counters at zero (code), so `watch` is the mode that gives a live series |
| Annotations | `started_at` and `ended_at` (code) | Grafana stores whatever `time` and `timeEnd` it is given (measured) |

Tempo has one catch. A span more than `ingestion_time_range_slack` (default 2 minutes) from now does not
move the time range of the block it lands in (sourced). The query frontend searches the live-store, the
component that holds the newest traces in memory, for the last `query_backend_after` (15 minutes) and
the backend for anything older (sourced). A block whose range is pinned to its ingestion time is
therefore never where a search over the run's own time range looks (measured: the test found 0 traces
with the stock configuration). A lookup by trace ID always works (measured). To search history in a
local stack, raise the slack under `live_store.wal` and under `block_builder.wal` and `storage.trace.wal`.
`e2e/tempo-config.yaml` shows how, and `integrations/grafana/stack` ships the same settings. Grafana
Cloud sets the slack for you (inferred from the absence of a tenant setting), so there you open
historical traces by ID from the log line or the annotation.

Loki has two limits. It rejects a line older than `reject_old_samples_max_age` (default 7 days) with
HTTP 400 (sourced), so `--logs-max-age` skips those lines and says how many (code). The local stack in
`integrations/grafana/stack` raises the limit to 30 days, so pass `--logs-max-age 720h` there. Loki
also keeps one stream for the exporter and drops a line more than `max_chunk_age / 2` (default 1 hour)
older than the newest line in that stream (sourced, and measured: 14 of 41 lines survived an export in
file name order). `export` therefore sorts every line by its own time and sends the oldest first (code).
Before sending, it compares its oldest line with the newest line already sent to the stream. When the
oldest line is more than an hour older, it exits 1 and sends nothing (code). Within one process that
newest line is what the process has sent so far. With `--verify-loki` it is also the newest line Loki
already holds, so a batch that ends more than an hour before an earlier export is refused rather than
silently lost. Lines Loki already holds are left out first, so repeating an export is still safe.
`watch` applies the same check to each file, counts the file's lines back from Loki after sending, and
reports the file when either step fails. Export runs
in `ended_at` order, and never export a batch that ends more than an hour before one already sent. The
collector in `grafana/otel-lgtm` answers 200 while Loki rejects (measured), so without the Loki URL the
exporter cannot see a loss caused by an earlier process. `--verify-loki` also counts the lines back after
the export and exits 1 naming the runs that are short. Or point `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` at
Loki itself, such as `http://localhost:3100/otlp/v1/logs`, and its rejections become exporter errors.

For charts over history, query Loki within its window or use the annotations. For charts of live
activity, run `dummyrun-otel watch` next to the runner and query Prometheus.

Sources:

- Tempo configuration reference, `ingestion_time_range_slack`, `query_backend_after` and `block_retention`:
  https://grafana.com/docs/tempo/latest/configuration/
- Loki `limits_config`, `reject_old_samples` and `reject_old_samples_max_age`:
  https://grafana.com/docs/loki/latest/configure/
- Prometheus OpenTelemetry guide, out-of-order ingestion and the `otlp-deltatocumulative` flag:
  https://prometheus.io/docs/guides/opentelemetry/
- Mimir out-of-order sample ingestion:
  https://grafana.com/docs/mimir/latest/configure/configure-out-of-order-samples-ingestion/

## Development

```sh
cd integrations/grafana/exporter
go vet ./... && go test -race ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...
go test -tags e2e -timeout 15m -v ./e2e/   # needs Docker and python3
```

The end-to-end test starts `grafana/otel-lgtm` on random ports and exports a copy of `examples/runs`
with its timestamps shifted to two days ago, plus a freshly generated set. It then searches Tempo by `dummyrun.run.id`, counts the lines in Loki and sums
the counters in Prometheus. A second process then exports the same records again: the annotation count
and the Prometheus total must not change, and `--verify-loki` must pass. A third process exports a set
that ended days earlier, and `--verify-loki` must refuse it before sending anything.

`e2e/tempo-config.yaml` is the `grafana/otel-lgtm` Tempo configuration with the slack raised to 30 days
and faster block flushing. `integrations/grafana/stack/config/tempo-config.yaml` carries the same
settings plus a 90 day retention for the local stack.

`internal/record/run-record.schema.json` is a copy of `schema/run-record.schema.json`. The command
`go generate ./internal/record` refreshes it, and a test fails when the two differ.
