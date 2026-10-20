# Contributing to Dummy Run

You can contribute through issues and pull requests. Propose a persona or a target adapter, fix a bug, or
point out a problem with the [plan](docs/PLAN.md). The engine is written in Go, and
[docs/ENGINE.md](docs/ENGINE.md) shows how to build and run it.

## Rules

1. **Evidence.** A claim about what a persona found needs a run ID, a step number, and a screenshot or a
   log line. We remove findings that have no evidence.
2. **No real people.** Personas are fictional. Never commit a real customer's, company's or person's name
   or contact details. Never commit data from a target environment, since it may hold production data.
3. **No identifiers from any customer deployment.** The "Tree is clean" check runs on `main`. If you spot
   one, report it through a private security advisory.
4. **No secrets.** Tokens, passwords and keys belong in your keychain or in CI secrets.
5. **Plain English, British spelling.** [docs/STYLE.md](docs/STYLE.md) has the rules, and
   `bin/check-prose` checks the main pages against them.

## Checks

The same checks run on your machine before each commit and in CI on each pull request. To install the
local hooks, run:

```sh
brew install lefthook gitleaks typos-cli actionlint zizmor uv
lefthook install
```

A hook skips a tool you haven't installed and tells you so. CI still runs every check.

| Check | What it catches |
|---|---|
| gitleaks | Secrets in staged changes and in history |
| `bin/check-prose` and the em-dash check | Breaks of the plain-wording rules in docs/STYLE.md |
| typos | Spelling mistakes |
| actionlint and zizmor | Broken or unsafe GitHub Actions workflows |
| check-jsonschema | Example runs that don't match the run-record schema |
| REUSE | Files without copyright and licence information |
| Generated references | `docs/CLI.md` or the provider table in `docs/PROVIDERS.md` out of step with the code |
| Doctests | A documented command that no longer runs |
| Semgrep and CodeQL | Security problems in code (CI only) |
| Dependency review | New dependencies with known vulnerabilities or barred licences (CI only) |
| OpenSSF Scorecard | Weak project security practices, weekly on `main` |

## Running the tests

`go test ./...` runs the unit tests.

```sh
go test ./...
```

On a small machine, add `GOFLAGS=-p=2 GOMAXPROCS=2` to keep the build from taking every core. On a shared machine, run the browser tests for one browser at a time.

The browser tests run only when `DUMMYRUN_BROWSERS` names the browsers. They start a local test server,
and each needs the browser set up as the Install section of [docs/ENGINE.md](docs/ENGINE.md) describes. Firefox is found
through `DUMMYRUN_FIREFOX`, and Chrome through `DUMMYRUN_CHROMEDRIVER` and `DUMMYRUN_CHROME`.

```sh
DUMMYRUN_BROWSERS=chrome,firefox go test ./internal/bidi/ ./internal/page/ ./internal/runner/ ./internal/a11y/ ./internal/mcpserver/
```

| Package | What its tests cover |
|---|---|
| `internal/bidi` | Conformance with WebDriver BiDi, including `browsingContext.setViewport` with `devicePixelRatio`, and the font sizes a page ends up with at text scales of 100 and 200 percent |
| `internal/page` | The persona's tools, with focus, the outline and zoom on both browsers |
| `internal/runner` | A full run with a scripted model, and the canary page of injection attempts in `internal/runner/testdata/canary.html` |
| `internal/a11y` | The accessibility sweep, on a fixture page with known violations, and the layout checks, on fixture pages with one overflow, one clipped control, one overlap and one small target |
| `internal/mcpserver` | The MCP server, driven through an in-process client |
| `internal/provider` | Every provider, plus one contract test each must pass. Replies are fixtures in `internal/provider/testdata/`, so no key is needed |
| `examples/inkwell` | The rules for each role in the test app, each defect switch, every page with every switch on, and bad manifest files |
| `internal/report` | Every report format. A test fails when the templates or any output say "pass", "passed", "passing" or "compliant", and when the example report in `examples/report/` is out of date. Rebuild it with `examples/report/generate.sh` |

Every test that launches a browser closes it with `t.Cleanup`. A test binary fails when it launched a
browser and never closed it, and the conformance suite fails when a browser's process or profile outlives
its test. After the browser tests, run `dummyrun doctor` to check that nothing was left behind. CI does the
same and fails when it finds anything.

CI runs all of these on ubuntu-latest with pinned browsers, with the race detector on. It also runs three
fuzz targets for 30 seconds each:

```sh
go test -run='^$' -fuzz=FuzzDecode -fuzztime=30s ./internal/bidi/
go test -run='^$' -fuzz=FuzzWrite -fuzztime=30s ./internal/record/
go test -run='^$' -fuzz=FuzzDecodeResponses -fuzztime=30s ./internal/provider/
```

The OpenAI-compatible chat fixtures were recorded from a local Ollama, and the other providers' fixtures
follow each vendor's documented format. To test against a local model, set `DUMMYRUN_OLLAMA_MODEL`. The
test is skipped when the variable is unset.

```sh
DUMMYRUN_OLLAMA_MODEL=gemma4:e4b go test -run TestOllamaLive -v ./internal/provider/
```

The project also runs Dummy Run on its own website. Every Monday at 06:00 UTC the `Dogfood` workflow plays
the four personas in `casts/dummyrun-dev` through their ten scenarios against https://dummyrun.dev. The
target is read-only, and each run stops at 20 steps or 300,000 tokens. The workflow exports the runs to the
project's Grafana Cloud stack under `service.namespace=dummyrun-dev` and keeps the run records for 90 days
as the `dogfood-runs` artifact. The workflow fails when an export fails, a scenario doesn't run or a
record isn't completed, and its summary says which.

[docs/TELEMETRY.md](docs/TELEMETRY.md) is the contract between the runner, the exporter and the
dashboards, and code comments cite its sections. Change that page first when you change a signal.

## Definition of done

A change a user would notice is done when the same pull request has all of these.

1. **Tests.** New behaviour has automated tests. A bug fix has a test that failed before the fix.
2. **Docs.** The page that describes the behaviour says the new thing and no longer says the old one. The
   pages are in `docs/`, the README, `integrations/*/README.md` and `casts/*/README.md`.
3. **Site content.** The pages in `site/content/` and the text in `site/layouts/` that mention the behaviour
   match it. Check the Get started page, the questions on the home page and the page for each role.
4. **Changelog.** `CHANGELOG.md` has an entry under Unreleased.
5. **Examples.** `examples/`, the library in `casts/library` and the run records in `examples/runs/` still
   work with the change. A new feature has an example where one helps.

Two generators and a doctest runner keep part of this honest. Run them before you push, though the
pre-push hook runs them too.

```sh
go generate ./cmd/dummyrun
go run ./internal/tools/docgen -check
go run ./internal/tools/doctest
```

`go generate ./cmd/dummyrun` rewrites `docs/CLI.md` from each command's help text. It also rewrites the
capability table in `docs/PROVIDERS.md` from `internal/provider/capabilities.go`. Change the code, then
generate. Never edit the generated text by hand. `docgen -check` fails when a file is out of date, or when
a provider or an engine tool has no entry in the page that describes it.

The doctest runner builds `dummyrun` and `dummyrun-otel` from your tree. It then runs every shell block in
`docs/`, `site/content/` and the other Markdown files that has the line `<!-- doctest -->` before it. Only
safe commands run. Those are `version` and help, `dummyrun-otel export --dry-run` and `dummyrun coverage`
without `--out`. `dummyrun a11y` runs too, on a site served on your machine. A block marked `<!-- doctest site -->` needs the
built site on port 1313 and runs with `go run ./internal/tools/doctest -site`. Put the marker on any block
a reader will copy, so a command that stops working fails a check before it fails a reader.

Review checks this too. The pull request template has a "Docs and site" item, and CodeRabbit warns when a
user-visible change to the code or the schema leaves out the docs, the changelog or the site content.

## Pull requests

- Branch from `main`. Pull requests are squash-merged, so the title becomes the commit message.
- Sign off every commit with `git commit -s`. This certifies the
  [Developer Certificate of Origin](https://developercertificate.org/), and CI checks it. Your
  contribution is licensed under [Apache-2.0](LICENSE). There's no other agreement to sign.
- New behaviour comes with automated tests in the same pull request, and a bug fix comes with a test that
  failed before the fix. Reviewers don't merge code changes without tests.
- Make one change per pull request. A persona and a harness change are two pull requests.
- Say what you changed and how you checked it.

## Reporting bugs and asking for features

Report bugs and request features in [GitHub issues](https://github.com/dummy-run/dummy-run/issues), and ask
questions in [Discussions](https://github.com/dummy-run/dummy-run/discussions). Report security problems
privately, as [SECURITY.md](SECURITY.md) describes. The maintainer replies to issues within 14 days.

## Proposing a persona

Use the "Persona proposal" issue template. Give the role and its jobs, its device and conditions, and what
it may see. Add three to six scenarios. Say where each trait comes from: a public source, an inference or
your own experience. In the profile, tag each trait `sourced` or `inferred` as
[docs/STYLE.md](docs/STYLE.md) describes. A trait from your own experience is `inferred`, and its source
line says it comes from your experience. Keep these tags, and keep the source lines when you copy a library persona. A trait
with no source is removed. We don't accept personas built from a real customer's data.

## Proposing a target adapter

Use the "Feature or adapter" issue template. Say how the platform exposes a preview for a pull request, how
it creates a test user with a role, and how it loads test data. We don't accept adapters that need
production credentials.

## Code of conduct

See the [Code of Conduct](CODE_OF_CONDUCT.md).
