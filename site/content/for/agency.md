---
title: For agencies
choice: at an agency
weight: 30
lead: One cast run against every client's preview, with each client's data kept in its market.
description: What an agency gets from Dummy Run. One cast of personas run against every client's preview, data residency per market, and a licence that allows client work.
---

## One cast, many clients

A cast is a set of personas with roles, goals, habits and devices. The example cast has four. `ada-editor` works on a desktop, `sam-author` on a phone, `ravi-reviewer` on a desktop and `lin-contributor` on a tablet. Run the same cast against each client's preview and every client gets the same kind of coverage. A client with its own roles gets its own personas, and the [persona proposal template](https://github.com/dummy-run/dummy-run/issues/new/choose) shows what one needs.

## Each client's data stays in its market

A market profile such as `uk` lists the regions that satisfy it, for example `europe-west2`, `uksouth` and `ukwest`. Every adapter declares the regions it can run in. A run refuses to start if the target, the model provider or the runner host cannot satisfy the market. Model providers are chosen per market. The choices are Vertex AI, Azure AI Foundry, Anthropic and an OpenAI-compatible local model. The run record stores the region the model ran in as `provider.region`, so you can show a client where their run happened.

## What the client sees

The results board is static HTML and JSON. Each finding shows a title and a severity. It names the persona and role affected, the steps involved and the screenshot. Nothing is posted to a tracker for you. You decide which findings go into the client's backlog.

## The licence allows client work

Dummy Run is under Apache-2.0. The [licensing](/docs/licensing/) table says you may run it for clients with no conditions. You may also change it for your own use. Offering it as a hosted service is allowed under a name other than Dummy Run, and the [trademark policy](/docs/trademarks/) explains the name and logo.

## What to read next

[Architecture](/docs/architecture/) covers the adapters and the data residency rule. [Security](/docs/security/) says what to do with a finding in an app you do not own. The project is pre-release, and [the plan](/docs/plan/) lists the milestones.
