# Alert rules

Four Grafana-managed alert rules for Dummy Run, written against the metric and log names in
[docs/TELEMETRY.md](../../../docs/TELEMETRY.md).

| File | Use it when |
|---|---|
| [provisioning/alerting/dummyrun-rules.yaml](provisioning/alerting/dummyrun-rules.yaml) | You provision Grafana from files. Mount the `provisioning/alerting` directory under `GF_PATHS_PROVISIONING` |
| [terraform/main.tf](terraform/main.tf) | You manage Grafana with Terraform and the `grafana/grafana` provider |
| [test/validate.sh](test/validate.sh) | You changed a rule. It starts the pinned Grafana image with the YAML mounted and checks the result |

## The rules

| UID | Fires when | Reads |
|---|---|---|
| `dummyrun-permission-confirmed` | A finding log record with `kind=permission` and `verdict=confirmed` arrived in the last 15 minutes. One alert per finding, with the run ID and title in the summary | Loki |
| `dummyrun-blocker-finding` | A finding log record with `severity=blocker` and a verdict other than `rejected` arrived in the last 15 minutes. Same shape as the permission rule | Loki |
| `dummyrun-blocked-run-rate` | Blocked runs were more than 25% of all runs in one environment over 6 hours, for 10 minutes | Prometheus |
| `dummyrun-runs-stopped` | No run was exported in 24 hours. Missing data also fires, so a dead exporter is caught | Prometheus |

The rules land in a folder called Dummy Run and a group called `dummyrun` evaluated every minute.
They have no notification policy of their own. Route them with the `severity` label (`high` or
`medium`) or the `dummyrun_kind` label.

## Logs for findings, counters for rates

A finding log record carries the run's own time, so the two finding rules read Loki and behave the same
whether runs are exported live or in a batch. A counter data point carries the time the exporter sent
it, and each exporter process starts its counters at zero. A batch export of runs from last week makes
`dummyrun_runs_total` rise today. The two Prometheus rules are therefore only meaningful with
`dummyrun-otel watch`, which exports each run as it ends. During the integration review a
metric-based blocker rule fired on runs that were days old after a batch export, which is why the
blocker rule now reads Loki.

## The names

The contract names metrics in OpenTelemetry form. Prometheus translates them on ingest: dots become
underscores, monotonic counters gain `_total` and histograms gain a unit suffix. Loki does the same to
log attributes and stores them as structured metadata. The names below were checked against the stack
in `integrations/grafana/stack/` fed by `dummyrun-otel` on 2026-10-18. Every one resolved. Mimir and
Grafana Cloud can turn the suffixes off, which would drop `_total`; the local stack keeps them.

| Contract | In Prometheus or Loki | Used by |
|---|---|---|
| `dummyrun.runs` (counter, `{run}`) | `dummyrun_runs_total` | blocked-run rate, runs stopped |
| `dummyrun.findings` (counter, `{finding}`) | `dummyrun_findings_total` | dashboards |
| `dummyrun.run.duration` (histogram, `s`) | `dummyrun_run_duration_seconds_bucket`, `_sum`, `_count` | dashboards |
| `dummyrun.step.duration` (histogram, `ms`) | `dummyrun_step_duration_milliseconds_bucket`, `_sum`, `_count` | dashboards |
| `dummyrun.cost.usd` (counter, `{USD}`) | `dummyrun_cost_usd_total` | dashboards |
| attribute `target.environment` | label `dummyrun_target_environment` | blocked-run rate |
| attribute `outcome` | label `dummyrun_outcome` | blocked-run rate |
| resource `service.name` on logs | Loki label `service_name` | permission, blocker |
| log attributes `dummyrun.finding.kind`, `dummyrun.finding.severity`, `dummyrun.finding.verdict`, `dummyrun.finding.id`, `dummyrun.finding.title`, `dummyrun.run.id`, `dummyrun.persona.id`, `dummyrun.target.environment` | structured metadata `dummyrun_finding_kind` and so on | permission, blocker |

The translation rules come from the OpenTelemetry Prometheus compatibility spec
(https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/). The suffix switch
is Mimir's `-distributor.otel-metric-suffixes-enabled`
(https://grafana.com/docs/mimir/latest/references/configuration-parameters/). The Loki mapping is on
its OTLP ingestion page (https://grafana.com/docs/loki/latest/send-data/otel/).

The data sources must have the UIDs `prometheus` and `loki`. The local stack and `grafana/otel-lgtm`
use those. The Terraform takes them as variables.

## Validation

`test/validate.sh` runs in CI (`.github/workflows/alerting.yml`) and on a laptop with Docker. Grafana
has no validate-only mode. The script starts `grafana/grafana` 13.2.2 with the YAML and two dummy data
sources mounted and waits for `/api/health`. That is the Grafana version inside `grafana/otel-lgtm`
0.35.0, which the local stack runs. Then it checks three things. The log holds no provisioning
error. `GET /api/v1/provisioning/alert-rules` returns every UID in the file with `provenance=file` and a
condition that names one of its own queries. The ruler at `/api/prometheus/grafana/api/v1/rules` holds
the same number of rules. The second check earns its place because Grafana accepts a rule whose
condition names a missing query and logs nothing. A run on 2026-10-18 with the image already pulled
took 8 seconds (measured); the first run also pulls the image.

The Terraform is checked with `terraform fmt -check`, `terraform init -backend=false` and
`terraform validate` against provider 4.47.0, with the hashes in `.terraform.lock.hcl`.

## Applying the Terraform

```sh
cd terraform
export TF_VAR_grafana_url=http://localhost:3000
export TF_VAR_grafana_auth=admin:admin        # or a service account token
terraform init && terraform apply
```

The YAML follows Grafana's file provisioning page
(https://grafana.com/docs/grafana/latest/alerting/set-up/provision-alerting-resources/file-provisioning/).
The Terraform follows the `grafana_rule_group` resource
(https://registry.terraform.io/providers/grafana/grafana/latest/docs/resources/rule_group). The Loki
rules follow the Loki alerting page (https://grafana.com/docs/grafana/latest/datasources/loki/alerting/).
The runs-stopped rule follows Grafana's guidance on missing data
(https://grafana.com/docs/grafana/latest/alerting/best-practices/missing-data/).
