# Changelog

All notable changes are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Inkwell is a made-up publishing app in `examples/inkwell`. It has five roles and seeded test accounts. A `-defects` manifest plants up to thirty-six faults, and a hidden prompt-injection test is available. `-test-login` signs a run in with a cookie route. `casts/inkwell` adds an author and a reviewer persona. `docs/INKWELL.md` covers it.
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
