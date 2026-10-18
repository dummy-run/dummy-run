# Grafana integration

Everything here follows the contract in [docs/TELEMETRY.md](../../docs/TELEMETRY.md). The guide for
each audience is [docs/integrations/grafana.md](../../docs/integrations/grafana.md).

| Folder | What it is |
|---|---|
| `exporter/` | `dummyrun-otel`, which reads run records and sends traces, metrics, logs and annotations over OTLP |
| `dashboards/` | Grafana dashboards for runs, findings, steps and spend, as provisioning JSON |
| `stack/` | A local Grafana with Tempo, Loki and Prometheus or Mimir, started with one command |
| [alerting/](alerting/README.md) | Four Grafana-managed alert rules, as file provisioning YAML and as Terraform, with a dry run in CI |
| [examples/propagation/](examples/propagation/README.md) | A stub app that proves the persona's `traceparent` and `baggage` reach the app's spans in Tempo |
