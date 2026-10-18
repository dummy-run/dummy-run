---
title: For QA leads
choice: a QA lead
weight: 10
lead: Exploratory testing by role on every pull request, with no scripts to maintain.
description: What a QA lead gets from Dummy Run. Exploratory testing by role on every pull request, evidence behind every finding and no scripts to maintain.
---

## What a run gives you

A run signs one persona in to the preview built for a pull request. It works through one scenario. The persona `sam-author` in the example cast is an author who writes on a phone. Its scenario "Reply to review comments" took 11 steps in run `2026-10-17-sam-author-006` and ended with one confirmed finding, which is that the Submit button on a phone sits under the keyboard. The finding points at step 6. The screenshot is there too.

Each run writes a run record. It is JSON. It lists every step with its URL and HTTP status and duration, and it counts the console errors and accessibility violations on each step. The eight synthetic records in [examples/runs/](https://github.com/dummy-run/dummy-run/tree/main/examples/runs) show the shape, and the [run record](../docs/run-record.md) page explains each field.

## What the reviewer rejects

A persona can be wrong, and sometimes it is. A second model reads each claim and looks for a step in the run record that supports it. That support is a screenshot, a console or network line, or a reference to the code. A claim with no support is dropped. Each finding that survives carries one of four evidence tags. The tags are measured, code, sourced and inferred. Blockers and major findings wait for a person to confirm them before they count.

## Where it fits beside your suite

Your unit tests and end-to-end scripts check the paths you already know. A persona has a role and a goal rather than a script, so it takes the paths a user of that role would take. A finding is a starting point for a bug report, with the run ID and the step number and the screenshot already attached. Nothing is filed for you, because a person decides which findings become tickets and which do not.

Every finding has a stable ID, so the same problem found in three runs is one row on the board rather than three. The two `sam-author` runs on 17 October both carry `submit-button-below-keyboard`. The [Grafana dashboards](../docs/grafana.md) use the same IDs to show first seen, last seen and the personas affected.

## What to read next

[How it works](../docs/how-it-works.md) has the full flow from cast to board. [Architecture](../../../docs/ARCHITECTURE.md) lists the target adapters that find a preview and create a test user. The project is pre-release, and [the plan](../../../docs/PLAN.md) puts a demo app and an example cast first.
