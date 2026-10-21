# The website

The site at dummyrun.dev is built with Hugo 0.165.0 from this directory and deployed to
Cloudflare Pages by `.github/workflows/site.yml`. The reference pages render from the repository's own
Markdown through mounts in `hugo.toml` and the content adapter in `content/docs/_content.gotmpl`, so
`docs/ARCHITECTURE.md` and the site's architecture page are one file. The audience pages and the home
page live in `content/`.

## Pages and navigation

| Part | Where it lives |
|---|---|
| Top-level pages | `content/how-it-works.md`, `content/start.md` and `content/personas.md`. The header and footer link to them by path |
| Audience pages | `content/for/`. A page sets `role`, `icon`, `lead_short` and `weight`. The home page and the index build their cards from those |
| Reference pages | `content/docs/`, and the adapter in `content/docs/_content.gotmpl` for pages rendered from the repository's Markdown. A page joins a group in `hugo.toml` with `group` |
| Next step | Set `next` in the front matter with `title`, `text`, `label` and `href`. Use `repo` for a path in the repository. A reference page without one points at the page after it. Any other page without one shows none, so give every new page one |
| Mock-ups | A graphic goes in through the `graphic` shortcode or `partials/example.html`, which adds the dashed frame and the Example label. Never place a graphic without it |
| Persona library | The cards come from `library` in the front matter of `content/personas.md` |

Links between pages never carry a `#fragment` to another page, because the offline link check cannot
resolve one on a directory URL.

## Build and check locally

```sh
cd site
npm ci
hugo --gc --minify --panicOnWarning
npm run check:html
npm run serve &
npm run check:a11y
lychee --offline --root-dir "$PWD/public" --include-fragments 'public/**/*.html'
```

To check one page on its own, with `dummyrun` installed and the site served on port 1313:

<!-- doctest site -->
```sh
dummyrun a11y --viewport desktop --scheme light --allow site/a11y-allow.txt http://127.0.0.1:1313/start/
```

`hugo server` serves a live preview on port 1313. The accessibility check is `dummyrun a11y`, the
engine's own sweep (see `docs/ENGINE.md`). It runs every page in the sitemap, and the 404 page, at
1280 and 375 pixels wide in the light and dark colour schemes. It needs Go, Chrome with chromedriver
(`dummyrun browsers install`, or `DUMMYRUN_CHROME` and `DUMMYRUN_CHROMEDRIVER`) and a built site served on port 1313. Add `--browser firefox`
to run it on Firefox. Accepted violations go in `a11y-allow.txt`, each with a reason.

## Large text

Phones apply a larger system text size in different ways. Some raise the browser's default font size, and some
scale the page. The site is sized in `rem` and `em` and its layout breaks (media queries) are in `em`, so it
follows the first kind and a larger size reaches the narrow layout on a wider screen. Do not give anything that
holds text a fixed `px` height or width. Use `min-height`, let buttons wrap their own text, and give grid and
flex children `min-width: 0` (`minmax(0, 1fr)` in grids) so they can shrink. Long words, URLs and emails wrap.
Code blocks and tables scroll inside themselves, never the page.

`dummyrun a11y --text-scale 100,200 --layout` checks it. With the built site served on port 1313, run this from
the repository root:

<!-- doctest site -->
```sh
dummyrun a11y --viewport small --scheme light --text-scale 200 --layout --allow site/a11y-allow.txt http://127.0.0.1:1313/start/
```

It opens each page in Chrome at 320 pixels wide with the browser's default font size doubled. It fails when the
page scrolls sideways. It fails when a button or label clips its text. It fails when two controls overlap. Add `--sitemap` and
`--rebase` to test every page, and `--screenshots DIR` to save a crop of each fault. `docs/ENGINE.md` describes the
rules. The site workflow runs it on every page in the sitemap.

## Social cards and structured data

Each main page has a 1200 by 630 link-preview card in `static/social/`, named for the page path with
hyphens, such as `for-qa-lead.png`. A page without its own card uses its section's card, then
`brand/logo/social-card.png` (code: the card lookup in `site/layouts/_partials/head.html`). Run `go run ./internal/tools/socialcards` from the repository root. It does not work from `site`. It draws the cards from the list in
that program and `-check` fails when one is out of date (code: `internal/tools/socialcards/main.go`). Add a card by adding a line to the list. The
home page also carries JSON-LD that describes the software as free and open source.

## Typefaces

Headings are Space Grotesk, the face of the wordmark. Body text is Atkinson Hyperlegible Next, which the
Braille Institute designed for readers with low vision. Both are self-hosted from `static/fonts/` under
the SIL Open Font License 1.1, recorded in `REUSE.toml`.

## Headers

`static/_headers` sets the Content Security Policy and the other security headers that Cloudflare Pages
serves. The policy allows one third party: the Cloudflare Web Analytics script from
`static.cloudflareinsights.com`, and its reports. The Pages setup reports to this site at `/cdn-cgi/rum`. A manual embed reports to `cloudflareinsights.com/cdn-cgi/rum`, so the policy allows that host too. Cloudflare
injects that script on the next deployment after the owner turns analytics on for the Pages project (sourced: Cloudflare Web Analytics documentation, Get started). It sets no cookies and does not
track individual visitors (sourced: the same documentation). There are no inline executable scripts or styles. The JSON-LD data block in the page head is not executable.
