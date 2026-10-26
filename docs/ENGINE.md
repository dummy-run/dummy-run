# The engine

`dummyrun` plays one persona through one scenario in a real browser and writes down what happened as a run
record. It drives Chrome or Firefox over WebDriver BiDi, so the same run works in either browser.

## Install

Download a release binary or use the container image, which has both browsers in it. [INSTALL.md](INSTALL.md)
covers both and walks through a first run in five minutes. To build from source, you'll need Go 1.27.1 or later.

```sh
go install github.com/dummy-run/dummy-run/cmd/dummyrun@latest
dummyrun browsers install
```

`dummyrun browsers install` downloads the pinned Chrome for Testing and its matching chromedriver into your user
cache, so you don't match them by hand. The engine finds them there when you set no `DUMMYRUN_CHROME` or
`DUMMYRUN_CHROMEDRIVER`.

| Browser | What the engine starts | Where it looks |
|---|---|---|
| Firefox | Firefox itself with `--remote-debugging-port`. No driver | `DUMMYRUN_FIREFOX`, then the Firefox app on macOS, then `firefox` on the PATH |
| Chrome | chromedriver, which starts Chrome for Testing or Chrome | `DUMMYRUN_CHROMEDRIVER` and `DUMMYRUN_CHROME`, then the pair from `dummyrun browsers install`, then the PATH |

To use your own Chrome, take Chrome for Testing and its chromedriver from the same
[version listing](https://googlechromelabs.github.io/chrome-for-testing/) so they match. The project's own CI
uses the versions that `dummyrun browsers list` shows, which are the ones `dummyrun browsers install` fetches.
For Firefox the engine reads `DUMMYRUN_FIREFOX` first. After that it uses a Firefox that
`dummyrun browsers install firefox` put in the cache, the Firefox app on macOS or `firefox` on the PATH.

Inside a container or a VM, set `DUMMYRUN_NO_SANDBOX=1` to start the browsers without their own sandboxes.
The container image does this already, and so does any run where `CI` is set. Don't set it on your own
machine.

## Run

To run a persona against your app, sign in to your model provider and point the engine at a cast and a
target. This example uses Azure OpenAI and Firefox.

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
| `--persona ID`, `--scenario ID` | Which persona to run, and which of its scenarios |
| `--scenario-file FILE` | Play a scenario file in place of `--scenario`. The file is named `NN-<id>.md` and is not looked up in the cast. Use it when a tool writes the scenario for one run, for example to list the pages to start from |
| `--target CONFIG` | A JSON file with `base_url`, `allowed_hosts`, `read_only`, `environment` and an optional `signin` or `auth` block. It can also set `identify` and `respect_robots`. An `allowed_hosts` entry is a host name or `host:port`. See `examples/targets/`, [Read-only policy](#read-only-policy), [Sign in](#sign-in) and [Protected previews](#protected-previews) |
| `--out DIR` | Where run records go. The record is `DIR/<run id>.json`. Screenshots and the persona's report go under `DIR/<run id>/` |
| `--browser` | `chrome` or `firefox` |
| `--provider`, `--model`, `--region` | The model provider, its deployment or model name, and the region to record. See [PROVIDERS.md](PROVIDERS.md) |
| `--endpoint`, `--api-key-env`, `--project` | The provider's base URL, the name of the environment variable that holds its API key, and the Google Cloud project for `vertex` |
| `--reasoning-effort`, `--openai-api` | Reasoning effort for OpenAI-style models. Set `--openai-api responses` to use the Responses API |
| `--api-version V` | The Azure OpenAI API version. The default is `2025-04-01-preview` |
| `--batch ID` | Groups runs you start together. It's kept in the record as `batch`. The default is the date plus the target's environment |
| `--seed N` | A sampling seed, 0 or more. It's sent to providers that accept one (see [PROVIDERS.md](PROVIDERS.md)) and recorded as `seed`. Where it isn't sent, the record has `seed: null` |
| `--repeat-index N` | The run's `repeat_index`. By default the engine counts the earlier records in `--out` with the same batch, persona, scenario and model |
| `--identify` | Adds `DummyRun/<version> (+https://dummyrun.dev/scanner/)` to the browser's user agent. `"identify": true` in the target file does the same |
| `--respect-robots` | Fetches `/robots.txt` from the target's origin and blocks what it disallows. `"respect_robots": true` in the target file does the same |
| `--session FILE` | Signs in from a saved session file instead of the login form. See [Sign in](#sign-in) |
| `--basic-auth-env USER_ENV:PASS_ENV`, `--header-env 'Name:ENV'` | Get past a protected preview without editing the target file. Each names environment variables, never the secrets, and `--header-env` can repeat. See [Protected previews](#protected-previews) |
| `--universe sitemap` | Reads the routes that exist from the target's `/sitemap.xml`, or from a sitemap URL you give, and writes them to `target.route_universe` for route coverage |
| `--steps N` | The step budget, up to 40. The default is 25. A scenario can ask for more with a `Budget: 30 steps` line |
| `--tokens N` | The token budget for input plus output. The default is 1,500,000 |
| `--headful` | Shows the browser window |

Every command prints its own help, and [CLI.md](CLI.md) collects all of it.

<!-- doctest -->
```sh
dummyrun --help
dummyrun run -h
```

### What happens during a run

The engine opens the target's base URL. Then the model calls one tool per turn until it calls `finish` or
runs out of budget. After each action the model sees the page as text, with every control numbered, plus the
focused element and an accessibility outline. The two newest steps come with a screenshot, and older steps
shrink to a single line so the context stays small. If the persona tries the same thing on the same page three
times and fails each time, it gives up and the run ends.

| Tool | What it does |
|---|---|
| `look`, `screenshot` | Read the page again: title, headings, focus, outline, numbered controls and text, with a screenshot |
| `visit` | Go to an address on the target. An address on a host outside `allowed_hosts` is refused before the browser sees it |
| `click` | Click `@12` from the last look, a control's text or `css:selector` |
| `fill`, `type` | Put text into a field in one go, or key by key for fields that react as you type |
| `select` | Choose an option in a select by its text |
| `press` | Press a key such as Enter, Escape or ArrowDown |
| `back`, `scroll`, `wait` | Go back one page, scroll by most of a screen, or wait up to 15 seconds |
| `a11y` | Run axe-core 4.13.0 with the WCAG 2.2 A and AA rules. The result names up to three failing elements of each rule. Only rule IDs and counts are recorded |
| `finish` | End the run with a verdict, `goal_met`, a report, and findings and observations that cite step numbers |

### Findings and observations

`finish` takes two lists. `findings` is for problems: something wrong, missing or confusing. `observations`
is for what worked and for neutral notes, such as "The home page accessibility check found no violations."

An observation is a sentence plus the steps that show it. It has no kind, severity or symptom, and it gets no
fingerprint. The list is optional. If a model sends a finding marked `class: observation`, the engine moves it
to the observations. The run record keeps both lists. Observations never count towards coverage, the findings
metric or the problems a report lists. A report shows them in its own What worked section.

Each finding names a `symptom` from a fixed list, and it can point at a control by its `@12` number. The
engine works out the finding's route, locus and fingerprint from the page. [COVERAGE.md](COVERAGE.md) has the
list and explains what the fingerprint is for. If the model gives a symptom that isn't on the list, it's sent
back to `finish` up to twice, and after that the finding is dropped.

The persona is never asked for a severity or a confidence. The engine sets the severity from a rule over the
symptom's impact class and the journey, and records the finding's `inputs`: the kind of evidence, the engines
that report it, the check and whether it's on a journey. A finding that names an axe rule which the persona's
own `a11y` check reported rests on that engine result; any other finding is the persona's own report. A report
turns those inputs into a confidence band. [REPORT.md](REPORT.md) explains both rules.

## Browser cleanup

You shouldn't find browsers left running after a run. The engine starts the driver and the browser in their
own process group, with a temporary profile directory named `dummyrun-chrome-*` or `dummyrun-firefox-*`. When
the run ends, the engine closes the BiDi session and signals the group. It waits for the group to exit and
kills anything left. It also stops any helper process that still names the profile, then removes the profile.

That happens whether the run succeeds or fails. It also happens on a panic, a timeout, Ctrl-C or SIGTERM. If the engine
itself disappears without cleaning up, for example after SIGKILL, a second watcher process stops the browser
group. To find anything left over, see [Troubleshooting](#troubleshooting).

Use Chrome for Testing where you can. On macOS each running Google Chrome keeps a code-signing clone of the
app of about 1.4 GB, and the engine looks for Chrome for Testing first. Set `DUMMYRUN_CHROME` and
`DUMMYRUN_CHROMEDRIVER` to a matching pair from the
[Chrome for Testing downloads](https://googlechromelabs.github.io/chrome-for-testing/).

## Persona and scenario settings

A persona's `profile.md` starts with front matter, one `key: value` per line. These keys control the browser
for that persona.

| Key | Meaning |
|---|---|
| `device` | `desktop` (1400 x 1000), `tablet` (820 x 1180) or `phone` (390 x 844), in device pixels. Required |
| `zoom` | Page zoom in percent, from 100 to 500. `200` and `200%` both work. The default is 100 |
| `text_scale` | System text size in percent, from 100 to 300. `200` and `200%` both work. The default is 100. See Text scale below |
| `tools-allowed` | The only tools the persona may use, separated by spaces or commas. `finish` is always allowed |
| `tools-denied` | Tools the persona may not use. If you set both keys, a tool has to be allowed and not denied |

To run one persona on several devices, set the device in the scenario instead. A scenario's `Device: phone`,
`Zoom: 200%` or `Text scale: 200%` line replaces the persona's setting. Scenario files usually open with a
line such as `Device: desktop. Start at the home page.`, and the engine reads the device from it. A `Device:`
line can carry other words after the device name, but a `Zoom:` or `Text scale:` value needs a line to
itself. A `Budget: 30 steps` line works the same way.

### Minimum steps

A scenario can say `Min steps: 8` on a line of its own. Until the persona has taken 8 steps of its own (the
engine's opening visit is not one of them), a call to `finish` gets a note to carry on and the run continues. The system prompt
tells the persona about the minimum too. Four cases still end the run early. A `finish` with the verdict
`blocked` is accepted. The engine takes the persona's word that the site stopped it. A `finish` that the engine forced, because the step or token budget
ran out or the persona lost patience, is accepted. So is a `finish` on the call that reaches the token budget. And after three refusals the next `finish` is
accepted, so a persona that insists can't use up the budget. The engine keeps at least three steps of the
budget for the work after the minimum, so a minimum above the budget less three is lowered to fit.

A run won't start with a device other than `desktop`, `tablet` or `phone`. It also won't start with a zoom
outside 100 to 500, or a text scale outside 100 to 300.

### Tool limits

The persona is still offered every tool. If it calls one its limits rule out, the engine does nothing in the
browser and the step still counts against the budget. The tool result says `REFUSED`, the record gives the
step a note starting `blocked:`, and `steps.jsonl` has a `refusal` field. Three refusals of the same tool on
the same page make the persona give up, like any other failed try. The system prompt lists the tools the
persona can't use, so refusals are rare.

If a persona may not use `screenshot`, the model isn't sent screenshots at all. They're still saved for the
reviewer. The engine's own opening visit is never refused.

### Zoom

Use zoom to test WCAG 2.2 SC 1.4.4 (resize text) and SC 1.4.10 (reflow). A zoom above 100 percent behaves
like browser zoom: the window keeps its device size and the page lays out in fewer CSS pixels. At 200 percent,
a tablet window of 820 x 1180 gives the page 410 x 590 CSS pixels and a `devicePixelRatio` of 2. Media
queries see the narrower viewport, text is twice the size, and screenshots stay 820 x 1180.

The engine sets this with
[`browsingContext.setViewport`](https://w3c.github.io/webdriver-bidi/#command-browsingContext-setViewport),
using the smaller viewport and the `devicePixelRatio` parameter, and it holds across navigations. Each
observation includes a `zoom:` line with the viewport and pixel ratio the page reports, so you can compare a
screenshot with the page's own numbers.

The run record holds the zoom in `persona.zoom`, and leaves it out at 100. The persona object allows no other
keys, so a reader with an older copy of the schema rejects a record that has `persona.zoom`. Update that
reader's copy of `schema/run-record.schema.json` first. The Grafana exporter in this repository already
reads it.

### Text scale

Use text scale to see your app the way someone does who has turned up the text size on their phone or in
their browser. That's different from zoom. With zoom, the page lays out in fewer CSS pixels and everything
gets bigger, so media queries see a narrower window. With a larger text size, the window stays as it is and
only text that follows the default font size grows.

| | `zoom: 200` | `text_scale: 200` |
|---|---|---|
| Viewport in CSS pixels | Halved (a 390 px phone becomes 195 px) | Unchanged (390 px) |
| `devicePixelRatio` | Doubled | Unchanged |
| Media queries | See the narrower window | See the same window |
| Text set in `rem`, `em` or `%`, and unstyled text | Doubles | Doubles, when the root and parent font sizes follow the browser default. A page that sets `html { font-size: 16px }` stops it |
| Text set in `px` | Doubles | Stays the same |
| Boxes, images and gaps set in `px` | Double | Stay the same |
| What it finds | Reflow at a narrow width (WCAG 1.4.10) | Text that outgrows a fixed box, overlaps its neighbour or runs off the screen (WCAG 1.4.4) |

The engine sets the browser's default font size when it starts the browser, the way a user would, so a
scale of 200 turns the 16 px default into 32 px. On Chrome it writes `webkit.webprefs.default_font_size` and
`default_fixed_font_size` into the profile through chromedriver's `prefs` option. On Firefox it writes
`font.size.variable.<group>` and `font.size.monospace.<group>` into `user.js` for every language group,
because Firefox picks the size from the page's `lang` attribute. Without that, a page in Russian, Japanese
or Arabic would stay at normal size.

There's no minimum font size, because a minimum would also lift text that a page sets in `px`. So a page
that sizes its text in `px` looks only half enlarged, just as it does to the person. A real phone can
differ. Android and iOS scale text by their own rules, and some Android browsers scale `px` text too. The
`large-text-phone` persona says so.

Each observation shows a `text size:` line, and no `zoom:` line unless zoom is set too. The system prompt
tells the model the text size and that the page isn't zoomed. The run record holds the value in
`persona.text_scale`, an optional key in schema version 2 that's left out at 100. A reader with an older copy
of `schema/run-record.schema.json` rejects a record that has it, so update that copy first. The Grafana
exporter already reads it and sends `dummyrun.persona.text_scale`.

To sweep at a larger text size, run `dummyrun a11y --text-scale 200`. A list such as `--text-scale 100,200`
runs both, with a fresh browser for each size, because the browser reads the setting from its profile when it
starts.

### Layout facts

When the engine measures something wrong with the page's layout, the observation gets a `layout:` line. The
engine measures in the browser, with no model call, and names controls by their `@number` so the persona can
look at them:

```text
layout: the page scrolls sideways (120 px wider than the window); 2 pairs of controls overlap (@4 @7, @5 @6); text is cut off in 1 control (@9)
```

The line can appear in any run, not only at a larger text size. It reports three things: the page is wider
than the window, interactive elements overlap, or an interactive element's text is cut off. Small targets are
left out, because they're common on a phone and would be noise on every step. The line never repeats a
selector or a label, since both come from the page. `dummyrun a11y` runs the same measurements as its layout
checks.

### Device in the record

`persona.device` in the run record is the device the run actually used. If the scenario set one, that's the
scenario's device, whatever `profile.md` says.

## Focus and the page outline

After each action the engine adds two things to what the model sees: the element with keyboard focus, and one
outline of the page. The outline is the only list of controls the persona reads. It holds the landmarks,
the headings from H1 to H6 with their levels, and the controls with role, name, states and the `@number` to act on.

```
focus: link "Pricing" @7
page outline (landmarks, headings and controls, named as a screen reader names them; act on a control by its @n; what a screen reader cannot reach is left out):
  banner
    link "Home" @1
  navigation "Main"
    link "Docs" @2
  main
    heading 1 "Pricing"
    link "Pricing" @7  <- focus
    heading 3 "Questions"
    textbox (no name) @8 [required]
```

The focus line gives the role, accessible name and `@ref` of the focused element, or `focus: nothing`. Each
outline line has a role, a name and sometimes a state:

- `disabled`, `checked` or `required`, and `invalid` for a field
- `current` for a link to the current page
- `expanded` or `collapsed` for a disclosure control
- `off screen` for a control above or below the window

In Chrome, roles, names, states and whether a screen reader can reach an element come from Chrome's own
accessibility tree. WebDriver BiDi has no command for it, so the engine asks for it over the Chrome DevTools
Protocol through chromedriver's `goog:cdp` passthrough. It reads `Accessibility.getFullAXTree` before each look
and each accessibility check. It marks each element the tree gives a role with `data-dummyrun-ax` and hands the
table of what Chrome says to its sandbox. Chrome leaves some elements out: one in a frame, one added after the
tree was read and any past the first 3,000. The script below names those. Chrome's names include CSS
generated content, such as an icon button's `::before` text, which the script can't read. A look in Chrome
whose tree was read records `names_from: chrome`. On GOV.UK's home page the extra commands add about a tenth of a second to a look.

Firefox refuses the passthrough. In Firefox, and in Chrome when the tree can't be read, names and visibility
come from one script, `internal/page/names.js`. The engine splices it into the control numbering, the outline and
the accessibility check, so an element is never named two ways. When axe-core is
loaded in the page, the script uses axe's own functions: `axe.commons.text.accessibleText` for the name and
`axe.commons.dom.isVisibleToScreenReaders` for whether a screen reader can reach it. The engine loads its own
axe into each web page before it reads it. It runs axe and its page scripts in a WebDriver BiDi sandbox, a realm
that shares the page's DOM but none of the page scripts' globals. A page can't see the engine's axe, plant a fake
one or change the functions the engine calls, and the page's own `window.axe`, if it has one, is never used. Without axe, or when axe gives an empty name, the script follows Accessible
Name and Description Computation 1.2 and HTML-AAM for the common cases. That means `aria-labelledby`,
`aria-label` and the element's `<label>` (buttons and comboboxes included), then `alt` on an image button and a
button's value. Links and buttons are then named from their rendered content, with image `alt` and SVG
`title`, and the last resorts are `title` and `placeholder`. A select takes its name from its label. WebDriver
BiDi has no command that returns the browser's own accessibility tree, so in Firefox this is a close
approximation of what the browser exposes. Shadow DOM and cross-origin frames are only partly covered.

Only controls a screen reader user can reach are listed and numbered. Links in a closed `<details>` or a
collapsed menu, controls under `aria-hidden` or `inert`, and anything not rendered are left out. The
`hidden` attribute counts only when it really hides the element, since a page's CSS can show it anyway. A
control the browser would give no name shows `(no name)`, sometimes with its link path as a hint. The
persona is told to confirm one with the accessibility check before it reports it. Names are cut at 80
characters, at a word boundary.

The outline is capped at 1,500 tokens (`page.TreeTokens`). At 3 characters to a token that's 4,500 characters.
On a bigger page the engine keeps the landmarks and the focused element first. H1 and H2 headings come next,
then H3 headings and the controls in the main content, then deeper headings, and the controls in the header,
menus, footer and sidebars come last. Page order is kept. A last line says how many entries were left out and
how many of them were controls and headings. If the outline can't be built, the numbered controls are listed
on their own. The page text that follows is cut at 4,000 characters.

Moving focus with Tab counts as a change on the page, so a persona tabbing through a page isn't told that
nothing happened. The focus line and the outline are also saved in `steps.jsonl` under `page.focus` and
`page.tree`.

A document that isn't a web page, such as an RSS feed, a PDF or JSON, is described as a file. The observation
says, "This is not a web page," with its content type. It has no controls or text, and the accessibility check
doesn't run on it. The type is saved as `page.content_type` in `steps.jsonl`.

### Test notes in the observation

Lines that come from the test are worded as the test's, never as page content. Requests the test stopped on the
site's own hosts, such as a form's POST on a read-only target, are listed on the step, because they explain why
something did nothing. Other requests the host rule stopped are counted once, on the first step where they
happen, with a line saying they're the test's rule and not a fault of the site. These are page loads and
writes on other sites, and requests to internal or unknown hosts. The `a11y` result names up
to three failing elements of each rule, by axe's selector and the element's accessible name, and says how many
more there are. The geometry heuristics behind the `layout:` line are unchanged for now.

## The run record

The run record follows `schema/run-record.schema.json` (schema version 2), and the engine validates it
before writing it. `dummyrun coverage` and the Grafana exporter can still read version 1 records from earlier
releases.

A version 2 record adds the fingerprint of each finding and the route of each step. It also adds the
persona's dimensions, `scenario.goal_met`, `repeat_index` and `seed`. Each finding has an `inputs` block with
its `evidence_kind`, `engines`, `check` and `on_journey`, and `reproduced` when a batch has repeated runs.
Records written before inputs existed have none, and a report then reads each of their findings as the
persona's own report. Its cost comes from the price table in
`internal/price/prices.json`, and it's null for a model that isn't in the table. If the target has no
`commit`, the engine makes one from a hash of the home page, fetched with a single GET request outside the
browser. Findings start with the verdict `pending` until a person reviews them (see [Review](#review)).

### What the site is built with

After the first page loads, the engine reads what the site is built with and keeps it in `target.tech`. It looks
only at what the page shows: its scripts, its globals, its meta tags, its markup and the cookies it can read. It
doesn't fetch anything more. Each entry has a `name`, a `category`, the `source` of its fingerprint and the
`evidence` that matched, such as `window.Shopify.shop`. A page can hide or fake a marker, so treat this as a hint.

The categories suggest journeys worth a persona's time, in `target.journeys`:

| Category | Journeys |
|---|---|
| `shop`, such as Shopify, WooCommerce or Magento | `basket`, `checkout` |
| `payments`, such as Stripe or PayPal | `checkout` |
| `search`, a search form or Algolia | `search` |
| `booking`, such as Calendly or OpenTable | `booking` |
| `forms`, such as HubSpot Forms or Typeform | `contact form` |

The engine's own fingerprints cover about 40 platforms that change what's worth testing. They're written from the
markers each platform serves, in `internal/tech/tech.go`. For a fuller list, point `DUMMYRUN_WEBAPPANALYZER` at
your copy of [webappanalyzer](https://github.com/enthec/webappanalyzer), which knows more than 6,000. The engine
reads its `technologies` files when the run starts and uses the markers a page shows. It leaves out HTTP headers,
URLs and request patterns, and a global counts when it exists, whatever its value. webappanalyzer is GPL-3.0, so
Dummy Run doesn't ship it. Its fingerprints come only from your copy. When detection fails, `target.tech_error`
says why, and the run goes on.

```sh
git clone https://github.com/enthec/webappanalyzer
DUMMYRUN_WEBAPPANALYZER="$PWD/webappanalyzer" dummyrun run ...
```

Next to the record, `report.md` holds the persona's own report. `steps.jsonl` holds the full observation for
each step, including the page text, so a reviewer can check the persona's quotes.

Every request to the target carries a `baggage` header with the run and persona IDs. It also gets a
`traceparent` header, unless the page set its own, which is kept. The trace ID and step span ID come from the
run ID and step number, as [TELEMETRY.md](TELEMETRY.md) describes. The browser's user agent ends with
`DummyRun/<version>`, or `DummyRun/<version> (+https://dummyrun.dev/scanner/)` with `--identify`.

## Review

A report shows only the findings a person has confirmed, unless you ask it to include pending ones, and
`dummyrun review` is where you confirm them. Point
it at a folder of run records, or at the record files themselves. It lists the findings that are still
`pending` and lets you set each one to `confirmed` or `rejected`.

To see what's waiting without changing anything:

<!-- doctest -->
```sh
dummyrun review examples/runs --list
```

Add `--json` for output a script can read, or `--verdict all` to include findings that are already reviewed.

### One finding at a time

Run `dummyrun review` in a terminal with no other flag and it walks you through the problems, most severe
first. Each one starts with its severity, kind and evidence tag, then the title. Next come the fingerprint,
route, locus and symptom. The persona and scenario follow. For every step the finding cites you get the
action and what the step saw. You also get the focused element and the screenshot file. Open the screenshot, then type a key and press Enter.

| Key | What it does |
|---|---|
| `c` | Confirm |
| `r` | Reject. You're asked for a short reason, and nothing happens if you leave it empty |
| `e` | Change the severity or the title. The change is saved with your next `c` or `r` |
| `s` | Skip. The finding stays `pending` |
| `q` | Quit. Nothing is lost, because each decision is saved as you make it |

A session looks like this (the screenshot path is shortened):

```text
2 problems to review, reviewer Pat Example. Type a key and press Enter.

--- 1 of 2 ---
minor  accessibility  evidence: measured  class: problem
  The draft title field has no accessible name
  fingerprint 87c7d46615a89adf  route /drafts  locus textbox:title  symptom a11y:label
  persona sam-author (author)  scenario submit-draft  run 2026-10-15-sam-author-001
  cited steps:
    step 3  click Schedule
      saw: status 200, route /drafts
      focus: button "Schedule"
      screenshot: runs/2026-10-15-sam-author-001/screens/003.png
[c]onfirm  [r]eject  [e]dit severity or title  [s]kip  [q]uit > e
Severity (blocker major minor info, Enter keeps minor): major
Title (Enter keeps it):
The edit is applied with your next confirm or reject.
[c]onfirm  [r]eject  [e]dit severity or title  [s]kip  [q]uit > c
confirmed: 1 finding in 1 run.

--- 2 of 2 ---
info  question  evidence: inferred  class: problem
  The payment status 'settled' is unclear to a contributor
  fingerprint 1718b75300702f7b  route /payments  locus region:payment status  symptom unclear-label
  persona lin-contributor (contributor)  scenario check-payment  run 2026-10-16-lin-contributor-004
  3 findings of this problem in all. One verdict covers the 2 still pending. The others:
    2026-10-16-lin-contributor-002  persona lin-contributor  confirmed  payment-status-wording
    2026-10-17-lin-contributor-008  persona lin-contributor  pending  payment-status-wording
  cited steps:
    step 9  visit
      saw: status 200, route /payments
[c]onfirm  [r]eject  [e]dit severity or title  [s]kip  [q]uit > r
Reason for rejecting (empty cancels): The wording matches the product glossary
rejected: 2 findings in 2 runs.

Confirmed 1, rejected 1, skipped 0. 3 findings changed.
```

### Without a terminal

In a script or a CI job, name the findings in flags:

```sh
dummyrun review runs/ --confirm 87c7d4 --reviewer "Pat Example"
dummyrun review runs/ --reject 1718b7 --reason "The wording matches the product glossary"
```

A selector can be a fingerprint, or a prefix of one at least six characters long, in either case. It can also
be `run-id/finding-id`, or a finding ID that only one problem uses. The command checks every selector before
writing anything, and if one is wrong it exits 2 and changes nothing.

A rejection needs `--reason`, and a confirmation can have one. You give `--reason` once and it applies to every
verdict in that command. `--reviewer` defaults to `git config user.name`, and the command stops if neither is
set. It exits 0 when it wrote the verdicts, 1 when a write failed and 2 when the command line was wrong.

### One problem is judged once

Findings with the same fingerprint are one problem seen in several runs, so you only decide it once. Your
verdict reaches every finding with that fingerprint that's still `pending`, and the command tells you how many
it changed. It never changes a finding someone has already confirmed or rejected. An edit to the severity or
the title reaches the same pending findings.

### What a decision writes

For each finding it changes, the command sets `verdict` and adds three optional fields from schema version 2.

| Field | Content |
|---|---|
| `reviewer` | The `--reviewer` value or `git config user.name` |
| `reviewed_at` | The time of the decision, in UTC |
| `reason` | Why. Required for a rejection |

Nothing else in the record changes. Just before writing, the command reads the record again and checks the new
text against the schema. It writes the result to a temporary file in the same folder and renames that over the record.
If another reviewer saved the record in the meantime, your verdict is applied to whatever is still pending.
A failure while the record is checked or replaced leaves the old record as it was.

Two reviewers working in the same folder take turns on a lock while each record is rewritten. That works on
Linux, macOS, the BSDs and Windows. On Windows the lock is a `.review.lock` file in the folder. Other systems have no
lock. There the command rereads the record just before the rename and starts over if it changed, which leaves
a brief moment when two reviewers can still collide.

After the records are written, the command adds one line to `review-log.jsonl` in the record's folder. The
line holds the time and the reviewer, plus the fingerprint, the verdict and the reason. It then lists each changed
finding with its run ID, finding ID and earlier severity and title. If writing the log fails, the records have
already changed and the command tells you. The tool never deletes a line from the log.

Version 1 records have no fingerprint and no review fields. The command skips them and says so.

### Reports and exports

`dummyrun coverage` counts only confirmed findings unless you pass `--confirmed-only=false`, so review before
you export. `dummyrun-otel` sends `verdict` with each finding and reads reviewed records as they are. It
doesn't update what it sent before, so sending a record again after a review duplicates its spans and log
lines. [TELEMETRY.md](TELEMETRY.md) explains what a repeat does.

## Read-only policy

Set `"read_only": true` in the target file to stop a run sending anything but reads. The engine intercepts
every request the browser makes. A request to an allowed host with any method other than GET, HEAD or OPTIONS
fails as a network error before it leaves the browser. The step that caused it gets a note such as
`blocked: POST https://example.test/subscribe (read-only target)`. A write to a host outside `allowed_hosts`
fails the same way, read-only or not. The only exception is the sign-in request, described under [Sign in](#sign-in).

Documents load only from `allowed_hosts`: the page in the persona's tab, frames and pop-ups. What a page loads
to show itself is different. These are its images, stylesheets, fonts, scripts, media and fetch calls. They may
come from any public host by GET or HEAD. Sites keep them on CDNs, and a page without them isn't the page a
visitor sees. A page's own scripts can therefore put data they can read into a GET address and send it to a
public host, as they can for any visitor. The persona and the engine never put secrets in an address. See
[the threat model](THREAT-MODEL.md).

The engine tells the two apart from what the browser reports for each request. A request with a navigation id
is a document load. So is one whose destination is a document, iframe, frame, object or embed. When the browser
reports neither, a request whose `Accept` header starts with `text/html` is a document load too. Everything
else is a subresource. A subresource is still refused when its host is, or resolves to, an internal address. That covers
loopback, private, link-local, carrier-grade NAT and other special-use ranges, including IPv6 forms that carry
an IPv4 address. It holds even for a local target. If your app loads assets from a second local port, list it
in `allowed_hosts`. The run's trace headers go only to the target's own origins. Allowed subresources aren't
noted on the step.

The address check runs once when the request starts, so it can't stop a name that later points somewhere else
(DNS rebinding). Frames from other hosts are refused for that reason, since a frame runs with its own origin. A
script on the page itself can't read a response from another host unless that host allows it. Run the engine on
a network that blocks the browser from internal addresses if a page you test might try.

The engine judges a request by its method, so a GET that changes state in your app still goes through. If
signing out is a GET link, list its path in `signin.block_paths`.

The persona's prompt mentions the policy too, but it's the interception that enforces it. The engine's own
browser tests check this with a form whose POST never reaches the test server, on both browsers.

An `allowed_hosts` entry is a host name or `host:port`. A bare host name matches the default port of the
request's scheme. It also matches the port of `base_url`, when it's the base URL's own host. Any other port
needs its own `host:port` entry. For example, with a bare `127.0.0.1` entry and no `127.0.0.1:9000` entry, a
page on `127.0.0.1:8080` can't reach a service on `127.0.0.1:9000`. Matching is exact, so each subdomain needs
its own entry. A page load or a `visit` to a host outside the list isn't followed, and nor is a write. The step
gets a `blocked:` note and the record gets an `off_origin_blocked` event.

## Untrusted page content

The engine treats the page under test as input the model mustn't trust. [Threat model](THREAT-MODEL.md) lists
the threats. Here is what the engine does about prompt injection.

- Everything from the page that's sent to the model sits between `<<<PAGE_CONTENT <id>` and
  `PAGE_CONTENT <id>>>>` markers. The id is random for each run, so the page can't close the fence. The system
  prompt says text inside the markers is data and never changes the task, the tools, the verdict or the
  findings.
- Text the browser doesn't render isn't sent. That covers `display: none`, `visibility: hidden` and comments.
  Text with zero size, zero opacity or `aria-hidden` is still part of the page text. It can reach the model,
  and the scan flags it as hidden. So does text that CSS only moves off the screen.
  The quoted-example exemption never covers it, so a page that plays a code demo has to keep the text out of
  those states. The terminal on dummyrun.dev covers the lines it has not played yet with a CSS mask instead of
  fading them out. The scan does not look at CSS masks, so it reads those lines as quoted output.
- The engine collects that hidden text along with the `aria-label`, `title`, `alt` and `placeholder` values.
  They're saved in `page.hidden` in `steps.jsonl`, for the scan and for the reviewer.
- A scan runs over the page text, the control names, the outline and the hidden text. For a persona with no
  screenshots it also covers what the screen reader says. It looks for phrases such
  as "ignore previous instructions", "system prompt" and "you are now", and for chat template tokens. It also
  looks for orders to report nothing or to call `finish`, and for claims that the site is fully accessible.
  Instructions to go to a host outside the allow list are flagged, and so are invisible Unicode characters. Hidden text that
  addresses an assistant and gives an order is flagged too. These are heuristics to help a reviewer. They
  don't filter anything.
- Code and quotes you can see on the page don't count, so a docs page that quotes an example attack isn't
  flagged. The scan skips visible `code` and `pre` text, keys and sample output (`kbd` and `samp`), and quotes
  (`blockquote` and `q`). Quoted text is still scanned when it's hidden, off screen or transparent, or sits
  inside `aria-hidden`. So are `aria-label`, `title`, `alt` and comments. The model still receives the skipped text, so a
  hostile page can hide an instruction in a visible code element without being flagged. Fencing still applies.
- A lone "system prompt" or "call `finish`" in visible prose is a weak match. It becomes an event with
  `low confidence:` in its detail, and it doesn't warn the persona.
- A strong match adds a `HARNESS WARNING` line to that step's result, which tells the persona the text is data
  from the website to ignore, and a `suspected_injection` event to the run record. Detector hits are recorded as events in the run record for the operator, never as findings. The
  detector also fires on ordinary text, and a site owner can't act on a guess about what a page meant.
- A finding that cites no step in the run is dropped and recorded as an `uncited_finding` event.
- A finding whose cited steps all show the test's own limits is dropped and recorded as a
  `harness_limit_finding` event that says why. These are the steps that count as the test's limits:
  - the browser couldn't read the page
  - a tool was refused or failed, such as a lost connection, a timeout or an accessibility check that didn't finish
  - the page was a feed or other file
  - the persona's action sent a request the test blocked and the address stayed the same, when the finding
    says something didn't work, was slow, showed an error or couldn't be reached
  - the persona's action sent a request the test blocked and the browser showed its own error page instead

  A request the page sent by itself, such as a beacon while the persona scrolled, doesn't count. A finding
  with at least one cited step that shows the real page is kept. The persona is also told that these limits
  aren't the site's faults.

The record's `events` array is optional. Each event has a `kind` and a short `detail`, plus a `step` where one
applies. A `suspected_injection` event also has the `source` (`visible`, `label` or `hidden`) and the `pattern`.
Its `detail` is page text, so treat it as data. `dummyrun-otel` doesn't export events.

A reader built from an older copy of `schema/run-record.schema.json` rejects a record that has `events`, the
outcome `provider_filtered`, the event kind `harness_limit_finding` or a finding of kind `security`. Records
written before these existed stay valid. The engine no longer files `security` findings itself, but the kind
stays in the schema. `dummyrun-otel` reads the outcome and the finding
kind and ignores `events`.

### When the provider refuses

Sometimes a model provider refuses a call because its content filter objects to the page. The run stops, and
the command exits with status 1. The record has the outcome `provider_filtered`, a `provider_filtered` event,
the steps so far and the model's findings, if any. `report.md` and the command's message both say the provider
refused.

Each provider signals a refusal in its own way, and the engine maps all of them to this outcome:

| Provider | What counts as a refusal |
|---|---|
| Azure OpenAI and other OpenAI-compatible services | A content filter finish reason, a `refusal` message or an API error |
| The Responses API | A cut-off response or a `refusal` part |
| Anthropic | A `refusal` stop reason |
| Gemini | A safety block |
| Bedrock | A guardrail stop reason. The call keeps its token counts |

## Sign in

Some apps show nothing until you sign in. The engine can sign in for the persona before it starts, and it never
hands the persona the credentials, so the persona can't be talked into revealing them. A signed-in page may
still show the user name, as [Where credentials never go](#where-credentials-never-go) explains.

There are two ways to sign in: the engine fills in the login form, or it loads a saved session file. Both need
a test account of your own with no multi-factor step. The engine can't handle single sign-on or multi-factor
prompts. For those, sign in once by hand or with a test account and save a session file.

### The target file

Add a `signin` block to the target file:

```json
{
  "adapter": "url",
  "environment": "staging",
  "base_url": "https://staging.example.test/",
  "allowed_hosts": ["staging.example.test"],
  "read_only": true,
  "signin": {
    "login_url": "/signin",
    "username": { "selector": "#email" },
    "password": { "label": "Password" },
    "submit": { "selector": "form[action='/signin'] button[type=submit]" },
    "allow_request": { "method": "POST", "url": "/signin" },
    "success": { "selector": "form[action='/signout']" },
    "block_paths": ["/logout"],
    "credentials": {
      "viewer": { "username_env": "STAGING_VIEWER_USER", "password_env": "STAGING_VIEWER_PASS" },
      "admin": { "username_env": "STAGING_ADMIN_USER", "password_env": "STAGING_ADMIN_PASS" }
    }
  }
}
```

| Key | Meaning |
|---|---|
| `login_url` | The page with the form. A path is resolved against `base_url` |
| `username`, `password`, `submit` | How to find each control. Give either a CSS `selector` or the text of its `label` |
| `allow_request` | The form's own request, by `method` (default POST) and `url`. The query is ignored. During sign-in, this is the one request the read-only policy lets through |
| `success` | A `url_matches` regular expression, a `selector` or both. Make it specific to a signed-in page. Sign-in only counts when the request above was sent and this check passes |
| `block_paths` | Paths that sign the user out, such as `/logout`. A request to one fails for the whole run, whatever its method |
| `credentials` | One entry per role. It names the environment variables that hold the user name and password, or a `session_file` |

The file never holds a credential itself. A value that isn't a valid variable name is rejected. Every host named
in the block must also be in `allowed_hosts`.

### Which role signs in

The engine uses the first of these that's set:

1. The scenario's `Sign in as: editor` line. `Sign in as: none` turns sign-in off.
2. The persona's `signin: editor` front matter key. `none` turns it off.
3. The persona's `role` key, when the target has credentials under that name. The library's `role-viewer`,
   `role-editor` and `role-admin` personas work this way, so a target with credentials under those three
   names needs nothing else.

If a role is asked for by name and the target doesn't list it, or an environment variable is missing, the run
stops before a browser starts. A persona whose role matches nothing runs signed out.

### Method one: the login form

The engine opens a tab of its own, loads the login page, fills in the two fields from the environment and
clicks submit. It waits for the declared request and the success check, then closes the tab. The persona's tab
starts clean, with the session cookies in the browser and no history. The model is told which role it's signed
in as, and that signing out is blocked.

### Method two: a session file

To sign in once and reuse the session, save it with `dummyrun session save`. It signs in through the form and
writes the browser's cookies to a file, along with the local storage of the target's origin and of the sign-in
page's origin.

<!-- doctest -->
```sh
dummyrun session -h
```

```sh
export STAGING_ADMIN_USER=... STAGING_ADMIN_PASS=...
dummyrun session save --target staging.json --role admin --out sessions/admin.json
dummyrun run --session sessions/admin.json ...
dummyrun a11y --session sessions/admin.json https://staging.example.test/admin/
```

To load a session without the flag, set a role's `session_file` in the target file. A relative path starts
from the target file's directory. `dummyrun a11y --session FILE` loads the file before the first page, so the
sweep tests pages as that user sees them.

The file uses Playwright's `storageState` format, so a file saved by Playwright loads here, and the other way
round. When the engine loads it:

- it skips cookies for hosts outside `allowed_hosts`, expired cookies, and cookies without the `Secure` flag
  on a host that isn't local
- it sets local storage only for https origins of allowed hosts, without comparing the port
- it stops with an error if the file loads no cookie and no local storage

The engine doesn't check whether a loaded session is still valid, so save a fresh one when it expires. The file
holds live session cookies and is written with mode 0600. Treat it like a password and keep it out of version
control.

### The read-only exception

On a read-only target the sign-in POST would be blocked, so the policy makes one exception. While the engine
signs in by form, it lets through exactly one request: the one `allow_request` declares, matched by method and
URL. A second matching request is blocked, and so is every other write. Hosts outside `allowed_hosts` stay
blocked. The exception ends before the persona's first step and never reopens, so if the persona finds the
login form and posts it, that's blocked like any other write. Only targets with a `signin` block get the
exception.

Signing out is blocked too. On a read-only target the usual sign-out button is a POST, which fails like any
write, and `block_paths` covers a sign-out link that's a GET.

### What the record says

The run record has an optional `signin` block, added in schema version 2.

```json
"signin": { "method": "form", "role": "admin", "ok": true,
  "allowed_request": { "method": "POST", "url": "https://staging.example.test/signin" }, "duration_ms": 850 }
```

`method` is `form` or `session`. `allowed_request` is the one request the exception let through, without its
query, and it's null for a session. A failed sign-in has `ok: false` and an `error`, and the run is `errored`
with no steps. The block holds the role name from the target file, never a user name or password.

### Where credentials never go

The engine reads credentials from the environment when the run starts, and types them into the form over the
BiDi connection. They go nowhere else. No screenshot is taken during sign-in, so the password field is never
captured, and both values are scrubbed from errors before anything is recorded or logged. The engine's browser
tests check the model's requests and every file a run writes for the test password, on both browsers, and
check the log and the record for the user name.

The engine doesn't edit page text, though. A page can still show the user name, on a list of users for
example. See [SECURITY.md](../SECURITY.md) for more.

## Protected previews

Staging and preview sites are often behind a gate: HTTP Basic auth, or a secret header that lets automation
through. Without a way past it, the persona only ever sees the gate. Add an `auth` block to the target file
and the engine gets past it for every request to your site. The persona never sees the secrets.

The block takes Basic auth, headers or both. Each value is the name of an environment variable, never the
secret itself.

```json
{
  "adapter": "url",
  "environment": "pr-42",
  "base_url": "https://pr-42.preview.example.test/",
  "allowed_hosts": ["pr-42.preview.example.test"],
  "read_only": true,
  "auth": {
    "headers": {
      "x-vercel-protection-bypass": { "value_env": "VERCEL_AUTOMATION_BYPASS_SECRET" }
    }
  }
}
```

That is Vercel's Protection Bypass for Automation. Copy the secret from your project's Deployment Protection
settings into `VERCEL_AUTOMATION_BYPASS_SECRET`. For Cloudflare Access, create a service token and send both
halves of it:

```json
"auth": {
  "headers": {
    "CF-Access-Client-Id": { "value_env": "CF_ACCESS_CLIENT_ID" },
    "CF-Access-Client-Secret": { "value_env": "CF_ACCESS_CLIENT_SECRET" }
  }
}
```

For Basic auth, name the two variables that hold the user name and password:

```json
"auth": { "basic": { "username_env": "PREVIEW_USER", "password_env": "PREVIEW_PASS" } }
```

A bearer token or any other custom header works the same way: `"Authorization": { "value_env": "PREVIEW_TOKEN" }`
holds the whole header value, such as `Bearer abc`. You can't mix that with `basic`, since both use the
Authorization header.

For a one-off run you can skip the file. `dummyrun run`, `dummyrun try`, `dummyrun a11y` and `dummyrun session save` take the
same thing as flags, and flags win over the file:

```sh
export PREVIEW_USER=... PREVIEW_PASS=...
dummyrun a11y --basic-auth-env PREVIEW_USER:PREVIEW_PASS https://staging.example.test/
dummyrun run ... --header-env 'x-vercel-protection-bypass:VERCEL_AUTOMATION_BYPASS_SECRET'
```

### How the secrets travel

- Only your own hosts get them, and only over https. A request over plain http carries nothing, except on
  `localhost` and other loopback addresses, which is where `base_url` may use http too. In a run, that means the hosts in `allowed_hosts`. The
  [read-only policy](#read-only-policy) matches them in the same way, including the port. In `dummyrun a11y`, it means the origins of the
  URLs, the sitemap and `--rebase` that you name. A request to any other host never carries a secret, and that
  includes a redirect from your site to another host.
- The engine adds the headers to each request in the browser and keeps what the request already had. With Basic
  auth it sends the `Authorization` header up front, unless the page set its own, and it also answers the
  browser's Basic challenge as a fallback. It answers once per request, so a wrong password fails with the
  site's 401 instead of looping.
- The headers never let a write through the read-only policy.
- The engine's own requests for `robots.txt`, sitemaps and the home page hash carry the headers too, on your
  hosts only.
- The values are read from the environment when the run starts. A missing or empty variable stops the run
  before the browser opens, and the error names the variable. A secret written into the target file is refused
  with an error that tells you to name a variable instead. The error never repeats the secret.
- The engine never writes a secret into the run record, the log, the screenshots, the telemetry or an error.
  A page can still expose a secret if the site echoes it. The record has an optional `preview_access`
  block. It was added in schema version 2. It holds the methods used and the header names, nothing else:

```json
"preview_access": { "methods": ["basic", "headers"], "header_names": ["x-vercel-protection-bypass"] }
```

`dummyrun a11y` writes the same block in its JSON report. The engine's browser tests check every output for
the test secrets on both browsers. Your site can still echo a header back in a page, so don't build a preview
that prints its own bypass token. [SECURITY.md](../SECURITY.md) has the rules for credentials.

## Identifying the engine and honouring robots.txt

Two switches let a run say who it is and stay out of places that ask crawlers to keep away. Both are off by
default.

`--identify` changes the end of the user agent. Without it, the browser's own user agent gets
`DummyRun/<version>` added. With it, the end reads `DummyRun/<version> (+https://dummyrun.dev/scanner/)`. The
browser's own text stays at the front, so sites render as they normally would. Both `run` and `a11y` set it
over WebDriver BiDi (`emulation.setUserAgentOverride`) on Chrome and Firefox. The engine's own requests, such
as for `robots.txt` and the sitemap, carry the same suffix.

`--respect-robots` makes the engine fetch `/robots.txt` from the target's origin before anything else,
including the home-page hash and the browser. It reads the group for the token `DummyRun`, or the `*` group if
there isn't one, following RFC 9309:

- Paths and patterns are compared after percent-encoding normalisation.
- The longest matching rule wins, and `Allow` wins a tie.
- `*` and a final `$` work in patterns. `Crawl-delay` is ignored.
- A `4xx` answer means there are no rules. A `5xx` answer or a network failure means the whole site is off
  limits.

A sitemap named by `--universe` or `--sitemap` is checked against its own origin's robots.txt before it's
requested.

With `run`, a request to the target's origin for a disallowed path is blocked by the same interception as the
read-only policy. The step notes it as `blocked: GET https://example.test/admin (robots.txt disallows DummyRun)`.
`/robots.txt` itself is always allowed. Other hosts in `allowed_hosts` aren't covered, because their robots.txt
files aren't read. If the start page is disallowed, which includes a site-wide `Disallow: /`, the run stops
before the browser starts. The record has the outcome `blocked` and an `outcome_reason`, and the command exits
with status 3.

With `a11y`, each page is checked against the robots.txt of its own origin. A disallowed page is skipped,
listed as `skipped` in the report and counted in `skipped_pages`, and it doesn't fail the sweep. If every page
is disallowed, no browser starts and the exit status is 3. The sweep doesn't intercept the requests a page makes
for its own files.

The record keeps both settings as `target.identify` and `target.respect_robots`, and leaves them out when
they're false.

## Providers

Pick a provider with `--provider`. [PROVIDERS.md](PROVIDERS.md) has the flags, the notes for each vendor and
where each one keeps your data. The engine works with Azure OpenAI and any OpenAI-compatible service. It
also works with Anthropic, Gemini and Amazon Bedrock. It knows what each one can do and picks fallbacks for you.

Azure OpenAI signs in with `DefaultAzureCredential`, which tries the environment, workload identity and managed
identity before your Azure CLI sign-in. It never reads an API key. The other providers read a key from the
environment when the run starts. The Azure API version defaults to `2025-04-01-preview`, and `--api-version`
overrides it.

Each call has a 120-second timeout and up to three retries. Token usage from every call is added up in the
record's `usage` block. `--region` is recorded as `provider.region`, because a deployment doesn't report where
it runs.

## The accessibility sweep

To check a set of pages for accessibility problems without calling a model, use `dummyrun a11y`. It runs the
same axe-core 4.13.0 as the `a11y` tool, over the same WebDriver BiDi connection, on every page you give it.
`dummyrun audit a11y` is the same command.

```sh
dummyrun a11y --browser chrome --sitemap https://example.com/sitemap.xml \
  --allow a11y-allow.txt --json report.json https://example.com/404.html
```

- Protected previews. `--basic-auth-env USER_ENV:PASS_ENV` and `--header-env 'Name:ENV'` get the sweep past Basic auth or a bypass header, as [Protected previews](#protected-previews) describes.
- Pages. Give URLs, a sitemap or both. A sitemap index is followed one level down. `--rebase URL` moves every
  sitemap address onto another origin, so you can test a local copy of a site from its live sitemap.
- Rules. The axe-core rules tagged WCAG 2.0, 2.1 and 2.2 at levels A and AA, plus axe's best-practice rules.
  A rule that has none of those WCAG level tags, such as `region` or `heading-order`, is advice: its results go
  to each page's `advice` list and the summary's `advice_nodes`, never to `violations`, and they never fail the
  sweep or change its exit code. Only WCAG rules go to `needs_review`.
- Sizes. `--viewport` is `desktop` (1280 wide), `phone` (375 wide) or `both`.
- Colour scheme. `--scheme` is `light`, `dark` or `both`. WebDriver BiDi has no command for
  `prefers-color-scheme`. So the sweep sets it at launch, as a Blink setting in Chrome or a preference in
  Firefox, and starts one browser per scheme. After each page loads, it reads the scheme the page actually got and reports a
  mismatch as a page error, so a sweep can't quietly test the wrong theme.
- Needs review. When axe-core can't decide, for example on text partly hidden by a scrolling container, the
  result is listed as "needs review" in the JSON and doesn't fail the run.
- Output. A short summary on standard output lists each failure once, with the browser, size and scheme where
  it appeared. `--json FILE` (or `-` for standard output) writes every page. Each failure has the rule ID and the
  impact. It also has the WCAG criteria, the element selector and the help link.
- Exit status. 0 when nothing fails. 1 when a violation isn't in the allow-list or a page can't be loaded. 2
  for a usage error. 3 when robots.txt disallows every page (with `--respect-robots`).

To run every check in one go, add `--all`. It turns on the keyboard, markup and content checks, the cookie banner, the other
rule engines, contrast over images, the HTML check, Lighthouse, the journeys, the link check, the layout checks and
plain language. Four of them use programs of their own: Vale for plain language, vnu for the HTML check, lychee for
the links, and Lighthouse, which also needs Chrome. The container image has Vale, vnu and lychee but not Lighthouse.
A check whose program is missing says so on each page, and the rest of the sweep still runs. Add `--text-scale
100,200` to test large text in the same run.

```sh
dummyrun a11y --all --text-scale 100,200 --json report.json https://example.com/
```

The allow-list holds violations you've accepted, one per line as `rule | page | selector | reason`. The page
is a URL path, and the selector is matched against the element's selector. A `*` matches anything. The reason
is required, so every exception is explained. If a line matches nothing in a sweep, the sweep reports it, so
you can remove stale exceptions.

The dummyrun.dev site is checked with this command on Chrome and Firefox, with the allow-list in
`site/a11y-allow.txt`.

### Layout checks

To check layout as well as the axe rules, add `--layout`. Passing `--text-scale` turns the checks on too. They
measure the page in the browser, with no model call, and report each problem as a finding next to the axe
results. A layout finding fails the sweep like an axe violation, and the allow-list can accept it. The checks
are off by default, so an existing sweep won't turn red until you ask for them.

The checks read the rectangles the browser lays out for elements and lines of text. Where it matters, they also ask
the browser what is painted on top at a point. A box that merely reaches into another box doesn't count. Only what
a reader would see goes wrong. The approach follows the geometry scorers in LayoutLens (MIT), written afresh for this
engine.

Unless you set `--viewport` or `--text-scale` yourself, `--layout` adds the two settings the criteria name. These
are a 320 px window for WCAG 1.4.10 and text at 200 percent for WCAG 1.4.4. So `--layout` alone tests desktop,
phone and the 320 px window, each at 100 and 200 percent text. Set `--viewport` or `--text-scale` to test fewer.

| Rule ID | Fails when | WCAG | Impact |
|---|---|---|---|
| `dummyrun-reflow` | The page is wider than the window, so it scrolls sideways. Each node is an outermost element that reaches past the window on the side the text runs towards. Content past the other edge, such as a skip link waiting off screen, can't be scrolled to | 1.4.10 | serious |
| `dummyrun-clipped-text` | A line of text reaches past the edge of a box that hides its overflow, while part of the text is still inside that box | 1.4.4 | serious |
| `dummyrun-overlap` | Where two controls meet, the browser has one of them on top of the other's text, at two or more points. A control with no text counts when most of its box is covered | 1.4.4, 1.4.10 | serious |
| `dummyrun-target-size` | A visible, enabled control is under 24 by 24 CSS px, and none of the measurable exceptions apply | 2.5.8 | moderate |
| `dummyrun-focus-obscured` | A control that takes focus is entirely hidden by something opaque, such as a sticky header, once the browser has scrolled it into view | 2.4.11 | serious |

What doesn't count, because WCAG allows it or a person wouldn't see a problem:

- For reflow, WCAG 1.4.10 lets some content keep a two-dimensional layout, so it can be wider than the window. That
  covers data tables, images, maps, video and toolbars. When those are the only elements past the edge, there's no
  finding, and `reflow_exempt` in the layout result counts them.
- For clipped text, the other slides of a carousel don't count: text with no line inside the box is hidden by
  it. A box of 1 px or less, or one with `clip` or `clip-path`, hides text from the eye on purpose. A box
  that scrolls, and has something to scroll to, lets the reader reach the rest, and so does a box that hides overflow around one that scrolls on the
  same axis, such as a terminal window with a scrolling line inside a rounded frame. Only words are measured, so the
  spaces that hang past the edge of a `pre-wrap` line don't count. Text that's moving, because it or a box
  around it has a CSS animation or transition running, is left out, since its box shows where it is mid-flight
  rather than where it comes to rest. Text shortened with an ellipsis or a line clamp counts only inside a
  control, where the missing words are the control's name.
- For overlap, a control inside another control doesn't count. Nor do boxes that meet without covering any text,
  such as transparent padding.
- For target size, the exceptions are a link inside a sentence and a small target with room around it. A button,
  input, select or text area the page leaves at the browser's own size is exempt too. Room around it means a 24 px circle on its centre meets no
  other target and no other small target's circle. A checkbox or radio counts as large as its label. The
  equivalent-control and essential exceptions need a person, so check a target size finding by eye before you
  act on it.
- For focus obscured, a control only partly hidden doesn't count, because WCAG 2.4.11 allows it at level AA. An
  image, video or canvas counts as cover only when its pixels are solid. One whose pixels can't be read, such as
  an image from another site, never counts. Focus moves to at most 150 controls on a page. The check runs only in a sweep, never during a persona's run, because a
  page can react to focus.

Disabled controls are skipped, and so are elements 1 px or smaller.

Each result in the JSON has `text_scale` (left out at 100) and `layout_checked`. Each node can have a `detail` and a
`screenshot`. `--screenshots DIR` writes a PNG crop of each flagged element to `DIR`, up to 12 for each page and
setting, and the reflow finding gets a shot of the window. The summary lists them. The settings in the summary and
report end in `/text-200` when a text scale was used.

```sh
dummyrun a11y --text-scale 100,200 --viewport phone --screenshots shots https://example.com/
```

### Keyboard checks

To check the page by keyboard, add `--keyboard`. The sweep presses Tab from the top of the page, up to 80
times and records where focus goes. It also clicks up to 5 controls on each page to compare mouse and keyboard, as
described below. It then reports what it measured. There is no model call. The checks are off by default.
The pixel comparison covers no more than the first 30 stops that need it. The walk ends at the first loop or repeated stop, so later stops go unchecked. When that happens the page's result says `keyboard_partial`, so a page with no keyboard findings is not read as one that was checked all the way through.

| Rule ID | Fails when | WCAG | Impact |
|---|---|---|---|
| `dummyrun-focus-not-visible` | An element that takes focus looks the same, in the pixels around it, with and without focus. An element whose outline or style changes is never checked, so a focus ring drawn another way passes only if it changes the picture | 2.4.7 | serious |
| `dummyrun-focus-low-contrast` | Pixels change when an element gets focus, but no pixel's colour with focus has a contrast ratio of 3:1 with its colour without focus, as with a pale grey outline or a faint change of background | 1.4.11 | serious |
| `dummyrun-focus-hidden` | Focus lands on an element with no size, hidden, or outside the window | 2.4.7 | serious |
| `dummyrun-keyboard-trap` | Focus keeps cycling through the same few elements, outside any dialog, and Escape and more Tab presses do not get it out | 2.1.2 | critical |
| `dummyrun-skip-link-broken` | The first Tab stop is a "skip to content" link whose target is not on the page or is not shown | 2.4.1 | serious |
| `dummyrun-skip-link-no-focus` | The skip link's target is there, but after Enter on the link the next Tab lands before the target, so focus never moved | 2.4.1 | serious |
| `dummyrun-mouse-only` | A click on a control does what it's for, but from the keyboard Tab never reaches it, or Enter and Space do nothing | 2.1.1 | serious |

For the focus rules, the sweep takes two pictures of each element and the 6 pixels round it, one with focus and
one without. It waits for the page to draw and for any transition to end, up to half a second, before each picture.
The pictures cover the first 30 stops with no focus style the walk can read, and another 30 with one. From them it
measures the indicator the way WCAG 2.4.13 does. It counts the pixels that change and how much their contrast
changes. It also works out the area of a line 2 CSS pixels thick round the element. Fewer than 6 changed screenshot pixels
don't count as an indicator, since a blinking caret can change that many. Each stop's `appearance` in the JSON
gives `changed` and `area`, the pixels that change by 3:1 or more, in square CSS pixels. It also gives `need`, the
area of that 2 pixel line, and `contrast`, the biggest change.

An indicator with enough contrast but less area than that line is advice, `dummyrun-focus-appearance` (WCAG
2.4.13). Level AAA asks for that area, and AA doesn't, so it never fails the sweep. A one pixel outline is a common
example. The picture shows only 6 pixels round the element, so an outline set further out than that isn't seen. A control wider than 588 or taller than 288 CSS pixels doesn't fit in the picture, so its indicator isn't measured.

The last two rules come from trying controls and seeing what happens. For up to 5 controls on each page, the
sweep picks ones it can test. These are a control with `aria-expanded`, one whose `aria-controls` names a hidden
element, a closed `<details>`, an unselected tab, a custom checkbox or switch, and something that shows the hand
pointer but isn't a control. Each has a success test that the page computes, such as "`aria-expanded` became true"
or, for the last kind, "the address, the words or the shown elements changed".

For each control, the sweep loads the page afresh and checks the test is false. It clicks the control with the
mouse and checks the test is now true. On another fresh load, it presses Tab until the control has focus, then
Enter, then Space, and checks the test again. Tab stops once focus has come back round to where it started, or has
stayed outside the page for three presses. A control counts only when the click worked and the keyboard didn't. A
control whose test was already true, that something else covers, that a click didn't change, or that Tab didn't reach
within 80 stops has no verdict, and its `skipped` says why. A link to another page, a `javascript:` link and a
form's submit button are never picked, and nor is anything holding one. Nor are text fields and comboboxes, where
Enter sends the form or the arrow keys do the work, or a tab that the arrow keys reach while another tab in its list
takes Tab. A control's own script can still navigate
or send a request when it's clicked. A page that never takes keyboard focus shows as an error.

The skip link is tried the same way. The sweep presses Tab on a fresh load. If focus is on a skip link whose target
is shown, it presses Enter and then Tab. Focus must then land inside the target or after it. When Tab alone gets
there, as it does when the link sits just before its target, the check gives no verdict.

When the sweep is signed in, with a session or preview access, it tries nothing: no controls and no skip link. Any
click or Enter could run a script that sends a request and changes the account or signs it out.

This loads each page a dozen times or so, so it runs once for each page and window size, on the first colour
scheme and text size. It runs in a tab of its own, so the rest of the sweep is unchanged. The results are in
`tasks` in each page's JSON.

Focus that moves into a frame or a media player is followed no further, so the walk counts as partial. A dialog that keeps focus inside itself is not a trap, even when it has only one control. A page with nothing that takes focus counts as fully walked. If the trap test cannot finish, the page shows an error instead of a clean result.

### Markup checks

Add `--markup` for two checks on the page's markup that axe leaves out by default. They run in the browser with no model call.

| Rule ID | Fails when | WCAG | Impact |
|---|---|---|---|
| `dummyrun-label-in-name` | A link or button has an `aria-label` or `aria-labelledby` name, and the words it shows are not part of that name. Punctuation and case are ignored, and a control that shows only a symbol is skipped | 2.5.3 | serious |
| `dummyrun-autocomplete-missing` | A field asks for the visitor's own email, phone number, first name, last name or postcode. It is visible, enabled and editable. It has no `autocomplete` value that names a purpose. A value names a purpose when it follows the HTML standard's autofill grammar, such as `email` or `shipping postal-code`. `off`, `on` and made-up values do not. Read-only fields and search forms are skipped, and so are fields that ask for another person's details, such as a friend's email | 1.3.5 | serious |

These findings fail the sweep like axe rules, and you can accept one with the allow-list. When the markup checks finish on a page, its JSON result has `markup_checked: true`, so a page with no markup findings is known to have been checked.

The markup checks also list the roles and `aria-*` attributes the page's shown elements use, and look each one up
in the results of [ARIA-AT](https://aria-at.w3.org/reports). That's the W3C community group that tests how JAWS,
NVDA and VoiceOver handle each ARIA pattern. When one screen reader and browser pair passes
under 60 percent of the tests for a role or attribute, the sweep reports advice, `dummyrun-aria-support`. The advice names the first element that
uses it, how many use it, and each pair's share, such as "NVDA with Chrome 18% (9 of 50)". For a role
attribute that lists fallbacks, the first role ARIA defines counts, since that's the one the browser uses.

ARIA-AT tests whole patterns, such as a tab list or a grid. A low figure means that screen reader and browser
passed few of ARIA-AT's tests of the patterns that use the role or attribute. Your page may still be right. Most roles and attributes have a pair below 90 percent. The line is drawn
at 60 so the advice is about clearly weak support. The figures were fetched on 2026-10-26 and ship with the engine.
`internal/a11y/ariaat/fetch.sh` fetches them again.
### Content checks

Some barriers are in the words and images a page's authors write: a link called "read more", alt text that is a
file name, or bold text standing in for a heading. Add `--content` for ten checks on them. They
run in the browser with no model call, and only on what a visitor can see.

| Rule ID | Reports | Related to |
|---|---|---|
| `dummyrun-content-alt-file-name` | An image whose alt text is a file name, such as `IMG_2041.jpg` or `hero-banner.png` | WCAG 1.1.1 |
| `dummyrun-content-vague-link` | A link whose whole name is a phrase such as "click here", "read more" or "learn more" | WCAG 2.4.4 |
| `dummyrun-content-fake-heading` | A short paragraph set entirely in bold, with no full stop, followed by a paragraph, list or table | WCAG 1.3.1 |
| `dummyrun-content-document-link` | A link to a PDF, Word, Excel, PowerPoint, OpenDocument, RTF or CSV file whose name doesn't say that kind of file. "Download" isn't enough, and nor is the wrong kind | GOV.UK content design |
| `dummyrun-content-new-window` | A link that opens a new tab and whose name doesn't say "new tab" or "new window" | WCAG 3.2.5 |
| `dummyrun-content-url-link` | A link whose name is a web address of more than 30 characters | WCAG 2.4.4 |
| `dummyrun-content-alt-redundant` | Alt text that starts "image of", "picture of" or "photo of" | W3C images tutorial |
| `dummyrun-content-alt-long` | Alt text of more than 150 characters | W3C images tutorial |
| `dummyrun-content-all-caps` | A block of text with more than 30 letters, all in capitals, typed or set by `text-transform` | GOV.UK style guide |
| `dummyrun-content-underline` | Underlined text that isn't a link, a button, an abbreviation or inserted text | WCAG 1.4.1 |

Each of these usually makes a page harder to use, but each can be right in context. A "read more" link inside a
sentence about the story it opens can meet WCAG 2.4.4, for example. So every result is advice, never fails the
sweep, and the allow-list can accept it. A link's name is its `aria-labelledby` or `aria-label`, or else its text and
the `alt` of its images and the `aria-label` of its icons, leaving out anything a screen reader skips, so a link that adds words for screen readers isn't reported. A `title` doesn't count, since few people see it and few screen readers read it. The checks that match
English phrases run only on a page whose language is English or isn't set. The checks on file names, addresses,
length, bold, capitals and underlines run on every page. When the checks finish on a page, its JSON result has
`content_checked: true`.

### Other rule engines

Each rule engine catches things the others miss, and each has its own false alarms. Add `--engines` to run four
more beside axe-core on every page: IBM Equal Access, HTML_CodeSniffer, Siteimprove Alfa and QualWeb. QualWeb
brings two sets, its ACT rules and its WCAG techniques. Its one ACT rule that needs the window resized
(QW-ACT-R40, text clipped when zoomed) is left out, so the window you chose never changes. The two sets run
apart, so if one fails the other's results are kept, and `engine_errors` says which one failed. They all run
inside the page, in a sandbox of their own, so the page's scripts can't see them or change them. Only their WCAG 2.2 A and AA
failures are kept. Their warnings, AAA rules and "check this by hand" results are left out. A contrast
failure (1.4.3) on text no one can see is left out too. That's text pushed off the page with `text-indent`, text
set at `font-size: 0` and text that its element or a parent clips away by hiding overflow. A search button's
hidden label is one, and some engines still measure it. Text that spills out of its element but stays on the page
is still checked, and so is text a scrolling box holds out of view, since you can scroll to it. A parent only clips what it contains, so fixed or absolutely placed text that sits outside it is still checked too. Fixed text set past the right or bottom edge of the window is left out, since no scrolling brings it into view.

The sweep then matches results by element and WCAG criterion. Two engines agree when they fail the same element
for the same criterion.

- Each axe violation node gets `engines`, the engines that fail the same element for one criterion axe's rule maps
  to. When the rule maps to more than one criterion, the criterion with the most engines counts. Its `confidence`
  is rated again with those engines, as [REPORT.md](REPORT.md) describes. An axe violation still fails the sweep
  either way.
- A failure axe doesn't report, but two of the other engines agree on, is a violation with the rule ID
  `dummyrun-engines-` and the criterion, such as `dummyrun-engines-1.1.1`. Its impact is serious. It fails the
  sweep and the allow-list can accept it. Its `detail` names each engine's rule, with the engine's message where it
  gives one. Alfa gives none, so its rule ID stands alone.
- A failure only one engine reports goes to the page's `candidates` list and the summary's `candidate_nodes`. It
  never fails the sweep. Look at the element before you change anything.

Every failing element in a sweep carries a `confidence` of `high`, `medium`, `low` or `unknown`, with or without
`--engines`. Each rule says what kind of `evidence` it is: `engine-rule` for axe and the other engines,
`measurement` for the layout and keyboard checks and `ax-fact` for the markup checks. `dummyrun-mouse-only` is a
`task-verdict`, because the sweep tried the control by keyboard and it did not work.

Each result has `engines_ran`. If an engine fails to load or throws on a page, its message goes to `engine_errors`.
The other engines still run, and the summary lists the error. IBM Equal Access, HTML_CodeSniffer and Alfa each have 25
seconds on a page, and one that takes longer is an error in the same way. If none of them runs, the page has an `error` and the sweep
fails, so it never passes on axe alone when you asked for more. A page keeps up to 200 groups of the other engines'
failures, the ones more engines agree on first, and `engine_groups_omitted` counts any left out. The report's
`engines` field gives the version of each engine.

```sh
dummyrun a11y --engines --json report.json https://example.com/
```

[Tools we build on](TOOLS.md) lists each engine with its licence.

### Colours that meet the ratio

When axe fails text on `color-contrast`, it records the text colour, the background
colour, the ratio between them and the ratio the text needs. The sweep keeps those in the element's `colours` and
works out two colours that would meet the ratio:

- `text_fix`, the nearest text colour that meets it on the same background
- `background_fix`, the nearest background that meets it with the same text

Each one changes how light the colour is, in the OKLCH colour space, and keeps its hue and, where it can, its
chroma. So a brand blue stays blue and only gets darker or lighter. The sweep tries darker and lighter and keeps the
smaller change. If no lightness at that hue meets the ratio, that suggestion is left out. The element's `detail` says
the same in words, for example "#777777 on #ffffff is 4.48:1, and needs 4.5:1. Text #767676 on the same background
meets it, or the same text on #060606."

A suggestion is a starting point. Check it against the rest of your design, and against the other colour scheme if
your site has both. Axe measures only text on a flat colour, so text on an image or a gradient has no suggestion.
Use `--image-contrast` for that.

### Contrast over images

axe-core can't judge the contrast of text that sits on a photo or a gradient, so it lists that text under
`needs_review`. Add `--image-contrast` and the sweep measures it instead. For each element axe left for review, up to
20 tries on a page, it hides the text, takes a picture of the box around the words and compares the text colour with
every pixel behind them. The text colour is the one the browser draws, its fill colour if the page sets one. The box
is the text's own, so padding and empty space beside a short label don't count. It runs after any cookie banner has
been answered, so the banner isn't in the picture.

A finding, `dummyrun-contrast-over-image` (WCAG 1.4.3, serious), means most of the text sits on too little contrast.
The median of the contrast ratios between the text colour and each pixel behind it is below 4.5:1, or 3:1 for large
text. The figures are rounded down to two places, so a ratio just under the threshold never shows as meeting it. The `detail` gives that figure and the ratio at the worst tenth
of the pixels, so you can see how much of the text is affected. Text that passes at the median and fails only in its
worst tenth isn't reported. When the check finishes on a result, the result has `image_contrast_checked`. If it fails, the result has an `error` instead. If the check
measured anything, `image_contrast` lists every measurement.

`image_contrast_unmeasured` counts the elements it couldn't measure, so check those by eye:

- elements inside a shadow root or a frame
- text in more than one colour, or that mixes large and smaller text
- text faded with opacity
- text with a shadow, since a halo can give the contrast and the picture wouldn't show it
- text on a page whose content security policy blocks the style that hides it
- elements it couldn't find or take a picture of, and any past the limit

The picture is taken as the page draws it, so a slideshow that moves on or a video behind the text can give a
different answer next time. Check a finding by eye before you change the design.

```sh
dummyrun a11y --consent --image-contrast https://example.com/
```

### Plain language

Long sentences and vague words make a page harder for everyone, and hardest for people who read slowly, read in a
second language or listen to a screen reader. Add `--plain-language` and the sweep reads each page's main content
with [Vale](https://vale.sh), a prose checker that runs on your machine. The main content is the `main` landmark,
the region a page marks as its main content, and the first one that is displayed when the page has more than one. On a page without one, it's the body. The sweep then leaves out the
body's header, navigation, footer and asides. Code, scripts and hidden text are left out, including text the
page's CSS hides with `display`, `visibility` or `opacity`.

| Rule ID | Reports | Based on |
|---|---|---|
| `dummyrun-plain-reading-ease` | A Flesch reading ease score below 70 for the page, with the score | Vale's Readability style |
| `dummyrun-plain-reading-grade` | A Flesch-Kincaid grade above 8 for the page, with the grade | Vale's Readability style |
| `dummyrun-plain-long-sentence` | Each sentence of more than 25 words, by its first word | The GOV.UK style guide |
| `dummyrun-plain-words-to-avoid` | Words such as "leverage", "facilitate" and "utilise", with a plainer word | The GOV.UK style guide |
| `dummyrun-plain-latin` | "e.g.", "i.e." and "etc.", with the English to use instead | The GOV.UK style guide |

The scores count syllables by rule, so treat them as a guide rather than a verdict. The rules from the GOV.UK
style guide are our own wording of its public advice. They leave out words the guide allows in some senses, such
as "deliver" for post. Every result goes to `advice`, because plain language at this level isn't a WCAG A or AA
requirement, so it never fails the sweep. A page that reaches the check has `plain_language_checked`, or
`plain_language_error` when the check couldn't complete, such as when Vale won't run. A page that fails earlier,
such as when it won't load, has neither. A main landmark the page hides completely has nothing to check. Vale reads
the first 200,000 characters of the main content's HTML. When there's more, `plain_language_partial` is true.

The sweep looks for `vale` on your `PATH`, or the program `DUMMYRUN_VALE` names. The container image includes Vale
3.24.0. Without Vale, the rest of the sweep still runs and each page says why it wasn't checked.

```sh
dummyrun a11y --plain-language https://example.com/
```

### HTML errors that change what assistive technology gets

Browsers forgive broken HTML, but they repair it in ways you didn't write, and some mistakes reach assistive
technology directly. Two elements with the same ID can send a label to the wrong field. A link inside a button may
be announced as one, the other or neither. Add `--html` and the sweep checks each page with the
[Nu HTML Checker](https://validator.github.io/validator/) (vnu), the checker behind the W3C's own validator. It
runs on your machine as a separate program.

The sweep checks two copies of each page in one vnu run:

- the HTML as the server sent it, fetched again from the page with the page's own cookies, which is where parse
  errors such as tags closed in the wrong order are;
- the page as the browser built it, with what scripts added, which is where a duplicate ID from a template is.

An error both copies have is reported once. vnu stops reading a document after some errors, such as a `div` inside
a `table`, so the second copy often finds what the first one couldn't.

Only errors that change what assistive technology gets are kept:

| Rule ID | Reports |
|---|---|
| `dummyrun-html-broken-reference` | An `aria-labelledby`, `aria-describedby` or label `for` that points at no element that can take it |
| `dummyrun-html-duplicate-id` | Two elements with the same ID |
| `dummyrun-html-nested-interactive` | A link, button or other control inside another one |
| `dummyrun-html-aria` | A role or `aria-*` attribute the element can't have, or a value it can't take |
| `dummyrun-html-structure` | A list, table or select with children that don't belong, or tags the browser had to rearrange |
| `dummyrun-html-img-alt` | An image with no `alt` attribute |

Everything else vnu reports, such as an obsolete `center` element or a stray end tag the browser ignores, is
counted in `html_ignored` and left out. Every result goes to `advice` and never fails the sweep. WCAG 2.2 dropped
parsing (4.1.1) as a criterion, and a validator can't tell whether the error also fails another one. axe-core and
the other engines report many of the same problems as WCAG failures when they do.

The HTML is checked once for each URL, on the first result for the URL that axe could check: the first window size
and colour scheme you set, desktop and light by default. Scripts can change the HTML at other sizes and schemes, and
vnu doesn't see those changes. That result has `html_checked` when vnu finished, and `html_source_checked` when the
source could be fetched again and wasn't empty. It has `html_error` instead when the check couldn't finish: vnu wasn't found,
the browser's copy of the HTML couldn't be read, vnu failed or took more than 30 seconds, or its output couldn't be
read. vnu reads the first 2 MB of each copy, and `html_partial` is true when there was more. When vnu hits a fatal
error, such as bytes that aren't valid in the page's encoding, it stops reading that copy, and `html_stopped` is
true. An error found in both copies is listed once, and the same error twice in one copy is listed twice. A source line number in a finding counts lines of the HTML as the server sent it.

The sweep looks for `vnu` on your `PATH`, or the program `DUMMYRUN_VNU` names. The container image includes the
Nu HTML Checker 26.10.12, on a Java runtime cut down to the modules it needs. Without vnu, the rest of the sweep
still runs and each page says why it wasn't checked. Each check starts Java, which takes a second or two.

```sh
dummyrun a11y --html https://example.com/
```

### Lighthouse

Add `--lighthouse` and the sweep runs Google [Lighthouse](https://github.com/GoogleChrome/lighthouse)'s
accessibility and best-practice audits once for each URL. It uses the Chrome the sweep already has open, through
the DevTools address chromedriver reports. Lighthouse opens its own tab there and keeps the browser's cookies, so a
signed-in sweep stays signed in. Throttling is off and the window is the browser's own. Lighthouse sends its own
user agent, which names Lighthouse. A run takes about 20 seconds, so it's done once for each URL, whatever the window
sizes and schemes.

Lighthouse's accessibility audits are axe-core rules, run by Lighthouse's own copy of axe-core. Lighthouse never
counts towards the other rule engines' agreement.

- Where Lighthouse fails an element that our axe also failed, for the same rule, the node gets `lighthouse: true`.
  `lighthouse_agreed` counts these nodes. The two can differ when the page changes between loads, or because
  Lighthouse ships a newer axe-core.
- An accessibility audit that fails where our axe found nothing goes to `candidates`, with the rule ID
  `dummyrun-lighthouse-` and the audit, such as `dummyrun-lighthouse-image-alt`. It counts in `candidate_nodes` and never
  fails the sweep.
- A best-practice audit that fails, such as `paste-preventing-inputs` or `charset`, goes to `advice` with the same
  kind of rule ID. Where it's about the page rather than an element, such as a deprecated API a script called, each
  problem is listed on `html` with Lighthouse's message and the script and line it came from.

The first result for each URL has `lighthouse_checked`, the two category scores from 0 to 1 in `lighthouse_scores`,
and `lighthouse_error` when Lighthouse couldn't run. When its elements can't be matched to axe's,
`lighthouse_match_error` says why, and its failures on single elements are left out, since some would repeat axe's.
Its page-wide failures and best-practice advice are kept. Lighthouse runs only in Chrome. In Firefox each page says
so. The report's `lighthouse` field names the version once Lighthouse has audited a page.

The sweep looks for `lighthouse` on your `PATH`, or the program `DUMMYRUN_LIGHTHOUSE` names. Install it with
`npm install -g lighthouse`. Lighthouse 13.5.0, which the sweep is written against, asks for Node.js 22.19 or
later. The container image doesn't include it, since it would need Node.js as well.

```sh
npm install -g lighthouse
dummyrun a11y --lighthouse https://example.com/
```

### Journeys

Add `--journeys` and the sweep finds the journeys each page offers and tries each one by mouse and by keyboard. It
finds them in the page itself, with no model:

| Journey | How it's found | It worked when |
|---|---|---|
| `search` | A search field: `type=search`, a field in a search form or landmark, or one named `q`, `s`, `query` or `search` | The words typed appear in the field. Enter is never pressed |
| `menu` | A menu or navigation button in the header or `nav` with `aria-expanded="false"` | It reports expanded, or the menu it controls shows its links |
| `signup` | A link or button labelled sign up, register, create an account or join | The page it leads to has an email or password field |
| `basket` | A link labelled basket, cart, bag or trolley, or to `/cart`, `/basket` or `/bag` | The page it leads to has basket, cart, bag or trolley in its title or main heading |
| `booking` | A link or button labelled book, reserve, reservation or appointment | The page it leads to, or a dialog it opens, has a date or time field, a calendar or a booking widget |
| `contact` | A link labelled contact or get in touch, or to `/contact` | The page it leads to has a form with an email field or a message box, and each of its fields takes text |

Every journey has a success test that must be false when the page loads and true after the action. A journey whose
test is already true is `already` and proves nothing. Each way of trying starts from a fresh load. The mouse clicks
the journey's start, after checking nothing covers it. The keyboard presses Tab from the top of the page until the
start has focus, up to 250 times, then presses Enter or types. The contact journey then fills each field the same
way, Tab and type or click and type.

Nothing is ever sent. Before each try the sweep stops every form on the page from submitting, Enter is never
pressed in a form, and a journey never starts from a submit button. The journeys run in a tab of their own, once
for each URL, at the first window size, after the page's other checks and the cookie banner.

Schema.org data the page publishes marks where a journey came from, but doesn't add one: when the page has a search
field and a `SearchAction`, or a booking link and a `ReserveAction`, that journey's `source` is `schema.org`. What the
page is built with (see [What the site is built with](#what-the-site-is-built-with)) can suggest a journey the page
has no element for: Shopify suggests a basket, Calendly a booking. A journey a technology suggests that the page
shows no way into is listed as `not found`, which says where to look.

A basket journey never starts from an "add to basket" link, or from a link whose address adds to the basket, since
following it would change the basket. The journeys' tab sends nothing but GET and HEAD requests: the browser fails
any other request before it leaves, whatever the page's scripts try. With `--respect-robots`, a journey whose link
leads to a page robots.txt disallows isn't tried and is `skipped`, and the tab loads no page robots.txt disallows,
in a frame or by a redirect either. A sweep that is signed in, with a session or preview access, tries no
journeys, for the same reason it tries no controls: a click could change the account or sign it out.

Each result lists its `journeys`. A journey names its `kind`, its `selector`, `label` and `href`, and its `source`.
It has a `mouse` and a `keyboard` verdict, each one of `pass`, `fail`, `unreachable`, `already`, `not found`,
`skipped` or `error`, with a `detail`. `error` means the browser couldn't load the page or carry out the action, or the journeys
ran out of time, so the page wasn't tested that way. A journey that passes by mouse, and fails or is unreachable by
keyboard, is a violation, `dummyrun-journey-keyboard` (WCAG 2.1.1, serious). One that fails both ways is recorded and
isn't reported: the page may simply not work the way the journey expects.

```sh
dummyrun a11y --journeys https://example.com/
```

### Broken links

Add `--links` and the sweep checks the links on each page with [lychee](https://lychee.cli.rs/), a link checker
that runs on your machine as its own program. Every `http` and `https` link in an `a` or `area` element counts,
without its `#fragment`, up to 300 on a page. `links_omitted` counts any past that. Each link is checked once across
the sweep, so a menu on every page costs one check per link. Each URL's links are checked once too, whatever the
window sizes and schemes.

The sweep reports a broken link as advice with the rule ID `dummyrun-broken-link`. A link is broken when it answers
with an error status such as 404 or 500, or can't be reached at all. Skipped, refused and timed-out links aren't
broken. Its detail gives the URL and the status code when there is one, such as
`https://example.com/old: HTTP 404 (Rejected status code: 404 Not Found)`. It never fails the sweep. Some sites
turn automated checkers away with 401, 403, 429 or 999 while the page works in a browser. Those count in
`links_refused`. A link that doesn't answer in time counts in `links_timed_out`: lychee waits 20 seconds, and the
browser, for links behind preview access, waits 15. `links_checked` counts every link looked at. That includes the
skipped ones: links to private addresses, links robots.txt disallows and links lychee leaves out, such as a
`.invalid` host. `links_error` says why the check couldn't run.

lychee sends the sweep's user agent, `DummyRun/<version> (+https://dummyrun.dev/scanner/)` with `--identify`. It
never gets your preview credentials. With [preview access](#protected-previews) on, links on the sites you're
sweeping, the same scheme and host, are checked from inside the browser instead, which already holds them, and
lychee only gets the rest. The browser counts a redirect as an answer and doesn't follow it.

A page decides where its links point, so a link to `localhost`, to `169.254.169.254` or to another address on a
private network would have the checker visit the machine it runs on or the network around it. Those links are
skipped, both by the sweep, which looks up each host first, and by lychee with `--exclude-all-private`. They're
checked only when every URL you sweep is itself on `localhost` or a private address, as when you sweep a site on
your own machine. With `--respect-robots`, a link to a page that robots.txt disallows is skipped too, and isn't
fetched. The sweep looks for `lychee` on your `PATH`, or the program `DUMMYRUN_LYCHEE` names. The container
image includes lychee 0.24.2. Without it, the rest of the sweep still runs and each page says why its links weren't
checked.

```sh
dummyrun a11y --links https://example.com/
```

### Cookie banners

A cookie banner covers part of the page, so it can turn up in the layout checks as overlap and cut-off text that
your visitors only see until they choose. Add `--consent` and the sweep answers the banner the way a careful
visitor would, by rejecting. It uses DuckDuckGo autoconsent, which knows over a thousand consent platforms and
falls back on reading the banner's own words.

The rule engines see the banner first, because its own accessibility matters: it's the first thing a visitor has
to get past. Then autoconsent answers it, and the layout checks measure the page underneath.

Before anything is clicked, autoconsent sorts each button on the banner into one of five kinds. These are reject,
accept, settings, acknowledge and other. A banner with an accept button and no reject button is reported as
`dummyrun-consent-no-reject`. Rejecting through a settings page takes more steps than accepting. It also takes more
reading and more precise clicks. That weighs most on people who tire easily, use a switch or read slowly. The finding
goes to `advice` because it isn't a WCAG failure, so it never fails the sweep.

A banner that turns up while autoconsent is still looking, in about the first three seconds, is read and answered
too. When autoconsent reports it found nothing, the sweep moves on at once, so a banner that appears after that isn't
answered. Otherwise the sweep waits up to 10 seconds for autoconsent to finish before it moves on. If autoconsent is still working when that time is up,
`settled` is false and it could still click something during the checks that follow. If autoconsent can't start
on the page at all, `failed` is true and the page has an `error`, so the sweep fails.

Each result has `consent`, which records:

- `popups`, the banners found, with each button's text and kind;
- `cmp`, the consent platform recognised;
- `opted_out`, true when autoconsent reports that it rejected;
- `cleared`, true when every banner it found is gone or out of the window afterwards, measured in the page, and
  false otherwise;
- `settled`, true when autoconsent had finished when the sweep moved on. When it's false, autoconsent could still
  act on the page later;
- `failed`, true when autoconsent couldn't start or read the page, with the reason in `error`;
- `steps`, autoconsent's own messages in order.

A banner inside a frame is answered but not read, so it has no buttons in `popups`. Rejecting sets cookies in the
sweep's browser, so later settings for the same site may show no banner.

```sh
dummyrun a11y --consent --layout https://example.com/
```

### Check the sweep against known problems

The benchmark in `internal/benchmark` measures how many known problems the sweep finds. It plants known barriers in
clean pages and counts the ones the sweep reports. Each planted barrier is one change a visitor would meet: a form field
with no label, an image with no text alternative, a focus outline removed, a skip link that goes nowhere, or a page that
traps the Tab key. The clean pages are in `testdata/benchmark/pages`.

Each barrier is planted in every clean page it fits. For each of those pairs the benchmark runs the sweep twice, on the
clean page and on the changed copy. A barrier counts as found when the copy shows one of its rules and the clean page
did not, so a rule that fires on the clean page earns nothing. The result is recall for each check: how many of the
barriers planted for it the sweep found, and which ones it missed. A barrier that lists several rules is found by
any of them, and counts toward the first rule listed. A pair the browser could not run is counted
separately, left out of recall, and fails the test.

```sh
DUMMYRUN_BROWSERS=chrome go test ./internal/benchmark -run TestBenchmark -v
```

Set `DUMMYRUN_BENCHMARK_OUT` to a file name to keep the scores as JSON. With more than one browser, each gets its own
file, with the browser's name before the extension. The browsers run one after another, so only one is open at a time.

The same run also tests rule by rule against real markup. `testdata/benchmark/accessguru/cases.json` holds 150 elements
that axe-core reported on real pages, taken from the AccessGuru dataset (MIT), each with the rule that failed. The
benchmark puts each element alone in a small clean page and checks that the sweep reports the rule. An element comes
without its page, so a miss can mean the rule needs the surrounding page, such as a required parent. Look at each miss before you
decide the check is weak.

To plant a new kind of barrier, add a `Defect` to `internal/benchmark/inject.go` with the rule IDs that should find it.
A test checks that every defect fits at least one of the clean pages.

## Troubleshooting

Leftover browsers fill the disk and the process table. To list the browsers and profile directories that
earlier runs left behind, run:

```sh
dummyrun doctor
```

It exits 1 if it finds any. Add `--stop` to stop them and remove the profile directories. Check first that no
run is in progress, because a browser that belongs to a live run is listed too. Without the command, this lists
the processes:

```sh
ps -axo pid=,args= | grep -E 'dummyrun-(chrome|firefox)-' | grep -v grep
```

The engine only removes directories it made itself, in the temporary directory, with those prefixes.
