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

## Not built yet

These ideas are written down so they aren't lost. None of them exists in the code today.

- A model reviewer that judges each finding claim by claim, with code references and three votes. It
  would run after deterministic checks such as axe, CSP, status codes and allowed hosts. A person would
  still check high-severity candidates.
- A results board of static HTML and JSON built from run records.
- Target adapters for GitHub Deployments, Vercel, Netlify and Cloudflare Pages. Each would find the preview
  for a pull request, create a test user with a role and load test data.
- Runner hosts as Kubernetes jobs from a Helm chart, Cloud Run jobs and Azure Container Apps jobs. Results
  in object storage such as GCS, Azure Blob or S3. Secrets from the OS keychain, Secret Manager or Key Vault.
- Real devices through a hosted browser grid, and separate models for playing and reviewing.
- Market profiles. A profile such as `uk` would list the regions that satisfy it, for example
  `europe-west2` and `uksouth`. A run would refuse to start if any part of the chain couldn't meet it. Today
  `--region` goes to the provider and into the record, and nothing checks it against a market.

The reasons behind past choices are in [DECISIONS.md](DECISIONS.md).
