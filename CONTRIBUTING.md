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

## Checks

The same checks run on your machine before each commit and in CI on each pull request. To install the
local hooks, run:

```sh
brew install lefthook gitleaks typos-cli actionlint zizmor uv
lefthook install
```

| Check | What it catches |
|---|---|
| gitleaks | Secrets in staged changes and in history |
| `bin/check-prose` and the em-dash check | Breaks of the plain-wording rules in docs/STYLE.md |
| typos | Spelling mistakes |
| actionlint and zizmor | Broken or unsafe GitHub Actions workflows |
| check-jsonschema | Example runs that do not match the run-record schema |
| REUSE | Files without copyright and licence information |
| Semgrep and CodeQL | Security problems in code (CI only) |
| Dependency review | New dependencies with known vulnerabilities or barred licences (CI only) |
| OpenSSF Scorecard | Weak project security practices, weekly on `main` |

## Pull requests

- Branch from `main`. Pull requests are squash-merged, so the title becomes the commit message.
- Sign off every commit with `git commit -s`. This certifies the
  [Developer Certificate of Origin](https://developercertificate.org/), and CI checks it. Your
  contribution is licensed under [Apache-2.0](LICENSE). There is no other agreement to sign.
- New behaviour comes with automated tests in the same pull request. A bug fix comes with a test that
  failed before the fix. Reviewers do not merge code changes without tests.
- One change per pull request. A persona and a harness change are two pull requests.
- Say what you changed and how you checked it.

## Reporting bugs and asking for features

Report bugs and request features through [GitHub issues](https://github.com/dummy-run/dummy-run/issues).
Ask questions in [Discussions](https://github.com/dummy-run/dummy-run/discussions). Report security
problems privately, as described in [SECURITY.md](SECURITY.md). The maintainer replies to issues within
14 days.

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
