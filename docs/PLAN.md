# Plan

The first milestone is a demo app and an example cast, so a full run works on a laptop. Packaging and
cloud adapters follow.

## Milestones

1. **Scaffold.** Community files, CI checks and docs. Done.
2. **Engine.** The harness and reviewer, plus the results board. The engine is written in Go and drives
   Chrome and Firefox over WebDriver BiDi. The `dummyrun` command offers four commands today: `run`, `coverage`, `review` and `a11y`. The
   harness is built. A person reviews findings with `dummyrun review`. The model reviewer and the board are next. The board is static HTML plus JSON.
3. **Demo.** A small open-source app with real roles, such as editor, author and reviewer. A cast of five
   or six personas runs against it and ends on a board with real findings.
4. **Packaging.** A Helm chart in `deploy/helm/dummyrun` and a compose file in `deploy/compose`, with a
   values file per cloud. Packages are published to the registries for the chosen language.
5. **Clouds.** Google Cloud (Vertex AI in `europe-west2`, Cloud Run jobs) and Azure (Azure AI Foundry in
   UK South, Container Apps). Every release runs the demo cast on both.
6. **Targets.** Adapters for plain URLs and GitHub Deployments, then Vercel, Netlify and Cloudflare Pages.

## Words we use

A cast is a set of personas. Each persona has scenarios. A preview is a copy of an app built for one pull
request. Findings land on the results board. The crash-test theme calls the cast "dummies" and gives us a
mascot. The cautions are in [NAMING.md](NAMING.md).

## Decisions

| Date | Decision | Why |
|---|---|---|
| 2026-10-18 | Name: Dummy Run, org `dummy-run`, command `dummyrun` | [NAMING.md](NAMING.md). The GitHub name `dummyrun` belongs to a dormant account |
| 2026-10-18 | Licence: Apache-2.0 with DCO sign-off and a trademark policy | [LICENSING.md](LICENSING.md) |
| 2026-10-19 | Body text at least 18px on every surface | [TYPOGRAPHY.md](TYPOGRAPHY.md) |
| 2026-10-18 | Logo: the Viewfinder mark, with the Dummy as mascot | [brand/README.md](../brand/README.md) |
| 2026-10-18 | Domain: dummyrun.dev | Docs and the public board will live there |
