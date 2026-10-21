#!/bin/sh
# Rebuilds the report pictures in docs/images from examples/report/report.html with headless Chrome:
#   report-example.png  the cover
#   report-summary.png  the summary page: key numbers and how sure you can be
#   report-finding.png  one finding card with its screenshot
#   report-print.png    the first three pages as printed to PDF, side by side
# The pictures are always the light theme. Run it from the repository root. It needs python3 with Pillow
# and pdftoppm. Set CHROME to the browser binary if it is not in the default place.
set -eu
CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
work="$(mktemp -d)"
# Work on a copy: switch the dark scheme off so the pictures do not depend on the machine, and add a
# script that writes the position of each part into the page so the crops need no hard-coded numbers.
sed 's/prefers-color-scheme: dark/prefers-color-scheme: never/g' examples/report/report.html |
  sed 's|</body>|<script>addEventListener("load",function(){var r=function(s){var b=document.querySelector(s).getBoundingClientRect();return [b.left,b.top+scrollY,b.width,b.height]};document.body.setAttribute("data-rects",JSON.stringify({cover:r("header.cover"),summary:r("#summary"),cards:r("#summary .cards"),sure:r("#summary .panel"),finding:r("article.finding"),page:document.documentElement.scrollHeight}))})</script></body>|' > "$work/report.html"
chrome() { "$CHROME" --headless=new --disable-gpu --no-first-run --user-data-dir="$work/profile" --hide-scrollbars --virtual-time-budget=3000 "$@" 2>/dev/null; }
chrome --window-size=1100,900 --dump-dom "file://$work/report.html" > "$work/dom.html"
rects="$(sed -n 's/.*data-rects="\([^"]*\)".*/\1/p' "$work/dom.html" | sed 's/&quot;/"/g')"
height="$(python3 -c 'import json,sys; print(int(json.loads(sys.argv[1])["page"])+40)' "$rects")"
chrome --window-size=1100,"$height" --screenshot="$work/full.png" "file://$work/report.html"
chrome --no-pdf-header-footer --print-to-pdf="$work/report.pdf" "file://$work/report.html"
pdftoppm -r 80 -png -f 1 -l 3 "$work/report.pdf" "$work/page"
python3 - "$work" docs/images "$rects" <<'PY'
import glob, json, sys
from PIL import Image
work, out, rects = sys.argv[1], sys.argv[2], json.loads(sys.argv[3])
full = Image.open(f"{work}/full.png").convert("RGB")
def crop(box, name, pad=0):
    x, y, w, h = box
    full.crop((0, max(0, int(y) - pad), full.width, min(full.height, int(y + h) + pad))).save(f"{out}/{name}", optimize=True)
crop(rects["cover"], "report-example.png")
top = rects["summary"][1] + 40
bottom = rects["sure"][1] + rects["sure"][3]
crop([0, top, 0, bottom - top], "report-summary.png", 0)
crop(rects["finding"], "report-finding.png", 24)
pages = [Image.open(p).convert("RGB") for p in sorted(glob.glob(f"{work}/page-*.png"))]
w, h = pages[0].size
sheet = Image.new("RGB", (w * len(pages) + 24 * (len(pages) + 1), h + 48), (222, 219, 211))
for i, p in enumerate(pages):
    sheet.paste(p, (24 + i * (w + 24), 24))
sheet.save(f"{out}/report-print.png", optimize=True)
PY
