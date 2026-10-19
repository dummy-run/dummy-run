# The engine

`dummyrun` is a Go program that plays one persona through one scenario in a real browser and writes a run
record. It drives Chrome and Firefox over WebDriver BiDi with its own client in `internal/bidi`, so the same
code runs both browsers.

## Install

You need Go 1.27 and a browser. Chrome also needs a matching chromedriver.

```sh
go install github.com/dummy-run/dummy-run/cmd/dummyrun@latest
```

| Browser | What the engine starts | How it finds it |
|---|---|---|
| Firefox | Firefox itself with `--remote-debugging-port`. No driver | `DUMMYRUN_FIREFOX`, then the Firefox app on macOS, then `firefox` on the PATH |
| Chrome | chromedriver, which starts Chrome for Testing or Chrome | `DUMMYRUN_CHROMEDRIVER` and `DUMMYRUN_CHROME`, then the PATH |

Chrome for Testing and its chromedriver come from the same version listing at
https://googlechromelabs.github.io/chrome-for-testing/. CI pins 155.0.8059.12 and Firefox 157.0 in
`.github/workflows/engine.yml`.

## Run

```sh
az login
export DUMMYRUN_AZURE_ENDPOINT=https://<resource>.openai.azure.com
dummyrun run --cast casts/dummyrun-dev --persona tom-developer --scenario find-the-quickstart \
  --target examples/targets/dummyrun-dev.json --out ~/.local/share/dummyrun/runs/dummyrun-dev \
  --browser firefox --provider azure --model <deployment> --region uksouth
```

| Flag | Meaning |
|---|---|
| `--cast DIR` | The cast directory. A persona is `DIR/<id>/profile.md`, `prompt.md` and `scenarios/NN-<id>.md` |
| `--persona ID`, `--scenario ID` | Which persona and which of its scenarios |
| `--target CONFIG` | A JSON file: `base_url`, `allowed_hosts`, `read_only`, `environment`. See `examples/targets/` |
| `--out DIR` | Where run records go. The record is `DIR/<run id>.json`; screenshots and the persona's report are under `DIR/<run id>/` |
| `--browser` | `chrome` or `firefox` |
| `--provider`, `--model`, `--region` | The model provider, its deployment or model name, and the region to record |
| `--steps N` | Step budget, up to 40. A scenario can also ask with a `Budget: 30 steps` line. The default is 25 |
| `--tokens N` | Token budget for input plus output. The default is 1,500,000 |
| `--headful` | Show the browser window |

A run visits the target's base URL, then lets the model call one tool per turn until it calls `finish` or
the budget runs out (code: `internal/runner/runner.go`). After every action the model sees the page as text
with numbered controls and, for the two newest steps, a screenshot. Older steps shrink to one line to keep
the context small. The run ends early when the persona gives up: three failed tries at the same thing on the
same page (code: `internal/runner/patience.go`).

| Tool | What it does |
|---|---|
| `look`, `screenshot` | Read the page again: title, headings, numbered controls and text, with a screenshot |
| `visit` | Go to an address on the target. Addresses on other hosts are refused before the browser is involved |
| `click` | Click `@12` from the last look, the control's text or `css:selector` |
| `fill`, `type` | Put text into a field in one go, or key by key for fields that react as you type |
| `select` | Choose an option in a select by its text |
| `press` | Press a key such as Enter, Escape or ArrowDown |
| `back`, `scroll`, `wait` | Go back one page, scroll by most of a screen, wait up to 15 seconds |
| `a11y` | Run axe-core 4.13.0 with the WCAG 2.2 A and AA rules. Only rule IDs and counts are recorded |
| `finish` | End the run with a verdict, a report and findings that cite step numbers |

The run record matches `schema/run-record.schema.json` and is validated before it is written. Findings
start with the verdict `pending`; the reviewer is a later stage. Next to the record, `report.md` holds the
persona's own report and `steps.jsonl` holds the full observation for each step, including page text the
reviewer can check quotes against.

Every request to the target carries a `baggage` header with the run and persona IDs. A request also gets a
`traceparent` header unless the page set one itself, in which case the page's header is kept. The trace ID
and the step span ID are derived from the run ID and the step number, as `docs/TELEMETRY.md` describes
(code: `internal/telemetry/telemetry.go`). The browser's user agent ends with `DummyRun/<version>`.

## Read-only policy

A target with `"read_only": true` cannot be changed by a run. The engine intercepts every request the
browser makes (code: `internal/policy/policy.go` and `internal/page/page.go`). A request to an allowed host with a method other than GET, HEAD or OPTIONS fails as a
network error before it leaves the browser, and the step that caused it gets a note such as
`blocked: POST https://example.test/subscribe (read-only target)`. A request to any host outside
`allowed_hosts` fails the same way. The persona is told about the policy in its prompt as well, but the
interception is what enforces it. `internal/page/tools_test.go` proves it with a form whose POST never
reaches the test server, on both browsers (measured on Firefox 158.0 and Chrome 155.0.8059.80).

## Providers

`internal/provider` defines one interface. A request carries a system prompt and the turns so far. Each turn
holds text and may hold images and tool calls. The request also lists the tools and may name one forced tool. The Azure OpenAI implementation uses `openai-go`'s `azure` package with
`azidentity.DefaultAzureCredential`, which tries the environment, workload identity and managed identity
before the Azure CLI sign-in (code: `internal/provider/azure.go`). The engine has no code path that reads an
API key. Each call has a 120 second timeout and up to three retries. The API version defaults to `2025-04-01-preview` and `--api-version` overrides
it. Token usage from every call is summed into the record's `usage` block, and
`--region` is recorded as `provider.region` because a deployment does not report where it runs.

Anthropic and Gemini are not implemented yet. The interface keeps room for them: `ForceTool` is a request
that a provider may downgrade to automatic tool choice when it cannot honour it, and images travel as PNG
bytes so each provider can encode them its own way.

## The accessibility sweep

`dummyrun a11y` checks pages for accessibility problems without calling a model. It runs the
axe-core 4.13.0 copy that the `a11y` tool uses, over the same WebDriver BiDi connection. It checks every page
you give it (code: `internal/a11y`, `cmd/dummyrun/a11y.go`). `dummyrun audit a11y` is the same command.

```sh
dummyrun a11y --browser chrome --sitemap https://example.com/sitemap.xml \
  --allow a11y-allow.txt --json report.json https://example.com/404.html
```

- **Pages.** Give URLs, a sitemap or both. A sitemap index is followed one level. `--rebase URL` moves
  every sitemap address onto another origin, so a local copy of a site can be tested from its live sitemap.
- **Rules.** The axe-core rules tagged WCAG 2.0, 2.1 and 2.2 at levels A and AA. Best-practice rules are not run.
- **Sizes.** `--viewport` is `desktop` (1280 wide), `phone` (375 wide) or `both`.
- **Colour scheme.** `--scheme` is `light`, `dark` or `both`. WebDriver BiDi has no command for
  `prefers-color-scheme`, so the sweep sets it when it starts the browser (a Blink setting in Chrome, a
  preference in Firefox) and starts one browser for each scheme. After each page loads it reads the scheme
  the page actually got. A mismatch is reported as a page error, so a run can never quietly test the wrong theme.
- **Needs review.** Where axe-core cannot decide, for example text that is partly hidden by a scrolling
  container, the result is listed as "needs review" in the JSON and does not fail the run.
- **Output.** A short summary on standard output, listing each failure once with the browser, size and
  scheme where it appeared. `--json FILE` (or `-` for standard output) writes every page. Each failure has the rule ID and the impact. It also has the WCAG
  criteria and the element selector. The help link is there too.
- **Exit status.** 0 when nothing fails. 1 when any violation is not in the allow-list or a page cannot be
  loaded. 2 for a usage error.

The allow-list holds violations you have accepted. Each line is `rule | page | selector | reason`. The page
is a URL path and the selector is matched against the element's selector. A `*` matches anything. The reason
is required, so every exception is explained. A line that matches nothing in a run is reported, so stale
exceptions can be removed.

The site's own check is this command, run on Chrome and Firefox against the built site by
`.github/workflows/site.yml`. Its allow-list is `site/a11y-allow.txt`.

## Tests

`go test ./...` runs the unit tests. Set `DUMMYRUN_BROWSERS=chrome,firefox` to also run the browser tests
against a local test server. They cover the conformance suite (`internal/bidi`) and the tool suite
(`internal/page`). They also cover the end-to-end run with a scripted model (`internal/runner`) and the
accessibility sweep (`internal/a11y`, on a fixture page with known violations). CI runs all of them on ubuntu-latest with pinned browsers.
