#!/usr/bin/env python3
"""Prove the join with Tempo's search API.

Polls a TraceQL query for the run ID until the trace arrives, checks that the trace ID is the
one the persona sent, then lists every span in the trace with its two dummyrun attributes.

Usage: check_tempo.py TRACE_ID [--tempo http://localhost:3200] [--run-id ID] [--persona-id ID]
Needs only the standard library.
"""
import argparse
import base64
import http.client
import json
import sys
import time
import urllib.parse


def get_json(url):
    """GET a URL and parse the JSON body. Plain http.client, so only http and https are possible."""
    parts = urllib.parse.urlsplit(url)
    conn_type = {"http": http.client.HTTPConnection, "https": http.client.HTTPSConnection}[parts.scheme]
    conn = conn_type(parts.netloc, timeout=10)
    try:
        conn.request("GET", parts.path + ("?" + parts.query if parts.query else ""), headers={"Accept": "application/json"})
        res = conn.getresponse()
        if res.status != 200:
            raise RuntimeError(f"GET {url} returned {res.status}")
        return json.load(res)
    finally:
        conn.close()


def search(tempo, query, attempts=30, pause=2.0):
    url = tempo + "/api/search?" + urllib.parse.urlencode({"q": query, "limit": 5})
    for _ in range(attempts):
        try:
            traces = get_json(url).get("traces") or []
        except Exception:  # Tempo may still be warming up
            traces = []
        if traces:
            return traces
        time.sleep(pause)
    return []


def hex_id(b64):
    """Tempo's /api/traces returns OTLP JSON, where IDs are base64. Show them as hex like Grafana does."""
    return base64.b64decode(b64).hex() if b64 else ""


def spans_of(trace):
    for batch in trace.get("batches", []):
        service = next((a["value"]["stringValue"] for a in batch["resource"]["attributes"]
                        if a["key"] == "service.name"), "?")
        for scope in batch.get("scopeSpans", []):
            for span in scope.get("spans", []):
                attrs = {a["key"]: a["value"].get("stringValue") for a in span.get("attributes", [])}
                yield service, span["name"], hex_id(span.get("parentSpanId", "")), attrs


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("trace_id")
    ap.add_argument("--tempo", default="http://localhost:3200")
    ap.add_argument("--run-id", default="2026-10-17-ravi-reviewer-005")
    ap.add_argument("--persona-id", default="ravi-reviewer")
    args = ap.parse_args()

    query = '{ resource.service.name = "inkwell" && span.dummyrun.run.id = "%s" }' % args.run_id
    print("TraceQL:", query)
    traces = search(args.tempo, query)
    if not traces:
        sys.exit("no trace matched; the app did not export spans carrying dummyrun.run.id")
    for t in traces:
        matched = (t.get("spanSets") or [{}])[0].get("matched", "?")
        print(f"matched trace {t['traceID']}: {matched} spans, root {t.get('rootServiceName')} {t.get('rootTraceName')}")
    ids = {t["traceID"] for t in traces}
    if args.trace_id not in ids:
        sys.exit(f"found {sorted(ids)} but not the persona trace {args.trace_id}; the app did not join the incoming traceparent")
    print(f"trace {args.trace_id} is the one the persona's traceparent named")

    rows = list(spans_of(get_json(f"{args.tempo}/api/traces/{args.trace_id}")))
    if not rows:
        sys.exit("trace has no spans")
    print(f"{'service':8} {'span':16} {'parent span':18} {'dummyrun.run.id':30} dummyrun.persona.id")
    for service, name, parent, attrs in rows:
        print(f"{service:8} {name:16} {parent:18} {attrs.get('dummyrun.run.id') or '-':30} {attrs.get('dummyrun.persona.id') or '-'}")
    bad = [name for _, name, _, attrs in rows
           if attrs.get("dummyrun.run.id") != args.run_id or attrs.get("dummyrun.persona.id") != args.persona_id]
    if bad:
        sys.exit(f"spans without both attributes: {bad}")
    print(f"all {len(rows)} spans carry dummyrun.run.id={args.run_id} and dummyrun.persona.id={args.persona_id}")


if __name__ == "__main__":
    main()
