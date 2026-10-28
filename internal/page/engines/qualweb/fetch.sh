#!/bin/sh
# Fetches the QualWeb in-page bundles at the pinned versions and checks them against the hashes below.
# Run from the repository root: internal/page/engines/qualweb/fetch.sh
set -eu
dir=internal/page/engines/qualweb
tmp=$(mktemp -d)
trap 'rm -rf -- "${tmp:?}"' EXIT
get() { # package version path-in-package file sha256
	curl -fsSL "https://registry.npmjs.org/@qualweb/$1/-/$1-$2.tgz" -o "$tmp/$1.tgz"
	mkdir -p "$tmp/$1"
	tar -xzf "$tmp/$1.tgz" -C "$tmp/$1"
	echo "$5  $tmp/$1/package/$3" | sha256sum -c -
	cp "$tmp/$1/package/$3" "$dir/$4"
}
get qw-page 0.3.8 dist/qw-page.bundle.js qw-page.bundle.js 5cb545b7e960a08efe9887353f269d5a1f24c198bb9e3b3debc2f318a021d795
get util 0.7.4 dist/__webpack/util.bundle.js util.bundle.js 54c1b470c6ab7e9464374ff604b970f5b51bb49ab6a39aa12906dcd8b5efaa46
get act-rules 0.8.6 dist/__webpack/act.bundle.js act.bundle.js 69f33d04b1351d56b994a4878894284ec6ca07fa9e0fb832531aa0656d54fc48
get wcag-techniques 0.4.9 dist/__webpack/wcag.bundle.js wcag.bundle.js aa1fe8a250f07b73f668a676fe298439d1fdd11f900a80facd9ef06ca617baee
