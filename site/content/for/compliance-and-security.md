---
title: For compliance and security
choice: in compliance or security
weight: 50
lead: What each role can see or do beyond its rights, verified before it is raised.
description: What compliance and security people get from Dummy Run. Permission findings verified against the code, a clear list of what leaves your environment, and data residency per market.
---

## Permission findings

A persona signs in with one role. The reviewer checks what that role could reach. In run [`2026-10-17-lin-contributor-008`](https://github.com/dummy-run/dummy-run/blob/main/examples/runs/2026-10-17-lin-contributor-008.json) the contributor opened the rates table for all writers at step 7. The reviewer confirmed the finding against the code and tagged it `code`. It is a permission finding of major severity, and every finding of blocker or major severity waits for a person to confirm it before it counts on the board.

## What leaves your environment

The run record and its screenshots stay wherever you run Dummy Run, and the results board built from them is a set of static files you host yourself. Nothing phones home. The [telemetry](../../../docs/TELEMETRY.md) exporter sends a subset of the record to the OpenTelemetry backend you choose. If a preview puts personal data in a URL, that URL reaches the backend, so keep previews on synthetic data.

| Exported | Never exported |
|---|---|
| The run ID, the persona ID and its role and device | Screenshots |
| The scenario and the outcome | Page text |
| The target environment, its commit and the page URLs visited | The persona's notes on each step |
| The model, the region it ran in and the token counts | Console bodies and network bodies |
| Each finding's title with its kind, severity and verdict | Test-user credentials and provider keys, which are never in a run record at all |

## Data residency

The [architecture](../../../docs/ARCHITECTURE.md) sets the residency rule. A market profile such as `uk` lists the regions that satisfy it. Every adapter declares its regions. A run refuses to start if any adapter in the chain cannot satisfy the market. The model provider is chosen per market from Vertex AI, Azure AI Foundry, Anthropic and local models. Each run record stores the region the model ran in.

## No real people

Personas are fictional, every one of them. The [contributing](../../../CONTRIBUTING.md) rules bar real names, contact details and data from a target environment. CI blocks credential patterns and known identifiers on every pull request. The repository holds no run results, screenshots or personas drawn from real customers.

## Reporting

[Security](../../../SECURITY.md) explains coordinated disclosure for Dummy Run itself. We reply within five working days and publish a fix with an advisory. A finding in someone else's app belongs to its owner and goes through their disclosure route.
