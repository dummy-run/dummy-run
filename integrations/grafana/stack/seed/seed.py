#!/usr/bin/env python3
"""Test seeder for the local Grafana stack. This is not the Dummy Run exporter.

It reads run records (schema/run-record.schema.json) and sends them as OpenTelemetry traces,
metrics and logs following docs/TELEMETRY.md, then posts one Grafana annotation per run. It is a
second, independent implementation of the telemetry contract, written to check the dashboards
before the real exporter in integrations/grafana/exporter/ exists.

Usage:
  uv run seed.py RUNS_DIR_OR_FILES... [--otlp http://localhost:4318] [--grafana http://localhost:3000]
                 [--end-now] [--no-annotations] [--namespace NAME] [--dry-run]

--end-now shifts every timestamp so the latest run ends now and the spacing between runs stays the
same. Use it with examples/generate-runs.py output, whose dates are fixed.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import sys
import urllib.error
import urllib.request
from collections import defaultdict
from datetime import datetime, timedelta, timezone
from pathlib import Path

SERVICE_NAME = "dummyrun"
ROOT_SPAN = "dummyrun.run"
FINDING_EVENT = "dummyrun.finding"

# OpenTelemetry SDK default explicit bucket boundaries, used for both histograms.
BUCKETS = [0.0, 5.0, 10.0, 25.0, 50.0, 75.0, 100.0, 250.0, 500.0, 750.0, 1000.0, 2500.0, 5000.0, 7500.0, 10000.0]

METRICS = {
    "dummyrun.runs": ("counter", "{run}", "Runs by persona, scenario, environment and outcome"),
    "dummyrun.run.duration": ("histogram", "s", "Run duration"),
    "dummyrun.steps": ("counter", "{step}", "Steps by persona and action"),
    "dummyrun.step.duration": ("histogram", "ms", "Step duration"),
    "dummyrun.findings": ("counter", "{finding}", "Findings by persona, environment, kind, severity and verdict"),
    "dummyrun.tokens": ("counter", "{token}", "Model tokens by persona, system, model and direction"),
    "dummyrun.cost.usd": ("counter", "{USD}", "Model cost in US dollars by persona, system and model"),
}


def urlopen(request, timeout: float = 30):
    """urllib.request.urlopen limited to http and https, so a URL can never name a local file."""
    url = request if isinstance(request, str) else request.full_url
    if not url.startswith(("http://", "https://")):
        raise ValueError(f"not an http URL: {url}")
    return urllib.request.urlopen(request, timeout=timeout)  # nosemgrep: python.lang.security.audit.dynamic-urllib-use-detected.dynamic-urllib-use-detected


# ---------------------------------------------------------------------------------------------
# Pure functions: the contract, with no OpenTelemetry dependency so they are easy to test.
# ---------------------------------------------------------------------------------------------


def parse_time(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def to_ns(value: str) -> int:
    return int(parse_time(value).timestamp() * 1_000_000_000)


def to_ms(value: str) -> int:
    return int(parse_time(value).timestamp() * 1000)


def load_runs(paths: list[str]) -> list[dict]:
    """Load run records from files or directories, oldest first."""
    files: list[Path] = []
    for raw in paths:
        p = Path(raw)
        if p.is_dir():
            files.extend(sorted(p.glob("*.json")))
        else:
            files.append(p)
    runs = [json.loads(f.read_text()) for f in files]
    for run, f in zip(runs, files):
        if run.get("schema_version") != 1:
            raise ValueError(f"{f}: schema_version must be 1")
    runs.sort(key=lambda r: r["started_at"])
    return runs


def shift_to_now(runs: list[dict], now: datetime) -> list[dict]:
    """Move every timestamp so the latest ended_at equals now. Spacing is unchanged."""
    if not runs:
        return runs
    latest = max(parse_time(r["ended_at"]) for r in runs)
    delta = now - latest

    def shift(value: str) -> str:
        return (parse_time(value) + delta).astimezone(timezone.utc).isoformat(timespec="milliseconds").replace(
            "+00:00", "Z"
        )

    out = []
    for run in runs:
        run = json.loads(json.dumps(run))
        run["started_at"] = shift(run["started_at"])
        run["ended_at"] = shift(run["ended_at"])
        for step in run["steps"]:
            step["at"] = shift(step["at"])
        out.append(run)
    return out


def root_attributes(run: dict) -> dict:
    attrs = {
        "dummyrun.run.id": run["id"],
        "dummyrun.persona.id": run["persona"]["id"],
        "dummyrun.persona.role": run["persona"]["role"],
        "dummyrun.persona.device": run["persona"]["device"],
        "dummyrun.scenario.id": run["scenario"]["id"],
        "dummyrun.target.adapter": run["target"]["adapter"],
        "dummyrun.target.environment": run["target"]["environment"],
        "url.full": run["target"]["base_url"],
        "gen_ai.system": run["provider"]["kind"],
        "gen_ai.request.model": run["provider"]["model"],
        "cloud.region": run["provider"]["region"],
        "browser.name": run["browser"]["name"],
        "dummyrun.outcome": run["outcome"],
        "gen_ai.usage.input_tokens": run["usage"]["input_tokens"],
        "gen_ai.usage.output_tokens": run["usage"]["output_tokens"],
    }
    if "batch" in run:
        attrs["dummyrun.batch.id"] = run["batch"]
    if "commit" in run["target"]:
        attrs["dummyrun.target.commit"] = run["target"]["commit"]
    return attrs


def root_is_error(run: dict) -> bool:
    return run["outcome"] in ("errored", "blocked")


def step_attributes(step: dict) -> dict:
    attrs = {"dummyrun.step.n": step["n"], "dummyrun.step.action": step["action"]}
    if "target" in step:
        attrs["dummyrun.step.target"] = step["target"]
    if "url" in step:
        attrs["url.full"] = step["url"]
    if "status" in step:
        attrs["http.response.status_code"] = step["status"]
    if "console_errors" in step:
        attrs["dummyrun.step.console_errors"] = step["console_errors"]
    if "a11y_violations" in step:
        attrs["dummyrun.step.a11y_violations"] = step["a11y_violations"]
    return attrs


def step_is_error(step: dict) -> bool:
    return step.get("status", 0) >= 500


def finding_attributes(finding: dict) -> dict:
    return {
        "dummyrun.finding.id": finding["id"],
        "dummyrun.finding.kind": finding["kind"],
        "dummyrun.finding.severity": finding["severity"],
        "dummyrun.finding.title": finding["title"],
        "dummyrun.finding.evidence": finding["evidence"],
        "dummyrun.finding.verdict": finding["verdict"],
        "dummyrun.finding.steps": list(finding["steps"]),
    }


def finding_time(run: dict, finding: dict) -> str:
    """The at time of the finding's first step."""
    first = min(finding["steps"])
    for step in run["steps"]:
        if step["n"] == first:
            return step["at"]
    raise ValueError(f"finding {finding['id']} refers to step {first}, which run {run['id']} does not have")


def run_log(run: dict) -> dict:
    severity = "ERROR" if run["outcome"] == "errored" else "INFO"
    body = f"run {run['id']} {run['outcome']}: {run['persona']['id']} / {run['scenario']['id']}"
    return {"time": run["ended_at"], "severity": severity, "body": body, "attributes": root_attributes(run)}


def finding_log(run: dict, finding: dict) -> dict:
    severity = "WARN" if finding["severity"] in ("blocker", "major") else "INFO"
    attrs = finding_attributes(finding)
    attrs["dummyrun.run.id"] = run["id"]
    attrs["dummyrun.persona.id"] = run["persona"]["id"]
    attrs["dummyrun.target.environment"] = run["target"]["environment"]
    return {"time": finding_time(run, finding), "severity": severity, "body": finding["title"], "attributes": attrs}


def metric_increments(run: dict) -> list[tuple[str, dict, float]]:
    """Every metric data point a run adds, as (metric, attributes, value)."""
    persona = run["persona"]["id"]
    duration_s = (parse_time(run["ended_at"]) - parse_time(run["started_at"])).total_seconds()
    out: list[tuple[str, dict, float]] = [
        (
            "dummyrun.runs",
            {
                "dummyrun.persona.id": persona,
                "dummyrun.scenario.id": run["scenario"]["id"],
                "dummyrun.target.environment": run["target"]["environment"],
                "dummyrun.outcome": run["outcome"],
            },
            1,
        ),
        (
            "dummyrun.run.duration",
            {"dummyrun.persona.id": persona, "dummyrun.scenario.id": run["scenario"]["id"], "dummyrun.outcome": run["outcome"]},
            duration_s,
        ),
    ]
    for step in run["steps"]:
        attrs = {"dummyrun.persona.id": persona, "dummyrun.action": step["action"]}
        out.append(("dummyrun.steps", attrs, 1))
        out.append(("dummyrun.step.duration", attrs, step["duration_ms"]))
    for finding in run["findings"]:
        out.append(
            (
                "dummyrun.findings",
                {
                    "dummyrun.persona.id": persona,
                    "dummyrun.target.environment": run["target"]["environment"],
                    "dummyrun.kind": finding["kind"],
                    "dummyrun.severity": finding["severity"],
                    "dummyrun.verdict": finding["verdict"],
                },
                1,
            )
        )
    model = {
        "dummyrun.persona.id": persona,
        "gen_ai.system": run["provider"]["kind"],
        "gen_ai.request.model": run["provider"]["model"],
    }
    out.append(("dummyrun.tokens", {**model, "dummyrun.direction": "input"}, run["usage"]["input_tokens"]))
    out.append(("dummyrun.tokens", {**model, "dummyrun.direction": "output"}, run["usage"]["output_tokens"]))
    if "cost_usd" in run["usage"]:
        out.append(("dummyrun.cost.usd", model, run["usage"]["cost_usd"]))
    return out


def annotation(run: dict) -> dict:
    confirmed = sum(1 for f in run["findings"] if f["verdict"] == "confirmed")
    tags = [
        "dummyrun",
        f"persona:{run['persona']['id']}",
        f"scenario:{run['scenario']['id']}",
        f"env:{run['target']['environment']}",
        f"outcome:{run['outcome']}",
    ]
    if confirmed:
        tags.append("findings")
    return {
        "time": to_ms(run["started_at"]),
        "timeEnd": to_ms(run["ended_at"]),
        "tags": tags,
        "text": f"{run['persona']['id']} / {run['scenario']['id']}: {run['outcome']}, {confirmed} confirmed findings",
    }


def trace_id_for(run_id: str) -> int:
    """A stable trace ID per run, so reseeding gives the same IDs."""
    return int.from_bytes(hashlib.sha256(run_id.encode()).digest()[:16], "big") or 1


def span_id_for(run_id: str, step: int | None) -> int:
    key = f"{run_id}/root" if step is None else f"{run_id}/{step}"
    return int.from_bytes(hashlib.sha256(key.encode()).digest()[:8], "big") or 1


class CumulativeState:
    """Cumulative counters and histograms, the way an SDK keeps them between exports."""

    def __init__(self) -> None:
        self.sums: dict[str, dict[tuple, float]] = defaultdict(dict)
        self.hists: dict[str, dict[tuple, dict]] = defaultdict(dict)

    @staticmethod
    def key(attrs: dict) -> tuple:
        return tuple(sorted(attrs.items()))

    def add(self, metric: str, attrs: dict, value: float) -> None:
        kind = METRICS[metric][0]
        k = self.key(attrs)
        if kind == "counter":
            self.sums[metric][k] = self.sums[metric].get(k, 0) + value
            return
        h = self.hists[metric].setdefault(
            k, {"count": 0, "sum": 0.0, "buckets": [0] * (len(BUCKETS) + 1), "min": value, "max": value}
        )
        h["count"] += 1
        h["sum"] += value
        h["min"] = min(h["min"], value)
        h["max"] = max(h["max"], value)
        for i, bound in enumerate(BUCKETS):
            if value <= bound:
                h["buckets"][i] += 1
                break
        else:
            h["buckets"][-1] += 1


# ---------------------------------------------------------------------------------------------
# Emitting through the OpenTelemetry SDK.
# ---------------------------------------------------------------------------------------------


def emit(runs: list[dict], otlp: str, namespace: str | None, version: str) -> None:
    from opentelemetry import trace
    from opentelemetry._logs import LogRecord, SeverityNumber
    from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
    from opentelemetry.exporter.otlp.proto.http.metric_exporter import OTLPMetricExporter
    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk._logs import LoggerProvider
    from opentelemetry.sdk._logs.export import BatchLogRecordProcessor
    from opentelemetry.sdk.metrics.export import (
        AggregationTemporality,
        Histogram,
        HistogramDataPoint,
        Metric,
        MetricsData,
        NumberDataPoint,
        ResourceMetrics,
        ScopeMetrics,
        Sum,
    )
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor
    from opentelemetry.sdk.trace.id_generator import IdGenerator
    from opentelemetry.sdk.util.instrumentation import InstrumentationScope
    from opentelemetry.trace import NonRecordingSpan, SpanContext, Status, StatusCode, TraceFlags

    class FixedIds(IdGenerator):
        trace_id = 1
        span_id = 1

        def generate_trace_id(self) -> int:
            return self.trace_id

        def generate_span_id(self) -> int:
            return self.span_id

    severities = {
        "INFO": SeverityNumber.INFO,
        "WARN": SeverityNumber.WARN,
        "ERROR": SeverityNumber.ERROR,
    }
    resource_attrs = {"service.name": SERVICE_NAME, "service.version": version}
    if namespace:
        resource_attrs["service.namespace"] = namespace
    resource = Resource.create(resource_attrs)
    scope = InstrumentationScope("dummyrun-seed", version)

    ids = FixedIds()
    tracer_provider = TracerProvider(resource=resource, id_generator=ids)
    span_exporter = OTLPSpanExporter(endpoint=f"{otlp}/v1/traces")
    tracer_provider.add_span_processor(BatchSpanProcessor(span_exporter))
    tracer = tracer_provider.get_tracer("dummyrun-seed", version)

    logger_provider = LoggerProvider(resource=resource)
    logger_provider.add_log_record_processor(BatchLogRecordProcessor(OTLPLogExporter(endpoint=f"{otlp}/v1/logs")))
    logger = logger_provider.get_logger("dummyrun-seed", version)

    metric_exporter = OTLPMetricExporter(endpoint=f"{otlp}/v1/metrics")
    state = CumulativeState()
    start_ns = min(to_ns(r["started_at"]) for r in runs)

    def emit_log(record: dict, trace_id: int, span_id: int) -> None:
        ctx = trace.set_span_in_context(
            NonRecordingSpan(SpanContext(trace_id, span_id, is_remote=False, trace_flags=TraceFlags(TraceFlags.SAMPLED)))
        )
        logger.emit(
            LogRecord(
                timestamp=to_ns(record["time"]),
                observed_timestamp=to_ns(record["time"]),
                context=ctx,
                severity_text=record["severity"],
                severity_number=severities[record["severity"]],
                body=record["body"],
                attributes=record["attributes"],
            )
        )

    def snapshot(time_ns: int) -> MetricsData:
        metrics = []
        for name, (kind, unit, description) in METRICS.items():
            if kind == "counter":
                points = [
                    NumberDataPoint(dict(k), start_ns, time_ns, v) for k, v in state.sums[name].items()
                ]
                if points:
                    metrics.append(Metric(name, description, unit, Sum(points, AggregationTemporality.CUMULATIVE, True)))
            else:
                points = [
                    HistogramDataPoint(
                        dict(k), start_ns, time_ns, h["count"], h["sum"], list(h["buckets"]), BUCKETS, h["min"], h["max"]
                    )
                    for k, h in state.hists[name].items()
                ]
                if points:
                    metrics.append(Metric(name, description, unit, Histogram(points, AggregationTemporality.CUMULATIVE)))
        return MetricsData([ResourceMetrics(resource, [ScopeMetrics(scope, metrics, "")], "")])

    # Metrics must reach Prometheus in time order, so sort by the time the run ended.
    for run in sorted(runs, key=lambda r: r["ended_at"]):
        ids.trace_id = trace_id_for(run["id"])
        ids.span_id = span_id_for(run["id"], None)
        root = tracer.start_span(ROOT_SPAN, start_time=to_ns(run["started_at"]), attributes=root_attributes(run))
        root_ctx = trace.set_span_in_context(root)
        for step in run["steps"]:
            ids.span_id = span_id_for(run["id"], step["n"])
            at = to_ns(step["at"])
            span = tracer.start_span(f"dummyrun.step {step['action']}", context=root_ctx, start_time=at, attributes=step_attributes(step))
            if step_is_error(step):
                span.set_status(Status(StatusCode.ERROR))
            span.end(end_time=at + step["duration_ms"] * 1_000_000)
        for finding in run["findings"]:
            root.add_event(FINDING_EVENT, finding_attributes(finding), timestamp=to_ns(finding_time(run, finding)))
            emit_log(finding_log(run, finding), ids.trace_id, span_id_for(run["id"], None))
        root.set_status(Status(StatusCode.ERROR) if root_is_error(run) else Status(StatusCode.OK))
        root.end(end_time=to_ns(run["ended_at"]))
        emit_log(run_log(run), ids.trace_id, span_id_for(run["id"], None))

        for metric, attrs, value in metric_increments(run):
            state.add(metric, attrs, value)
        if not metric_exporter.export(snapshot(to_ns(run["ended_at"]))):
            raise SystemExit(f"metric export failed for run {run['id']}")

    tracer_provider.force_flush()
    logger_provider.force_flush()
    tracer_provider.shutdown()
    logger_provider.shutdown()
    metric_exporter.shutdown()


def post_annotations(runs: list[dict], grafana: str, auth: str) -> int:
    token = base64.b64encode(auth.encode()).decode()
    posted = 0
    for run in runs:
        body = json.dumps(annotation(run)).encode()
        req = urllib.request.Request(
            f"{grafana}/api/annotations",
            data=body,
            headers={"Content-Type": "application/json", "Authorization": f"Basic {token}"},
            method="POST",
        )
        try:
            with urlopen(req, 10) as resp:
                resp.read()
            posted += 1
        except urllib.error.HTTPError as e:
            raise SystemExit(f"annotation for {run['id']} failed: {e.code} {e.read().decode()[:200]}") from e
    return posted


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("runs", nargs="+", help="run record files or directories")
    ap.add_argument("--otlp", default=os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"))
    ap.add_argument("--grafana", default="http://localhost:3000")
    ap.add_argument(
        "--grafana-auth",
        default=os.environ.get("GRAFANA_AUTH", "admin:admin"),
        help="user:password for posting annotations. Default is Grafana's out of the box admin account",
    )
    ap.add_argument("--end-now", action="store_true", help="shift timestamps so the latest run ends now")
    ap.add_argument("--no-annotations", action="store_true")
    ap.add_argument("--namespace", help="service.namespace, for example the app under test")
    ap.add_argument("--service-version", default="0.0.0-seed")
    ap.add_argument("--dry-run", action="store_true", help="load and shift, then print a summary and stop")
    a = ap.parse_args(argv)

    runs = load_runs(a.runs)
    if not runs:
        print("no runs found", file=sys.stderr)
        return 1
    if a.end_now:
        runs = shift_to_now(runs, datetime.now(timezone.utc) - timedelta(seconds=5))
    first, last = runs[0]["started_at"], max(r["ended_at"] for r in runs)
    print(f"{len(runs)} runs from {first} to {last}")
    if a.dry_run:
        return 0
    emit(runs, a.otlp, a.namespace, a.service_version)
    print(f"sent traces, metrics and logs to {a.otlp}")
    if not a.no_annotations:
        n = post_annotations(runs, a.grafana, a.grafana_auth)
        print(f"posted {n} annotations to {a.grafana}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
