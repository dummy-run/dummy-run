# Propagation example

A runnable proof that the headers the persona's browser sends reach an app's spans. The app is a stub
of Inkwell, the fictional publishing app the example personas use. It is Go `net/http` with the
OpenTelemetry SDK. The lines that matter are in `inkwell/main.go`: a composite propagator for
`tracecontext` and `baggage`, and the `baggagecopy` span processor filtered to the two members the
contract names.

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

`persona-request.sh` derives the IDs from the run ID as the contract says and sends `GET /drafts`,
`/payments` and `/reviews` as steps 1 to 3. The trace ID is the first 16 bytes of SHA-256 of
`dummyrun.run:<id>`. The parent in each `traceparent` is that step's span ID, the first 8 bytes of
SHA-256 of `dummyrun.span:<id>:step:<n>`. Each request also carries the `baggage` header from
docs/TELEMETRY.md. The user agent is Chrome's with `DummyRun/0.1.0` at the end. `check_tempo.py` then runs this TraceQL query against
Tempo's search API until the trace arrives:

```traceql
{ resource.service.name = "inkwell" && span.dummyrun.run.id = "2026-10-17-ravi-reviewer-005" }
```

It fails unless the trace ID Tempo returns is the one the script sent, and unless every span in the
trace carries both `dummyrun.run.id` and `dummyrun.persona.id`. It also derives the three step span IDs
itself and fails unless each server span is a child of one of them. A run on 2026-10-18 printed:

```text
trace id: 32d794d3234508fe094bb2afc7eb5dca (sha256 of dummyrun.run:2026-10-17-ravi-reviewer-005, first 16 bytes)
step 1: parent span 2a7010411282690a (sha256 of dummyrun.span:2026-10-17-ravi-reviewer-005:step:1, first 8 bytes)
step 2: parent span 7491ec26188ed427 (sha256 of dummyrun.span:2026-10-17-ravi-reviewer-005:step:2, first 8 bytes)
step 3: parent span 88da3ef451acca7c (sha256 of dummyrun.span:2026-10-17-ravi-reviewer-005:step:3, first 8 bytes)
TraceQL: { resource.service.name = "inkwell" && span.dummyrun.run.id = "2026-10-17-ravi-reviewer-005" }
matched trace 32d794d3234508fe094bb2afc7eb5dca: 6 spans, root <root span not yet received> None
trace 32d794d3234508fe094bb2afc7eb5dca is the one the persona's traceparent named
service  span             parent span        dummyrun.run.id                dummyrun.persona.id
inkwell  GET /drafts      2a7010411282690a   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  GET /payments    7491ec26188ed427   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  GET /reviews     88da3ef451acca7c   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  inkwell.render   1eebd251eda1fa48   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  inkwell.render   525f3caa8db699b2   2026-10-17-ravi-reviewer-005   ravi-reviewer
inkwell  inkwell.render   68bcdfc0516cd51d   2026-10-17-ravi-reviewer-005   ravi-reviewer
all 6 spans carry dummyrun.run.id=2026-10-17-ravi-reviewer-005 and dummyrun.persona.id=ravi-reviewer
GET /drafts is a child of step 1 (span 2a7010411282690a)
GET /payments is a child of step 2 (span 7491ec26188ed427)
GET /reviews is a child of step 3 (span 88da3ef451acca7c)
```

Tempo reports the root span as not received because only the app sends here. The exporter derives
the same trace ID from the run ID, so in a full setup it supplies the `dummyrun.run` root span and the
step spans and the trace is complete.

`inkwell/main_test.go` checks the same thing without Docker, with an in-memory exporter. Both spans
join the trace and the server span's parent is the derived span ID of step 1. The two attributes are present. Two
other baggage members are dropped. A third test checks a request to an unlisted path is named after
the catch-all route. A second test checks the CORS preflight lists
`traceparent`, `tracestate` and `baggage`.

## Pins

| What | Version |
|---|---|
| `grafana/otel-lgtm` | 0.35.0 (2026-10-02), by digest in `compose.yaml` |
| `golang` build image | 1.27.1-alpine (2026-09-21), by digest in `inkwell/Dockerfile` |
| OpenTelemetry Go SDK | 1.47.0 (2026-10-02) |
| `otelhttp` and `baggagecopy` | 0.72.0 and 0.17.0 (2026-10-02) |

The runtime image is `scratch`, so there is nothing else to pin.
