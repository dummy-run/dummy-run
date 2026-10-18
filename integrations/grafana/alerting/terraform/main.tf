# The same four rules as ../provisioning/alerting/dummyrun-rules.yaml, for teams that manage
# Grafana with Terraform. Keep the two files in step: the YAML is the reference and its header
# explains the names and the `dummyrun-otel watch` caveat on the two Prometheus rules.

terraform {
  required_version = ">= 1.9"
  required_providers {
    grafana = {
      source  = "grafana/grafana"
      version = "4.47.0" # released 2026-09-30
    }
  }
}

provider "grafana" {
  url  = var.grafana_url
  auth = var.grafana_auth
}

variable "grafana_url" {
  type        = string
  description = "Grafana URL, for example http://localhost:3000 or https://<stack>.grafana.net"
}

variable "grafana_auth" {
  type        = string
  description = "Service account token, or admin:password for a local stack. Pass it as TF_VAR_grafana_auth, never in a file."
  sensitive   = true
}

variable "prometheus_uid" {
  type        = string
  default     = "prometheus"
  description = "UID of the Prometheus or Mimir data source that receives Dummy Run metrics"
}

variable "loki_uid" {
  type        = string
  default     = "loki"
  description = "UID of the Loki data source that receives Dummy Run logs"
}

variable "blocked_rate_threshold" {
  type        = number
  default     = 0.25
  description = "Blocked-run share of all runs in an environment over 6 hours that triggers an alert"
}

resource "grafana_folder" "dummyrun" {
  title = "Dummy Run"
  uid   = "dummyrun"
}

locals {
  threshold = { type = "__expr__", uid = "__expr__" }
}

resource "grafana_rule_group" "dummyrun" {
  name             = "dummyrun"
  folder_uid       = grafana_folder.dummyrun.uid
  interval_seconds = 60

  rule {
    name           = "Dummy Run confirmed a permission finding"
    uid            = "dummyrun-permission-confirmed"
    condition      = "C"
    for            = "0s"
    no_data_state  = "OK"
    exec_err_state = "Error"
    annotations = {
      summary     = "Run {{ $labels.dummyrun_run_id }} ({{ $labels.dummyrun_persona_id }} on {{ $labels.dummyrun_target_environment }}) confirmed a permission finding: {{ $labels.dummyrun_finding_title }}"
      description = "Finding {{ $labels.dummyrun_finding_id }}. Open the run on the results board and check the screenshot before raising a ticket."
    }
    labels = { dummyrun_kind = "permission", severity = "high" }

    data {
      ref_id         = "A"
      datasource_uid = var.loki_uid
      relative_time_range {
        from = 900
        to   = 0
      }
      model = jsonencode({
        refId     = "A"
        queryType = "instant"
        expr      = <<-EOT
          sum by (dummyrun_run_id, dummyrun_persona_id, dummyrun_target_environment, dummyrun_finding_id, dummyrun_finding_title) (
            count_over_time({service_name="dummyrun"}
              | dummyrun_finding_kind="permission"
              | dummyrun_finding_verdict="confirmed" [15m])
          )
        EOT
      })
    }
    data {
      ref_id         = "C"
      datasource_uid = "__expr__"
      relative_time_range {
        from = 0
        to   = 0
      }
      model = jsonencode({
        refId      = "C"
        type       = "threshold"
        expression = "A"
        datasource = local.threshold
        conditions = [{ evaluator = { type = "gt", params = [0] } }]
      })
    }
  }

  rule {
    name           = "Dummy Run found a blocker"
    uid            = "dummyrun-blocker-finding"
    condition      = "C"
    for            = "0s"
    no_data_state  = "OK"
    exec_err_state = "Error"
    annotations = {
      summary     = "Run {{ $labels.dummyrun_run_id }} ({{ $labels.dummyrun_persona_id }} on {{ $labels.dummyrun_target_environment }}) hit a blocker: {{ $labels.dummyrun_finding_title }}"
      description = "Finding {{ $labels.dummyrun_finding_id }}. A blocker stopped the persona from doing its job. Check the step and screenshot on the results board."
    }
    labels = { dummyrun_kind = "blocker", severity = "high" }

    data {
      ref_id         = "A"
      datasource_uid = var.loki_uid
      relative_time_range {
        from = 900
        to   = 0
      }
      model = jsonencode({
        refId     = "A"
        queryType = "instant"
        expr      = <<-EOT
          sum by (dummyrun_run_id, dummyrun_persona_id, dummyrun_target_environment, dummyrun_finding_id, dummyrun_finding_title) (
            count_over_time({service_name="dummyrun"}
              | dummyrun_finding_severity="blocker"
              | dummyrun_finding_verdict!="rejected" [15m])
          )
        EOT
      })
    }
    data {
      ref_id         = "C"
      datasource_uid = "__expr__"
      relative_time_range {
        from = 0
        to   = 0
      }
      model = jsonencode({
        refId      = "C"
        type       = "threshold"
        expression = "A"
        datasource = local.threshold
        conditions = [{ evaluator = { type = "gt", params = [0] } }]
      })
    }
  }

  rule {
    name           = "Dummy Run blocked-run rate is high"
    uid            = "dummyrun-blocked-run-rate"
    condition      = "C"
    for            = "10m"
    no_data_state  = "OK"
    exec_err_state = "Error"
    annotations = {
      summary     = "{{ printf \"%.0f\" (mul $values.A.Value 100) }}% of runs against {{ $labels.dummyrun_target_environment }} were blocked in the last 6 hours"
      description = "Threshold is ${var.blocked_rate_threshold * 100}%. Compare the blocked runs on the dashboard to see whether one scenario or one persona accounts for them."
    }
    labels = { dummyrun_kind = "blocked-rate", severity = "medium" }

    data {
      ref_id         = "A"
      datasource_uid = var.prometheus_uid
      relative_time_range {
        from = 21600
        to   = 0
      }
      model = jsonencode({
        refId   = "A"
        instant = true
        expr    = "sum by (dummyrun_target_environment) (increase(dummyrun_runs_total{dummyrun_outcome=\"blocked\"}[6h])) / sum by (dummyrun_target_environment) (increase(dummyrun_runs_total[6h]))"
      })
    }
    data {
      ref_id         = "C"
      datasource_uid = "__expr__"
      relative_time_range {
        from = 0
        to   = 0
      }
      model = jsonencode({
        refId      = "C"
        type       = "threshold"
        expression = "A"
        datasource = local.threshold
        conditions = [{ evaluator = { type = "gt", params = [var.blocked_rate_threshold] } }]
      })
    }
  }

  rule {
    name           = "Dummy Run has not run for a day"
    uid            = "dummyrun-runs-stopped"
    condition      = "C"
    for            = "0s"
    no_data_state  = "Alerting"
    exec_err_state = "Error"
    annotations = {
      summary     = "No Dummy Run run has ended in the last 24 hours"
      description = "Check the runner host and the exporter. If runs are not expected (for example a freeze), pause this rule."
    }
    labels = { dummyrun_kind = "runs-stopped", severity = "medium" }

    data {
      ref_id         = "A"
      datasource_uid = var.prometheus_uid
      relative_time_range {
        from = 86400
        to   = 0
      }
      model = jsonencode({
        refId   = "A"
        instant = true
        expr    = "sum(increase(dummyrun_runs_total[24h])) or vector(0)"
      })
    }
    data {
      ref_id         = "C"
      datasource_uid = "__expr__"
      relative_time_range {
        from = 0
        to   = 0
      }
      model = jsonencode({
        refId      = "C"
        type       = "threshold"
        expression = "A"
        datasource = local.threshold
        conditions = [{ evaluator = { type = "lt", params = [1] } }]
      })
    }
  }
}
