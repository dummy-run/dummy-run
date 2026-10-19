---
title: For agencies
role: Agency
icon: buildings
lead_short: Every client, in its market
weight: 30
lead: One cast run against every client's preview, with each client's data kept in its market.
description: What an agency gets from Dummy Run. One cast of personas run against every client's preview, data residency per market, and a licence that allows client work.
next:
  title: Check the licence first
  text: Apache-2.0 lets you run Dummy Run for clients with no conditions. The trademark policy covers the name.
  label: Read the licence
  href: /docs/licensing/
---

## One cast, every client

{{< graphic name="persona-card" who="lin-contributor" >}}

A cast is a set of personas. The example cast has four. Two work on a desktop and the others on a phone and a tablet. Run the same cast against each client's preview, or give a client with its own roles its own personas. The [persona library](../personas.md) holds ready-made personas for the users every site has.

## Each client's data stays in its market

{{< graphic name="residency" >}}

A market profile lists the regions that satisfy it. Every adapter declares its regions, and a run refuses to start if any adapter in the chain cannot satisfy the market. Each run record stores the region the model ran in as `provider.region`. [Compliance and security](compliance-and-security.md) covers exported telemetry.

## The board is theirs

{{< graphic name="board" >}}

The results board is static files. You decide which rows go into the client's backlog. Apache-2.0 lets you [run Dummy Run for clients](../../../docs/LICENSING.md) with no conditions, and the [trademark policy](../../../TRADEMARKS.md) covers the name.

## Sources

The residency rule and the adapters are in the [architecture](../../../docs/ARCHITECTURE.md), sourced. `provider.region` is defined in the [run record schema](../docs/run-record.md), code. The personas are the synthetic cast in `examples/runs/`, measured.
