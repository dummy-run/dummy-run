#!/bin/sh
# Fetches the ARIA-AT support figures for each ARIA feature, by screen reader and browser, from the ARIA-AT app's
# public API, and writes the response unchanged to features.json. The figures are under the W3C Document License;
# see NOTICE in this directory. Update FETCHED in ../ariasupport.go to the date you ran this.
set -eu
cd "$(dirname "$0")"
query='{ ariaHtmlFeaturesMetrics { ariaFeaturesByAtBrowser { refId type linkText rawLinkText value rawValue total passed failed untestable passedPercentage formatted atName browserName atId browserId } } }'
printf '{"query":%s}' "$(printf '%s' "$query" | python3 -c 'import json,sys;print(json.dumps(sys.stdin.read()))')" |
	curl -sSf -X POST https://aria-at.w3.org/api/graphql -H 'content-type: application/json' --data @- -o features.json.new
# A response can be 200 and still carry GraphQL errors or no figures, so it replaces features.json only when it has both
# no errors and some figures.
python3 -I -c 'import json,sys
d = json.load(open(sys.argv[1]))
rows = ((d.get("data") or {}).get("ariaHtmlFeaturesMetrics") or {}).get("ariaFeaturesByAtBrowser") or []
if d.get("errors") or not rows: sys.exit("fetch.sh: the response has errors or no figures; features.json is unchanged")' features.json.new || { rm -f features.json.new; exit 1; }
mv features.json.new features.json
