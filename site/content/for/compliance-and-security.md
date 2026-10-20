---
title: For compliance and security
role: Compliance and security
icon: shield
lead_short: What each role can reach
weight: 50
lead: What each role can see or do beyond its rights, and exactly what leaves your environment.
description: What you get from Dummy Run in compliance and security. Permission findings verified against the code, a plain list of what leaves your environment, whether screenshots reach the model provider and how data residency covers exported telemetry.
next:
  title: Read the rules in full
  text: The architecture page defines adapters, market profiles and the evidence rule.
  label: Read the architecture
  href: /docs/architecture/
---

## What goes where {#answers}

| Question | Answer |
|---|---|
| Where do run records and screenshots live? | In the results store you choose, such as a local directory or your own object storage. |
| Do screenshots reach the model provider? | Yes. The persona is a model that reads what the browser shows, so the provider you choose receives page text and screenshots (inferred). |
| Does exported telemetry carry screenshots? | No. It carries IDs, outcomes and the target environment. It adds page URLs, the model, its region and the cost. It adds each finding's title and verdict. |
| Does a market profile cover exported telemetry? | No. The profile checks the adapters. Your OpenTelemetry backend is not one, so host it in the same market. |

The provider and the backend both receive what the persona sees, so run against previews that hold synthetic data. [Ethics and limits](../../../docs/ETHICS.md) covers provider retention.

## Permission findings

{{< graphic name="finding-card" kind="permission" >}}

A persona signs in with one role and the reviewer checks what that role could reach. This example is tagged code because the reviewer confirmed it in the source. Every blocker or major finding waits for a person before it counts. The reviewer and the board are planned. The run record, the report and the screenshots work today.

## The audit report

`dummyrun report` writes the findings and the accessibility sweep as an audit report. Each finding carries its WCAG 2.2 criterion where one is known, and its review status. A finding that no person confirmed is marked not reviewed. The report states that it is a sample and not a conformance claim, a certificate or a statement about the law. The [report guide](../../../docs/REPORT.md) lists the limits.

## What the exporter sends {#export}

{{< graphic name="export" >}}

The run record and its screenshots stay wherever you run Dummy Run. The exporter sends the subset on the left to the OpenTelemetry backend you choose. If a preview puts personal data in a URL, that URL reaches the backend.

## Residency by market {#residency}

{{< graphic name="residency" >}}

A market profile lists the regions that satisfy it. A run refuses to start if any adapter cannot satisfy it. The five kinds of adapter are:

- Target
- Model provider
- Runner host
- Results store
- Secrets

Each trace carries the model's region as `cloud.region`, taken from `provider.region` in the run record.

Personas are fictional, and the [contributing](../../../CONTRIBUTING.md) rules bar real names, contact details and data from a target environment. [Security](../../../SECURITY.md) explains coordinated disclosure for Dummy Run itself.

## Sources

The adapters, the market profile and the confirmation rule are in the [architecture](../../../docs/ARCHITECTURE.md), sourced. The exported attributes and `cloud.region` are in [Telemetry](../../../docs/TELEMETRY.md), sourced. That the persona's model receives screenshots follows from the architecture and is inferred. The example finding is synthetic, measured.
