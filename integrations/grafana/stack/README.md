# Local Grafana stack

One container from the `grafana/otel-lgtm` image. It runs Grafana and Tempo with Loki and
Prometheus, plus an OpenTelemetry Collector in front of them. The data sources and the dashboards in
[../dashboards/json](../dashboards/json) are provisioned. It is for local use only. Anyone who can
reach port 3000 is a viewer without signing in.

```sh
cd integrations/grafana/stack
docker compose up -d
open http://localhost:3000
```

The image is about 2.5 GB on disk (measured: `docker image inspect`), so the first start takes a
while. The home dashboard links to the four Dummy Run dashboards.
Editing a dashboard in the UI needs the admin account, which keeps Grafana's default password
(`admin` / `admin`) until you change it.

| Port | Service |
|---|---|
| 3000 | Grafana |
| 4317 | OTLP over gRPC |
| 4318 | OTLP over HTTP |
| 9090 | Prometheus |
| 3200 | Tempo |
| 3100 | Loki |

## Point an exporter at it

OTLP is the OpenTelemetry protocol, and gRPC is its binary transport. Set
`OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318` for OTLP over HTTP, or `http://localhost:4317`
for gRPC. No headers or credentials are needed. The ports are bound to 127.0.0.1, so only this
machine can reach them. From another container on the same Docker network the host is
`dummyrun-lgtm`.

## Seed it with synthetic runs

`seed/` holds a test seeder. It is a second implementation of
[docs/TELEMETRY.md](../../../docs/TELEMETRY.md), separate from the product exporter in
`../exporter/`, and it exists to check the dashboards. It turns run records into traces, metrics and
logs, then posts one Grafana annotation per run when `GRAFANA_TOKEN` holds a service account token
with the Editor role. Make one with the admin account:

```sh
auth=admin:admin
sa=$(curl -s -u "$auth" -H 'Content-Type: application/json' -d '{"name":"dummyrun-seed","role":"Editor"}' localhost:3000/api/serviceaccounts | jq -r .id)
export GRAFANA_TOKEN=$(curl -s -u "$auth" -H 'Content-Type: application/json' -d '{"name":"local"}' "localhost:3000/api/serviceaccounts/$sa/tokens" | jq -r .key)
```

Then seed:

```sh
python3 ../../../examples/generate-runs.py --runs 300 --days 14 --out /tmp/runs
cd seed
uv run seed.py /tmp/runs --end-now
```

To send the same records with the product exporter instead, write the shifted records and export
them: `uv run seed.py /tmp/runs --end-now --write /tmp/shifted`, then
`dummyrun-otel export --grafana-url http://localhost:3000 /tmp/shifted`.

`--end-now` moves every timestamp so the latest run ends now and the gaps between runs stay the
same. The generator writes fixed dates, and the stores only accept recent data (code: `config/*.yaml`,
with the image defaults they replace). Loki rejects lines older than `reject_old_samples_max_age`.
Prometheus rejects samples older than its out-of-order window. Tempo clamps block times to the
ingestion time plus `ingestion_time_range_slack`. Loki only asks its ingester about the last three hours, so older lines stay hidden until a
flush. The files in `config/` raise those limits to 30 days and make Loki ask the ingester for any
range, so a seeded fortnight lands at its real times and shows at once. Tempo searches reach the new blocks within a
minute (measured: 300 runs seeded into this stack). Reseeding an old history into a stack that
already holds newer samples fails in Prometheus (inferred: samples must fall inside the out-of-order
window), so run `docker compose down -v` first.

`uv run pytest` runs the seeder's tests. `uv run check_panels.py` sends every panel query through
Grafana's API and prints data or no data per panel, with `--retries` for a stack that is still
flushing. `uv run screenshot.py --out shots` takes a
screenshot of each dashboard with headless Chrome.

## Files

| Path | What it does |
|---|---|
| `compose.yaml` | The one service, pinned by image digest, with the mounts below |
| `grafana/provisioning/datasources/dummyrun.yaml` | Prometheus, Tempo and Loki with the UIDs `prometheus`, `tempo` and `loki` |
| `grafana/provisioning/dashboards/dummyrun.yaml` | Loads `../dashboards/json` into a Dummy Run folder |
| `compose.yaml` mount of `../alerting/provisioning/alerting` | Loads the alert rules from [../alerting](../alerting) |
| `config/prometheus.yaml` | The image default plus a 30 day out of order window |
| `config/tempo-config.yaml` | The image default plus 30 days of slack, faster block flushes, no search length limit and 90 days of retention |
| `config/loki-config.yaml` | The image default plus 30 days for old samples, and queries that always ask the ingester |

Data lives in the `lgtm-data` volume. `docker compose down -v` removes it.
