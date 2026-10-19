#!/usr/bin/env bash
# Smoke test a Dummy Run image: it runs as a non-root user, prints its version, and the accessibility
# sweep passes on a local page in Chrome and in Firefox.
# Usage: scripts/smoke-image.sh IMAGE
# The page is served from docker/smoke on the host and the container shares the host network.
# Set SMOKE_URL to test another page instead, for example http://host.docker.internal:8099/ on Docker Desktop.
# Such a page is reached over the container's ordinary network.
set -euo pipefail

image="${1:?usage: smoke-image.sh IMAGE}"
root="$(cd "$(dirname "$0")/.." && pwd)"
url="${SMOKE_URL:-}"
server=""
network=()

cleanup() {
  if [ -n "$server" ]; then kill "$server" 2>/dev/null || true; fi
}
trap cleanup EXIT

if [ -z "$url" ]; then
  port="${SMOKE_PORT:-$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')}"
  network=(--network host)
  python3 -m http.server "$port" --bind 127.0.0.1 --directory "$root/docker/smoke" >/dev/null 2>&1 &
  server=$!
  url="http://127.0.0.1:${port}/"
  for _ in $(seq 1 25); do
    if curl -fsS -o /dev/null "$url"; then break; fi
    sleep 0.2
  done
  if ! kill -0 "$server" 2>/dev/null; then
    echo "smoke: the page server did not start on port $port" >&2
    exit 1
  fi
fi

uid=$(docker run --rm --entrypoint id "$image" -u)
if [ "$uid" = "0" ]; then
  echo "smoke: the image runs as root" >&2
  exit 1
fi
echo "smoke: user id $uid"

docker run --rm "$image" version

for browser in chrome firefox; do
  echo "smoke: a11y on $browser"
  docker run --rm ${network[@]+"${network[@]}"} "$image" a11y --browser "$browser" "$url"
done
echo "smoke: passed"
