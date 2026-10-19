# Inkwell audit

Prepared for Inkwell Press. Report date 2026-10-19. Made with dummyrun 0.0.0-test. Findings included: confirmed findings and findings not reviewed.

## Summary

Dummy Run played 2 fictional users in 4 runs against https://pr-41.inkwell.example between 2026-10-16 and 2026-10-17.

The runs found 4 distinct problems: 2 major, 1 minor, 1 info. A person has confirmed 3 of them. 1 is marked not reviewed. No person has checked it yet.

The axe sweep tested 2 pages and found 2 failing elements on 2 rules. 1 setting could not be tested.

There are too few repeated findings to estimate how many problems are still unfound.

| Severity | Distinct problems |
|---|---|
| Blocker | 0 |
| Major | 2 |
| Minor | 1 |
| Info | 1 |

Not reviewed: 1. A model reported these findings and no person has checked them against the evidence.

## Scope and method

Each run signs one fictional user, called a persona, in to the app in a real browser. The persona works through a scenario and reports what broke, what confused it and what its role should not have seen. Findings are folded together when they share a fingerprint, which is a hash of the route, the control and the kind of symptom.

- Period: 2026-10-16T15:42:31Z to 2026-10-17T13:54:03Z
- Runs: 4 in total: 3 completed, 1 blocked, 0 errored and 0 out of budget
- Model cost: $0.48 over 47 model calls (339091 input and 22363 output tokens)
- Routes visited: 4 (`/drafts` `/issues` `/payments` `/reviews`)

| Address | Environment | Build | Adapter | Runs |
|---|---|---|---|---|
| https://pr-41.inkwell.example | pr-41 | `a865ffd` (commit) | url | 4 |

| Persona | Role | Device | Dimensions | Runs |
|---|---|---|---|---|
| ada-editor | editor | desktop | viewport 1400x1000 | 1 |
| lin-contributor | contributor | tablet at 200 percent zoom | viewport 410x590 | 3 |

| Scenario | Runs | Goal met | Partly | Not met | Not recorded |
|---|---|---|---|---|---|
| Check a payment status | 2 | 1 | 1 | 0 | 0 |
| Send a first pitch | 1 | 1 | 0 | 0 | 0 |
| Reassign a stalled draft | 1 | 0 | 0 | 1 | 0 |

| Browser | Version | Runs |
|---|---|---|
| chrome | 148.0 | 2 |
| firefox | 146.0 | 2 |

| Model | Provider | Region | Runs |
|---|---|---|---|
| claude-sonnet-5-5 | anthropic | global | 2 |
| gemini-3.5-flash-lite | vertex-gemini | europe-west2 | 2 |

The axe sweep ran dummyrun a11y with axe-core 4.13.0 on chrome 148.0.7778.97. It tested 2 pages in 3 settings.

## Coverage

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

- Findings that no person has confirmed are counted. Read these numbers as a demonstration of the method and not as an estimate..
- Identity reliability is not measured (no reviewer clusters): every problem count is conditional on the fingerprint rule.
- T=3 completed runs: Chao recommends at least 5, so read the interval as decorative.
- Stratum desktop has 0 completed runs; remaining-problem estimates need enough runs per stratum (at least 4).
- Stratum tablet has 3 completed runs; remaining-problem estimates need enough runs per stratum (at least 4).

## Findings

Findings are sorted by severity, then by the number of runs that found them. Each one is a distinct problem. A problem found in several runs appears once.

### Major: A contributor can open the rates table for all writers

- Status: Confirmed by a reviewer
- Kind: permission
- Symptom: access-wrongly-allowed
- Route: `/payments`
- Control or region: link:rates
- Evidence: code
- Found by: lin-contributor in 1 of 4 runs
- WCAG 2.2 criterion: not recorded for this finding
- Review: confirmed by Priya Shah on 2026-10-18T09:30:00Z. Reason: Reproduced on the preview with a contributor account.
- Fingerprint: `933d06cb3cee90e4`

How to reproduce. Run 2026-10-17-lin-contributor-008: persona lin-contributor signed in as contributor on a tablet at 200 percent zoom in firefox 146.0.

7. Look on /payments.

Screenshot from run 2026-10-17-lin-contributor-008, step 7: look on /payments. The file is `screens/008-07.png`, next to the run record.

### Major: Scheduling an issue for a past date returns a server error

- Status: Confirmed by a reviewer
- Kind: bug
- Symptom: error-shown
- Route: `/issues`
- Control or region: button:schedule
- Evidence: measured
- Found by: ada-editor in 1 of 4 runs
- WCAG 2.2 criterion: not recorded for this finding
- Review: confirmed
- Fingerprint: `72be5e1e780d79bb`

How to reproduce. Run 2026-10-16-ada-editor-003: persona ada-editor signed in as editor on a desktop in chrome 148.0.

7. Fill Submit on /issues.

### Minor: The hint under the schedule field is too faint to read

- Status: Not reviewed
- Kind: accessibility
- Symptom: a11y:color-contrast
- Route: `/issues`
- Control or region: region:schedule form
- Evidence: measured
- Found by: ada-editor in 1 of 4 runs
- WCAG 2.2 criterion: 1.4.3
- Review: not reviewed. No person has checked this finding
- Fix guidance: https://dequeuniversity.com/rules/axe/4.13/color-contrast
- Fingerprint: `64afd5191821762c`

How to reproduce. Run 2026-10-16-ada-editor-003: persona ada-editor signed in as editor on a desktop in chrome 148.0.

3. Visit on /drafts.

Screenshot from run 2026-10-16-ada-editor-003, step 3: visit on /drafts. The file is `screens/003-03.png`, next to the run record.

### Info: The payment status 'settled' is unclear to a contributor

- Status: Confirmed by a reviewer
- Kind: question
- Symptom: unclear-label
- Route: `/payments`
- Control or region: region:payment status
- Evidence: inferred
- Found by: lin-contributor in 3 of 4 runs
- WCAG 2.2 criterion: not recorded for this finding
- Review: confirmed
- Fingerprint: `1718b75300702f7b`

How to reproduce. Run 2026-10-17-lin-contributor-008: persona lin-contributor signed in as contributor on a tablet at 200 percent zoom in firefox 146.0.

5. Look on /payments.

Also seen in: 2026-10-16-lin-contributor-002, 2026-10-16-lin-contributor-004 (not reviewed), 2026-10-17-lin-contributor-008 (not reviewed).

Screenshot from run 2026-10-17-lin-contributor-008, step 5: look on /payments. The file is `screens/008-05.png`, next to the run record.

## Axe sweep

The sweep ran axe-core 4.13.0 on 2 pages in 3 settings (browser, viewport and colour scheme). It checked the rules with these tags: `wcag21a` `wcag21aa` `wcag22aa` `wcag2a` `wcag2aa`. It found 2 failing elements and set aside 1 on the allow-list.

### Critical: Buttons must have discernible text

- Rule: `button-name`
- WCAG 2.2 criterion: 4.1.2
- Elements: 1
- Fix guidance: https://dequeuniversity.com/rules/axe/4.13/button-name

| Page | Elements | Settings |
|---|---|---|
| https://pr-41.inkwell.example/issues | `#menu-toggle` | chrome/desktop/light |

### Serious: Elements must meet minimum color contrast ratio thresholds

- Rule: `color-contrast`
- WCAG 2.2 criterion: 1.4.3
- Elements: 1
- Fix guidance: https://dequeuniversity.com/rules/axe/4.13/color-contrast

| Page | Elements | Settings |
|---|---|---|
| https://pr-41.inkwell.example/issues | `.hint` | chrome/desktop/light, chrome/phone/light |

### Needs a person to check

Axe cannot decide these rules alone. A person has to look.

| Rule | What it checks | WCAG 2.2 criterion | Pages |
|---|---|---|---|
| `color-contrast` | Elements must meet minimum color contrast ratio thresholds | 1.4.3 | https://pr-41.inkwell.example/issues |

### Pages that could not be tested

| Page | Setting | Error |
|---|---|---|
| https://pr-41.inkwell.example/payments | chrome/desktop/light | the page did not load within 30 seconds |

## What this report is not

- This report is a sample. Personas played a set of scenarios on one build. A problem they did not meet is not in it.
- It is not a WCAG conformance claim, a certificate or a statement about the law.
- Automated checks find only some accessibility problems. The sweep covers the pages it lists and nothing else.
- It does not replace testing with disabled people.
- A finding marked not reviewed is a claim by a model that no person has checked.
- Parts of the app that no run visited are not covered, whether or not the report has findings.

