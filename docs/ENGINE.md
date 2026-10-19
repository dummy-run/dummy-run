# The engine

`dummyrun` is a Go program that plays one persona through one scenario in a real browser and writes a run
record. It drives Chrome and Firefox over WebDriver BiDi with its own client in `internal/bidi`, so the same
code runs both browsers.

## Install

[INSTALL.md](INSTALL.md) covers release binaries and the container image, which has both browsers in it.
To build from source you need Go 1.27 and a browser. Chrome also needs a matching chromedriver.

```sh
go install github.com/dummy-run/dummy-run/cmd/dummyrun@latest
```

| Browser | What the engine starts | How it finds it |
|---|---|---|
| Firefox | Firefox itself with `--remote-debugging-port`. No driver | `DUMMYRUN_FIREFOX`, then the Firefox app on macOS, then `firefox` on the PATH |
| Chrome | chromedriver, which starts Chrome for Testing or Chrome | `DUMMYRUN_CHROMEDRIVER` and `DUMMYRUN_CHROME`, then the PATH |

Set `DUMMYRUN_NO_SANDBOX=1` to start the browsers without their own sandboxes. Do this only inside a container
or a VM, as the container image does. CI has the same effect through `CI` (code: `internal/bidi/launch.go`).

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
| `--provider`, `--model`, `--region` | The model provider, its deployment or model name, and the region to record. See [PROVIDERS.md](PROVIDERS.md) |
| `--endpoint`, `--api-key-env`, `--project` | The provider's base URL, the name of the environment variable holding its API key, and the Google Cloud project for `vertex` |
| `--reasoning-effort`, `--openai-api` | Reasoning effort, and `responses` to use the Responses API, for OpenAI-style models |
| `--api-version V` | The Azure OpenAI API version. The default is `2025-04-01-preview` |
| `--batch ID` | A batch ID, kept in the run record as `batch`, to group runs started together. The default is the date and the target's environment |
| `--seed N` | A sampling seed, 0 or more. It is sent to providers that take one (see [PROVIDERS.md](PROVIDERS.md)) and recorded as `seed`. Where it is not sent the record has `seed: null` |
| `--repeat-index N` | The run's `repeat_index`. The default counts the earlier records in `--out` with the same batch, persona, scenario and model |
| `--universe sitemap` | Read the routes that exist from the target's `/sitemap.xml`, or from a sitemap URL, and write them to `target.route_universe` for route coverage |
| `--steps N` | Step budget, up to 40. A scenario can also ask with a `Budget: 30 steps` line. The default is 25 |
| `--tokens N` | Token budget for input plus output. The default is 1,500,000 |
| `--headful` | Show the browser window |

[CLI.md](CLI.md) holds the help text of every command, and the commands print it too.

<!-- doctest -->
```sh
dummyrun --help
dummyrun run -h
```

A run visits the target's base URL, then lets the model call one tool per turn until it calls `finish` or
the budget runs out (code: `internal/runner/runner.go`). After every action the model sees the page as text
with numbered controls. It also sees the focused element and an accessibility outline. The two newest steps
come with a screenshot. Older steps shrink to one line to keep
the context small. The run ends early when the persona gives up: three failed tries at the same thing on the
same page (code: `internal/runner/patience.go`).

| Tool | What it does |
|---|---|
| `look`, `screenshot` | Read the page again: title, headings, focus, outline, numbered controls and text, with a screenshot |
| `visit` | Go to an address on the target. Addresses on other hosts are refused before the browser is involved |
| `click` | Click `@12` from the last look, the control's text or `css:selector` |
| `fill`, `type` | Put text into a field in one go, or key by key for fields that react as you type |
| `select` | Choose an option in a select by its text |
| `press` | Press a key such as Enter, Escape or ArrowDown |
| `back`, `scroll`, `wait` | Go back one page, scroll by most of a screen, wait up to 15 seconds |
| `a11y` | Run axe-core 4.13.0 with the WCAG 2.2 A and AA rules. Only rule IDs and counts are recorded |
| `finish` | End the run with a verdict, `goal_met`, a report and findings that cite step numbers |

Each finding names a `symptom` from a closed list, and may name the control it is about by its `@12` number. The
engine computes the finding's route, locus and fingerprint from the page. [COVERAGE.md](COVERAGE.md) explains
the list and what the fingerprint is for. A symptom outside the list sends the model back to `finish` twice at
most, and then the finding is dropped.

## Browser cleanup

The engine starts the driver and the browser in a process group of its own. Their profile is a temporary
directory named `dummyrun-chrome-*` or `dummyrun-firefox-*` (code: `internal/bidi/launch.go` and
`internal/bidi/cleanup.go`). When a run ends the engine closes the BiDi session and signals the group. It waits
for the group to exit and kills what is left. It also stops any helper that still names the profile. Then it
removes the profile. This happens when a run succeeds or fails. It also happens on a panic, a timeout, Ctrl-C
and SIGTERM. A second process watches the engine and stops the browser group if the engine disappears without
cleaning up, for example after SIGKILL.

Use Chrome for Testing where you can. On macOS each running Google Chrome keeps a code-signing clone of the
app of about 1.4 GB, and the engine looks for Chrome for Testing first. Set `DUMMYRUN_CHROME` and
`DUMMYRUN_CHROMEDRIVER` to a matching pair. [Chrome for Testing](https://googlechromelabs.github.io/chrome-for-testing/)
lists the downloads.

## Persona and scenario settings

A persona's `profile.md` starts with a front matter block of `key: value` lines. These keys set how the
browser behaves for that persona.

| Key | Meaning |
|---|---|
| `device` | `desktop` (1400 x 1000), `tablet` (820 x 1180) or `phone` (390 x 844). The window size is in device pixels. This key is required. |
| `zoom` | Page zoom in percent, from 100 to 500. `200` and `200%` both work. The default is 100 |
| `tools-allowed` | The only tools the persona may use, separated by spaces or commas. `finish` is always allowed |
| `tools-denied` | Tools the persona may not use. When both keys are set, a tool must be allowed and not denied |

A scenario file can put `Device: phone` and `Zoom: 200%` on lines of their own. A scenario's setting replaces
the persona's, so one persona can be run on several devices. The scenario files already start with a line such
as `Device: desktop. Start at the home page.`, and the engine reads that line. A `Budget: 30 steps` line works
the same way. A `Device:` line may carry other words after the device name. A `Zoom:` value has its line to
itself. A run does not start with a device other than `desktop`, `tablet` or `phone`. It does not start with a
zoom that is not a number from 100 to 500.

### Tool limits

The engine offers the persona every tool. When the persona calls one that its limits rule out, the engine does
nothing in the browser. The step still counts against the budget. Its result says `REFUSED`, the run record
gives the step a note beginning `blocked:`, and `steps.jsonl` has a `refusal` field. Three refusals of
the same tool on the same page make the persona give up, as with any other failed try. The system prompt lists
the tools the persona may not use, so a refusal is rarely a surprise.
If a persona may not use `screenshot`, the model is not sent screenshots either. They are still saved for the
reviewer. The engine's own opening visit is never refused.

### Zoom

A zoom above 100 percent behaves like browser zoom. The window keeps its device size, and the page lays out in
fewer CSS pixels. At 200 percent a tablet window of 820 x 1180 gives the page 410 x 590 CSS pixels and a
`devicePixelRatio` of 2. Media queries see the narrower viewport, text is twice its size, and screenshots stay
820 x 1180 pixels. The engine sets this with `browsingContext.setViewport`, giving it the smaller viewport and
the `devicePixelRatio` parameter ([WebDriver BiDi, setViewport](https://w3c.github.io/webdriver-bidi/#command-browsingContext-setViewport)).
The setting is kept across navigations. CSS `zoom` set from a script would not survive a page load, would not
change media queries and is not standard on Firefox. Observations show a `zoom:` line with the viewport and
pixel ratio the page reports, so a screenshot and the page's own numbers can be compared. The run record
holds the zoom in `persona.zoom`. It is left out at 100. The persona object does not allow other keys, so a
reader that still has the earlier schema rejects a record that has `persona.zoom`. Update the reader's copy of
`schema/run-record.schema.json` first. The Grafana exporter in this repository is already updated.

This is the setting for WCAG 2.2 SC 1.4.4 (resize text) and SC 1.4.10 (reflow).

### Device in the record

`persona.device` in the run record is the device the run used. When a scenario sets its own, that is the
scenario's device and not the one in `profile.md`.

## Focus and the accessibility outline

Every observation carries two extra parts, built in the page after each action
(code: `internal/page/tree.js`).

```
focus: link "Pricing" @7
accessibility tree (roles and names computed from the page; @n matches the controls):
  banner
    link "Home" @1
  navigation "Main"
    link "Docs" @2
  main
    heading 1 "Pricing"
    link "Pricing" @7  <- focus
    textbox (no name) @8 [required]
```

The focus line gives the role, accessible name and `@ref` of the element that has keyboard focus. It says
`focus: nothing` when no element has it. The outline lists landmarks, headings and interactive elements in page
order. Each line has a role and a name, and may have states. A state is `disabled`, `checked` or `required`. A field can be `invalid`.
A link to the current page is `current`. A disclosure control shows `expanded` or `collapsed`. An element with no accessible name shows `(no name)`. Moving focus with Tab counts as a change on
the page, so a persona that tabs through a page is not told that nothing happened. Both parts are also in
`steps.jsonl` under `page.focus` and `page.tree`.

WebDriver BiDi has no command that returns an accessibility tree. Its only accessibility feature is the
`accessibility` locator for `browsingContext.locateNodes`, which finds nodes by role and name
([WebDriver BiDi, locators](https://w3c.github.io/webdriver-bidi/#type-browsingContext-Locator)). The engine
therefore runs a script with `script.callFunction` that reads the DOM and ARIA attributes. It follows HTML-AAM
for implicit roles and Accessible Name and Description Computation 1.2 for names, in the common cases. The
roles and names are an approximation of what each browser exposes, the same on Chrome and Firefox. Shadow DOM
and cross-origin frames are only partly covered.

The outline has a budget of 600 tokens (`page.TreeTokens`). The engine counts 3 characters to a token, so
the limit is 1,800 characters. When a page has more, the landmarks and the focused element stay. Headings come next, then
interactive elements in page order. A last line says how many entries were left out.

The run record matches `schema/run-record.schema.json` (schema version 2) and is validated before it is written.
Version 1 records, written by earlier releases, stay readable by `dummyrun coverage` and the Grafana exporter.
A version 2 record adds the fingerprint of each finding and the route of each step. It also adds the persona's
dimensions, `scenario.goal_met`, `repeat_index` and `seed`. Its cost comes from the price table in
`internal/price/prices.json`. The cost is null for a model that is not in the table. A target without a `commit` gets one from a hash of its
home page, which the engine fetches with one GET request outside the browser. Findings
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
before the Azure CLI sign-in (code: `internal/provider/azure.go`). The Azure provider has no code path that
reads an API key. The other providers read a key from the environment when the run starts, as
[PROVIDERS.md](PROVIDERS.md) says. Each call has a 120 second timeout and up to three retries. The API version defaults to `2025-04-01-preview` and `--api-version` overrides
it. Token usage from every call is summed into the record's `usage` block, and
`--region` is recorded as `provider.region` because a deployment does not report where it runs.

Other backends sit behind the same interface. They are any OpenAI-compatible service, Anthropic, Gemini and
Amazon Bedrock. A capability table in `internal/provider/capabilities.go` tells the runner
what each can do, so it picks fallbacks without checking names. `docs/PROVIDERS.md` has the flags, the notes for
each vendor and the data region of each.

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
The conformance suite includes `browsingContext.setViewport` with `devicePixelRatio`, and the tool suite
covers focus, the outline and zoom on both browsers.

Every test that launches a browser closes it with `t.Cleanup`. Each test binary fails when a browser was
launched and never closed. The conformance suite also fails when a browser's process or profile outlives its
test. CI runs `dummyrun doctor` after the browser tests and fails when it finds anything.

## Troubleshooting

Leftover browsers fill the disk and the process table. To list the browsers and profile directories that
earlier runs left behind, run this:

```sh
dummyrun doctor
```

It exits 1 when it finds any. Add `--stop` to stop them and remove the profile directories. Check first that
no run is in progress, since a browser that belongs to a live run is listed too. Without the command, this
lists the processes:

```sh
ps -axo pid=,args= | grep -E 'dummyrun-(chrome|firefox)-' | grep -v grep
```

The engine only removes directories that it made itself in the temporary directory with those prefixes.
