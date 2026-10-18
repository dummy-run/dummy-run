# The website

[dummyrun.dev](https://dummyrun.dev) is built with Hugo 0.165.0 from this directory and deployed to
Cloudflare Pages by `.github/workflows/site.yml`. The reference pages render from the repository's own
Markdown through mounts in `hugo.toml` and the content adapter in `content/docs/_content.gotmpl`, so
`docs/ARCHITECTURE.md` and the site's architecture page are one file. The audience pages and the home
page live in `content/`.

## Build and check locally

```sh
cd site
npm ci
hugo --gc --minify --panicOnWarning
npm run check:html
npm run serve &
THEME=light npm run check:a11y
THEME=dark npm run check:a11y
lychee --offline --root-dir "$PWD/public" --include-fragments 'public/**/*.html'
```

`hugo server` serves a live preview on port 1313. The accessibility check runs every page at 1280 and
375 pixels wide. Set `CHROME_PATH` to a Chrome binary if Puppeteer has not downloaded one.

## Typefaces

Headings are Space Grotesk, the face of the wordmark. Body text is Atkinson Hyperlegible Next, which the
Braille Institute designed for readers with low vision. Both are self-hosted from `static/fonts/` under
the SIL Open Font License 1.1, recorded in `REUSE.toml`.

## Headers

`static/_headers` sets the Content Security Policy and the other security headers that Cloudflare Pages
serves. The policy allows nothing from third parties. There are no inline scripts or styles, no
analytics and no cookies.
