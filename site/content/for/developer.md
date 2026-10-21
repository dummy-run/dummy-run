---
title: For developers
role: Developer
icon: terminal
lead_short: Your laptop or CI
weight: 20
lead: Watch your product being used the way a customer would use it, on your laptop or in CI, with a record you can read like a log.
description: What you get from Dummy Run as a developer. Two install commands, a real browser on your preview, a read-only mode, one JSON record per run and traces that join your own.
next:
  title: Run it
  text: Install both commands, send one persona to a site in read-only mode and look at the run in a local Grafana.
  label: Get started
  href: /start/
---

## Install

You need Go 1.27.1 or later. `dummyrun` plays a persona, and `dummyrun-otel` sends the run to any OpenTelemetry backend.

```sh
go install github.com/dummy-run/dummy-run/cmd/dummyrun@latest
go install github.com/dummy-run/dummy-run/integrations/grafana/exporter/cmd/dummyrun-otel@latest
```

[Get started](../start.md) takes you from there to your first run and a local Grafana.

## A real browser on your preview

{{< graphic name="headers" >}}

A run drives a real Chrome or Firefox through WebDriver BiDi, so your app gets the same JavaScript, cookies and layout a customer's browser would. Set `"read_only": true` in the target and the browser blocks anything that could change data. Every request to your hosts carries `traceparent` and `baggage` headers, and the user agent ends with `DummyRun/<version>`. If your app is instrumented with OpenTelemetry, its spans join the persona's trace, so you can see which backend call sat behind the step where the persona got stuck.

## One JSON record per run

{{< graphic name="record" >}}

Each run writes one record that validates against the [run record schema](../docs/run-record.md): the persona, the target, every step, every finding and what the model cost. It's plain JSON, so you can diff it, script against it or turn it into tickets yourself.

## In CI and in your editor

`dummyrun report` can write SARIF for code scanning and JUnit for any CI that reads test results, plus a badge for your README. If you work with a coding agent, `dummyrun mcp` lets Claude Code, Cursor, VS Code or Codex check a page for accessibility failures, or run a persona on it, while you work. The [MCP guide](../../../docs/MCP.md) shows the setup for each.
