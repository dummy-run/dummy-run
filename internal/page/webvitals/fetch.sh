#!/bin/sh
# Fetches the web-vitals attribution build, unchanged, from the npm registry, and checks the package against the
# SHA-256 below before using it. Update the version, the digest and WebVitalsVersion in ../inp.go together. The
# digest of a new version is the SHA-256 of its .tgz; check it matches the sha512 "integrity" the registry lists.
set -eu
cd "$(dirname "$0")"
version=6.2.3
sha256=0345c05eceae57e921485b0696ab979ef559f3c3dda56abad6ad8145e399a262
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -sSfL "https://registry.npmjs.org/web-vitals/-/web-vitals-$version.tgz" -o "$tmp/wv.tgz"
echo "$sha256  $tmp/wv.tgz" | sha256sum -c -
tar -xzf "$tmp/wv.tgz" -C "$tmp"
cp "$tmp/package/dist/web-vitals.attribution.iife.js" .
cp "$tmp/package/LICENSE" LICENSE
