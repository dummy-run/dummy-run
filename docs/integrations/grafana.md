# Grafana

Dummy Run describes every run in OpenTelemetry. [docs/TELEMETRY.md](../TELEMETRY.md) is the contract.
It gives each run one trace and one Grafana annotation. It adds a log record per run and per finding.
Counters and histograms cover runs and steps. This page says how to use that in Grafana, one section
per audience.

The code is in [integrations/grafana/](../../integrations/grafana/README.md).

## I run Dummy Run

### Five minutes, on your laptop

1. Start the local stack. It runs Grafana with Tempo, Loki and Prometheus, and listens for OTLP on
   port 4318. Grafana Cloud uses Mimir in place of Prometheus; the queries are the same.

   ```sh
   cd integrations/grafana/stack && docker compose up -d
   ```

2. Install the exporter and send it the example runs. It reads run records that match
   `schema/run-record.schema.json` and sends them to `http://localhost:4318` by default.

   ```sh
   go install github.com/dummy-run/dummy-run/integrations/grafana/exporter/cmd/dummyrun-otel@latest
   dummyrun-otel export --logs-max-age 720h examples/runs
   ```

3. Open Grafana at `http://localhost:3000`. The dashboards are already loaded. Search Tempo with `{ resource.service.name = "dummyrun" }` to see one trace per run.

### Then Grafana Cloud

Grafana Cloud takes OTLP over HTTP with protobuf encoding at
`https://otlp-gateway-<region>.grafana.net/otlp` [[1]](#sources). The exporter appends `/v1/traces`,
`/v1/metrics` and `/v1/logs`. Authentication is HTTP basic. The username is your stack's instance ID
and the password is a Cloud Access Policy token with the `metrics:write`, `logs:write` and
`traces:write` scopes [[2]](#sources). Set the standard variables and the exporter picks them up:

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT="https://otlp-gateway-prod-gb-south-1.grafana.net/otlp"
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic $(printf '%s:%s' "$INSTANCE_ID" "$TOKEN" | base64)"
export GRAFANA_TOKEN="<service account token with the Editor role>"   # for run annotations
dummyrun-otel export --grafana-url "https://<your-stack>.grafana.net" runs/
```

`runs/` is the folder of run records. The [exporter README](../../integrations/grafana/exporter/README.md)
lists every flag.

Keep the token in your keychain or in CI secrets. The region slug in the endpoint decides where the
data is stored. `prod-gb-south-1` is the UK and `prod-eu-west-2` is Germany; the full list is in
Grafana's region reference [[3]](#sources).

## I own the app being tested

### What the persona's browser sends

Every request the runner sends to your hosts carries three things (docs/TELEMETRY.md, "Linking to the
app's own telemetry"):

| Header | Example | Meaning |
|---|---|---|
| `traceparent` | `00-32d794d3234508fe094bb2afc7eb5dca-2a7010411282690a-01` | W3C Trace Context. The trace is the run and the parent is the current step's span [[4]](#sources) |
| `baggage` | `dummyrun.run.id=2026-10-17-ravi-reviewer-005,dummyrun.persona.id=ravi-reviewer` | W3C Baggage with the run and persona IDs [[5]](#sources) |
| `User-Agent` | `Mozilla/5.0 (...) Chrome/148.0.0.0 Safari/537.36 DummyRun/0.1.0` | The normal browser string with `DummyRun/<version>` at the end |

The IDs are derived from the run ID, so the exporter and the runner agree without talking to each
other (docs/TELEMETRY.md, Traces). The trace ID is the first 16 bytes of SHA-256 of `dummyrun.run:<id>`.
The root span ID is the first 8 bytes of SHA-256 of `dummyrun.span:<id>:root`, and step `n` has the
span ID from `dummyrun.span:<id>:step:<n>`. During step `n` the runner sends that step's span ID as the
parent in every `traceparent`. The exporter writes the `dummyrun.run` root span and the step spans with
the same IDs. Your app's spans therefore nest under the step that caused them, whether the exporter ran
before or after your app recorded them.

An app instrumented with OpenTelemetry joins the trace by default. The baggage does not reach your
spans by default. OpenTelemetry keeps baggage apart from span attributes and says you must copy it
yourself [[6]](#sources). Each SDK has a processor that does the copy at span start.

### Step 1: copy the baggage onto spans

Add the processor for your SDK and filter it to the two keys `dummyrun.run.id` and
`dummyrun.persona.id`. Copying every baggage key lets any caller write attributes into your traces.

| SDK | Package | Source |
|---|---|---|
| Java | `io.opentelemetry.contrib:opentelemetry-baggage-processor`, class `BaggageSpanProcessor`. The Java agent has it built in behind `otel.java.experimental.span-attributes.copy-from-baggage.include=dummyrun.*` | [[7]](#sources) |
| Go | `go.opentelemetry.io/contrib/processors/baggagecopy`, `NewSpanProcessor(filter)`. The older `processors/baggage/baggagetrace` is deprecated | [[8]](#sources) |
| Python | `opentelemetry-processor-baggage`, `BaggageSpanProcessor(predicate)` | [[9]](#sources) |
| Node | `@opentelemetry/baggage-span-processor`, `new BaggageSpanProcessor(predicate)` | [[10]](#sources) |
| .NET | `OpenTelemetry.Extensions`, `AddBaggageActivityProcessor(predicate)` | [[11]](#sources) |
| Ruby | `opentelemetry-processor-baggage`, `BaggageSpanProcessor.new(predicate)` | [[12]](#sources) |

The Go version, with its test, is in
[integrations/grafana/examples/propagation/](../../integrations/grafana/examples/propagation/README.md).
Run `./run.sh` there and it starts a stub app with `grafana/otel-lgtm`, sends three persona-shaped
requests and proves with a TraceQL query that every span carries `dummyrun.run.id`.

The OpenTelemetry Collector and Grafana Alloy cannot do this copy for you. The `baggage` header exists
only on the request into your app. Collector processors read attributes that are already on the span,
and the `from_context` source reads the headers of the OTLP export request, which is a different
request [[13]](#sources).

### Step 2: check the propagators

The SDK default for `OTEL_PROPAGATORS` is `tracecontext,baggage` [[14]](#sources). If your setup lists
only `tracecontext` or only `b3`, the baggage header is never parsed and step 1 has nothing to copy.

### Step 3: let the headers through

Four things stop the headers before they reach your SDK.

1. CORS. `traceparent`, `tracestate` and `baggage` are not CORS-safelisted request headers, so a
   browser sends a preflight for a cross-origin request that carries them [[15]](#sources). Your API
   must answer with `Access-Control-Allow-Headers: traceparent, tracestate, baggage`. Same-origin
   requests and page navigations are not affected. The stub app shows the preflight handler.
2. Proxies and CDNs. Some edges rewrite `traceparent`. Cloudflare's tracing can accept an incoming
   `traceparent` or forward a new one to your origin, depending on its propagation policy
   [[16]](#sources). Check your edge keeps both headers unchanged.
3. Sampling. The SDK default sampler is `parentbased_always_on`. Dummy Run sets the sampled flag, so
   your spans are recorded. If your app uses a `ParentBased` sampler and a proxy clears the flag to
   `00`, the SDK drops the span [[17]](#sources).
4. Baggage limits. The W3C spec requires support for at least 64 members and 8192 bytes. Dummy Run
   sends two short members, so a limit only bites if your own code adds many more [[5]](#sources).

### Step 4: find the spans in Tempo

TraceQL searches span attributes with the `span.` scope. Dots in attribute names need no quoting
[[18]](#sources).

```traceql
# Everything the app did during one run
{ span.dummyrun.run.id = "2026-10-17-ravi-reviewer-005" }

# Server errors while any persona was active, by persona
{ span.dummyrun.persona.id != "" && span.http.response.status_code >= 500 }
  | by(span.dummyrun.persona.id)

# Traces that hold both the persona's steps and the app's spans
{ resource.service.name = "dummyrun" } && { resource.service.name = "inkwell" }

# Slow app spans for one persona
{ resource.service.name = "inkwell" && span.dummyrun.persona.id = "ravi-reviewer" && duration > 500ms }
```

The second and third queries only return something when the Dummy Run exporter and your app send to
the same Tempo. The trace is the same either way, because the run's trace ID is in the `traceparent`.

### Logs

Your app's log lines carry the trace ID when your logging bridge adds it. In Loki, `{service_name="inkwell"} |= "<trace id>"` lists them, and the trace to logs link in Grafana does the same from a span. A run end record from Dummy Run has the same trace ID, so the two sets of lines meet in Explore.

### Faro, for a frontend team

Faro records the full user agent as `browser.userAgent` and flattens it into the Loki line as
`browser_userAgent` [[19]](#sources). To look at persona sessions alone:

```logql
{app="inkwell"} | logfmt | browser_userAgent =~ ".*DummyRun/.*"
```

To keep them out of your real-user data, Grafana's bot filter pattern tests `navigator.userAgent` and
sets the session sampling rate to 0 for matches; add `DummyRun/` to the pattern [[20]](#sources).
Or return `null` from `beforeSend` for items whose user agent contains the token [[21]](#sources).

Faro's tracing uses the OpenTelemetry fetch instrumentation and adds its own `traceparent` to requests
your page makes, controlled by `propagateTraceHeaderCorsUrls` [[22]](#sources). When the page has set
one, the runner keeps it and adds only the baggage (docs/TELEMETRY.md, Linking to the app's own
telemetry). Faro's client span stays the parent of your server span, so that request sits in Faro's
trace rather than the persona's. The baggage still reaches your server, so searching by
`dummyrun.run.id` finds those spans too.

### Beyla, for an app with no SDK

Beyla reads an incoming `traceparent` and continues the trace, so an app with no SDK still joins the
run's trace [[23]](#sources). The conditions are kernel 5.17 or later for header parsing, and for
non-Go processes `BEYLA_BPF_TRACK_REQUEST_HEADERS=true` [[24]](#sources). Beyla does not parse
`baggage`. The feature request is open [[25]](#sources). It can export raw request headers as
`http.request.header.<name>` attributes when you enable `ebpf.payload_extraction.http.enrichment`, which
is off by default and not available for Go [[24]](#sources). That gives you the baggage string on the
span, and you can search it with `{ span.http.request.header.baggage =~ ".*dummyrun.run.id=2026-10-17-ravi-reviewer-005.*" }`.
Beyla is now Grafana's distribution of OpenTelemetry eBPF Instrumentation [[26]](#sources).

## I watch quality

Dashboards are in `integrations/grafana/dashboards/`. They show runs by outcome and findings by kind
and verdict. They also show step durations and token spend. Every run appears as a region annotation
tagged `dummyrun` plus `persona:<id>` and `scenario:<id>` plus `env:<id>` and `outcome:<outcome>`. Add the
annotation query to your own app dashboards to see when personas were active.

Alert rules are in [integrations/grafana/alerting/](../../integrations/grafana/alerting/README.md), as
Grafana file provisioning and as Terraform:

| Rule | Fires when |
|---|---|
| Confirmed permission finding | A finding log record has `kind=permission` and `verdict=confirmed` in the last 15 minutes. The alert names the run and the finding |
| Blocker | A finding log record has `severity=blocker` and a verdict other than rejected in the last 15 minutes |
| Blocked-run rate | More than 25% of runs against one environment ended `blocked` over 6 hours, for 10 minutes |
| Runs stopped | No run was exported in 24 hours, or the metric is missing |

The rules use the Prometheus and Loki names that OTLP translation produces from the contract names,
such as `dummyrun_runs_total` and `dummyrun_target_environment` [[27]](#sources). They were checked
against the local stack fed by `dummyrun-otel` on 2026-10-18. The two Prometheus rules read counters,
and a counter carries the time the exporter sent it. Run them with `dummyrun-otel watch`, which exports
runs as they end. A batch export of old runs would make them fire today.

### The project tests its own site

Every Monday at 06:00 UTC the `Dogfood` workflow plays the four personas of `casts/dummyrun-dev` through all ten
of their scenarios against https://dummyrun.dev. The target is read-only. The workflow signs in with a short-lived OIDC token. Each run uses a model on Azure OpenAI
and stops at 20 steps and 300,000 tokens. The workflow then runs
`dummyrun-otel` against the project's Grafana Cloud stack, so the runs appear on the dashboards and annotations
described above under `service.namespace=dummyrun-dev`. It keeps the run records for 90 days as the `dogfood-runs`
artifact on the workflow run. The workflow run turns red and says why in its summary when the export fails, when a scenario does not run or when a record is not completed.

## I'm in compliance or security

### What goes to Grafana

Everything the exporter sends is derived from the run record. The full list is in docs/TELEMETRY.md.
The attributes fall into six groups.

| Group | Attributes |
|---|---|
| Identity of the run | `dummyrun.run.id`, `dummyrun.batch.id`, `dummyrun.scenario.id`, `dummyrun.outcome` |
| The persona | `dummyrun.persona.id`, `dummyrun.persona.role`, `dummyrun.persona.device`. Personas are fictional |
| The target | `dummyrun.target.adapter`, `dummyrun.target.environment`, `dummyrun.target.commit`, `url.full` (the preview base URL and page URLs) |
| The model | `gen_ai.system`, `gen_ai.request.model`, `cloud.region`, token counts and cost |
| Each step | action, selector or label used, page URL, HTTP status, duration, counts of console errors and accessibility violations |
| Each finding | ID, kind, severity, title, evidence tag, verdict, step numbers |

Page URLs and selector labels come from your preview. If a preview puts personal data in a URL, that URL
reaches Grafana. Keep previews on synthetic data.

### What never goes

Screenshots and page text stay in the run record and on the results board. So do the persona's notes
(`steps[].note` in the schema) and the console and network bodies. Test-user credentials and provider
keys are never in a run record at all. The exporter sends none of these.

### Baggage is public to whoever receives it

The run and persona IDs travel in a clear-text header on every request to your hosts. If your app
forwards baggage to a third party, that party sees them. The W3C spec says to keep baggage inside a
trust boundary [[5]](#sources). The IDs name a fictional persona and a run, so they are not personal
data, and they still tell an outside service that your preview is under test.

### Keeping it in-region

The run itself stays in the region the market profile allows (docs/ARCHITECTURE.md, Adapters). For
Grafana, choose one of two paths. Run the stack in integrations/grafana/stack/ on your own
infrastructure in the same region. Or pick a Grafana Cloud stack in a region that satisfies the market
and use that region's OTLP gateway [[3]](#sources). `cloud.region` on each trace records where the
model ran, so a dashboard can show that every run respected the profile.

## k6 and Synthetic Monitoring

Grafana k6 runs load tests and scripted browser checks you write in JavaScript [[28]](#sources).
Grafana Synthetic Monitoring runs scheduled checks from Grafana's probe locations. The check types
include HTTP, DNS, TCP and scripted browser checks [[29]](#sources). Dummy Run signs a persona with a role into a preview and
lets it explore. Use k6 and Synthetic Monitoring for the paths you already know, and Dummy Run for the
ones a role might find.

## Sources

1. Grafana Cloud, Send data using OTLP: https://grafana.com/docs/grafana-cloud/send-data/otlp/send-data-otlp/
2. Grafana Cloud, OTLP format considerations: https://grafana.com/docs/grafana-cloud/send-data/otlp/otlp-format-considerations/
3. Grafana Cloud, Regional availability: https://grafana.com/docs/grafana-cloud/account-management/regional-availability/
4. W3C Trace Context: https://www.w3.org/TR/trace-context/
5. W3C Baggage: https://www.w3.org/TR/baggage/
6. OpenTelemetry, Baggage: https://opentelemetry.io/docs/concepts/signals/baggage/
7. opentelemetry-java-contrib, baggage-processor: https://github.com/open-telemetry/opentelemetry-java-contrib/blob/main/baggage-processor/README.md
8. opentelemetry-go-contrib, baggagecopy: https://pkg.go.dev/go.opentelemetry.io/contrib/processors/baggagecopy
9. opentelemetry-python-contrib, processor-baggage: https://github.com/open-telemetry/opentelemetry-python-contrib/tree/main/processor/opentelemetry-processor-baggage
10. opentelemetry-js-contrib, baggage-span-processor: https://github.com/open-telemetry/opentelemetry-js-contrib/tree/main/packages/baggage-span-processor
11. opentelemetry-dotnet-contrib, OpenTelemetry.Extensions: https://github.com/open-telemetry/opentelemetry-dotnet-contrib/blob/main/src/OpenTelemetry.Extensions/README.md
12. opentelemetry-ruby-contrib, processor/baggage: https://github.com/open-telemetry/opentelemetry-ruby-contrib/tree/main/processor/baggage
13. OpenTelemetry Collector, attributes processor: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/attributesprocessor/README.md
14. OpenTelemetry, SDK environment variables: https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/
15. Fetch Standard, CORS-safelisted request header: https://fetch.spec.whatwg.org/#cors-safelisted-request-header
16. Cloudflare, Tracing: https://blog.cloudflare.com/cloudflare-tracing/
17. OpenTelemetry, Tracing SDK, ParentBased sampler: https://opentelemetry.io/docs/specs/otel/trace/sdk/#parentbased
18. Grafana Tempo, Construct a TraceQL query: https://grafana.com/docs/tempo/latest/traceql/construct-traceql-queries/
19. Grafana Faro, browser meta source: https://github.com/grafana/faro-web-sdk/blob/main/packages/web-sdk/src/metas/browser/meta.ts and Alloy faro.receiver payload: https://github.com/grafana/alloy/blob/main/internal/component/faro/receiver/internal/payload/payload.go
20. Grafana Frontend Observability, Filter bots: https://grafana.com/docs/grafana-cloud/observe-and-act/monitor-applications/frontend-observability/configure/filter-bots/
21. Grafana Cloud, Reduce Frontend Observability costs (beforeSend): https://grafana.com/docs/grafana-cloud/platform/cost-management-and-billing/analyze-costs/reduce-costs/frontend-observability-costs/
22. Grafana Frontend Observability, Tracing instrumentation: https://grafana.com/docs/grafana-cloud/observe-and-act/monitor-applications/frontend-observability/instrument/tracing-instrumentation/
23. Grafana Beyla, Distributed traces: https://grafana.com/docs/beyla/latest/distributed-traces/
24. Grafana Beyla, Controlling instrumentation: https://grafana.com/docs/beyla/latest/configure/controlling-instrumentation/
25. grafana/beyla issue 758, baggage support: https://github.com/grafana/beyla/issues/758
26. Grafana Labs, Beyla donation to OpenTelemetry: https://grafana.com/blog/opentelemetry-ebpf-instrumentation-beyla-donation/
27. OpenTelemetry, Prometheus and OpenMetrics compatibility: https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/ and Grafana Loki, Ingest OTLP: https://grafana.com/docs/loki/latest/send-data/otel/
28. Grafana k6: https://grafana.com/docs/k6/latest/
29. Grafana Synthetic Monitoring: https://grafana.com/docs/grafana-cloud/testing/synthetic-monitoring/
