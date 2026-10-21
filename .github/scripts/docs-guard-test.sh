#!/usr/bin/env bash
# Tests for docs-guard.sh. Run: .github/scripts/docs-guard-test.sh
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fail=0

# check <name> <expected exit> <files> [VAR=value ...]
check() {
  local name=$1 want=$2 files=$3
  shift 3
  local got
  printf '%b' "$files" | tr '\n' '\0' | env GITHUB_STEP_SUMMARY=/dev/null "$@" "$here/docs-guard.sh" >/dev/null 2>&1
  got=$?
  if [ "$got" -ne "$want" ]; then
    echo "FAIL: $name (want $want, got $got)"
    fail=1
  else
    echo "ok:   $name"
  fi
}

check "no behaviour paths"               0 'docs/CLI.md\nsite/assets/x.css\n'
check "tests only"                       0 'internal/report/report_test.go\n'
check "tooling only"                     0 'internal/tools/docgen/main.go\n'
check "code only"                        1 'internal/report/report.go\n'
check "code and docs, no changelog"      1 'internal/report/report.go\ndocs/REPORT.md\n'
check "code and changelog, no docs"      1 'internal/report/report.go\nCHANGELOG.md\n' DOCS_GUARD_CHANGELOG=yes
check "code, docs and changelog"         0 'internal/report/report.go\ndocs/REPORT.md\nCHANGELOG.md\n' DOCS_GUARD_CHANGELOG=yes
check "site content counts as docs"      0 'cmd/dummyrun/main.go\nsite/content/start.md\n' DOCS_GUARD_CHANGELOG=yes
check "layout alone is not docs"         1 'site/layouts/index.html\n' DOCS_GUARD_CHANGELOG=yes
check "layout plus content"              0 'site/layouts/index.html\nsite/content/start.md\n' DOCS_GUARD_CHANGELOG=yes
check "casts library"                    1 'casts/library/x.json\n'
check "goreleaser"                       1 '.goreleaser.yaml\n'
check "label without reason"             1 'schema/report.schema.json\n' DOCS_GUARD_LABELS=no-docs-needed
check "reason without label"             1 'schema/report.schema.json\n' DOCS_GUARD_BODY='Docs: not needed because it is a typo'
check "label and reason"                 0 'schema/report.schema.json\n' DOCS_GUARD_LABELS=$'bug\nno-docs-needed' DOCS_GUARD_BODY=$'x\nDocs: not needed because it is a typo\n'
check "label as a JSON array"           0 'schema/report.schema.json\n' DOCS_GUARD_LABELS='["bug","no-docs-needed"]' DOCS_GUARD_BODY='Docs: not needed because it is a typo'
check "similar label is not enough"      1 'schema/report.schema.json\n' DOCS_GUARD_LABELS='["no-docs-needed-yet"]' DOCS_GUARD_BODY='Docs: not needed because it is a typo'
check "empty reason"                     1 'schema/report.schema.json\n' DOCS_GUARD_LABELS=no-docs-needed DOCS_GUARD_BODY='Docs: not needed because '
check "placeholder reason is rejected"   1 'schema/report.schema.json\n' DOCS_GUARD_LABELS=no-docs-needed DOCS_GUARD_BODY='Docs: not needed because ...'
check "dependabot exempt"                0 'internal/report/report.go\n' DOCS_GUARD_AUTHOR='dependabot[bot]'
check "a lookalike author is not exempt" 1 'internal/report/report.go\n' DOCS_GUARD_AUTHOR='dependabot-fan'

# Hints name the right page.
out=$(printf 'internal/a11y/x.go\0cmd/dummyrun/x.go\0' | GITHUB_STEP_SUMMARY=/dev/null "$here/docs-guard.sh" 2>&1 || true)
for want in 'docs/ENGINE.md and docs/CLI.md' 'docs/CLI.md (generated'; do
  if ! printf '%s' "$out" | grep -qF "$want"; then echo "FAIL: hint missing: $want"; fail=1; else echo "ok:   hint $want"; fi
done

# The CHANGELOG check against real git history.
repo=$(mktemp -d)
(
  cd "$repo" && git init -q -b main && git config user.email t@example.com && git config user.name t && git config commit.gpgsign false
  printf '# Changelog\n\n## [Unreleased]\n\n### Added\n- old entry\n\n## [0.1.0] - 2026-01-01\n' > CHANGELOG.md
  git add . && git commit -qm base
  git checkout -qb pr
  mkdir -p docs internal && echo x > docs/A.md && echo x > internal/a.go
  git add . && git commit -qm "code and docs, no entry"
  # Main moves on with its own entry. That must not satisfy the guard.
  git checkout -q main
  printf '# Changelog\n\n## [Unreleased]\n\n### Added\n- old entry\n- landed on main\n\n## [0.1.0] - 2026-01-01\n' > CHANGELOG.md
  git commit -qam "main entry"
  git checkout -q pr
  git merge -q main -m merge 2>&1 | tail -1
) >/dev/null 2>&1
git_check() { # name want
  local name=$1 want=$2 got
  (
    cd "$repo" || exit 9
    base=$(git rev-parse main)
    head=$(git rev-parse HEAD)
    printf 'docs/A.md\0internal/a.go\0' | DOCS_GUARD_BASE="$base" DOCS_GUARD_HEAD="$head" GITHUB_STEP_SUMMARY=/dev/null "$here/docs-guard.sh" >/dev/null 2>&1
  )
  got=$?
  if [ "$got" -ne "$want" ]; then echo "FAIL: $name (want $want, got $got)"; fail=1; else echo "ok:   $name"; fi
}
git_check "base entry added after branching does not count" 1
(
  cd "$repo" && printf '# Changelog\n\n## [Unreleased]\n\n### Added\n- old entry\n- landed on main\n- mine\n\n## [0.1.0] - 2026-01-01\n' > CHANGELOG.md && git commit -qam "my entry"
) >/dev/null 2>&1
git_check "an added line under Unreleased counts" 0
(
  cd "$repo" && printf '# Changelog\n\n## [Unreleased]\n\n### Added\n\n## [0.1.0] - 2026-01-01\n' > CHANGELOG.md && git commit -qam "remove entries"
) >/dev/null 2>&1
git_check "removing a line does not count" 1

exit $fail
