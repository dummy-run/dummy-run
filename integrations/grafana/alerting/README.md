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
| `dummyrun-blocker-finding` | `dummyrun.findings` increased for `severity=blocker` with a verdict other than `rejected` in the last 15 minutes | Prometheus |
| `dummyrun-blocked-run-rate` | Blocked runs were more than 25% of all runs in one environment over 6 hours, for 10 minutes | Prometheus |
| `dummyrun-runs-stopped` | No run ended in 24 hours. Missing data also fires, so a dead exporter is caught | Prometheus |

The rules land in a folder called Dummy Run and a group called `dummyrun` evaluated every minute.
They have no notification policy of their own. Route them with the `severity` label (`high` or
`medium`) or the `dummyrun_kind` label.

## The names are assumed

The contract names metrics in OpenTelemetry form. Prometheus and Mimir translate them on ingest:
dots become underscores, monotonic counters gain `_total` and histograms gain a unit suffix. Loki does
the same to log attributes and stores them as structured metadata. Grafana Cloud and Mimir can turn
the suffixes off, so the names below depend on the stack's settings. Check every name in a running
stack before trusting an alert. Each query in the YAML is marked `CHECK NAME`.

| Contract | Assumed in Prometheus or Loki | Used by |
|---|---|---|
| `dummyrun.runs` (counter, `{run}`) | `dummyrun_runs_total` | blocked-run rate, runs stopped |
| `dummyrun.findings` (counter, `{finding}`) | `dummyrun_findings_total` | blocker |
| `dummyrun.run.duration` (histogram, `s`) | `dummyrun_run_duration_seconds_bucket`, `_sum`, `_count` | dashboards |
| `dummyrun.step.duration` (histogram, `ms`) | `dummyrun_step_duration_milliseconds_bucket`, `_sum`, `_count` | dashboards |
| `dummyrun.cost` (counter, `USD`) | `dummyrun_cost_USD_total`, if the translator keeps unknown units | dashboards |
| attribute `persona.id` | label `dummyrun_persona_id` | all |
| attribute `target.environment` | label `dummyrun_target_environment` | blocked-run rate |
| attribute `outcome` | label `dummyrun_outcome` | blocked-run rate |
| attributes `kind`, `severity`, `verdict` | labels `dummyrun_kind`, `dummyrun_severity`, `dummyrun_verdict` | blocker |
| resource `service.name` on logs | Loki label `service_name` | permission |
| log attributes `dummyrun.finding.kind`, `dummyrun.finding.verdict`, `dummyrun.finding.id`, `dummyrun.finding.title`, `dummyrun.run.id`, `dummyrun.persona.id` | structured metadata `dummyrun_finding_kind` and so on | permission |

The translation rules come from the OpenTelemetry Prometheus compatibility spec
(https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/). The suffix switch
is Mimir's `-distributor.otel-metric-suffixes-enabled`
(https://grafana.com/docs/mimir/latest/references/configuration-parameters/). The Loki mapping is on
its OTLP ingestion page (https://grafana.com/docs/loki/latest/send-data/otel/).

The data sources must have the UIDs `prometheus` and `loki`. The local stack and `grafana/otel-lgtm`
use those. The Terraform takes them as variables.

## Validation

`test/validate.sh` runs in CI (`.github/workflows/alerting.yml`) and on a laptop with Docker. Grafana
has no validate-only mode. The script starts `grafana/grafana` 13.2.3 with the YAML and two dummy data
sources mounted and waits for `/api/health`. Then it checks three things. The log holds no
provisioning error. `GET /api/v1/provisioning/alert-rules` returns every UID in the file with
`provenance=file` and a condition that names one of its own queries. The ruler at
`/api/prometheus/grafana/api/v1/rules` holds the same number of rules. The second check earns its
place because Grafana accepts a rule whose condition names a missing query and logs nothing. The run
takes under 10 seconds once the image is cached.

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
rule follows the Loki alerting page (https://grafana.com/docs/grafana/latest/datasources/loki/alerting/).
The runs-stopped rule follows Grafana's guidance on missing data
(https://grafana.com/docs/grafana/latest/alerting/best-practices/missing-data/).
