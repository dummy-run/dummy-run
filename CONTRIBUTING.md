# Contributing to Dummy Run

Contribute through issues and pull requests. Propose a persona or a target adapter, fix a bug, or point out a
problem with the [plan](docs/PLAN.md). The engine is in Go, and [docs/ENGINE.md](docs/ENGINE.md) shows how to
build and run it.

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
| `bin/check-prose` and the em-dash check | Breaks of the writing rules in docs/STYLE.md. Long sentences only warn |
| typos | Spelling mistakes |
| actionlint and zizmor | Broken or unsafe GitHub Actions workflows |
| check-jsonschema | Example runs that do not match the run-record schema |
| REUSE | Files without copyright and licence information |
| Generated references | `docs/CLI.md` or the provider table in `docs/PROVIDERS.md` out of step with the code |
| Doctests | A documented command that no longer runs |
| Semgrep and CodeQL | Security problems in code (CI only) |
| Dependency review | New dependencies with known vulnerabilities or barred licences (CI only) |
| OpenSSF Scorecard | Weak project security practices, weekly on `main` |

## Definition of done

A change that a user would notice is done when it has all of these, in the same pull request.

1. **Tests.** New behaviour has automated tests. A bug fix has a test that failed before the fix.
2. **Docs.** The page that describes the behaviour says the new thing and no longer says the old thing. The
   pages are in `docs/`, the README, `integrations/*/README.md` and `casts/*/README.md`.
3. **Site content.** The pages in `site/content/` and the text in `site/layouts/` that mention the behaviour
   match it. Check the Get started page, the questions on the home page and the page for each role.
4. **Changelog.** `CHANGELOG.md` has an entry under Unreleased.
5. **Examples.** `examples/`, the library in `casts/library` and the run records in `examples/runs/` still
   work with the change, and a new feature has an example where one helps.

Two generators and a doctest runner keep part of this honest. Run them before you push. The pre-push hook runs
them as well.

```sh
go generate ./cmd/dummyrun
go run ./internal/tools/docgen -check
go run ./internal/tools/doctest
```

`go generate ./cmd/dummyrun` rewrites `docs/CLI.md` from each command's help text and the capability table in
`docs/PROVIDERS.md` from `internal/provider/capabilities.go`. Change the code, then generate, and never edit
the generated text by hand. `docgen -check` fails when a file is out of date, and it also fails when a provider
or an engine tool has no entry in the page that describes it.

The doctest runner builds `dummyrun` and `dummyrun-otel` from your tree and runs every shell block that has
the line `<!-- doctest -->` before it, in `docs/`, `site/content/` and the other Markdown files. Only safe
commands run: `version`, help, `dummyrun-otel export --dry-run` and `dummyrun a11y` on a site served on your
machine. A block marked `<!-- doctest site -->` needs the built site on port 1313 and runs with
`go run ./internal/tools/doctest -site`. Put the marker on a block that a reader will copy, so a command that
stops working fails a check and not a reader.

Review checks this as well. The pull request template has a "Docs and site" item. CodeRabbit warns when a
user-visible change to the code or the schema leaves out the docs, the changelog or the site content.

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
