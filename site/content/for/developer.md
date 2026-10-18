---
title: For developers
choice: a developer
weight: 20
lead: Your product used the way a customer uses it, on your laptop or in CI.
description: What a developer gets from Dummy Run. A real browser against your preview, a JSON run record per run, OpenTelemetry traces and headers that join your own telemetry.
---

## What runs where

[The plan](../../../docs/PLAN.md) gives the `dummyrun` command three subcommands: `run`, `review` and `board`. None of them ships yet. A run will drive Chrome or Firefox through WebDriver BiDi, so one session layer covers both browsers. Mobile is a viewport and user agent profile per persona. The first target adapter takes a plain URL. Adapters for GitHub Deployments, Vercel, Netlify and Cloudflare Pages follow. Each one finds the preview for a pull request and creates a test user with a named role. It also loads fixtures.

The [architecture](../../../docs/ARCHITECTURE.md) plans four runner hosts. The runner can sit on your laptop or in GitHub Actions. It can run as a Kubernetes job from the Helm chart in `deploy/helm/dummyrun`, or as a Cloud Run or Azure Container Apps job. `deploy/compose` will run the whole thing on one machine.

## What you get back

Every run writes one run record. It is JSON that validates against [schema/run-record.schema.json](../docs/run-record.md) and sits next to its screenshots. The record lists each step with the action and the target it used. It adds the page URL, the HTTP status and the duration. It counts console errors and accessibility violations. The results board is static HTML and JSON built from those records, so you can host it anywhere that serves files.

Every run is also one OpenTelemetry trace. The root span is `dummyrun.run` and each step is a child span such as `dummyrun.step click`. [Telemetry](../../../docs/TELEMETRY.md) is the contract between the runner, the exporter and the dashboards.

## Joining your own traces

The runner adds a W3C `traceparent` header and a `baggage` header carrying `dummyrun.run.id` and `dummyrun.persona.id` to every request it sends to your hosts. An app instrumented with OpenTelemetry joins the persona's trace. The browser's user agent ends with `DummyRun/<version>`, so frontend tools can filter persona sessions out of real-user data. The [Grafana](../docs/grafana.md) page shows the dashboards built on this.

## Models

The persona and the reviewer are two separate model roles. Providers are Vertex AI, Azure AI Foundry, Anthropic and any OpenAI-compatible local server. A market profile such as `uk` lists the regions that satisfy it, and a run refuses to start if any adapter in the chain cannot.

## Contributing

Until the engine lands, contributions are issues. Propose a persona or a target adapter, or point out a problem with [the plan](../../../docs/PLAN.md). The checks that run on every pull request are in [Contributing](../../../CONTRIBUTING.md), and commits are signed off with `git commit -s`.
