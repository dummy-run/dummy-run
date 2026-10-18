#!/usr/bin/env bash
# Send requests to Inkwell shaped exactly like the ones the Dummy Run browser sends:
# a W3C traceparent for the step span, a baggage header with the run and persona IDs,
# and a user agent ending in DummyRun/<version>. See docs/TELEMETRY.md, "Linking to the app's own telemetry".
#
# Usage: ./persona-request.sh [base_url]
# Prints the trace ID it used. check-tempo.sh reads it from .trace-id.
set -euo pipefail

BASE_URL="${1:-http://localhost:${PROPAGATION_INKWELL_PORT:-8090}}"
RUN_ID="${DUMMYRUN_RUN_ID:-2026-10-17-ravi-reviewer-005}"
PERSONA_ID="${DUMMYRUN_PERSONA_ID:-ravi-reviewer}"
VERSION="${DUMMYRUN_VERSION:-0.1.0}"

hex() { od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'; }

TRACE_ID="$(hex 16)"
echo "trace id: $TRACE_ID"
printf '%s' "$TRACE_ID" > .trace-id

# One request per persona step, each with its own parent span ID, all in one trace.
for path in /drafts /payments /reviews; do
  STEP_SPAN_ID="$(hex 8)"
  curl -sS "$BASE_URL$path" \
    -H "traceparent: 00-$TRACE_ID-$STEP_SPAN_ID-01" \
    -H "baggage: dummyrun.run.id=$RUN_ID,dummyrun.persona.id=$PERSONA_ID" \
    -A "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36 DummyRun/$VERSION"
  echo
done
