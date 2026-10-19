---
title: For QA leads
role: QA lead
icon: clipboard
lead_short: Every role, every pull request
weight: 10
lead: Exploratory testing by role on every pull request, with evidence behind each finding.
description: What a QA lead gets from Dummy Run. Exploratory testing by role on every pull request, how it sits beside a scripted suite, which browsers it drives and the evidence behind every finding.
next:
  title: See what a run writes down
  text: The run record is the contract between the runner, the reviewer and the board.
  label: Read the run record format
  href: /docs/run-record/
---

## Beside your scripted suite {#scripted-suite}

Keep your suite. Dummy Run covers what a suite does not, which is the jobs, roles and devices nobody wrote a script for.

| What | Scripted suite | Dummy Run |
|---|---|---|
| What it checks | The steps you wrote | A job a persona tries in its own way |
| Same every run | Yes | No, so a reviewer checks every claim |
| Who writes it | You, as code | You, as a persona and a scenario in Markdown |
| What it costs | Upkeep when the page changes | Model tokens for each run |
| Best for | Regressions you can state | Roles, devices and journeys nobody scripted |

When a persona confirms a finding you want guarded for good, write the test for it. Nothing is filed for you.

## One persona, one scenario, one preview

{{< graphic name="run-timeline" >}}

A run signs one persona in to the preview built for a pull request and works through one scenario. This example run took 11 steps and ended with one confirmed finding at step 6.

## Evidence or nothing

{{< graphic name="finding-card" >}}

A second model reads each claim and looks for a step in the run record that supports it. A claim with no screenshot, log line or code reference is dropped. Blockers and major findings wait for a person to confirm them.

## Same problem, one row

{{< graphic name="board" >}}

Every finding has a stable ID, so the same problem in three runs is one row on the board. You decide which rows become tickets.

## Browsers {#browsers}

| Browser | Driven through | Notes |
|---|---|---|
| Chrome | WebDriver BiDi | A persona has a desktop or a phone profile |
| Firefox | WebDriver BiDi | The same session code as Chrome |
| Real phones and tablets | A hosted browser grid, through an adapter | Not available yet. A phone persona is a Chrome or Firefox window with a phone's screen size and user agent |
| Safari | Not supported | Dummy Run does not drive it |

## Sources

The run contract is the [run record](../docs/run-record.md). The evidence rule and the browser list are in the [architecture](../../../docs/ARCHITECTURE.md), sourced. The examples come from the synthetic records in `examples/runs/`, measured. The comparison with a scripted suite is our reasoning, inferred.
