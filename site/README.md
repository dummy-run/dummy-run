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
(`DUMMYRUN_CHROME`, `DUMMYRUN_CHROMEDRIVER`) and a built site served on port 1313. Add `--browser firefox`
to run it on Firefox. Accepted violations go in `a11y-allow.txt`, each with a reason.

## Social cards and structured data

Each main page has a 1200 by 630 link-preview card in `static/social/`, named for the page path with
hyphens, such as `for-qa-lead.png`. A page without its own card uses its section's card, then
`brand/logo/social-card.png`. `go run ./internal/tools/socialcards` draws the cards from the list in
that program and `-check` fails when one is out of date. Add a card by adding a line to the list. The
home page also carries JSON-LD that describes the software as free and open source.

## Typefaces

Headings are Space Grotesk, the face of the wordmark. Body text is Atkinson Hyperlegible Next, which the
Braille Institute designed for readers with low vision. Both are self-hosted from `static/fonts/` under
the SIL Open Font License 1.1, recorded in `REUSE.toml`.

## Headers

`static/_headers` sets the Content Security Policy and the other security headers that Cloudflare Pages
serves. The policy allows nothing from third parties. There are no inline scripts or styles, no
analytics and no cookies.
