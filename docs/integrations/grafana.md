# Grafana

Each run can go to Grafana as one trace and one annotation, plus a log line when the run ends and one
for each finding. Counters and histograms cover runs and steps. [Telemetry](../TELEMETRY.md) lists
every field. This page shows how to use them in Grafana, with a section for each kind of reader.

The code is in [integrations/grafana/](../../integrations/grafana/README.md).

## I run Dummy Run

### Five minutes, on your laptop

1. Start the local stack. It runs Grafana with Tempo, Loki and Prometheus, and listens for OTLP on
   port 4318. Grafana Cloud uses Mimir in place of Prometheus, and the queries are the same.

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

To send runs to Grafana Cloud, set the standard OpenTelemetry variables and point the exporter at your
stack:

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT="https://otlp-gateway-prod-gb-south-1.grafana.net/otlp"
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic $(printf '%s:%s' "$INSTANCE_ID" "$TOKEN" | base64)"
export GRAFANA_TOKEN="<service account token with the Editor role>"   # for run annotations
dummyrun-otel export --grafana-url "https://<your-stack>.grafana.net" runs/
```

`runs/` is your folder of run records. The [exporter README](../../integrations/grafana/exporter/README.md)
lists every flag.

Grafana Cloud takes OTLP over HTTP with protobuf at `https://otlp-gateway-<region>.grafana.net/otlp`, as
its guide to [sending data with OTLP](https://grafana.com/docs/grafana-cloud/send-data/otlp/send-data-otlp/)
explains. The exporter adds `/v1/traces`, `/v1/metrics` and `/v1/logs` itself. Sign-in is HTTP basic
auth. The username is your stack's instance ID, and the password is a Cloud Access Policy token with the
`metrics:write`, `logs:write` and `traces:write` scopes.

Keep the token in your keychain or in CI secrets. The region slug in the endpoint decides where your data
is stored. `prod-gb-south-1` is the UK and `prod-eu-west-2` is Germany. Grafana's
[regional availability](https://grafana.com/docs/grafana-cloud/account-management/regional-availability/)
page has the full list.

## I own the app being tested

### What the persona's browser sends

Every request the runner sends to your hosts carries these headers (see
"Linking to the app's own telemetry" in [TELEMETRY.md](../TELEMETRY.md)):

| Header | Example | Meaning |
|---|---|---|
| `traceparent` | `00-32d794d3234508fe094bb2afc7eb5dca-2a7010411282690a-01` | [W3C Trace Context](https://www.w3.org/TR/trace-context/). The trace is the run and the parent is the current step's span |
| `baggage` | `dummyrun.run.id=2026-10-17-ravi-reviewer-005,dummyrun.persona.id=ravi-reviewer` | [W3C Baggage](https://www.w3.org/TR/baggage/) with the run and persona IDs |
| `User-Agent` | `Mozilla/5.0 (...) Chrome/148.0.0.0 Safari/537.36 DummyRun/0.1.0` | The normal browser string with `DummyRun/<version>` at the end |

The runner and the exporter both derive the span IDs from the run ID, as "Traces" in [TELEMETRY.md](../TELEMETRY.md)
describes. During step `n`, the runner sends that step's span ID as the parent in every `traceparent`,
and the exporter later writes the step span with the same ID. So your app's spans nest under the step
that caused them, whether you export the run before or after your app records them.

If your app uses OpenTelemetry, it joins the trace without any changes. The baggage is a different
story. OpenTelemetry keeps [baggage](https://opentelemetry.io/docs/concepts/signals/baggage/) apart from
span attributes, so you have to copy it onto your spans yourself. Each SDK has a processor that does
this when a span starts.

### Step 1: copy the baggage onto spans

Add the processor for your SDK, and filter it to the two keys `dummyrun.run.id` and
`dummyrun.persona.id`. If you copy every baggage key, any caller can write attributes into your traces.

| SDK | Package |
|---|---|
| Java | [`io.opentelemetry.contrib:opentelemetry-baggage-processor`](https://github.com/open-telemetry/opentelemetry-java-contrib/blob/main/baggage-processor/README.md), class `BaggageSpanProcessor`. The Java agent has it built in behind `otel.java.experimental.span-attributes.copy-from-baggage.include=dummyrun.*` |
| Go | [`go.opentelemetry.io/contrib/processors/baggagecopy`](https://pkg.go.dev/go.opentelemetry.io/contrib/processors/baggagecopy), `NewSpanProcessor(filter)`. The older `processors/baggage/baggagetrace` is deprecated |
| Python | [`opentelemetry-processor-baggage`](https://github.com/open-telemetry/opentelemetry-python-contrib/tree/main/processor/opentelemetry-processor-baggage), `BaggageSpanProcessor(predicate)` |
| Node | [`@opentelemetry/baggage-span-processor`](https://github.com/open-telemetry/opentelemetry-js-contrib/tree/main/packages/baggage-span-processor), `new BaggageSpanProcessor(predicate)` |
| .NET | [`OpenTelemetry.Extensions`](https://github.com/open-telemetry/opentelemetry-dotnet-contrib/blob/main/src/OpenTelemetry.Extensions/README.md), `AddBaggageActivityProcessor(predicate)` |
| Ruby | [`opentelemetry-processor-baggage`](https://github.com/open-telemetry/opentelemetry-ruby-contrib/tree/main/processor/baggage), `BaggageSpanProcessor.new(predicate)` |

There's a working Go example, with a test, in
[integrations/grafana/examples/propagation/](../../integrations/grafana/examples/propagation/README.md).
Run `./run.sh` there. It starts a stub app with `grafana/otel-lgtm`, sends three requests shaped like a
persona's, and runs a TraceQL query to show that every span carries `dummyrun.run.id`.

The OpenTelemetry Collector and Grafana Alloy can't do this copy for you. The `baggage` header only
exists on the request into your app. Collector processors read attributes already on the span, and the
[attributes processor's](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/attributesprocessor/README.md)
`from_context` source reads the headers of the OTLP export request, which is a different request.

### Step 2: check the propagators

The SDK default for `OTEL_PROPAGATORS` is `tracecontext,baggage` (see the
[SDK environment variables](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/)).
If your setup lists only `tracecontext` or only `b3`, the baggage header is never parsed and step 1 has
nothing to copy.

### Step 3: let the headers through

Four things can stop the headers before they reach your SDK.

1. CORS. `traceparent`, `tracestate` and `baggage` aren't
   [CORS-safelisted request headers](https://fetch.spec.whatwg.org/#cors-safelisted-request-header), so
   the browser sends a preflight for a cross-origin request that carries them. Your API must answer with
   `Access-Control-Allow-Headers: traceparent, tracestate, baggage`. Same-origin requests and page
   navigations aren't affected. The stub app shows a preflight handler.
2. Proxies and CDNs. Some edges rewrite `traceparent`. Depending on its propagation policy,
   [Cloudflare's tracing](https://blog.cloudflare.com/cloudflare-tracing/) can accept an incoming
   `traceparent` or forward a new one to your origin. Check that your edge passes both headers through
   unchanged.
3. Sampling. The SDK's default sampler is `parentbased_always_on`, and Dummy Run sets the sampled flag,
   so your spans are recorded. If a proxy clears the flag to `00` and your app uses a
   [`ParentBased` sampler](https://opentelemetry.io/docs/specs/otel/trace/sdk/#parentbased), the SDK
   drops the span.
4. Baggage limits. The W3C spec requires support for at least 64 members and 8192 bytes. Dummy Run sends
   two short members, so you'll only hit a limit if your own code adds many more.

### Step 4: find the spans in Tempo

TraceQL searches span attributes with the `span.` scope, and dots in attribute names need no quoting.
Grafana's guide to [TraceQL queries](https://grafana.com/docs/tempo/latest/traceql/construct-traceql-queries/)
covers the syntax.

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

The second and third queries only return results when the Dummy Run exporter and your app send to the
same Tempo. The trace is the same either way, because the run's trace ID is in the `traceparent`.

### Logs

Your app's log lines carry the trace ID when your logging bridge adds it. In Loki, `{service_name="inkwell"} |= "<trace id>"` lists them, and Grafana's trace to logs link does the same from a span. The run-end line from Dummy Run has the same trace ID, so both sets of lines meet in Explore.

### Faro, for a frontend team

Faro records the full user agent as `browser.userAgent`, and the Alloy receiver flattens it into the
Loki line as `browser_userAgent`. To look at persona sessions on their own:

```logql
{app="inkwell"} | logfmt | browser_userAgent =~ ".*DummyRun/.*"
```

To keep personas out of your real-user data, add `DummyRun/` to the pattern in Grafana's
[bot filter](https://grafana.com/docs/grafana-cloud/observe-and-act/monitor-applications/frontend-observability/configure/filter-bots/).
It tests `navigator.userAgent` and sets the session sampling rate to 0 for matches. You can also return
`null` from `beforeSend` for items whose user agent contains the token, as Grafana's
[cost guide](https://grafana.com/docs/grafana-cloud/platform/cost-management-and-billing/analyze-costs/reduce-costs/frontend-observability-costs/)
shows.

Faro's [tracing](https://grafana.com/docs/grafana-cloud/observe-and-act/monitor-applications/frontend-observability/instrument/tracing-instrumentation/)
uses the OpenTelemetry fetch instrumentation, and adds its own `traceparent` to requests your page makes
to the URLs in `propagateTraceHeaderCorsUrls`. When the page has set one, the runner keeps it and adds
only the baggage. Faro's client span stays the parent of your server span, so that request sits in
Faro's trace instead of the persona's. The baggage still reaches your server, so a search by
`dummyrun.run.id` finds those spans too.

### Beyla, for an app with no SDK

[Beyla](https://grafana.com/docs/beyla/latest/distributed-traces/) reads an incoming `traceparent` and
continues the trace, so an app with no SDK still joins the run's trace. It needs kernel 5.17 or later to
parse headers, and `BEYLA_BPF_TRACK_REQUEST_HEADERS=true` for processes not written in Go.

Beyla doesn't parse `baggage` ([issue 758](https://github.com/grafana/beyla/issues/758) asks for it). It
can export raw request headers as `http.request.header.<name>` attributes when you turn on
`ebpf.payload_extraction.http.enrichment`, which is off by default and not available for Go (see
[controlling instrumentation](https://grafana.com/docs/beyla/latest/configure/controlling-instrumentation/)).
That puts the baggage string on the span, and you can search it with `{ span.http.request.header.baggage =~ ".*dummyrun.run.id=2026-10-17-ravi-reviewer-005.*" }`.

## I watch quality

The dashboards in `integrations/grafana/dashboards/` show runs by outcome, findings by kind and verdict,
step durations and token spend. Every run appears as a region annotation tagged `dummyrun`. It also has the
tags `persona:<id>` and `scenario:<id>`, plus `env:<id>` and `outcome:<outcome>`. Add the annotation query to your
own app's dashboards to see when personas were active.

Alert rules are in [integrations/grafana/alerting/](../../integrations/grafana/alerting/README.md), both
as Grafana file provisioning and as Terraform:

| Rule | Fires when |
|---|---|
| Confirmed permission finding | A finding log record has `kind=permission` and `verdict=confirmed` in the last 15 minutes. The alert names the run and the finding |
| Blocker | A finding log record has `severity=blocker` and a verdict other than rejected in the last 15 minutes |
| Blocked-run rate | More than 25% of runs against one environment ended `blocked` over 6 hours, for 10 minutes |
| Runs stopped | No run was exported in 24 hours, or the metric is missing |

The rules use the names that OTLP translation produces in Prometheus and Loki, such as
`dummyrun_runs_total` and `dummyrun_target_environment`. The
[Prometheus compatibility spec](https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/)
and Loki's [OTLP guide](https://grafana.com/docs/loki/latest/send-data/otel/) explain the mapping.

The two Prometheus rules read counters, and a counter carries the time the exporter sent it. So run
them with `dummyrun-otel watch`, which exports runs as they end. A batch export of old runs would make
them fire today.

## I'm in compliance or security

### What goes to Grafana

Everything the exporter sends comes from the run record, and [Telemetry](../TELEMETRY.md) has the full
list. The attributes fall into these groups:

| Group | Attributes |
|---|---|
| Identity of the run | `dummyrun.run.id`, `dummyrun.batch.id`, `dummyrun.scenario.id`, `dummyrun.outcome` |
| The persona | `dummyrun.persona.id`, `dummyrun.persona.role`, `dummyrun.persona.device`. Personas are fictional |
| The target | `dummyrun.target.adapter`, `dummyrun.target.environment`, `dummyrun.target.commit`, `url.full` (the preview base URL and page URLs) |
| The model | `gen_ai.system`, `gen_ai.request.model`, `cloud.region`, token counts and cost |
| Each step | action, selector or label used, page URL, HTTP status, duration, counts of console errors and accessibility violations |
| Each finding | ID, kind, severity, title, evidence tag, verdict, step numbers |

Page URLs and selector labels come from your preview. If a preview puts personal data in a URL, that URL
reaches Grafana, so keep previews on synthetic data.

### What never goes

Screenshots and page text stay in the run record. So do the persona's notes (`steps[].note` in the
schema) and the console and network bodies. Test-user passwords and provider keys are never in a run
record at all. The exporter sends none of these.

### Baggage is public to whoever receives it

The run and persona IDs travel in a plain-text header on every request to your hosts. If your app
forwards baggage to a third party, that party sees them. The W3C Baggage spec says to keep baggage
inside a trust boundary. The IDs name a fictional persona and a run, so they aren't personal data. They
do tell an outside service that your preview is under test.

### Keeping it in-region

The model provider sees screenshots and page text in the region you pick with `--region`. Grafana sees
the telemetry wherever you host it. To keep both in one market, either run the stack in
`integrations/grafana/stack/` on your own infrastructure in that region, or pick a Grafana Cloud stack
in that region and use its OTLP gateway. Each trace records where the model ran in `cloud.region`, so a
dashboard can show that every run stayed in-region.

## k6 and Synthetic Monitoring

[Grafana k6](https://grafana.com/docs/k6/latest/) runs load tests and scripted browser checks that you
write in JavaScript. [Synthetic Monitoring](https://grafana.com/docs/grafana-cloud/testing/synthetic-monitoring/)
runs scheduled HTTP, DNS, TCP and scripted browser checks from Grafana's probe locations. Dummy Run
signs a persona into your preview with a role and lets it explore. Use k6 and Synthetic Monitoring for
the paths you already know, and Dummy Run to find the ones you don't.
