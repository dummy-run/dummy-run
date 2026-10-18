# Services free for open-source projects

**Reference** · 18 October 2026 · maintainers

Checked 2026-10-18 against vendor pages and three maintained trackers (openperks.fru.dev, foropensource.com, thetechbasket.com). Terms change; the vendor page is the final word.

## The eligibility trap (!)

About half of these programmes require "non-commercial" or "no pathway to commercialization" (Docker DSOS, JetBrains, Netlify, 1Password, Crowdin, Mintlify, Snyk "no corporate backing"). Dummy Run's maintainer offers paid services around the project, so do not apply to those: the terms are explicit and some (Docker, Netlify) ask you to confirm it. The first group below has no such clause.

## Group A: usable by a company-backed open-source project

| Service | What you get | Condition | Apply |
|---|---|---|---|
| GitHub public repo | Unlimited Actions minutes and storage on standard runners, CodeQL, Dependabot, secret scanning, Pages | public repo | automatic |
| BrowserStack Open Source | Live, Automate and Percy, 5 users, 5 parallels, lifetime of the project. Directly relevant: real-device and cross-browser runs of the persona harness | "a qualifying open source project (public project URL)"; a company-domain email | https://www.browserstack.com/open-source |
| Sentry for Open Source | Business features: 5M errors, 1B spans, 5,000 GB logs, 100k replays, cron and uptime monitors, Seer for 5 contributors, no term limit | permissive licence (MIT or Apache-2.0) | https://sentry.io/for/open-source/ |
| Codecov | unlimited uploads and users on public repos, PR comments and status checks | public repo | https://about.codecov.io/for/open-source/ |
| SonarQube Cloud | Pro features on public repos | public, OSI licence | https://www.sonarsource.com/plans-and-pricing/sonarcloud/ |
| CircleCI open source | up to 400,000 Linux credits + 30,000 macOS/Windows credits a month | public repo on Free plan | https://circleci.com/open-source/ |
| Read the Docs Community | docs hosting, builds, versioning, PR previews, ad-supported | public repo and docs | https://about.readthedocs.com/pricing/ |
| Algolia DocSearch | hosted docs search and crawler | public technical docs, production ready | https://www.algolia.com/for-open-source |
| Chromatic | 35,000 snapshots a month (visual testing of the results board) | open source | https://www.chromatic.com/docs/open-source |
| Argos | free visual testing | open source | https://argos-ci.com/docs/open-source |
| CodeRabbit for open source | AI PR review, free | public repo | https://www.coderabbit.ai/blog/coderabbit-commits-1-million-to-open-source |
| Greptile | free AI code reviews | open source | https://www.greptile.com/open-source |
| SignPath | free code signing for release binaries | open source | https://signpath.io/solutions/open-source-community |
| Tailscale | free for the project's GitHub org (handy for reaching private previews) | OSI licence, GitHub auth | https://tailscale.com/kb/1154/free-plans-discounts |
| Blacksmith | faster Actions runners on request | open source | https://www.blacksmith.sh/pricing |
| Atlassian for open source | Standard cloud plans | open source licence request | https://www.atlassian.com/software/views/open-source-license-request |
| Datadog for open source | Datadog at no cost | open source | https://www.datadoghq.com/partner/open-source/ |
| Zulip | Cloud Standard for community chat | open source | https://zulip.com/for/open-source/ |
| GitHub Copilot Pro for maintainers | free Copilot Pro once the repo is "popular" (checked monthly) | automatic | https://github.com/open-source |
| GitLab for Open Source | Ultimate + 50,000 compute minutes | OSI, public, non-commercial | only if mirroring; non-commercial clause |

## Group B: non-commercial only (skip if a paid tier is planned)
Docker-Sponsored Open Source (no pull limits, Docker Team for a year; "must not seek to make a profit through services or by charging for higher tiers"), JetBrains All Products Pack, Netlify Open Source (10,000 credits a month, needs a "powered by Netlify" badge), 1Password Teams, Crowdin, Transifex, Mintlify Pro, Snyk Secure Developer (no corporate backing), Cloudflare Project Alexandria (non-profit only).

## Group C: money and cloud credits
| Programme | Amount | Status | Fit |
|---|---|---|---|
| Microsoft Azure credits for open source | yearly grants | "The application form is not currently open"; Microsoft now works through "strategic, organic relationships" with product teams | route via a sponsor subscription instead, or via the Secure OSS Fund below |
| GitHub Secure Open Source Fund | "$10,000 per project" (rechecked 2026-10-18; earlier notes of Azure credits were not on the page) | rolling; needs "demonstrated community traction" and a "clear open source license"; "15 hours over a 3-week period" | apply once there are external users; strong fit given the permission-leak angle |
| Microsoft FOSS Fund | up to $12,500 a quarter | in rounds, nominated by Microsoft employees | long shot |
| AWS Cloud Credits for Open Source | promotional credits for CI and testing | OSI licence, "not be dominated by a single vendor or VC-funded"; monthly review, none in December | borderline for a company-backed project; worth one application for the AWS target adapter |
| Google Cloud | no general OSS credit programme found; GCP for Startups is the usual route | | the current GCP spend stays on the existing project |
| Vercel Open Source Program | $3,600 over 3 years + partner starter pack | applications closed at check time | re-check for the results-board hosting |
| DigitalOcean credits for OSS | yearly credits tiered by stars | rolling | demo target app hosting |
| Neon Open Source Program | up to $10,000 credits | open | Postgres for the hosted results board |
| Claude for Open Source | 6 months Claude Max 20x per maintainer | needs 100+ external merged PRs or 20+ contributors | later |
| Codex for Open Source / Codex Open Source Fund | 6 months ChatGPT Pro; up to $25,000 API credits | rolling | later; useful for the pluggable-provider matrix |
| NLnet NGI Zero | €5,000 to €50,000 | next deadline 2026-11-03 12:00 CET | plausible: EU-relevant, privacy and accessibility angle (permission leaks, screen-reader findings) |
| Sovereign Tech Fund | from €50,000 | rolling | only for base infrastructure; not a fit yet |

## Suggested order

Licence decided (Apache-2.0, [LICENSING.md](LICENSING.md)) and the commercial model is paid services, so Group B is out.
1. Day one (no eligibility risk): GitHub public repo features, Codecov, SonarQube Cloud, Read the Docs, Sentry, BrowserStack, SignPath.
2. When the docs site exists: Algolia DocSearch; Chromatic or Argos for the results board.
3. Skip Group B: the project has a commercial pathway.
4. After first external users: GitHub Secure Open Source Fund, NLnet (deadline 3 Nov), AWS credits for the AWS adapter.
