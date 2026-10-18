#!/usr/bin/env bash
# Prove the join with Tempo's search API. Reads the trace ID that persona-request.sh wrote.
# Usage: ./check-tempo.sh [tempo_url]
set -euo pipefail
cd "$(dirname "$0")"
exec python3 -I check_tempo.py "$(cat .trace-id)" --tempo "${1:-http://localhost:${PROPAGATION_TEMPO_PORT:-3230}}" \
  --run-id "${DUMMYRUN_RUN_ID:-2026-10-17-ravi-reviewer-005}" --persona-id "${DUMMYRUN_PERSONA_ID:-ravi-reviewer}"
