---
title: For executives
choice: an executive
weight: 60
lead: One board showing what is broken, who it affects and the evidence.
description: What an executive gets from Dummy Run. One results board with evidence behind each finding, the cost of each run, and an Apache-2.0 licence.
---

## The board

Each finding on the results board names the problem, its severity, the persona and role it affected and the step where it happened. A screenshot sits behind each one. A finding only reaches the board after a reviewer has checked it against the evidence, and blockers wait for a person to confirm them. The board is a set of static files, so it can sit behind your own login.

## What a run costs

Each run record stores the model calls it made, the tokens it used and the cost in US dollars. In the synthetic examples, run [`2026-10-16-ada-editor-003`](https://github.com/dummy-run/dummy-run/blob/main/examples/runs/2026-10-16-ada-editor-003.json) cost $0.04 on Gemini in `europe-west2` and run [`2026-10-17-ravi-reviewer-005`](https://github.com/dummy-run/dummy-run/blob/main/examples/runs/2026-10-17-ravi-reviewer-005.json) cost $0.47 on Anthropic. Both figures are the `usage.cost_usd` field of the record. The [Grafana](../docs/grafana.md) dashboards show spend by model over time.

## Where it stands

Dummy Run is pre-release and nothing installs yet. [The plan](../../../docs/PLAN.md) has six milestones. The first is done and the second is the engine. The code is under [Apache-2.0](../../../docs/LICENSING.md), which your legal team will recognise. Telemetry is OpenTelemetry, so any backend you already pay for can store it.

## Who runs it

A QA lead or a developer runs it against the preview of each pull request. Compliance reads the permission findings. Product reads the friction findings. Nothing is posted to a tracker for you, and a person decides what becomes a ticket. The pages for [QA leads](qa-lead.md), [developers](developer.md), [compliance and security](compliance-and-security.md) and [product and UX](product-and-ux.md) say what each of them gets.
