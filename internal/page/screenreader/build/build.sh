#!/usr/bin/env sh
# Rebuilds ../vsr.min.js from the pinned virtual screen reader. Run it after changing a version in package.json,
# then update the version in internal/page/screenreader.go, NOTICE and docs/TOOLS.md.
set -eu
cd "$(dirname "$0")"
npm ci --ignore-scripts --no-audit --no-fund
npm rebuild esbuild
npm run build
rm -rf node_modules
