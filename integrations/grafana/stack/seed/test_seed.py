"""Tests for the seeder against docs/TELEMETRY.md, using the synthetic runs in examples/runs."""

from datetime import datetime, timedelta, timezone
from pathlib import Path

import pytest

import seed

EXAMPLES = Path(__file__).resolve().parents[4] / "examples" / "runs"

ROOT_ATTRIBUTES = {
    "dummyrun.run.id", "dummyrun.batch.id", "dummyrun.persona.id", "dummyrun.persona.role", "dummyrun.persona.device",
    "dummyrun.scenario.id", "dummyrun.target.adapter", "dummyrun.target.environment", "dummyrun.target.commit",
    "url.full", "gen_ai.system", "gen_ai.request.model", "cloud.region", "browser.name", "dummyrun.outcome",
    "gen_ai.usage.input_tokens", "gen_ai.usage.output_tokens",
}
FINDING_ATTRIBUTES = {
    "dummyrun.finding.id", "dummyrun.finding.kind", "dummyrun.finding.severity", "dummyrun.finding.title",
    "dummyrun.finding.evidence", "dummyrun.finding.verdict", "dummyrun.finding.steps",
}
METRIC_ATTRIBUTES = {
    "dummyrun.runs": {"dummyrun.persona.id", "dummyrun.scenario.id", "dummyrun.target.environment", "dummyrun.outcome"},
    "dummyrun.run.duration": {"dummyrun.persona.id", "dummyrun.scenario.id", "dummyrun.outcome"},
    "dummyrun.steps": {"dummyrun.persona.id", "dummyrun.action"},
    "dummyrun.step.duration": {"dummyrun.persona.id", "dummyrun.action"},
    "dummyrun.findings": {"dummyrun.persona.id", "dummyrun.target.environment", "dummyrun.kind", "dummyrun.severity", "dummyrun.verdict"},
    "dummyrun.tokens": {"dummyrun.persona.id", "gen_ai.system", "gen_ai.request.model", "dummyrun.direction"},
    "dummyrun.cost.usd": {"dummyrun.persona.id", "gen_ai.system", "gen_ai.request.model"},
}


@pytest.fixture(scope="module")
def runs():
    return seed.load_runs([str(EXAMPLES)])


def test_loads_examples_oldest_first(runs):
    assert len(runs) == 8
    assert [r["started_at"] for r in runs] == sorted(r["started_at"] for r in runs)


def test_root_attributes_follow_the_contract(runs):
    for run in runs:
        attrs = seed.root_attributes(run)
        assert set(attrs) == ROOT_ATTRIBUTES
        assert attrs["dummyrun.run.id"] == run["id"]
        assert attrs["url.full"] == run["target"]["base_url"]
        assert attrs["gen_ai.usage.input_tokens"] == run["usage"]["input_tokens"]


def test_root_attributes_drop_optional_fields():
    run = seed.load_runs([str(EXAMPLES)])[0]
    del run["batch"]
    del run["target"]["commit"]
    del run["usage"]["cost_usd"]
    attrs = seed.root_attributes(run)
    assert "dummyrun.batch.id" not in attrs
    assert "dummyrun.target.commit" not in attrs
    assert "dummyrun.cost.usd" not in seed.run_log(run)["attributes"]


def test_root_status_is_error_for_errored_and_blocked():
    assert seed.root_is_error({"outcome": "errored"})
    assert seed.root_is_error({"outcome": "blocked"})
    assert not seed.root_is_error({"outcome": "completed"})
    assert not seed.root_is_error({"outcome": "budget_exhausted"})


def test_step_attributes_and_status(runs):
    step = {"n": 3, "at": "2026-10-17T05:41:24.803Z", "action": "click", "duration_ms": 10, "target": "Submit",
            "url": "https://x.example/a", "status": 503, "console_errors": 2, "a11y_violations": 1}
    assert seed.step_attributes(step) == {
        "dummyrun.step.n": 3, "dummyrun.step.action": "click", "dummyrun.step.target": "Submit",
        "url.full": "https://x.example/a", "http.response.status_code": 503,
        "dummyrun.step.console_errors": 2, "dummyrun.step.a11y_violations": 1,
    }
    assert seed.step_is_error(step)
    assert not seed.step_is_error({"n": 1, "at": "", "action": "look", "duration_ms": 1, "status": 404})
    assert seed.step_attributes({"n": 1, "at": "", "action": "look", "duration_ms": 1}) == {"dummyrun.step.n": 1, "dummyrun.step.action": "look"}


def test_finding_event_and_time(runs):
    run = next(r for r in runs if r["findings"])
    finding = run["findings"][0]
    attrs = seed.finding_attributes(finding)
    assert set(attrs) == FINDING_ATTRIBUTES
    assert attrs["dummyrun.finding.steps"] == finding["steps"]
    first = min(finding["steps"])
    assert seed.finding_time(run, finding) == next(s["at"] for s in run["steps"] if s["n"] == first)


def test_run_log_record(runs):
    run = runs[0]
    rec = seed.run_log(run)
    assert rec["body"] == f"run {run['id']} {run['outcome']}: {run['persona']['id']} / {run['scenario']['id']}"
    assert rec["severity"] == ("ERROR" if run["outcome"] == "errored" else "INFO")
    assert rec["time"] == run["ended_at"]
    expected = seed.root_attributes(run) | {"dummyrun.steps.count": len(run["steps"]), "dummyrun.cost.usd": run["usage"]["cost_usd"]}
    assert rec["attributes"] == expected


def test_finding_log_severity(runs):
    run = next(r for r in runs if r["findings"])
    for finding in run["findings"]:
        rec = seed.finding_log(run, finding)
        assert rec["severity"] == ("WARN" if finding["severity"] in ("blocker", "major") else "INFO")
        assert rec["body"] == finding["title"]
        assert rec["attributes"]["dummyrun.run.id"] == run["id"]
        assert rec["attributes"]["dummyrun.persona.id"] == run["persona"]["id"]
        assert rec["attributes"]["dummyrun.scenario.id"] == run["scenario"]["id"]
        assert rec["attributes"]["dummyrun.target.environment"] == run["target"]["environment"]


def test_metric_increments_follow_the_contract(runs):
    for run in runs:
        increments = seed.metric_increments(run)
        for metric, attrs, value in increments:
            assert metric in seed.METRICS
            assert set(attrs) == METRIC_ATTRIBUTES[metric], metric
            assert value >= 0
        by_metric = {}
        for metric, _, value in increments:
            by_metric.setdefault(metric, []).append(value)
        assert by_metric["dummyrun.runs"] == [1]
        assert len(by_metric["dummyrun.steps"]) == len(run["steps"])
        assert sum(by_metric["dummyrun.step.duration"]) == sum(s["duration_ms"] for s in run["steps"])
        assert len(by_metric.get("dummyrun.findings", [])) == len(run["findings"])
        assert sorted(by_metric["dummyrun.tokens"]) == sorted([run["usage"]["input_tokens"], run["usage"]["output_tokens"]])
        assert by_metric["dummyrun.cost.usd"] == [run["usage"]["cost_usd"]]
        duration = by_metric["dummyrun.run.duration"][0]
        assert duration == pytest.approx((seed.parse_time(run["ended_at"]) - seed.parse_time(run["started_at"])).total_seconds())


def test_metric_units():
    assert seed.METRICS["dummyrun.run.duration"][1] == "s"
    assert seed.METRICS["dummyrun.step.duration"][1] == "ms"
    assert seed.METRICS["dummyrun.cost.usd"][1] == "{USD}"
    assert seed.METRICS["dummyrun.runs"][0] == "counter"
    assert seed.METRICS["dummyrun.step.duration"][0] == "histogram"


def test_histogram_buckets_follow_the_contract():
    assert seed.BUCKETS["dummyrun.run.duration"] == [15, 30, 60, 120, 180, 300, 600, 900, 1800, 3600]
    assert seed.BUCKETS["dummyrun.step.duration"] == [100, 250, 500, 1000, 2000, 5000, 10000, 30000, 60000]


def test_cumulative_state_histogram_buckets():
    state = seed.CumulativeState()
    for value in (50, 100, 101, 250, 70000):
        state.add("dummyrun.step.duration", {"dummyrun.persona.id": "a", "dummyrun.action": "click"}, value)
    (h,) = state.hists["dummyrun.step.duration"].values()
    assert h["count"] == 5
    assert h["sum"] == 70501
    assert h["min"] == 50 and h["max"] == 70000
    # 50 and 100 fall in the first bucket (<=100), 101 and 250 in <=250, 70000 overflows.
    assert h["buckets"][0] == 2 and h["buckets"][1] == 2 and h["buckets"][-1] == 1
    assert len(h["buckets"]) == len(seed.BUCKETS["dummyrun.step.duration"]) + 1
    assert sum(h["buckets"]) == h["count"]


def test_cumulative_state_counters_add_up():
    state = seed.CumulativeState()
    attrs = {"dummyrun.persona.id": "a", "dummyrun.direction": "input", "gen_ai.system": "x", "gen_ai.request.model": "m"}
    state.add("dummyrun.tokens", attrs, 10)
    state.add("dummyrun.tokens", dict(reversed(list(attrs.items()))), 5)
    assert list(state.sums["dummyrun.tokens"].values()) == [15]


def test_annotation(runs):
    run = next(r for r in runs if any(f["verdict"] == "confirmed" for f in r["findings"]))
    a = seed.annotation(run)
    confirmed = sum(1 for f in run["findings"] if f["verdict"] == "confirmed")
    assert a["tags"][:5] == ["dummyrun", f"persona:{run['persona']['id']}", f"scenario:{run['scenario']['id']}",
                             f"env:{run['target']['environment']}", f"outcome:{run['outcome']}"]
    assert a["tags"][5] == "findings"
    noun = "confirmed finding" if confirmed == 1 else "confirmed findings"
    assert a["text"] == f"{run['persona']['id']} / {run['scenario']['id']}: {run['outcome']}, {confirmed} {noun}"
    assert a["timeEnd"] - a["time"] == seed.to_ms(run["ended_at"]) - seed.to_ms(run["started_at"])
    plain = next(r for r in runs if not any(f["verdict"] == "confirmed" for f in r["findings"]))
    assert "findings" not in seed.annotation(plain)["tags"]
    assert seed.annotation(plain)["text"].endswith(", 0 confirmed findings")


def test_annotation_text_singular():
    run = seed.load_runs([str(EXAMPLES)])[0]
    run["findings"] = [{"id": "x", "kind": "bug", "severity": "major", "title": "t", "steps": [1], "evidence": "measured", "verdict": "confirmed"}]
    assert seed.annotation(run)["text"].endswith(", 1 confirmed finding")


def test_shift_to_now_keeps_spacing(runs):
    now = datetime(2027, 1, 1, tzinfo=timezone.utc)
    shifted = seed.shift_to_now(runs, now)
    assert max(seed.parse_time(r["ended_at"]) for r in shifted) == now
    before = [seed.parse_time(r["started_at"]) for r in runs]
    after = [seed.parse_time(r["started_at"]) for r in shifted]
    deltas = {a - b for a, b in zip(after, before, strict=True)}
    assert len(deltas) == 1
    for old, new in zip(runs, shifted, strict=True):
        assert len(old["steps"]) == len(new["steps"])
        assert seed.parse_time(new["steps"][0]["at"]) - seed.parse_time(new["started_at"]) == timedelta(0)
    # The originals are untouched.
    assert runs[0]["started_at"].startswith("2026-10-15")


def test_ids_follow_the_contract():
    import hashlib

    run_id = "2026-10-17-sam-author-006"
    assert seed.trace_id_for(run_id) == int.from_bytes(hashlib.sha256(b"dummyrun.run:" + run_id.encode()).digest()[:16], "big")
    assert seed.span_id_for(run_id, None) == int.from_bytes(hashlib.sha256(b"dummyrun.span:" + run_id.encode() + b":root").digest()[:8], "big")
    assert seed.span_id_for(run_id, 3) == int.from_bytes(hashlib.sha256(b"dummyrun.span:" + run_id.encode() + b":step:3").digest()[:8], "big")


def test_ids_are_stable_and_distinct():
    assert seed.trace_id_for("2026-10-17-sam-author-006") == seed.trace_id_for("2026-10-17-sam-author-006")
    assert seed.trace_id_for("a") != seed.trace_id_for("b")
    assert 0 < seed.trace_id_for("a") < 2**128
    assert seed.span_id_for("a", None) != seed.span_id_for("a", 1)
    assert seed.span_id_for("a", None) != seed.trace_id_for("a") >> 64
    assert 0 < seed.span_id_for("a", 1) < 2**64


def test_write_shifted_records(tmp_path):
    out = tmp_path / "out"
    assert seed.main([str(EXAMPLES), "--end-now", "--write", str(out)]) == 0
    written = sorted(out.glob("*.json"))
    assert len(written) == 8
    newest = max(seed.parse_time(__import__("json").loads(f.read_text())["ended_at"]) for f in written)
    assert (datetime.now(timezone.utc) - newest).total_seconds() < 120


def test_token_only_goes_to_https_or_loopback(monkeypatch):
    with pytest.raises(SystemExit):
        seed.post_annotations([], "http://grafana.example", "glsa_x")


def test_rejects_other_schema_versions(tmp_path):
    (tmp_path / "x.json").write_text('{"schema_version": 3, "started_at": "2026-01-01T00:00:00Z"}')
    with pytest.raises(ValueError):
        seed.load_runs([str(tmp_path)])
