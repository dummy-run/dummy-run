---
title: For product and UX
choice: in product or UX
weight: 40
lead: Friction and confusion, each tied to the step and screenshot where it happened.
description: What product and UX people get from Dummy Run. Friction, confusion and accessibility findings tied to a step and a screenshot, from personas with habits and devices.
---

## Findings about people, with evidence

Findings have five kinds. Three of them are about the experience rather than the code. A friction finding says a persona could do its job but struggled. A question finding says a persona did not understand something. An accessibility finding comes from an automated check on a page the persona visited. The other two kinds are bug and permission.

In the example cast, `sam-author` on a phone found that the Submit button sits under the keyboard. That is a friction finding of major severity, tied to step 6 of run [`2026-10-17-sam-author-006`](https://github.com/dummy-run/dummy-run/blob/main/examples/runs/2026-10-17-sam-author-006.json) and to the screenshot taken there. `lin-contributor` asked what the payment status "settled" means in run [`2026-10-16-lin-contributor-002`](https://github.com/dummy-run/dummy-run/blob/main/examples/runs/2026-10-16-lin-contributor-002.json), which is a question finding. The reviewer confirmed the first and left the second pending, because it rests on the persona's reasoning rather than a measurement.

## Personas have habits and devices

A persona has a role with permissions, plus goals, habits and a device. `sam-author` writes on a phone. `lin-contributor` uses a tablet. The device sets the viewport and the user agent for the run, so the same scenario on a phone and a desktop can produce different findings. Persona traits need a source, and the [writing guide](../../../docs/STYLE.md) says every trait without a citation is removed.

## What it is not

It is not analytics and it does not watch real users. Personas are fictional. The run record keeps the persona's notes on each step (the `note` field), and those notes stay with you. A finding is a claim with a screenshot, and a person decides whether it becomes a ticket.

## What to read next

[How it works](../docs/how-it-works.md) covers the flow from cast to board. The [run record](../docs/run-record.md) page lists the fields behind each finding. The project is pre-release, and [the plan](../../../docs/PLAN.md) puts a demo app and an example cast first.
