# Inkwell audit

*Audit report*

- Prepared for: Inkwell Press
- Target: https://pr-41.inkwell.example
- Tested: 16 October 2026 to 17 October 2026
- Report date: 19 October 2026
- Findings included: confirmed findings and findings not reviewed
- Made with: dummyrun 0.0.0-test

Dummy Run played 2 fictional users in 4 runs against https://pr-41.inkwell.example between 16 October 2026 and 17 October 2026.

The runs found 4 distinct problems: 2 major, 1 minor, 1 info. A person has confirmed 3 of them. 1 is marked not reviewed. No person has checked it yet.

The axe sweep tested 2 pages and found 2 failing elements on 2 rules. 1 setting could not be tested.

There are too few repeated findings to estimate how many problems are still unfound.

## Summary

| Severity | Problems found |
|---|---:|
| Blocker | 0 |
| Major | 2 |
| Minor | 1 |
| Info | 1 |

Not reviewed: 1 finding. A model reported what it found and no person has checked the evidence.

### The key numbers

| Measure | Value |
|---|---:|
| Problems found | 4 (3 confirmed by a person) |
| Runs | 4 (3 completed) |
| Personas | 2 |
| Routes visited | 4 |
| Pages in the axe sweep | 2 (2 failing elements) |

### How sure you can be

Too few findings repeated to estimate how many problems are still unfound.

### What was tested

- Target: https://pr-41.inkwell.example, build `a865ffd` (commit), pr-41
- Personas: ada-editor (editor), lin-contributor (contributor)
- Scenarios: Check a payment status; Send a first pitch; Reassign a stalled draft
- Browsers: chrome 148.0, firefox 146.0
- Axe sweep: 2 pages in 3 settings

> Read this as a sample. This report tells you what the runs met. It is not a certificate and it does not say the site meets a standard. A problem nobody ran into is not in it. See [What this report is not](#what-this-report-is-not).

## Findings

The findings are sorted by severity, then by the number of runs that found them. Each is a distinct problem, and one found in several runs appears once.

### Major (2 findings)

#### A contributor can open the rates table for all writers

Major. Confirmed by a reviewer.

The Payments page, the Rates link (`/payments`)

| | |
|---|---|
| What went wrong | Let in when it should have been blocked |
| Who hit it | Lin, a contributor, in 1 of 4 runs |
| Review | Confirmed by example-reviewer on 18 October 2026. Reason: Reproduced on the preview with a contributor account. |

How to reproduce:

1. Sign in as a contributor.
2. Use a tablet at 200% zoom in Firefox.
3. On the Payments page, look at what is shown.

What goes wrong: A contributor can open the rates table for all writers

*The Payments page as Lin saw it.* What to look at: the Rates link.

Technical details: run `2026-10-17-lin-contributor-008`, step 7; persona `lin-contributor` on tablet in firefox 146.0; kind permission; symptom `access-wrongly-allowed`; locus `link:rates`; evidence code; fingerprint `933d06cb3cee90e4`; screenshot `screens/008-07.png` next to the run record.

#### Scheduling an issue for a past date returns a server error

Major. Confirmed by a reviewer.

The Issues page, the Schedule button (`/issues`)

| | |
|---|---|
| What went wrong | An error appeared |
| Who hit it | Ada, an editor, in 1 of 4 runs |
| Review | Confirmed |

How to reproduce:

1. Sign in as an editor.
2. Use a desktop computer in Chrome.
3. Type into Submit.

What goes wrong: Scheduling an issue for a past date returns a server error

Technical details: run `2026-10-16-ada-editor-003`, step 7; persona `ada-editor` on desktop in chrome 148.0; kind bug; symptom `error-shown`; locus `button:schedule`; evidence measured; fingerprint `72be5e1e780d79bb`.

### Minor (1 finding)

#### The hint under the schedule field is too faint to read

Minor. Not reviewed.

The Issues page, the Schedule form area (`/issues`)

| | |
|---|---|
| What went wrong | An automated accessibility check failed (color-contrast) |
| Who hit it | Ada, an editor, in 1 of 4 runs |
| WCAG 2.2 | 1.4.3 |
| Review | Not reviewed. No person has checked this finding |
| Fix guidance | https://dequeuniversity.com/rules/axe/4.13/color-contrast |

How to reproduce:

1. Sign in as an editor.
2. Use a desktop computer in Chrome.
3. Open the Drafts page.

What goes wrong: The hint under the schedule field is too faint to read

*The Drafts page as Ada saw it.*

Technical details: run `2026-10-16-ada-editor-003`, step 3; persona `ada-editor` on desktop in chrome 148.0; kind accessibility; symptom `a11y:color-contrast`; locus `region:schedule form`; evidence measured; fingerprint `64afd5191821762c`; screenshot `screens/003-03.png` next to the run record.

### Info (1 finding)

#### The payment status 'settled' is unclear to a contributor

Info. Confirmed by a reviewer.

The Payments page, the Payment status area (`/payments`)

| | |
|---|---|
| What went wrong | The name does not say what it does |
| Who hit it | Lin, a contributor, in 3 of 4 runs |
| Review | Confirmed |

How to reproduce:

1. Sign in as a contributor.
2. Use a tablet at 200% zoom in Chrome.
3. Click Submit.

What goes wrong: The payment status 'settled' is unclear to a contributor

Technical details: run `2026-10-16-lin-contributor-002`, step 7; persona `lin-contributor` on tablet in chrome 148.0; kind question; symptom `unclear-label`; locus `region:payment status`; evidence inferred; fingerprint `1718b75300702f7b`. Also seen in: 2026-10-16-lin-contributor-002, 2026-10-16-lin-contributor-004 (not reviewed), 2026-10-17-lin-contributor-008 (not reviewed).

## Axe sweep and layout checks

The sweep ran axe-core 4.13.0 with dummyrun a11y on 2 pages in 3 settings (browser, viewport and colour scheme), on chrome 148.0.7778.97. It checked the rules with these tags: `wcag21a` `wcag21aa` `wcag22aa` `wcag2a` `wcag2aa`. It found 2 failing elements and set aside 1 on the allow-list.

### Rules that failed

| Rule | What it checks | Impact | WCAG 2.2 | Elements | Fix guidance |
|---|---|---|---|---:|---|
| `button-name` | Buttons must have discernible text | Critical | 4.1.2 | 1 | https://dequeuniversity.com/rules/axe/4.13/button-name |
| `color-contrast` | Elements must meet minimum color contrast ratio thresholds | Serious | 1.4.3 | 1 | https://dequeuniversity.com/rules/axe/4.13/color-contrast |

### Where each rule failed

| Rule | Page | Elements | Settings |
|---|---|---|---|
| `button-name` | https://pr-41.inkwell.example/issues | `#menu-toggle` | chrome/desktop/light |
| `color-contrast` | https://pr-41.inkwell.example/issues | `.hint` | chrome/desktop/light, chrome/phone/light |

### Needs a person to check

Axe cannot decide these rules alone. A person has to look.

| Rule | What it checks | WCAG 2.2 | Pages |
|---|---|---|---|
| `color-contrast` | Elements must meet minimum color contrast ratio thresholds | 1.4.3 | https://pr-41.inkwell.example/issues |

### Pages that could not be tested

| Page | Setting | Error |
|---|---|---|
| https://pr-41.inkwell.example/payments | chrome/desktop/light | the page did not load within 30 seconds |

## How it was tested

Each run signs one fictional user, called a persona, in to the app in a real browser. The persona works through a scenario and reports what broke, what confused it and what its role should not have seen. Findings are folded together when they share a fingerprint, which is a hash of the route, the control and the kind of symptom.

- Period: 2026-10-16T15:42:31Z to 2026-10-17T13:54:03Z
- Runs: 4 in total: 3 completed, 1 blocked, 0 errored and 0 out of budget
- Model cost: $0.48 over 47 model calls (339091 input and 22363 output tokens)
- Routes visited: 4 (`/drafts` `/issues` `/payments` `/reviews`)

| Address | Environment | Build | Adapter | Runs |
|---|---|---|---|---:|
| https://pr-41.inkwell.example | pr-41 | `a865ffd` (commit) | url | 4 |

| Persona | Role | Device | Dimensions | Runs |
|---|---|---|---|---:|
| ada-editor | editor | desktop | viewport 1400x1000 | 1 |
| lin-contributor | contributor | tablet at 200 percent zoom | viewport 410x590 | 3 |

| Scenario | Runs | Goal met | Partly | Not met | Not recorded |
|---|---:|---:|---:|---:|---:|
| Check a payment status | 2 | 1 | 1 | 0 | 0 |
| Send a first pitch | 1 | 1 | 0 | 0 | 0 |
| Reassign a stalled draft | 1 | 0 | 0 | 1 | 0 |

| Browser | Version | Runs |
|---|---|---:|
| chrome | 148.0 | 2 |
| firefox | 146.0 | 2 |

| Model | Provider | Region | Runs |
|---|---|---|---:|
| claude-sonnet-5-5 | anthropic | global | 2 |
| gemini-3.5-flash-lite | vertex-gemini | europe-west2 | 2 |

### Coverage

These numbers come from `dummyrun coverage` over the same runs. They say how much the sample can tell you.

- Counted: findings that no person has confirmed, so a demonstration of the method and not an estimate
- Completed runs: 3
- Distinct problems seen: 2
- Still unfound: not estimable from these runs
- Sample coverage: not estimable
- New problems per extra run: 0.33
- Stopping rule: Stopped: none; not stopped: desktop (0 completed runs, need 4); tablet (3 completed runs, need 4)

How to read these numbers:

- The Chao2 figure is a floor. The real number of problems is at least that high. It is never a count of what remains.
- Sample coverage is the share of the chance of finding a problem that sits in problems already found. Below 0.50 do not extrapolate from it.
- New problems per extra run is how many new problems one more run is expected to find. A low figure suggests more runs will add little.
- With fewer than 5 completed runs the estimates are weak. The stopping rule needs 4 completed runs in each group.

Cautions:

- Findings that no person has confirmed are counted. Read these numbers as a demonstration of the method and not as an estimate.
- Identity reliability is not measured (no reviewer clusters): every problem count is conditional on the fingerprint rule.
- T=3 completed runs: Chao recommends at least 5, so read the interval as decorative.
- Stratum desktop has 0 completed runs; remaining-problem estimates need enough runs per stratum (at least 4).
- Stratum tablet has 3 completed runs; remaining-problem estimates need enough runs per stratum (at least 4).

## What this report is not

- This report is a sample. Personas played a set of scenarios on the builds listed under scope. A problem they did not meet is not in it.
- It is not a WCAG conformance claim, a certificate or a statement about the law.
- Automated checks find only some accessibility problems. The sweep covers the pages it lists and nothing else.
- It does not replace testing with disabled people.
- A finding marked not reviewed is a claim by a model that no person has checked.
- Parts of the app that no run visited are not covered, whether or not the report has findings.

