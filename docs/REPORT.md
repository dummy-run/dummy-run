# The audit report

`dummyrun report` turns run records into a report for someone who was not there. Add the results of `dummyrun a11y` and the report covers pages as well as personas. The report is a sample, so read the limits at the end before you share it.

![The cover of an example report with a logo, a title and a bar of problems by severity](images/report-example.png)

## Make a report

```sh
dummyrun report --runs examples/runs --build a865ffd \
  --a11y examples/report/a11y-sweep.json --include pending \
  --title "Inkwell preview audit" --client "Inkwell Press" --out report.html
```

The default format is one HTML file. It holds its own styles and screenshots and loads nothing from the network. Open it in a browser or print it to PDF. The print styles start each section on a new page, keep a finding in one piece where it fits, and put the title and page numbers in the page margins in browsers that support it.

| Flag | What it does |
|---|---|
| `--runs DIR` | A directory or a file of run records. Repeat the flag to read several. Paths may also follow the flags |
| `--a11y FILE` | A file from `dummyrun a11y --json`. Repeat the flag for several sweeps |
| `--title`, `--client` | The title, and free text naming who the report is for |
| `--format` | `html` (the default), `md`, `json`, `sarif`, `junit`, `badge` or `shields`. The last two are described in [BADGES.md](BADGES.md) |
| `--include pending` | Also include findings no reviewer has confirmed. Without it only confirmed findings appear |
| `--baseline FILE` | An earlier report in JSON form, to mark each finding new or still present |
| `--build SHA` | Report on one commit or build hash. At least 7 characters |
| `--date` | The report date as `YYYY-MM-DD`. The default is today |
| `--out FILE` | Write to a file instead of standard output |

Rejected findings never appear in any format. Observations, which are notes that are not problems, do not appear either.

## What is in your report

The HTML report is built to be read, handed on and printed. It uses the Dummy Run typefaces, Space Grotesk and Atkinson Hyperlegible Next, which are embedded in the file. It has a light theme for print and screen, and a dark one on screen only when your system asks for it. The example is `examples/report/report.html`, and it is about 150 KB with its fonts, logo and one screenshot.

**The cover.** It has the logo, the title and who the report is for (`--title` and `--client`). Under them are the target, the dates of the runs, the report date and which findings are counted. A short paragraph says in plain words what was run and what it found. A bar and four labelled chips count the problems by severity. The severity is always written out, so no one has to tell the colours apart. The cover leaves room for an agency logo: put it inside the element with the class `cover-brand` in your copy.

**The summary.** Cards give the key numbers. They cover problems found, runs, personas and routes visited. They also cover the pages in the axe sweep and, with a baseline, what is new. "How sure you can be" says in plain words how many problems are still unfound, as a floor and not a count. The page also says what was tested and what worked. It ends with a short reminder that the report is a sample.

![The summary page: key number cards and how sure you can be](images/report-summary.png)

**The findings.** A contents list links to every finding, grouped by severity. Each finding is a card with a colour band and two badges for severity and review. It says where the problem is as a person would, for example "the Payments page, the Rates link". It says what went wrong in plain words and who hit it. The steps to reproduce read as things to do, such as signing in as a contributor and opening a page. The screenshot has a caption, and for a permission or security finding a line says what to look at. Rows with nothing to say are left out. A collapsed "Technical details" block holds the run, the steps, the fingerprint and the other raw fields. It is open in print.

![One finding card with its badges, steps and screenshot](images/report-finding.png)

**The axe sweep and layout checks.** One table lists each rule with its impact, WCAG criterion, the number of elements and a link to the fix guidance. A second shows where each rule failed. With `--baseline`, each finding and rule is labelled new or still present, and a section lists what was not seen again.

**On paper.** Print to PDF from the browser. Each section starts on a new page, and the margins carry the report title, who it is for and the page number.

![The first three printed pages](images/report-print.png)

The pictures are rebuilt by `examples/report/screenshot.sh`. The HTML makes no request to the network, and a test checks that every URL in it is a data URI. The Markdown format has the same parts as headings and tables.

## What is in it

1. The summary. It is plain language and says how many problems were found, how many a person confirmed and how many are not reviewed.
2. Scope and method. It names the target, the build and the dates. It lists the personas with the dimensions they varied, and the scenarios. It lists the browsers, the models and providers, the number of runs and the model cost.
3. Coverage. It reuses the numbers of [dummyrun coverage](COVERAGE.md).
4. Findings, worst first.
5. What worked, when the runs hold observations. These are notes by a model that no person has checked. They are not findings and never enter a count (code: `buildWorked` in `internal/report/build.go`).
6. The axe sweep, when you gave one.
7. A comparison with the baseline, when you gave one.
8. What the report is not.

## Coverage

The coverage section says how much the sample can tell you. The line to read is "still unfound". It is the Chao2 lower bound on all distinct problems, less the number seen. It is a floor, never a count of what remains. The form of the estimate is named next to it.

Sample coverage is the share of the chance of finding a problem that sits in problems already found. Below 0.50 do not extrapolate from it. When the numbers cannot be estimated the report says so and does not print a guess. When the records come from more than one build, coverage is not computed and the report says why. Pass `--build` to pick one.

[COVERAGE.md](COVERAGE.md) explains each number and when to trust it.

## How to read a finding

Findings are grouped by fingerprint, so a problem that three runs found appears once. The list is sorted by severity and then by how many runs found the problem.

Each finding has these parts.

- Title, severity and kind, as the persona and the reviewer left them.
- Symptom, route and the control or region. These make up the fingerprint.
- Review status. A confirmed finding shows the reviewer, the date and the reason when the record has them. A finding without a verdict is marked not reviewed.
- The WCAG 2.2 success criterion when it is known. A persona finding has one only when its symptom names an axe rule that appears in a sweep you gave. Otherwise the report says it is not recorded.
- Fix guidance when there is some. For an axe rule it is the link to the rule's help page.
- Steps to reproduce, taken from the steps the finding cites. The report names the run and the persona. It also gives the role, the device and the browser.
- Screenshots of the cited steps, embedded in the HTML when the files are next to the record. A file that is missing is named and the report still builds.

A finding is not reviewed when its verdict is `pending`. Nobody has checked the model's claim against the evidence. The HTML marks it with a dashed badge and the words "Not reviewed". Read those findings as leads.

When a problem was found in several runs and only some were confirmed, the problem counts as confirmed. The list of runs marks the sightings that are not reviewed.

## The axe sweep

The sweep section folds the results of every page and setting into one entry for each rule. It shows the impact and the WCAG criteria. It lists the pages and elements and the settings in which the rule failed. Elements on the allow-list are counted and left out. Rules that axe cannot decide are listed apart under "needs a person to check". Pages that could not be tested are listed too.

When the sweep ran with `--layout` or `--text-scale`, its layout rules appear in the same section, because they are in the sweep file as violations. The rule IDs all begin `dummyrun-`. They map to WCAG 1.4.10, 1.4.4 and 2.5.8. Each is a measurement made in the browser, and the help link goes to the W3C page for the criterion. The settings of a rule end in `/text-200` when the sweep used a text size of 200 percent, for example `chrome/phone/light/text-200`. Persona runs at a larger text size are listed with the device, as in "phone with text at 200 percent". [ENGINE.md](ENGINE.md) explains the checks.

## Compare with an earlier report

Write the earlier report as JSON and give it back with `--baseline`.

```sh
dummyrun report --runs runs/week-1 --format json --out week-1.json
dummyrun report --runs runs/week-2 --baseline week-1.json --out week-2.html
```

Findings are matched by fingerprint, and axe rules by rule. Each is marked new or still present. A problem that the baseline had and the new report did not find again is listed as fixed. Axe rules are listed only when the new report has a sweep that tested every page, and persona findings only when it has runs. A sample can miss a problem that is still there, so check a fixed problem before you close it.

## Other formats

| Format | Use |
|---|---|
| `md` | The same report as Markdown, for a pull request or a wiki. Screenshots are links to the files |
| `json` | The full data, described by [schema/report.schema.json](../schema/report.schema.json). It is the input for `--baseline` |
| `sarif` | SARIF 2.1.0 for tools that read it. Each problem and each failing axe element is a result. The location is the page address and not a file in your repository, so GitHub code scanning cannot place a result on a source file |
| `junit` | JUnit XML for a CI job. Blocker, major and minor problems and failing axe rules are failures. Info problems are skipped |

## The limits

The report prints these limits on its last page.

- It is a sample. Personas played a set of scenarios on the builds listed under scope. A problem they did not meet is not in it.
- It is not a WCAG conformance claim, a certificate or a statement about the law.
- Automated checks find only some accessibility problems. The sweep covers the pages it lists and nothing else.
- It does not replace testing with disabled people.
- A finding marked not reviewed is a claim by a model that no person has checked.
- Parts of the app that no run visited are not covered, whether or not the report has findings.

The report never says that the target passed or is compliant. A test in `internal/report` fails when the templates or any rendered format use those words. A report with no findings says that the runs found none and that this says little on its own.

## Example

`examples/report/` holds a report made from the synthetic runs in `examples/runs/` and a synthetic sweep. `examples/report/generate.sh` rebuilds it, and a test fails when the committed files are out of date. The records cite screenshots, and `examples/runs/screens/` holds the one the first finding uses, a capture of the Inkwell test app. The HTML names the others as missing.
