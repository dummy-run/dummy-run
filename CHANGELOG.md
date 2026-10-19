# Changelog

All notable changes are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed
- Browsers are no longer left running after runs and tests (#49). The engine starts the driver and the browser in their own process group. It closes the session and stops the group. It then waits for the exit and removes the temporary profile. This holds after an error or a panic. It also holds after a timeout, Ctrl-C and SIGTERM. A watchdog stops the browser if the engine is killed. Every test closes its browser, and CI fails when one is left behind. `docs/ENGINE.md` covers cleanup and troubleshooting.

### Added
- The README, the How it works page and the Telemetry page answer four questions that the dogfood personas raised. Dummy Run does not replace a scripted regression suite. Screenshots and page text go to the model provider you choose. Telemetry carries no screenshots or page text and goes only to your OpenTelemetry backend, so its residency is wherever that backend is. Findings become tickets by hand or from the run record JSON. The architecture and How it works pages list the model providers and link the provider table.
- `gpt-5-4-mini-global` is in the price table, from the Azure retail prices API, so runs on that deployment get a `cost_usd`.
- A Watch a run page on the website. It replays one real run of the dummyrun.dev cast. Each step shows the screenshot, the action, the outline the persona read and the findings that cite it. The data is in `site/data/replay.json`, from run `2026-10-19-priya-qa-lead-does-it-replace-my-suite-31099a2c8c853e8f`.
- `docs/ETHICS.md`, on the website as Ethics and limits. It says what a persona can and cannot tell you, how the screen-reader persona works and what goes to the model provider.
- The persona library page on the website shows a card for each persona with filters by need, what the persona does differently, what it looks for and the command to run it. It is built from the files in `casts/library`. Each persona gains five front matter keys that the page reads.
- `dummyrun doctor` lists the browsers and profile directories that an earlier run left behind. `--stop` stops them and removes the directories.
- `dummyrun coverage` counts what a set of runs covers. It reports the problems found and a Chao2 lower bound on the problems still unfound, always as "at least" with its form named. It also reports sample coverage and the expected new problems per extra run. Route and persona-dimension coverage, a stratified stopping rule and scoring bands complete the report. It refuses to pool runs from different builds. `docs/COVERAGE.md` says what each number means and when to trust it.
- Run record schema version 2. Each finding has a `route` and a `locus`. It also has a `symptom` from a closed list and a `fingerprint` that the engine computes. Steps have a `route`. The persona has `dimensions` and the scenario has `goal_met`. The record has `repeat_index`, `seed`, a required `batch` and `target.route_universe`. `usage.cost_usd` comes from the price table in `internal/price`, with a source and date for each model, and is null for a model that is not in it. Version 1 records stay readable by `dummyrun coverage` and `dummyrun-otel`. The version 1 schema is kept as `schema/run-record.v1.schema.json`. The exporter gains the attributes listed in `docs/TELEMETRY.md`, such as `dummyrun.finding.fingerprint`.
- `dummyrun run` takes `--seed`, `--repeat-index` and `--universe`. The `finish` tool asks for `goal_met` and for a `symptom` on each finding. A finding may name the control it is about. A URL target without a commit is identified by a hash of its home page.
- A weekly workflow plays the `casts/dummyrun-dev` cast against dummyrun.dev, sends the run records to Grafana Cloud and keeps them as an artifact (code: `.github/workflows/dogfood.yml`). It fails when a scenario does not run, when a record is not completed or when the export fails.
- `dummyrun run` plays one persona through one scenario in Chrome or Firefox over WebDriver BiDi, with thirteen tools and a step and token budget. A read-only mode blocks every request that could change data. The run record matches `schema/run-record.schema.json`. `docs/ENGINE.md` covers it.
- Each observation names the element that has focus and lists an accessibility outline. A persona can set `zoom` from 100 to 500 percent and `tools-allowed` or `tools-denied`, and a scenario can set its own `Device:` and `Zoom:`. The run record gains `persona.zoom`.
- `dummyrun a11y` takes `--rebase`, `--viewport`, `--scheme` and `--headful`. `dummyrun audit a11y` is the same command.
- `casts/library` holds ten generic personas, from a screen-reader user to the viewer, editor and admin trio. `docs/PERSONAS.md` says how to copy one. `keyboard-only-user` cannot click, `screen-reader-user` is not sent screenshots and `low-vision-zoom` runs at 200 percent.
- `dummyrun-otel` sends run records to Grafana as OpenTelemetry traces, metrics and logs, with annotations. Four dashboards, a local stack and four alert rules go with it. `docs/integrations/grafana.md` is the guide.
- `docs/CLI.md` is written from the commands' help text, and the capability table in `docs/PROVIDERS.md` is written from the code. CI fails when either is out of date, and shell blocks marked `<!-- doctest -->` run in CI against a local build.
- The website at dummyrun.dev, built with Hugo from this repository and checked for accessibility on every change.
- Model providers beyond Azure: any OpenAI-compatible service or local server, Anthropic, Gemini (Vertex AI and the Developer API) and Amazon Bedrock. `docs/PROVIDERS.md` covers each one.
- `--reasoning-effort` and `--openai-api responses` for OpenAI-style models, and `cached_input_tokens` in the run record's usage.
- `dummyrun a11y` sweeps pages with axe-core for WCAG 2.2 A and AA on Chrome and Firefox, with no model calls. The site's accessibility check now uses it in place of pa11y-ci, which removes the extract-zip alerts.
- The project is licensed under Apache-2.0, with a trademark policy for the name and logo.
- Contributors sign off commits under the Developer Certificate of Origin, and CI checks every commit.
- CI also blocks credentials and known customer identifiers, broken links and em dashes.
- `bin/check-prose` enforces the plain-wording rules in `docs/STYLE.md`.
- The docs cover the plan and the architecture. Others explain the licence, the name and the type scale.
- `brand/` holds the Viewfinder logo, the Dummy mascot and the colour palette.
