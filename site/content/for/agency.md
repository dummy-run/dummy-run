---
title: For agencies
role: Agency
icon: buildings
lead_short: Every client, in the right region
weight: 30
lead: Run the same personas against every client's site, keep each client's data in the right region and hand over a report they can read.
description: What you get from Dummy Run as an agency. One cast of personas for every client, the model in the region each client needs, a report to hand over and a licence that allows client work.
next:
  title: Check the licence first
  text: Apache-2.0 lets you run Dummy Run for paying clients. The trademark policy covers how you can use the name.
  label: Read the licence
  href: /docs/licensing/
---

## One cast, every client

{{< graphic name="persona-card" who="lin-contributor" >}}

A cast is a set of personas, and you can reuse the same one across clients. Run the screen-reader user, the keyboard-only user and the person on a phone against every client's preview, then add personas for the roles a particular client has, like Lin here, a contributor who should only ever see what Lin is owed. The [persona library](../personas.md) is a good place to start.

## Each client's data stays where it should

{{< graphic name="residency" >}}

The persona's model sees your client's pages, so where that model runs matters. You pick the provider and the region for each run, so a UK client's runs can stay on a London deployment and an EU client's on one inside the EU. Every run record notes the region the model ran in, which gives you something concrete to show the client. Market profiles that refuse to start a run outside an agreed region are coming soon. [Compliance and security](compliance-and-security.md) covers what else leaves your network.

## A report for the client

`dummyrun report` turns a set of runs and an accessibility sweep into one HTML file you can send as it is or print to PDF. It sets out what was tested, the findings by severity with their steps and screenshots, and what the report doesn't claim. Give it last month's report in JSON and it marks each finding as new, still there or fixed, which makes the next conversation with the client a lot easier. The [report guide](../../../docs/REPORT.md) shows a full example.

## You stay in charge

{{< graphic name="board" >}}

Nothing reaches a client's backlog unless you put it there. You review each finding before it goes in a report, and you decide which ones become tickets. Apache-2.0 lets you [run Dummy Run for clients](../../../docs/LICENSING.md) without asking anyone, and the [trademark policy](../../../TRADEMARKS.md) explains how you can use the name.
