---
title: For developers
role: Developer
icon: terminal
lead_short: Your laptop or CI
weight: 20
lead: Your product used the way a customer uses it, on your laptop or in CI.
description: What a developer gets from Dummy Run. An install command that works today and a real browser on your preview. One JSON run record per run, and headers that join your own telemetry.
next:
  title: Run it
  text: Install the exporter, start a local Grafana and see the example runs in under ten minutes.
  label: Get started
  href: /start/
---

## Install

Go 1.27 or later builds the telemetry exporter. It reads run records and sends them to any OpenTelemetry backend.

```sh
go install github.com/dummy-run/dummy-run/integrations/grafana/exporter/cmd/dummyrun-otel@latest
```

The [get started](../start.md) page runs it against the example records and a local Grafana.

## A real browser on your preview

{{< graphic name="headers" >}}

A run drives Chrome or Firefox through WebDriver BiDi. Every request to your hosts carries a `traceparent` and a `baggage` header, and the user agent ends with `DummyRun/<version>`. An app instrumented with OpenTelemetry joins the persona's trace.

## One JSON record per run

{{< graphic name="record" >}}

Each run writes one record that validates against the [run record schema](../docs/run-record.md). The results board is static HTML and JSON built from those records.

## Adapters

An adapter finds the preview for a pull request, creates a test user with the right role and loads test data. The [architecture](../../../docs/ARCHITECTURE.md) lists the adapters for targets and model providers. It also lists them for runner hosts, results stores and secrets.

## Sources

The headers are from [Telemetry](../../../docs/TELEMETRY.md) and the browsers and adapters from the [architecture](../../../docs/ARCHITECTURE.md), both sourced. The install command is in `integrations/grafana/exporter/README.md`, code. The record excerpt is a synthetic example, measured.
