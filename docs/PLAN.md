# Plan: from private deployment to open-source product

**Plan** · 18 October 2026 · maintainers

> - Dummy Run already exists and works, inside a private deployment: a cast of personas, several hundred
>   reviewed runs, a region-pinned reviewer, and a results board.
> - This repository starts with a fresh history. The engine, reviewer and board are carried over by
>   selection; the deployment's personas, run data, research and branding are not.
> - First milestone: a demo target app and an example cast so anyone can see a full run on a laptop.
> - Then packaging (Helm, docker-compose) and the cloud adapters, tested on Google Cloud and Azure.

## 1. Why a fresh repository

The private repository's history contains details of the product it tests: names, preview URLs,
pull-request numbers, tickets, research and findings. A string-level
scrub of the working tree would leave all of that in `git log`. Publishing the findings is a disclosure
decision for the product's owner, not a side effect of a rename. So nothing is moved with `git mv`; files
are copied in after being generalised, and CI's `clean-tree` job keeps the identifiers out for good.

## 2. What comes across, and what does not

| Comes across, generalised | Stays private |
|---|---|
| Harness: session, observation model, step log, screenshot capture, review checks, record-batch, compare-batches | All run results, sweeps and findings |
| The preview-environment adapter, rewritten as the first pluggable **target adapter** (find the preview for a PR, provision a role-scoped test user, seed fixtures) with every host name and repository moved to configuration | The deployment's personas and scenarios |
| Reviewer: claim-by-claim checking with code references and three-vote verdicts | Research built from the deployment's traffic, tickets and PRs |
| Results board build and template, re-skinned | Brand assets |
| Docs: running, when it runs, running on Google Cloud, reviewer design, efficacy, harness ideas, prior art, model and runner settings, each read for deployment facts before it is copied | Roadmap and handover notes for that deployment |
| Persona format and the persona validator, with a new example cast for a demo app | |

The private repository stays as the deployment, with a note pointing here.

## 3. Milestones

1. **Scaffold** (this commit): community files, CI gates, docs.
2. **Engine**: harness, reviewer and board copied in under the grep gate; `dummyrun` CLI with `run`,
   `review`, `board`; the board is static HTML plus JSON. The engine language is chosen by an evidence-based
   comparison before the port (not Node or Playwright; WebDriver BiDi for Chrome and Firefox is a hard requirement).
3. **Demo**: a small open-source target app with real roles (for example an internal tools app with
   admin, manager, operative and external approver), a cast of five or six personas, a scripted first run
   that ends on a board with real findings.
4. **Packaging**: `deploy/helm/dummyrun` and `deploy/compose`, with a values file per cloud; placeholder
   packages on npm, RubyGems and PyPI become real.
5. **Clouds**: Google Cloud (Vertex AI in `europe-west2`, Cloud Run jobs) as today; Azure (Azure AI
   Foundry in UK South, Container Apps) tested on a sponsor's subscription with new resources only; an
   integration matrix that runs both against the demo app on every release.
6. **Targets**: adapters for plain URL, Vercel, Netlify, Cloudflare Pages and GitHub Deployments.
7. **Programmes**: apply for the free-for-open-source services in [FREE-FOR-OSS.md](FREE-FOR-OSS.md) in
   the order given there.

## 4. Vocabulary

Personas, scenarios and cast stay; they are generic. "Soundboard" is retired in favour of **results
board** (or just the board). The deployment's word for a preview environment is replaced by **preview**.
The crash-test theme gives us "dummies" for the cast when we want to be playful, and a mascot; see
[NAMING.md](NAMING.md) for the cautions.

## 5. Decisions log

| Date | Decision | Why |
|---|---|---|
| 2026-10-18 | Name: Dummy Run; org `dummy-run`; CLI `dummyrun` | Research in NAMING.md; the unhyphenated GitHub name is held by a dormant account GitHub will not release without a trademark |
| 2026-10-18 | Licence: Apache-2.0, DCO sign-off, no CLA; name protected by TRADEMARKS.md | [LICENSING.md](LICENSING.md). AGPL-3.0 was the runner-up and is rejected there with reasons |
| 2026-10-18 | Commercial model: paid services (fixed-price audits, async support); no hosted version yet | Lowest upkeep for a solo maintainer; a hosted service brings data-processor duties and security-audit costs |
| 2026-10-18 | Fresh history | Section 1 |
| 2026-10-18 | Body text 17px minimum on every surface | TYPOGRAPHY.md |
| 2026-10-18 | Domain: dummyrun.dev, registered on Cloudflare | .dev is HSTS-preloaded; docs and the public board will live there |
