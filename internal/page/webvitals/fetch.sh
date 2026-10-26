#!/bin/sh
# Fetches the web-vitals attribution build, unchanged, from the npm registry. Update the version here and
# WebVitalsVersion in ../inp.go together.
set -eu
cd "$(dirname "$0")"
version=6.2.3
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -sSfL "https://registry.npmjs.org/web-vitals/-/web-vitals-$version.tgz" -o "$tmp/wv.tgz"
tar -xzf "$tmp/wv.tgz" -C "$tmp"
cp "$tmp/package/dist/web-vitals.attribution.iife.js" .
cp "$tmp/package/LICENSE" LICENSE
