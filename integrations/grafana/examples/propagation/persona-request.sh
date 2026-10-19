#!/usr/bin/env bash
# Send requests to Inkwell shaped exactly like the ones the Dummy Run browser sends:
# a W3C traceparent for the step span, a baggage header with the run and persona IDs,
# and a user agent ending in DummyRun/<version>. See docs/TELEMETRY.md, "Linking to the app's own telemetry".
#
# Usage: ./persona-request.sh [base_url]
# Prints the trace ID it used. check-tempo.sh reads it from .trace-id and derives the step span IDs
# the same way, so it can check each server span hangs under its step.
set -euo pipefail

BASE_URL="${1:-http://localhost:${PROPAGATION_INKWELL_PORT:-8090}}"
RUN_ID="${DUMMYRUN_RUN_ID:-2026-10-17-ravi-reviewer-005}"
PERSONA_ID="${DUMMYRUN_PERSONA_ID:-ravi-reviewer}"
VERSION="${DUMMYRUN_VERSION:-0.1.0}"

# Wait until Inkwell answers, in case it was started without the compose health check.
curl -sSf --retry 15 --retry-connrefused --retry-delay 1 -o /dev/null "$BASE_URL/healthz"

# The contract derives every ID from the run ID, so the exporter's trace and the app's spans meet
# (docs/TELEMETRY.md, Traces). The trace ID is the first 16 bytes of SHA-256 of "dummyrun.run:<id>".
# The span ID of step n is the first 8 bytes of SHA-256 of "dummyrun.span:<id>:step:<n>".
sha() { python3 -I -c 'import hashlib,sys; print(hashlib.sha256(sys.argv[1].encode()).hexdigest()[:int(sys.argv[2])])' "$1" "$2"; }
TRACE_ID="$(sha "dummyrun.run:$RUN_ID" 32)"
echo "trace id: $TRACE_ID (sha256 of dummyrun.run:$RUN_ID, first 16 bytes)"
printf '%s' "$TRACE_ID" > .trace-id

# One request per persona step. Its parent is that step's span, so the app's spans nest under the step.
n=0
for path in /drafts /payments /reviews; do
  n=$((n + 1))
  STEP_SPAN_ID="$(sha "dummyrun.span:$RUN_ID:step:$n" 16)"
  echo "step $n: parent span $STEP_SPAN_ID (sha256 of dummyrun.span:$RUN_ID:step:$n, first 8 bytes)"
  curl -sS "$BASE_URL$path" \
    -H "traceparent: 00-$TRACE_ID-$STEP_SPAN_ID-01" \
    -H "baggage: dummyrun.run.id=$RUN_ID,dummyrun.persona.id=$PERSONA_ID" \
    -A "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36 DummyRun/$VERSION"
  echo
done
