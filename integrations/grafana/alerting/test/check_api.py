#!/usr/bin/env python3
"""Check a running Grafana against the rules file. Called by validate.sh.

Usage: check_api.py RULES_FILE GRAFANA_URL USER:PASSWORD
Reads the rule UIDs from the YAML with a regular expression, so it needs only the standard library.
"""
import base64
import http.client
import json
import re
import sys
import urllib.parse


def get(url, auth):
    """GET a URL with basic auth and parse the JSON body. Plain http.client, so only http and https are possible."""
    parts = urllib.parse.urlsplit(url)
    conn_type = {"http": http.client.HTTPConnection, "https": http.client.HTTPSConnection}[parts.scheme]
    conn = conn_type(parts.netloc, timeout=10)
    try:
        conn.request("GET", parts.path, headers={"Authorization": "Basic " + base64.b64encode(auth.encode()).decode()})
        res = conn.getresponse()
        if res.status != 200:
            raise RuntimeError(f"GET {url} returned {res.status}")
        return json.load(res)
    finally:
        conn.close()


def main():
    rules_file, grafana, auth = sys.argv[1:4]
    expected = re.findall(r"^\s+- uid:\s*(\S+)", open(rules_file).read(), re.M)
    if len(expected) < 4:
        sys.exit(f"rules file names {len(expected)} rule UIDs, expected at least 4; an empty file must not pass")
    print(f"expecting {len(expected)} rules: {' '.join(expected)}")

    # 1. Every rule in the file is in the provisioning API, with provenance=file.
    rules = {r["uid"]: r for r in get(grafana + "/api/v1/provisioning/alert-rules", auth)}
    missing = [u for u in expected if u not in rules]
    if missing:
        sys.exit(f"rules missing from the API: {missing}; present: {sorted(rules)}")
    for u in expected:
        r = rules[u]
        print(f"  {u}: {r['title']!r} condition={r['condition']} for={r['for']} "
              f"noData={r['noDataState']} execErr={r['execErrState']} provenance={r.get('provenance')}")
        if r.get("provenance") != "file":
            sys.exit(f"{u} is not marked as file-provisioned")
        ref_ids = {d["refId"] for d in r["data"]}
        if r["condition"] not in ref_ids:
            sys.exit(f"{u}: condition {r['condition']} is not one of its queries {sorted(ref_ids)}")
    print(f"all {len(expected)} rules provisioned")

    # 2. The ruler loaded them into a group, so the scheduler accepted them.
    groups = get(grafana + "/api/prometheus/grafana/api/v1/rules", auth)["data"]["groups"]
    names = sorted(r["name"] for g in groups for r in g["rules"])
    if len(names) != len(expected):
        sys.exit(f"ruler has {len(names)} rules, expected {len(expected)}: {names}")
    print("ruler groups:", [(g["name"], g["file"], len(g["rules"])) for g in groups])


if __name__ == "__main__":
    main()
