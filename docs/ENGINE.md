# The engine

`dummyrun` is a Go program that plays one persona through one scenario in a real browser and writes a run
record. It drives Chrome and Firefox over WebDriver BiDi with its own client in `internal/bidi`, so the same
code runs both browsers. This page covers installing it, running it, the read-only policy and the model
providers.

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
the budget runs out. The thirteen tools are listed in the table below. After every action the model sees the page as text with numbered
controls and, for the two newest steps, a screenshot. Older steps shrink to one line to keep the context
small. The run ends early when the persona gives up: three failed tries at the same thing on the same page.

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

Every request to the target carries a `baggage` header with the run and persona IDs and a `traceparent`
header derived from the run ID, as `docs/TELEMETRY.md` describes. The browser's user agent ends with
`DummyRun/<version>`.

## Read-only policy

A target with `"read_only": true` cannot be changed by a run. The engine intercepts every request the
browser makes. A request to an allowed host with a method other than GET, HEAD or OPTIONS fails as a
network error before it leaves the browser, and the step that caused it gets a note such as
`blocked: POST https://example.test/subscribe (read-only target)`. A request to any host outside
`allowed_hosts` fails the same way. The persona is told about the policy in its prompt as well, but the
interception is what enforces it. `internal/page/tools_test.go` proves it with a form whose POST never
reaches the test server, on both browsers.

## Providers

`internal/provider` defines one interface. A request carries a system prompt and the turns so far. Each turn
holds text and may hold images and tool calls. The request also lists the tools and may name one forced tool. The Azure OpenAI implementation uses `openai-go`'s `azure` package with
`azidentity.DefaultAzureCredential`, so your `az login` session is the credential and no API key exists in
the engine or on the resource. The API version defaults to `2025-04-01-preview` and `--api-version` overrides
it. Token usage from every call is summed into the record's `usage` block, and
`--region` is recorded as `provider.region` because a deployment does not report where it runs.

Anthropic and Gemini are not implemented yet. The interface keeps room for them: `ForceTool` is a request
that a provider may downgrade to automatic tool choice when it cannot honour it, and images travel as PNG
bytes so each provider can encode them its own way.

## Tests

`go test ./...` runs the unit tests. Set `DUMMYRUN_BROWSERS=chrome,firefox` to also run the conformance
suite (`internal/bidi`), the tool suite (`internal/page`) and the end-to-end run with a scripted model
(`internal/runner`) against a local test server. CI runs all of them on ubuntu-latest with pinned browsers.
