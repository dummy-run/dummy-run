# The audit report

`dummyrun report` turns your run records into a report you can hand to someone who wasn't there. Add the
results of `dummyrun a11y` and it covers your pages as well as your personas. The report is a sample, so read
[the limits](#the-limits) before you share it.

![The top of an example report: title, summary, severity counts and the start of the scope](images/report-example.png)

## Make a report

```sh
dummyrun report --runs examples/runs --build a865ffd \
  --a11y examples/report/a11y-sweep.json --include pending \
  --title "Inkwell preview audit" --client "Inkwell Press" --out report.html
```

You get a single HTML file with its styles and screenshots inside it. It loads nothing from the network. Open
it in a browser, or print it to PDF. When you print, the contents list is dropped and the browser is asked not
to split a finding across pages.

| Flag | What it does |
|---|---|
| `--runs DIR` | A directory or a file of run records. Repeat the flag to read several. Paths can also follow the flags |
| `--a11y FILE` | A file from `dummyrun a11y --json`. Repeat the flag for several sweeps |
| `--title`, `--client` | The title, and free text naming who the report is for |
| `--format` | `html` (the default), `md`, `json`, `sarif`, `junit`, `badge` or `shields`. [BADGES.md](BADGES.md) covers the last two |
| `--include pending` | Also include findings no reviewer has confirmed. Without it you only see confirmed findings |
| `--baseline FILE` | An earlier report in JSON, so each finding is marked new or still present |
| `--build SHA` | Report on one commit or build hash, at least 7 characters long |
| `--date` | The report date as `YYYY-MM-DD`. The default is today |
| `--out FILE` | Write to a file instead of standard output |

Rejected findings never appear, in any format. Neither do observations, which are notes about things that
worked.

## What is in it

1. A plain-language summary of how many problems were found, how many a person confirmed and how many
   nobody has reviewed yet.
2. Scope and method. This names the target, the build and the dates. It lists the personas, the dimensions
   they varied and the scenarios. It also lists the browsers, the models and providers, the number of runs and
   what the model calls cost.
3. Coverage, using the same numbers as [dummyrun coverage](COVERAGE.md).
4. Findings, worst first.
5. What worked, when the runs hold observations. These are a model's notes that no person has checked. They
   aren't findings and never enter a count.
6. The axe sweep, if you gave one.
7. A comparison with the baseline, if you gave one.
8. What the report is not.

## Coverage

The coverage section tells you how much the sample can say. The line to read first is "still unfound". It's
the Chao2 lower bound on all distinct problems, minus the number already seen. Treat it as a floor. It never
counts what remains, and the form of the estimate is named beside it.

Sample coverage is the share of the chance of finding a problem that sits in problems already found. Below
0.50, don't extrapolate from it. When a number can't be estimated, the report says so instead of guessing. If
your records come from more than one build, coverage isn't computed and the report tells you why. Pass
`--build` to pick one.

[COVERAGE.md](COVERAGE.md) explains each number and when to trust it.

## How to read a finding

Findings are grouped by fingerprint, so a problem that three runs found appears once. They're sorted by
severity, then by how many runs found them.

Each finding shows:

- the title, severity and kind, as the persona and the reviewer left them
- the symptom, the route and the control or region, which together make the fingerprint
- the review status, with the reviewer, date and reason when the record has them
- the WCAG 2.2 success criterion, when it's known
- fix guidance, when there is some (for an axe rule, a link to the rule's help page)
- steps to reproduce, taken from the steps the finding cites
- the run and persona that found it, with the role, device and browser
- screenshots of the cited steps

A persona finding only gets a WCAG criterion when its symptom names an axe rule that appears in a sweep you
gave. Otherwise the report says it isn't recorded. Screenshots are embedded when the files sit next to the
record. If one is missing, the report names it and still builds.

A finding with a `pending` verdict hasn't been reviewed, so nobody has checked the model's claim against the
evidence. The HTML marks it with a dashed badge and the words "Not reviewed". Read those findings as leads.

When several runs found a problem and only some sightings were confirmed, the problem counts as confirmed. The
list of runs marks the sightings nobody has reviewed.

## The axe sweep

The sweep section folds every page and setting into one entry per rule. Each entry shows the impact and the
WCAG criteria, plus the pages, elements and settings where the rule failed. Elements on your allow-list are
counted and left out. Rules axe can't decide are listed separately under "needs a person to check", and so are
pages that couldn't be tested.

## Compare with an earlier report

To see what changed since last time, write the earlier report as JSON and pass it back with `--baseline`:

```sh
dummyrun report --runs runs/week-1 --format json --out week-1.json
dummyrun report --runs runs/week-2 --baseline week-1.json --out week-2.html
```

Findings are matched by fingerprint, and axe results by rule. Each is marked new or still present. A problem
the baseline had that the new runs didn't find again is listed as fixed. Axe rules only appear in the
comparison when the new report has a sweep that tested every page, and persona findings only when it has runs.
A sample can miss a problem that's still there, so check a fixed problem before you close it.

## Other formats

| Format | Use |
|---|---|
| `md` | The same report in Markdown, for a pull request or a wiki. Screenshots are links to the files |
| `json` | The full data, described by [schema/report.schema.json](../schema/report.schema.json). It's what `--baseline` reads |
| `sarif` | SARIF 2.1.0 for tools that read it. Each problem and each failing axe element is a result. The location is a page address rather than a file in your repository, so GitHub code scanning can't place a result on a source file |
| `junit` | JUnit XML for a CI job. Blocker, major and minor problems and failing axe rules are failures. Info problems are skipped |

## The limits

The report prints these on its last page.

- It's a sample. Personas played a set of scenarios on the builds listed under scope. A problem they didn't
  meet isn't in it.
- It isn't a WCAG conformance claim, a certificate or a statement about the law.
- Automated checks find only some accessibility problems. The sweep covers the pages it lists and nothing else.
- It doesn't replace testing with disabled people.
- A finding marked not reviewed is a model's claim that no person has checked.
- Parts of the app no run visited aren't covered, whether or not the report has findings.

The report never says the target passed or is compliant. If the runs found nothing, it says so, and adds that
this tells you little on its own.

## Example

[examples/report/](../examples/report/) holds a report built from the synthetic runs in
[examples/runs/](../examples/runs/) and a synthetic sweep. Those records have no screenshot files, so the HTML
names the missing ones.
