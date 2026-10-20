#!/bin/sh
# Rebuilds the example report from examples/runs and examples/report/a11y-sweep.json.
# Run it from the repository root. A test in cmd/dummyrun fails when the committed files are out of date.
set -eu
for format in html json; do
  go run ./cmd/dummyrun report --runs examples/runs --build a865ffd --a11y examples/report/a11y-sweep.json \
    --include pending --title "Inkwell preview audit" --client "Inkwell Press (a fictional client)" \
    --date 2026-10-19 --format "$format" --out "examples/report/report.$format"
done
