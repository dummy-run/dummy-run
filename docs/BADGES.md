# Badges

`dummyrun report` can write a small badge for your README or dashboard. It shows how many findings are open and
when the newest run ended. Think of it as a link to the report, never a verdict on the product.

![Dummy Run badge: 4 open, 17 Oct 2026](images/badge-example.svg)
![Dummy Run badge: 5 open unreviewed, 17 Oct 2026](images/badge-example-unreviewed.svg)

## Make a badge

```sh
dummyrun report --runs runs --format badge --out badge.svg
dummyrun report --runs runs --format shields --out badge.json
```

`--format badge` writes an SVG. `--format shields` writes the JSON that
[shields.io](https://shields.io/badges/endpoint-badge) reads. Both say the same thing.

The other `dummyrun report` flags are accepted too. `--build SHA` counts one build, and `--include pending` is
covered below. The badge ignores `--baseline` and `--a11y`.

## What the badge says

The left half says Dummy Run, with a small viewfinder mark. The right half gives the number of open findings
and the date the newest run ended, in UTC, written as `D Mon YYYY`.

- The count is confirmed findings that the report lists. Findings are grouped by fingerprint, so a problem
  three runs found counts once.
- With `--include pending`, the count also includes findings nobody has reviewed, and the message says
  `open unreviewed`.
- The colour follows the worst open severity. Blocker is red, major is orange and minor is amber. Info, or no
  findings at all, is grey-blue.
- Axe rules from `--a11y` aren't counted. You'll find them in the report.
- If there are no runs, say because you only gave an axe sweep, the date is the report date.

The SVG is under 2 KB. It has a `<title>` and an `aria-label` for screen readers, runs no script and loads no
font.

## What the badge does not say

A badge can't tell anyone your product has no problems. The runs are a sample, so zero open findings only
means the sample didn't confirm any. A grey-blue badge says nothing about how much of the product the personas
saw, and it doesn't claim any standard is met. Unless you chose `--include pending`, it says nothing about
findings nobody has reviewed.

Link the badge to the published report, so readers can see its scope and limits. Or link it to
[dummyrun.dev](https://dummyrun.dev/) if you'd rather say what made it. The badge never uses words like
"passed" or "compliant".

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

Host the file anywhere that serves it over HTTPS at a stable URL, then point shields at it:

```md
[![Dummy Run](https://img.shields.io/endpoint?url=https%3A%2F%2Fexample.github.io%2Frepo%2Fbadge.json)](https://example.github.io/repo/report.html)
```

The shields badge uses its own mark and style. If you want the Dummy Run viewfinder and no third party, use
the SVG from `--format badge`.

## Publish from GitHub Actions

You can publish the files in three places: the `gh-pages` branch, a release or a gist.

### The gh-pages branch

This job rebuilds the badge and the report after your runs finish, and commits both to `gh-pages`. The job
needs `contents: write`, and the `gh-pages` branch must already exist. Replace `VERSION` with a release tag
you've reviewed.

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
        mkdir -p badge-out
        dummyrun report --runs runs --out badge-out/report.html
        dummyrun report --runs runs --format badge --out badge-out/badge.svg
        dummyrun report --runs runs --format shields --out badge-out/badge.json
    - run: |
        git fetch origin gh-pages
        git worktree add -B gh-pages ../pages FETCH_HEAD
        cp badge-out/* ../pages/
        cd ../pages
        git add .
        if git diff --cached --quiet; then exit 0; fi
        git -c user.name=ci -c user.email=ci@users.noreply.github.com commit -m "Update badge"
        git push origin gh-pages
```

In the repository settings, set the Pages source to the `gh-pages` branch and the root folder. You only do
this once. The badge is then at `https://OWNER.github.io/REPO/badge.svg`. Pin the actions to commit hashes if
your policy asks for it.

### A release asset

To attach the files to a release, use the GitHub CLI:

```sh
gh release upload badges badge.svg badge.json --clobber
```

The URL is `https://github.com/OWNER/REPO/releases/download/badges/badge.svg`. GitHub serves it through a
redirect, so shields may need the final URL. Publishing to `gh-pages` avoids that.

### A gist

To use a gist, create it once by hand with `gh gist create badge.json` and note its ID. Then update it from
the workflow and point shields at its raw URL. The workflow needs a token with the `gist` scope, stored as a
secret.

```sh
gh gist edit GIST_ID --filename badge.json badge.json
```

The raw URL is `https://gist.githubusercontent.com/OWNER/GIST_ID/raw/badge.json`. Leave the revision out, so
it always serves the newest file.
