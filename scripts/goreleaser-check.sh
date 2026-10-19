#!/usr/bin/env bash
# Run "goreleaser check". The Homebrew formula uses the brews section, which GoReleaser has deprecated in
# favour of casks. A cask for a command line tool runs into macOS Gatekeeper, so we keep the formula and
# accept that one notice. Any other notice or error fails the check.
set -uo pipefail

log=$(mktemp)
trap 'rm -f "$log"' EXIT
if goreleaser check >"$log" 2>&1; then
  cat "$log"
  exit 0
fi
cat "$log"
if grep -E 'DEPRECATED:' "$log" | grep -vq 'brews'; then
  echo "goreleaser check: a deprecation other than brews" >&2
  exit 1
fi
if grep -E '(error|⨯)' "$log" | grep -vqE 'uses deprecated properties|check failed|configuration file\(s\) have issues'; then
  echo "goreleaser check: an error other than the brews deprecation" >&2
  exit 1
fi
echo "goreleaser check: only the accepted brews deprecation"
