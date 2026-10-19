#!/usr/bin/env bash
# Start the stack, send persona-shaped requests, prove the join in Tempo, then stop.
# This is what CI runs. Usage: ./run.sh
set -euo pipefail
cd "$(dirname "$0")"
cleanup() { docker compose down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker compose up --build --wait --wait-timeout 180
./persona-request.sh
./check-tempo.sh
