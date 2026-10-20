# Badges

`dummyrun report` can write a small badge for a README or a dashboard. The badge shows how many findings are open and when the newest run ended. It is a pointer to the report. It is not a verdict.

![Dummy Run badge: 4 open, 17 Oct 2026](images/badge-example.svg)
![Dummy Run badge: 5 open unreviewed, 17 Oct 2026](images/badge-example-unreviewed.svg)

## Make a badge

```sh
dummyrun report --runs runs --format badge --out badge.svg
dummyrun report --runs runs --format shields --out badge.json
```

Two formats keep one command for one file. `--format badge` writes the SVG. `--format shields` writes the JSON that [shields.io](https://shields.io/badges/endpoint-badge) reads. Both say the same thing. A separate flag would need a second output path, so a format is the simpler fit.

Every other `dummyrun report` flag works. Use `--build SHA` to count one build. Use `--baseline` and `--a11y` as you would for a report. The badge ignores them apart from the build choice.

## What the badge says

The left half says Dummy Run and carries a small viewfinder mark. The right half says how many findings are open and the date of the newest run. The date is the end of that run in UTC, written as `D Mon YYYY`.

- The count is confirmed findings. A finding is open when the report lists it. Findings are grouped by fingerprint, so a problem that three runs found counts once.
- With `--include pending` the count also holds findings no person has reviewed. The message then says `open unreviewed`.
- The colour follows the highest open severity. Blocker is red and major is orange. Minor is amber. Info and no findings are grey-blue.
- The count does not include axe rules from `--a11y`. They are in the report.
- With no runs, for example when you give only an axe sweep, the date is the report date.

The SVG is under 2 KB. It has a `<title>` and an `aria-label` for screen readers. It uses no script and loads no font. The text widths come from a fixed table.

## What the badge does not say

A badge cannot say that the product has no problems. The runs are a sample, so zero open findings means only that the sample did not confirm any. A grey-blue badge says nothing about how much of the product the personas saw. It does not say that any standard is met. It says nothing about the findings no one has reviewed unless you chose `--include pending`.

Link the badge to the published report so a reader can see the scope and the limits. Link it to [dummyrun.dev](https://dummyrun.dev/) if you want to say what made it.

The badge text never uses the words that read as a verdict. A unit test checks this.

## Shields.io endpoint

The JSON follows the shields.io endpoint schema.

```json
{
  "schemaVersion": 1,
  "label": "Dummy Run",
  "message": "4 open · 17 Oct 2026",
  "color": "b85c00",
  "namedLogo": "none"
}
```

Host the file anywhere that serves it over HTTPS with a stable URL. Then point shields at it.

```md
[![Dummy Run](https://img.shields.io/endpoint?url=https%3A%2F%2Fexample.github.io%2Frepo%2Fbadge.json)](https://example.github.io/repo/report.html)
```

The shields badge has its own mark and style. The SVG from `--format badge` keeps the Dummy Run viewfinder and needs no third party.

## Publish from GitHub Actions

There are three places to put the files.

### The gh-pages branch

This job rebuilds the badge and the report after your runs finish. It commits both to `gh-pages`. Give the job `contents: write`. The `gh-pages` branch must already exist. Replace `VERSION` with a release tag you have reviewed.

```yaml
publish-badge:
  runs-on: ubuntu-latest
  permissions:
    contents: write
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: stable
    - run: go install github.com/dummy-run/dummy-run/cmd/dummyrun@VERSION
    - run: |
        mkdir site
        dummyrun report --runs runs --out site/report.html
        dummyrun report --runs runs --format badge --out site/badge.svg
        dummyrun report --runs runs --format shields --out site/badge.json
    - run: |
        git fetch origin gh-pages
        git worktree add -B gh-pages ../pages FETCH_HEAD
        cp site/* ../pages/
        cd ../pages
        git add .
        if git diff --cached --quiet; then exit 0; fi
        git -c user.name=ci -c user.email=ci@users.noreply.github.com commit -m "Update badge"
        git push origin gh-pages
```

The badge is then at `https://OWNER.github.io/REPO/badge.svg`. Pin the actions to commit hashes if your policy asks for it.

### A release asset

Upload the files to a release with the GitHub CLI.

```sh
gh release upload badges badge.svg badge.json --clobber
```

The URL is `https://github.com/OWNER/REPO/releases/download/badges/badge.svg`. GitHub serves it with a redirect, so shields may need the final URL. The gh-pages route avoids that.

### A gist

Upload the JSON to a gist and point shields at the raw URL. Use a token with the `gist` scope, stored as a secret.

```sh
gh gist edit GIST_ID --filename badge.json badge.json
```

The raw URL is `https://gist.githubusercontent.com/OWNER/GIST_ID/raw/badge.json`. Leave the revision out of it so it always serves the newest file.
