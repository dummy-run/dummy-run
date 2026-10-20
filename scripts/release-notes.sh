#!/usr/bin/env bash
# Print the CHANGELOG.md section for a version, without its heading.
# Usage: scripts/release-notes.sh v0.1.0 [CHANGELOG.md]
# Fails when the section is missing or empty, so a release never goes out with blank notes.
set -euo pipefail

version="${1:?usage: release-notes.sh VERSION [CHANGELOG]}"
version="${version#v}"
file="${2:-CHANGELOG.md}"

notes=$(awk -v v="$version" '
  /^## \[/ {
    if (found) exit
    if (index($0, "## [" v "]") == 1) { found = 1; next }
  }
  found { print }
' "$file")

if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
  echo "release-notes: no entries under ## [$version] in $file" >&2
  exit 1
fi
printf '%s\n' "$notes"
