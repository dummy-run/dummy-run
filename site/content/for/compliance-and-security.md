---
title: For compliance and security
role: Compliance and security
icon: shield
lead_short: What each role can reach
weight: 50
lead: Find out what each role can see or do that it shouldn't, and know exactly what leaves your environment while you test.
description: What you get from Dummy Run in compliance and security. Permission findings you can check against the code, a plain list of what leaves your environment, whether screenshots reach the model provider and where telemetry lives.
next:
  title: Read the rules in full
  text: The architecture page explains how runs, providers and storage fit together.
  label: Read the architecture
  href: /docs/architecture/
---

## What goes where {#answers}

| Question | Answer |
|---|---|
| Where do run records and screenshots live? | Wherever you keep your runs, such as a local folder or your own object storage. |
| Do screenshots reach the model provider? | Yes. The persona is a model reading what the browser shows, so the provider you choose receives page text and screenshots. |
| Does exported telemetry carry screenshots or page text? | No screenshots and no page snapshots. It carries run IDs, outcomes and the target environment, plus page addresses, what each step acted on, the model, its region and the cost. It also carries each finding's title, evidence label and verdict. A step's target can be a button label and a title is a sentence the persona wrote, so either can quote a few words of the page. |
| Where does telemetry live? | In your own OpenTelemetry backend. Dummy Run sends it nowhere else. |
| Can I keep the model in one region? | Yes. You choose the provider and region for each run, and the run record notes the region the model used. Keep your OpenTelemetry backend in the same region too. |

The provider sees what the persona sees, and your backend gets the telemetry. If you pass `--grafana-url`, Grafana also gets an annotation for each run. That's all that leaves. Run against previews that hold test data rather than real customers. [Ethics and limits](../../../docs/ETHICS.md) has more on how long providers keep prompts.

## Permission findings

{{< graphic name="finding-card" kind="permission" >}}

A persona signs in with one role and tries things that role shouldn't be able to do. In this example a contributor opened the rates table for every writer. The finding points at the exact step, and the person who reviewed it traced the route in the code before confirming it. Every finding waits for a person to confirm it before it counts in a report. The library's viewer, editor and admin personas are built for exactly this. Run all three through the same job and compare what each one could reach.

## The audit report

`dummyrun report` writes the persona findings and the accessibility sweep as one audit report. Each finding carries its WCAG 2.2 criterion where there is one, and shows whether a person has reviewed it. Anything nobody has confirmed is clearly marked as not reviewed. The report says up front that it's a sample, and that it isn't a conformance claim, a certificate or a legal opinion. The [report guide](../../../docs/REPORT.md) sets out its limits.

## What the exporter sends {#export}

{{< graphic name="export" >}}

The run record and its screenshots stay wherever you run Dummy Run. The exporter sends only what's on the left to the OpenTelemetry backend you choose. One thing to watch for: if your preview puts personal data in a URL, that URL will reach the backend.

## Keeping the model in region {#residency}

{{< graphic name="residency" >}}

You choose the provider and the region for every run, so a UK team can keep its runs on `uksouth` or `europe-west2`. Each run record stores the region as `provider.region`, and each exported trace carries it as `cloud.region`, so you can show where every run happened. Market profiles that refuse to start a run outside the regions you allow are coming soon.

The personas are fictional. The [contributing](../../../CONTRIBUTING.md) rules bar real names, contact details and data from a live environment. If you find a security problem in Dummy Run itself, [Security](../../../SECURITY.md) explains how to tell us privately.
