---
title: For compliance and security
choice: in compliance or security
icon: shield
lead_short: What each role can reach
weight: 50
lead: What each role can see or do beyond its rights, verified before it is raised.
description: What compliance and security people get from Dummy Run. Permission findings verified against the code, a clear list of what leaves your environment, and data residency per market.
---

## Permission findings

{{< graphic name="finding-card" kind="permission" >}}

A persona signs in with one role and the reviewer checks what that role could reach. This finding is from run [`2026-10-17-lin-contributor-008`](../../../examples/runs/2026-10-17-lin-contributor-008.json), tagged code because the reviewer confirmed it in the source. Every blocker or major finding waits for a person before it counts.

## What leaves your environment

{{< graphic name="export" >}}

The run record and its screenshots stay wherever you run Dummy Run. The exporter sends a subset to the OpenTelemetry backend you choose. If a preview puts personal data in a URL, that URL reaches the backend, so keep previews on synthetic data.

## In region

{{< graphic name="residency" >}}

A market profile lists the regions that satisfy it and a run refuses to start if any adapter cannot. Personas are fictional, and the [contributing](../../../CONTRIBUTING.md) rules bar real names, contact details and data from a target environment. [Security](../../../SECURITY.md) explains coordinated disclosure for Dummy Run itself.

## Sources

The confirmation rule and the residency rule are in the [architecture](../../../docs/ARCHITECTURE.md), sourced. The exported attributes are in [Telemetry](../../../docs/TELEMETRY.md), sourced. The run above is a synthetic example in `examples/runs/`, measured.
