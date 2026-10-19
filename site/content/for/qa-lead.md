---
title: For QA leads
choice: a QA lead
icon: clipboard
lead_short: Every role, every pull request
weight: 10
lead: Exploratory testing by role on every pull request, with no scripts to maintain.
description: What a QA lead gets from Dummy Run. Exploratory testing by role on every pull request, evidence behind every finding and no scripts to maintain.
---

## One persona, one scenario, one preview

{{< graphic name="run-timeline" >}}

A run signs one persona in to the preview built for a pull request and works through one scenario. Run [`2026-10-17-sam-author-006`](../../../examples/runs/2026-10-17-sam-author-006.json) took 11 steps and ended with one confirmed finding at step 6.

## Evidence or nothing

{{< graphic name="finding-card" >}}

A second model reads each claim and looks for a step in the run record that supports it. A claim with no screenshot, log line or code reference is dropped. Blockers and major findings wait for a person to confirm them.

## Same problem, one row

{{< graphic name="board" >}}

Every finding has a stable ID, so the same problem in three runs is one row on the board. Nothing is filed for you. You decide which rows become tickets.

## Sources

The run contract is the [run record](../docs/run-record.md) and the evidence rule is in the [architecture](../../../docs/ARCHITECTURE.md), both sourced. The run above is a synthetic example in `examples/runs/`, so its figures are measured from that file.
