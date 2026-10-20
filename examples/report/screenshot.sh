#!/bin/sh
# Rebuilds docs/images/report-example.png from examples/report/report.html with headless Chrome.
# Run it from the repository root. Set CHROME to the browser binary if it is not in the default place.
set -eu
CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
profile="$(mktemp -d)"
"$CHROME" --headless=new --disable-gpu --no-first-run --user-data-dir="$profile" \
  --hide-scrollbars --window-size=1100,1500 \
  --screenshot="$PWD/docs/images/report-example.png" "file://$PWD/examples/report/report.html"
