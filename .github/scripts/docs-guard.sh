#!/usr/bin/env bash
# Docs move with the code. Reads the changed file paths on standard input, separated by NUL (git diff -z), and decides whether the pull
# request touches user-facing behaviour without touching the docs.
#
# Environment:
#   DOCS_GUARD_LABELS     Labels on the pull request: one per line, or a JSON array.
#   DOCS_GUARD_BODY       The pull request description.
#   DOCS_GUARD_AUTHOR     Login of the pull request author. dependabot[bot] is exempt.
#   DOCS_GUARD_BASE/HEAD  Commits to compare CHANGELOG.md between (merge base to head). Needs a checkout with history.
#   DOCS_GUARD_CHANGELOG  yes or no, when BASE and HEAD are not set (used by the tests).
#
# Exit status: 0 when the pull request passes, 1 when it needs docs.
set -euo pipefail

labels="${DOCS_GUARD_LABELS:-}"
body="${DOCS_GUARD_BODY:-}"
author="${DOCS_GUARD_AUTHOR:-}"

if [ "$author" = "dependabot[bot]" ]; then
  echo "Dependabot pull request: exempt."
  exit 0
fi

# Paths that change what a user sees. Tests and the repository's own tooling do not count.
is_behaviour() {
  case "$1" in
    *_test.go | */testdata/* | internal/tools/*) return 1 ;;
    cmd/* | internal/* | casts/library/* | schema/* | integrations/* | docker/* | .goreleaser.yaml) return 0 ;;
    site/layouts/*) return 0 ;;
  esac
  return 1
}

# Paths that count as documentation.
is_doc() {
  case "$1" in
    docs/* | README.md | CHANGELOG.md | site/content/* | site/layouts/*) return 0 ;;
    integrations/*/README.md | casts/*/README.md) return 0 ;;
  esac
  return 1
}

# Which pages are likely affected by a path.
hint() {
  case "$1" in
    internal/report/*) echo "docs/REPORT.md" ;;
    internal/a11y/*) echo "docs/ENGINE.md and docs/CLI.md" ;;
    internal/provider/*) echo "docs/PROVIDERS.md (the capability table is generated: go generate ./cmd/dummyrun)" ;;
    internal/telemetry/*) echo "docs/TELEMETRY.md" ;;
    internal/mcpserver/*) echo "docs/MCP.md" ;;
    internal/coverage/*) echo "docs/COVERAGE.md" ;;
    internal/policy/* | internal/robots/*) echo "docs/ETHICS.md and docs/ENGINE.md" ;;
    internal/cast/* | casts/library/*) echo "docs/PERSONAS.md" ;;
    internal/review/* | internal/record/*) echo "docs/ENGINE.md and docs/TELEMETRY.md" ;;
    internal/price/* | internal/signin/*) echo "docs/CLI.md and docs/ENGINE.md" ;;
    internal/*) echo "docs/ENGINE.md" ;;
    cmd/dummyrun/*) echo "docs/CLI.md (generated: go generate ./cmd/dummyrun)" ;;
    cmd/*) echo "docs/CLI.md and docs/ENGINE.md" ;;
    schema/*) echo "docs/TELEMETRY.md and the schema pages" ;;
    integrations/*) echo "the README under integrations/ and docs/integrations/" ;;
    docker/*) echo "docs/INSTALL.md and docs/ENGINE.md" ;;
    .goreleaser.yaml) echo "docs/INSTALL.md and docs/RELEASING.md" ;;
    site/layouts/*) echo "site/content/ and the page that uses the layout" ;;
  esac
}

behaviour=""
doc_changed=no
while IFS= read -r -d '' f; do
  [ -z "$f" ] && continue
  if is_behaviour "$f"; then behaviour="$behaviour$f"$'\n'; fi
  if is_doc "$f"; then
    case "$f" in
      CHANGELOG.md) ;; # Checked separately, by what changed under Unreleased.
      site/layouts/*) ;; # A layout is code that happens to be a doc path. It never satisfies the guard alone.
      *) doc_changed=yes ;;
    esac
  fi
done

# Nothing user-facing changed, so there is nothing to check.
if [ -z "$behaviour" ]; then
  echo "No user-facing behaviour paths changed. Nothing to check."
  exit 0
fi

# A new line under Unreleased.
changelog=no
if [ -n "${DOCS_GUARD_BASE:-}" ] && [ -n "${DOCS_GUARD_HEAD:-}" ]; then
  unreleased() { git show "$1:CHANGELOG.md" 2>/dev/null | awk '/^## \[Unreleased\]/{on=1;next} /^## \[/{on=0} on'; }
  # Compare with the merge base, as the file list does, so changes that landed on the base branch later do not count.
  mb=$(git merge-base "$DOCS_GUARD_BASE" "$DOCS_GUARD_HEAD")
  # Only lines the pull request added count. Removing or editing nothing new does not.
  added=$(unreleased "$DOCS_GUARD_HEAD" | grep -vxFf <(unreleased "$mb") | grep -Ev '^(#|[[:space:]]*$)' || true)
  if [ -n "$added" ]; then changelog=yes; fi
else
  changelog="${DOCS_GUARD_CHANGELOG:-no}"
fi

has_label=no
if printf '%s\n' "$labels" | grep -Eq '(^|")no-docs-needed("|$)'; then has_label=yes; fi
has_reason=no
if printf '%s\n' "$body" | tr -d '\r' | grep -Eq '^Docs: not needed because [[:alnum:]]'; then has_reason=yes; fi

if [ "$has_label" = yes ] && [ "$has_reason" = yes ]; then
  echo "The no-docs-needed label and a reason are present. Passing."
  exit 0
fi

problems=""
if [ "$doc_changed" = no ]; then problems="${problems}docs "; fi
if [ "$changelog" = no ]; then problems="${problems}changelog "; fi

if [ -z "$problems" ]; then
  echo "Docs and CHANGELOG moved with the code."
  exit 0
fi

{
  echo "This pull request changes behaviour a user can see, so the docs need to move with it."
  echo
  echo "Changed paths that count as behaviour:"
  printf '%s' "$behaviour" | while IFS= read -r f; do echo "  - $f"; done
  echo
  echo "Pages likely affected:"
  printf '%s' "$behaviour" | while IFS= read -r f; do
    h=$(hint "$f")
    [ -n "$h" ] && echo "  - $f -> $h"
  done | sort -u
  echo
  case "$problems" in *docs*) echo "Missing: a change under docs/, README.md or site/content/." ;; esac
  case "$problems" in *changelog*) echo "Missing: a CHANGELOG.md entry under Unreleased." ;; esac
  echo
  echo "If no docs are needed, add the no-docs-needed label and put this line in the pull request description:"
  echo "  Docs: not needed because <one-line reason>"
  if [ "$has_label" = yes ] && [ "$has_reason" = no ]; then
    echo
    echo "The label is on, but the description has no 'Docs: not needed because ...' line yet."
  fi
} | tee -a "${GITHUB_STEP_SUMMARY:-/dev/null}" >&2
echo "::error title=Docs move with the code::Update the docs and CHANGELOG, or add the no-docs-needed label with a reason. See the job summary."
exit 1
