#!/usr/bin/env python3
"""Check that every panel in the Dummy Run dashboards returns data through Grafana's API.

Usage:
  uv run check_panels.py [--grafana http://localhost:3000] [--from now-14d] [--run RUN_ID]
                         [--persona PERSONA] [--uids dummyrun-overview,...]

It loads each dashboard through /api/dashboards/uid/<uid>, fills in the template variables with
values that match everything (or the given run and persona), sends every panel query to
/api/ds/query as the anonymous viewer, and reports data or no data per panel. It exits 1 when any
panel has no data or any query returns an error. The smoke test in CI runs it with --allow-empty,
which only fails on errors.
"""

from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

DASHBOARDS = ["dummyrun-overview", "dummyrun-personas", "dummyrun-findings", "dummyrun-run"]


def urlopen(request, timeout: float = 30):
    """urllib.request.urlopen limited to http and https, so a URL can never name a local file."""
    url = request if isinstance(request, str) else request.full_url
    if not url.startswith(("http://", "https://")):
        raise ValueError(f"not an http URL: {url}")
    return urllib.request.urlopen(request, timeout=timeout)  # nosemgrep: python.lang.security.audit.dynamic-urllib-use-detected.dynamic-urllib-use-detected


def get(url: str):
    with urlopen(url, 30) as resp:
        return json.load(resp)


def post(url: str, body: dict):
    req = urllib.request.Request(url, data=json.dumps(body).encode(), headers={"Content-Type": "application/json"})
    try:
        with urlopen(req, 60) as resp:
            return resp.status, json.load(resp)
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.load(e)
        except Exception:
            return e.code, {"message": e.reason}


def substitute(text: str, values: dict[str, str]) -> str:
    for name, value in values.items():
        text = text.replace("${" + name + "}", value).replace("$" + name, value)
    return text


def panels(dashboard: dict):
    """Yield every panel with queries, including panels inside collapsed rows."""
    for p in dashboard.get("panels", []):
        if p.get("type") == "row":
            yield from (q for q in p.get("panels", []) if q.get("targets"))
        elif p.get("targets"):
            yield p


def frame_rows(frames: list[dict]) -> int:
    rows = 0
    for f in frames:
        values = f.get("data", {}).get("values", [])
        if values:
            rows += max(len(v) for v in values)
    return rows


def check_dashboard(grafana: str, uid: str, values: dict[str, str], time_from: str, time_to: str) -> list[tuple[str, str, str]]:
    dashboard = get(f"{grafana}/api/dashboards/uid/{uid}")["dashboard"]
    results = []
    for panel in panels(dashboard):
        raw = json.dumps(panel["targets"])
        targets = json.loads(substitute(raw, values))
        queries = []
        for t in targets:
            q = dict(t)
            q.setdefault("datasource", panel.get("datasource"))
            q["intervalMs"] = 3_600_000
            q["maxDataPoints"] = 60
            queries.append(q)
        status, body = post(f"{grafana}/api/ds/query", {"queries": queries, "from": time_from, "to": time_to})
        if status != 200:
            results.append((dashboard["title"], panel["title"], f"error {status}: {json.dumps(body)[:200]}"))
            continue
        rows, errors = 0, []
        for ref, result in body.get("results", {}).items():
            if result.get("error"):
                errors.append(f"{ref}: {result['error']}")
            rows += frame_rows(result.get("frames", []))
        if errors:
            results.append((dashboard["title"], panel["title"], "error: " + "; ".join(errors)[:200]))
        else:
            results.append((dashboard["title"], panel["title"], f"data ({rows} rows)" if rows else "no data"))
    return results


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--grafana", default="http://localhost:3000")
    ap.add_argument("--from", dest="time_from", default="now-14d")
    ap.add_argument("--to", dest="time_to", default="now")
    ap.add_argument("--run", default="", help="run ID for the run dashboard. Default: the latest run in Loki")
    ap.add_argument("--persona", default=".*", help="persona for the repeated rows. Default: match all")
    ap.add_argument("--uids", default=",".join(DASHBOARDS))
    ap.add_argument("--allow-empty", action="store_true", help="only fail on query errors")
    ap.add_argument("--expect-empty", default="", help="comma separated panel titles that may return no data")
    ap.add_argument("--retries", type=int, default=0, help="repeat the check this many times while problems remain")
    ap.add_argument("--wait", type=int, default=10, help="seconds between retries")
    a = ap.parse_args(argv)

    for attempt in range(a.retries + 1):
        run = a.run or latest_run(a.grafana, a.time_from, a.time_to)
        values = {"environment": ".*", "persona": a.persona, "scenario": ".*", "run": run, "__range": "14d"}
        failures = 0
        lines = []
        for uid in a.uids.split(","):
            for dashboard, panel, status in check_dashboard(a.grafana, uid, values, a.time_from, a.time_to):
                lines.append(f"{dashboard:28} {panel:48} {status}")
                allowed = a.allow_empty or panel in a.expect_empty.split(",")
                if status.startswith("error") or (status == "no data" and not allowed):
                    failures += 1
        if not failures or attempt == a.retries:
            print("\n".join(lines))
            print(f"{failures} problems")
            return 1 if failures else 0
        print(f"{failures} problems, retrying in {a.wait}s")
        time.sleep(a.wait)
    return 1


def latest_run(grafana: str, time_from: str, time_to: str) -> str:
    """Find the newest run ID through the Loki data source."""
    query = {
        "queries": [{"refId": "A", "datasource": {"type": "loki", "uid": "loki"}, "expr": '{service_name="dummyrun"} | dummyrun_outcome != ""', "queryType": "range", "maxLines": 1}],
        "from": time_from,
        "to": time_to,
    }
    status, body = post(f"{grafana}/api/ds/query", query)
    if status != 200:
        return ""
    for frame in body["results"]["A"].get("frames", []):
        fields = frame["schema"]["fields"]
        for i, f in enumerate(fields):
            if f["name"] == "labels":
                labels = frame["data"]["values"][i]
                if labels:
                    return labels[0].get("dummyrun_run_id", "")
    return ""


if __name__ == "__main__":
    sys.exit(main())
