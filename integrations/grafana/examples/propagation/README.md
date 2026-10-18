# Propagation example

A runnable proof that the headers the persona's browser sends reach an app's spans. The app is a stub
of Inkwell, the fictional publishing app the example personas use. It is Go `net/http` with the
OpenTelemetry SDK and 60 lines that matter: a composite propagator for `tracecontext` and `baggage`,
and the `baggagecopy` span processor filtered to keys that start with `dummyrun.`.

```sh
./run.sh          # start, prove, stop. Takes about a minute with the images cached
```

Or step by step:

```sh
docker compose up --build --wait
./persona-request.sh          # three requests shaped like persona steps, one trace
./check-tempo.sh              # TraceQL search by run ID, then the span table
docker compose down -v
```

Grafana is at `http://localhost:3030` (admin / admin) while the stack runs. Host ports avoid the
defaults so this runs beside the full stack in `../../stack/`. Override them with
`PROPAGATION_GRAFANA_PORT`, `PROPAGATION_OTLP_PORT`, `PROPAGATION_TEMPO_PORT` and
`PROPAGATION_INKWELL_PORT`.

## What it proves

`persona-request.sh` derives the trace ID from the run ID as the contract says (the first 16 bytes
of SHA-256 of `dummyrun.run:<id>`) and sends `GET /drafts`, `/payments` and `/reviews`.
Each request carries a `traceparent` for a fresh step span and the `baggage` header from
docs/TELEMETRY.md. The user agent is Chrome's with `DummyRun/0.1.0` at the end. `check_tempo.py` then runs this TraceQL query against
Tempo's search API until the trace arrives:

```traceql
{ resource.service.name = "inkwell" && span.dummyrun.run.id = "2026-10-17-ravi-reviewer-005" }
```

It fails unless the trace ID Tempo returns is the one the script sent, and unless every span in the
trace carries both `dummyrun.run.id` and `dummyrun.persona.id`. A run on 2026-10-18 printed:

```text
trace id: 32d794d3234508fe094bb2afc7eb5dca (sha256 of dummyrun.run:2026-10-17-ravi-reviewer-005, first 16 bytes)
TraceQL: { resource.service.name = "inkwell" && span.dummyrun.run.id = "2026-10-17-ravi-reviewer-005" }
matched trace 32d794d3234508fe094bb2afc7eb5dca: 6 spans, root <root span not yet received> None
trace 32d794d3234508fe094bb2afc7eb5dca is the one the persona's traceparent named
service  span             parent span        dummyrun.run.id                dummyrun.persona.id
inkwell  inkwell.render   946f3a26d709fea9   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  inkwell.render   492bd0a709202e85   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  inkwell.render   32ac415b7e39cc13   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  GET /drafts      99edeccdab1732bc   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  GET /payments    9be6c4f7bb7ae27c   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  GET /reviews     22b2cf7709369a37   2026-10-17-ravi-reviewer-005   ravi-reviewer
all 6 spans carry dummyrun.run.id=2026-10-17-ravi-reviewer-005 and dummyrun.persona.id=ravi-reviewer
```

Tempo reports the root span as not received because only the app sends here. The exporter derives
the same trace ID from the run ID, so in a full setup it supplies the `dummyrun.run` root span and the
step spans and the trace is complete.

`inkwell/main_test.go` checks the same thing without Docker, with an in-memory exporter. Both spans
join the trace and the server span's parent is the step span. The two attributes are present. A third
baggage member outside `dummyrun.` is dropped. A second test checks the CORS preflight lists
`traceparent`, `tracestate` and `baggage`.

## Pins

| What | Version |
|---|---|
| `grafana/otel-lgtm` | 0.35.0 (2026-10-02), by digest in `compose.yaml` |
| `golang` build image | 1.27.1-alpine (2026-09-21), by digest in `inkwell/Dockerfile` |
| OpenTelemetry Go SDK | 1.47.0 (2026-10-02) |
| `otelhttp` and `baggagecopy` | 0.72.0 and 0.17.0 (2026-10-02) |

The runtime image is `scratch`, so there is nothing else to pin.
