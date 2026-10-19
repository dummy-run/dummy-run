#!/usr/bin/env python3
"""Take a full page screenshot of each Dummy Run dashboard with headless Chrome.

Usage:
  uv run screenshot.py --out DIR [--grafana http://localhost:3000] [--chrome PATH] [--run RUN_ID]
                       [--width 1600] [--uids dummyrun-overview,...]

The page height comes from the dashboard's grid so every panel is inside the viewport and renders.
Anonymous viewer access must be on, as it is in the local stack.
"""

from __future__ import annotations

import argparse
import json
import platform
import shutil
import subprocess
import sys
import urllib.parse
import urllib.request
from pathlib import Path

DASHBOARDS = ["dummyrun-overview", "dummyrun-personas", "dummyrun-findings", "dummyrun-run"]
ROW_PX = 38  # Grafana grid row height plus margin
HEADER_PX = 160


def urlopen(request, timeout: float = 30):
    """urllib.request.urlopen limited to http and https, so a URL can never name a local file."""
    url = request if isinstance(request, str) else request.full_url
    if not url.startswith(("http://", "https://")):
        raise ValueError(f"not an http URL: {url}")
    return urllib.request.urlopen(request, timeout=timeout)  # nosemgrep: python.lang.security.audit.dynamic-urllib-use-detected.dynamic-urllib-use-detected


def find_chrome(explicit: str | None) -> str:
    if explicit:
        return explicit
    candidates = ["google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"]
    if platform.system() == "Darwin":
        candidates.insert(0, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
    for c in candidates:
        path = c if Path(c).exists() else shutil.which(c)
        if path:
            return path
    raise SystemExit("Chrome not found, pass --chrome PATH")


def grid_height(dashboard: dict, repeats: int) -> int:
    """Rows in the grid, allowing for a repeated row appearing `repeats` times."""
    bottom = 0
    repeat_rows = 0
    for p in dashboard.get("panels", []):
        g = p.get("gridPos", {})
        bottom = max(bottom, g.get("y", 0) + g.get("h", 0))
        if p.get("type") == "row" and p.get("repeat"):
            repeat_rows += 1
    if repeat_rows and repeats > 1:
        bottom *= repeats
    return bottom


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--out", required=True)
    ap.add_argument("--grafana", default="http://localhost:3000")
    ap.add_argument("--chrome")
    ap.add_argument("--run", default="", help="run ID for the run dashboard")
    ap.add_argument("--width", type=int, default=1600)
    ap.add_argument("--repeats", type=int, default=4, help="how many times a repeated row is expected to appear")
    ap.add_argument("--uids", default=",".join(DASHBOARDS))
    ap.add_argument("--wait-ms", type=int, default=20000)
    a = ap.parse_args(argv)

    chrome = find_chrome(a.chrome)
    out = Path(a.out)
    out.mkdir(parents=True, exist_ok=True)
    for uid in a.uids.split(","):
        dashboard = json.load(urlopen(f"{a.grafana}/api/dashboards/uid/{uid}"))["dashboard"]
        height = HEADER_PX + ROW_PX * grid_height(dashboard, a.repeats)
        params = {"kiosk": "", "from": "now-14d", "to": "now"}
        if a.run:
            params["var-run"] = a.run
        url = f"{a.grafana}/d/{uid}?" + urllib.parse.urlencode(params)
        target = out / f"{uid.removeprefix('dummyrun-')}.png"
        cmd = [
            chrome, "--headless=new", "--disable-gpu", "--hide-scrollbars", "--no-sandbox",
            f"--window-size={a.width},{height}", f"--virtual-time-budget={a.wait_ms}",
            f"--screenshot={target}", url,
        ]
        subprocess.run(cmd, check=True, capture_output=True)
        print(f"{target} ({a.width}x{height})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
