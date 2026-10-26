# Decisions

Why the project made the choices it did. This page is for contributors. The user docs describe what the
product does and leave the reasoning here.

## Log

| Date | Decision | Why |
|---|---|---|
| 2026-10-18 | Name: Dummy Run, org `dummy-run`, command `dummyrun` | [NAMING.md](NAMING.md). The GitHub name `dummyrun` belongs to a dormant account |
| 2026-10-18 | Licence: Apache-2.0 with DCO sign-off and a trademark policy | [Licence](#licence), and [LICENSING.md](LICENSING.md) for what it lets you do |
| 2026-10-19 | Body text at least 18px on every surface | [TYPOGRAPHY.md](TYPOGRAPHY.md) |
| 2026-10-18 | Logo: the Viewfinder mark, with the Dummy as mascot | [brand/README.md](../brand/README.md) |
| 2026-10-18 | Domain: dummyrun.dev | Docs and the public board will live there |

## Licence

### Why Apache-2.0

| | MIT | **Apache-2.0** | MPL-2.0 | AGPL-3.0 | FSL-1.1 | BSL 1.1 | Elastic 2.0 |
|---|---|---|---|---|---|---|---|
| OSI-approved open source | yes | **yes** | yes | yes | no | no | no |
| Friction for organisations | none | **none** | low | high (Google bans it) | high | high | high |
| Explicit patent grant | no | **yes** | yes | yes | limited | no | yes |
| A closed hosted copy must share its changes | no | **no** | only changed MPL files | yes, for changed versions | barred for 2 years | barred until a set date | barred |
| Compatible with bundled axe-core (MPL-2.0) | yes | **yes** | yes | yes | yes | yes | yes |

1. **Adoption.** QA teams, agencies and compliance staff run Dummy Run against their own apps. Their
   legal teams approve Apache-2.0 routinely and copyleft slowly.
2. **Patents.** Section 3 grants a patent licence from every contributor. MIT has no patent grant.
3. **Open source.** NLnet NGI Zero funds only software "published under a recognised free and open
   source license in its entirety". FSL, BSL and the Elastic License are not OSI-approved.
4. **No contributor agreement.** Contributions arrive under Apache-2.0, so a Developer Certificate of
   Origin sign-off is enough. The maintainer gets no rights that contributors lack.

### Why not AGPL-3.0

AGPL-3.0 was the runner-up. Its section 13 makes anyone who runs a changed version as a network service
share their changes, which is why Plausible, Elastic and Redis adopted it. We rejected it for adoption.
Google's policy says AGPL code "MUST NOT be used at Google", and other organisations ban it or send it to
legal review. Selling commercial licences alongside it would also need a contributor agreement. The cost
of Apache-2.0 is that anyone may build Dummy Run into a closed hosted product without sharing changes.
The trademark policy stops them calling it Dummy Run.

### Sources

- Apache License 2.0: https://www.apache.org/licenses/LICENSE-2.0
- SPDX licence list ("OSI Approved?" column): https://spdx.org/licenses/
- AGPL-3.0 section 13: https://spdx.org/licenses/AGPL-3.0-only.html
- Google AGPL policy: https://opensource.google/documentation/reference/using/agpl-policy
- NLnet: https://nlnet.nl/propose/ and https://nlnet.nl/NGI0/
- Plausible (2020): https://plausible.io/blog/open-source-licenses
- Elastic (2024): https://www.elastic.co/blog/elasticsearch-is-open-source-again
- Redis (2025): https://redis.io/blog/agplv3/
- Developer Certificate of Origin 1.1: https://developercertificate.org/

## Engine

- Zoom uses `browsingContext.setViewport` with `devicePixelRatio`. CSS `zoom` set from a script wouldn't
  survive a page load or change media queries, and it isn't standard on Firefox.
- WebDriver BiDi's only accessibility feature is the `accessibility` locator for
  `browsingContext.locateNodes` (https://w3c.github.io/webdriver-bidi/#type-browsingContext-Locator). It
  finds nodes by role and name and returns no tree. So the outline is built by a script run with
  `script.callFunction`, in `internal/page/tree.js`.
- `internal/page/tools_test.go` proves the read-only policy with a form whose POST never reaches the test
  server. `internal/runner/signin_browser_test.go` checks that credentials never leak into a run's output.

### HTML checking: vnu or html-validate

`--html` runs the Nu HTML Checker (vnu), which needs Java. The comparison below is from `PR #139`, which
gives the corpus, the commands and the counts. We compared vnu with
[html-validate](https://gitlab.com/html-validate/html-validate) 11.16.2 (MIT). That has a browser build, so it could run
inside the page like the rule engines. The corpus was 167 pages: the 150 AccessGuru cases, the three benchmark pages
and 14 pages we wrote, two for each kind of error `--html` keeps and two with none. html-validate used its
recommended rules with `no-missing-references` on.

| | vnu 26.10.12 | html-validate 11.16.2 |
|---|---|---|
| Added to the container image | about 150 MB (a jlink Java runtime and the 31 MB jar) | none: a 458 KB script (135 KB gzipped) in the binary |
| Runs where | its own process, on the HTML as sent and as built | inside the page, on the HTML as built, or on a string |
| Time for the 167 pages | 3.7 s | 1.9 s |
| Duplicate IDs, controls inside controls, list and table structure, images with no alt | 7 of the 8 planted pages: an image with a `title` and no alt passes | all 8 planted pages |
| A `label for` that names no element | found | found |
| An `aria-labelledby` or `aria-describedby` that names no element | missed | found |
| A misspelt role or an ARIA value that isn't allowed | found | missed |
| AccessGuru and benchmark pages with a finding `--html` keeps | 94 | 106 |
| Of those, found by both | 84 | 84 |

The 10 pages only vnu flags are all ARIA roles and values: 7 `aria-allowed-role` cases, an `aria-required-parent`
case and 2 `presentation-role-conflict` cases. axe-core already checks those on every page, with
`aria-roles`, `aria-allowed-role` and `aria-valid-attr-value`. So they add little to a sweep. The 22 pages only
html-validate flags are mostly elements in places the content model doesn't allow, plus empty headings and
references to missing IDs.

The recommendation is to move `--html` to html-validate in the page, and to keep vnu as an option for anyone who wants the
reading of the HTML as sent. The image would get about 150 MB smaller and the check would work outside the
container with no Java. The cost is the HTML-as-sent check, which html-validate can do only on a string the sweep
fetches. That switch isn't made yet because it changes what `--html` reports.

## Coverage and reports

- `dummyrun coverage` answers its three questions separately, because one blended number would hide which
  of them failed. Every figure names its universe, and a figure with no stated universe isn't printed.
- The coverage model was built from 19 real runs on two sites and from simulation. Nothing yet shows that
  the bands order problems correctly on real apps or that the stopping rule works there. The stopping-rule
  defaults are policy numbers that nobody has calibrated.
- Two ideas for coverage aren't built. One is similarity candidates, which would suggest findings on one
  route for a reviewer to join. The other is reading a list of the dimension values a team says matter.
- The badge is a report format (`badge` and `shields`) rather than a flag, so one command still writes one
  file. A flag would need a second output path. Text widths come from a fixed table, and the badge never
  uses verdict words. `internal/report/badge_test.go` checks that.

## Telemetry

- A record's `events` aren't exported, because the exporter's record type has no `events` field.
- The exporter reads observations only so that the record passes validation.
