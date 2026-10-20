---
title: For QA leads
role: QA lead
icon: clipboard
lead_short: Every role, every pull request
weight: 10
lead: Exploratory testing by role on every pull request, with a screenshot behind every finding and no new scripts to maintain.
description: What you get from Dummy Run as a QA lead. Exploratory testing by role on every pull request, how it sits beside your scripted suite, which browsers it drives and the evidence behind every finding.
next:
  title: Run one persona
  text: Install the command and send one persona to a site without changing anything on it.
  label: Get started
  href: /start/
---

## Beside your scripted suite {#scripted-suite}

Keep your suite. It's the right tool for regressions you can describe in advance. Dummy Run covers the other part, the jobs, roles and devices nobody wrote a script for.

| What | Scripted suite | Dummy Run |
|---|---|---|
| What it checks | The steps you wrote | A job a persona tries in its own way |
| Same every run | Yes | No, so you review each finding |
| Who writes it | You, as code | You, as a persona and a scenario in Markdown |
| What it costs | Upkeep when the page changes | Model tokens for each run, recorded in the run record |
| Best for | Regressions you can state | Roles, devices and journeys nobody scripted |

When a persona finds something you never want to see again, write the test for it. Nothing is filed for you.

## One persona, one job, one preview

{{< graphic name="run-timeline" >}}

A run signs one persona in to the preview for a pull request and works through one job. Run [`2026-10-17-sam-author-006`](../../../examples/runs/2026-10-17-sam-author-006.json) took 11 steps and turned up one finding at step 6.

## Every finding comes with its evidence

{{< graphic name="finding-card" >}}

A persona's finding has to point at the steps behind it, with the screenshot, console line or network request that shows the problem. Every finding starts as pending. You work through them with `dummyrun review`, confirm the real ones and reject the rest with a reason. Reports count only what you've confirmed.

## Same problem, one row

{{< graphic name="board" >}}

Every finding has a stable fingerprint, so the same problem turning up in three runs counts once in the report and you review it once. `dummyrun coverage` tells you how much of the app the personas reached and whether more runs are still turning up new problems, which helps you decide when to stop.

## Browsers {#browsers}

| Browser | Driven through | Notes |
|---|---|---|
| Chrome | WebDriver BiDi | Desktop, tablet or phone-sized window, with page zoom if the persona needs it |
| Firefox | WebDriver BiDi | The same session code as Chrome |
| Real phones and tablets | Not available | Phone and tablet personas use a window at the device's size |
| Safari | Not supported | Dummy Run doesn't drive it |

A phone persona is a Chrome or Firefox window at a phone's screen size. The user agent is the browser's own, with `DummyRun/<version>` added at the end.
