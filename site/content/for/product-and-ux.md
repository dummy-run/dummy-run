---
title: For product and UX
role: Product and UX
icon: cursor
lead_short: Friction, with the screenshot
weight: 40
lead: Friction and confusion, each tied to the step and screenshot where it happened.
description: What product and UX people get from Dummy Run. Friction, confusion and accessibility findings tied to a step and a screenshot, from personas with habits and devices.
next:
  title: Start from personas that already exist
  text: The library has a screen-reader user, a phone on a slow connection, a first-time visitor and five more.
  label: Browse the persona library
  href: /personas/
---

## Friction, with the screenshot

{{< graphic name="finding-card" >}}

Findings have five kinds. Friction, question and accessibility are about the experience. Bug and permission are about the code. This example is measured at step 6 of a run and confirmed by the reviewer.

## Personas have devices and habits

{{< graphic name="persona-card" who="sam-author" >}}

The device sets the viewport and the user agent, so the same scenario on a phone and a desktop can produce different findings. Every persona trait needs a source, and the [writing guide](../../../docs/STYLE.md) removes traits without one.

## Questions are findings too

{{< graphic name="board" >}}

In run [`2026-10-16-lin-contributor-002`](../../../examples/runs/2026-10-16-lin-contributor-002.json) the contributor asked what the payment status "settled" means. That is a question finding, tagged inferred because it rests on the persona's reasoning. Personas are fictional and nothing watches real users.

## Sources

The finding kinds and evidence tags are in the [run record schema](../docs/run-record.md), code. The runs above are synthetic examples in `examples/runs/`, measured.
