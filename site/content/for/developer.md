---
title: For developers
choice: a developer
icon: terminal
lead_short: Your laptop or CI
weight: 20
lead: Your product used the way a customer uses it, on your laptop or in CI.
description: What a developer gets from Dummy Run. A real browser against your preview, a JSON run record per run, OpenTelemetry traces and headers that join your own telemetry.
---

## A real browser on your preview

{{< graphic name="headers" >}}

A run drives Chrome or Firefox through WebDriver BiDi. Every request to your hosts carries a `traceparent` and a `baggage` header, and the user agent ends with `DummyRun/<version>`. An app instrumented with OpenTelemetry joins the persona's trace.

## One JSON record per run

{{< graphic name="record" >}}

Each run writes one record that validates against the [run record schema](../docs/run-record.md). The results board is static HTML and JSON built from those records.

## What ships when

{{< graphic name="milestones" >}}

[The plan](../../../docs/PLAN.md) gives the `dummyrun` command three subcommands: `run`, `review` and `board`. None of them ships yet. The [architecture](../../../docs/ARCHITECTURE.md) plans a plain URL adapter first. GitHub Deployments, Vercel, Netlify and Cloudflare Pages follow. Until the engine lands, contributions are [issues](../../../CONTRIBUTING.md).

## Sources

The headers are from [Telemetry](../../../docs/TELEMETRY.md) and the browsers and adapters from the [architecture](../../../docs/ARCHITECTURE.md), both sourced. The record excerpt is from `examples/runs/2026-10-17-sam-author-006.json`, measured.
