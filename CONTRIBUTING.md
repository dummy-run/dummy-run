# Contributing to Dummy Run

Until the engine lands, contribute through issues. Propose a persona or a target adapter, or point out a
problem with the [plan](docs/PLAN.md).

## Rules

1. **Evidence.** A claim about what a persona found needs a run ID, a step number and a screenshot or log
   line. Findings without evidence are removed.
2. **No real people.** Personas are fictional. Never commit a real customer's, company's or person's
   name or contact details. Never commit data from a target environment, which may hold production data.
3. **No identifiers from any customer deployment.** The "Tree is clean" check runs on `main`. If you
   spot one, report it through a private security advisory.
4. **No secrets.** Tokens, passwords and keys go in your keychain or CI secrets.
5. **Plain English, British spelling.** See [docs/STYLE.md](docs/STYLE.md). `bin/check-prose` checks the
   main pages.

## Pull requests

- Branch from `main`. Pull requests are squash-merged, so the title becomes the commit message.
- Sign off every commit with `git commit -s`. This certifies the
  [Developer Certificate of Origin](https://developercertificate.org/), and CI checks it. Your
  contribution is licensed under [Apache-2.0](LICENSE). There is no other agreement to sign.
- One change per pull request. A persona and a harness change are two pull requests.
- Say what you changed and how you checked it.

## Proposing a persona

Use the "Persona proposal" issue template. Give the role and its jobs, its device and conditions, and
what it may see. Add three to six scenarios. Say where each trait comes from: a public source, an inference
or your own experience. Personas built from a real customer's data are not accepted.

## Proposing a target adapter

Use the "Feature or adapter" issue template. Say how the platform exposes a preview for a pull request,
creates a test user with a role, and loads test data. Adapters that need production credentials are not
accepted.

## Code of conduct

See the [Code of Conduct](CODE_OF_CONDUCT.md).
