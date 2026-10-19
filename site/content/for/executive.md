---
title: For executives
choice: an executive
icon: chart
lead_short: One board, with evidence
weight: 60
lead: One board showing what is broken, who it affects and the evidence.
description: What an executive gets from Dummy Run. One results board with evidence behind each finding, the cost of each run, and an Apache-2.0 licence.
---

## One board

{{< graphic name="board" >}}

Each row names the problem, its severity, the persona affected and the reviewer's verdict. A screenshot sits behind each one. The board is static files, so it can sit behind your own login.

## What a run costs

{{< graphic name="cost" >}}

Each run record stores its model calls, tokens and cost in US dollars. These two are synthetic examples, and the [Grafana](../docs/grafana.md) dashboards show spend by model over time.

## Where it stands

{{< graphic name="milestones" >}}

Dummy Run is pre-release. The code is under [Apache-2.0](../../../docs/LICENSING.md) and telemetry is OpenTelemetry, so any backend you already pay for can store it. A QA lead or developer runs it. Compliance and product read the board.

## Sources

The milestones are in [the plan](../../../docs/PLAN.md), sourced. The costs are the `usage.cost_usd` field of [`2026-10-16-ada-editor-003`](../../../examples/runs/2026-10-16-ada-editor-003.json) and [`2026-10-17-ravi-reviewer-005`](../../../examples/runs/2026-10-17-ravi-reviewer-005.json), measured.
