#!/usr/bin/env bash
# Dry run of the alert rule provisioning in a real Grafana.
#
# Grafana has no validate-only mode, so this starts the pinned grafana/grafana image with the rules
# mounted, waits for it to be healthy, and then checks three things: the log has no provisioning
# error, the provisioning API lists every rule in the file, and the ruler reports each rule
# (the last two in check_api.py).
# Usage: test/validate.sh   (needs Docker and python3)
set -euo pipefail
cd "$(dirname "$0")/.."

# grafana/grafana:13.2.3, published 2026-09-29
IMAGE="grafana/grafana@sha256:b28bae15e219c998fb0e0424ed724930cc61b1f61fb404d47c862f9a23f9e572"
PORT="${GRAFANA_TEST_PORT:-3330}"
NAME="dummyrun-alerting-validate-$$"
RULES="provisioning/alerting/dummyrun-rules.yaml"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$NAME" -p "$PORT:3000" \
  -e GF_SECURITY_ADMIN_PASSWORD=validate \
  -v "$PWD/provisioning/alerting:/etc/grafana/provisioning/alerting:ro" \
  -v "$PWD/test/datasources:/etc/grafana/provisioning/datasources:ro" \
  "$IMAGE" >/dev/null

for _ in $(seq 1 60); do
  if curl -sf "http://localhost:$PORT/api/health" >/dev/null 2>&1; then break; fi
  if [ "$(docker inspect -f '{{.State.Running}}' "$NAME")" != "true" ]; then
    docker logs "$NAME" | tail -40
    echo "Grafana exited before it became healthy"; exit 1
  fi
  sleep 2
done
curl -sf "http://localhost:$PORT/api/health" >/dev/null || { docker logs "$NAME" | tail -40; echo "Grafana did not become healthy"; exit 1; }

# 1. No provisioning error in the log.
if docker logs "$NAME" 2>&1 | command grep -iE 'level=error.*(provision|alerting)|Failed to provision' ; then
  echo "provisioning logged an error"; exit 1
fi
echo "no provisioning error in the log"

# 2 and 3. Every rule is in the provisioning API and the ruler accepted it.
python3 -I test/check_api.py "$RULES" "http://localhost:$PORT" admin:validate
echo "alert rule provisioning dry run passed"
