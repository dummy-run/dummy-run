---
title: For executives
role: Executive
icon: chart
lead_short: One report, with evidence
weight: 60
lead: A clear report of what's broken, who it affects and the screenshot that shows it, at a cost you can see run by run.
description: What you get from Dummy Run as an executive. A report with evidence behind each finding, the cost of each run, and an Apache-2.0 licence.
next:
  title: See how a run works
  text: How a persona's visit to your app becomes a finding you can act on.
  label: Read how it works
  href: /how-it-works/
---

## One report, with the evidence

{{< graphic name="board" >}}

Each finding names the problem, how serious it is and which kind of user hit it, and a screenshot sits behind every one. By default the report holds only findings someone on your team has checked, and anything unreviewed that they choose to include is clearly marked. The report also says plainly that it's a sample of your product and no kind of certificate. It's a single HTML file, so it can go round by email or sit behind your own login. A results board that gathers findings from every run is coming soon.

## What a run costs

{{< graphic name="cost" >}}

Every run records its model calls, tokens and cost in US dollars, so there are no surprises on the bill. These two example runs cost four cents and thirty-two cents. The [Grafana](../docs/grafana.md) dashboards show spend by model over time.

## What you take on

The code is open source under [Apache-2.0](../../../docs/LICENSING.md), and the telemetry is standard OpenTelemetry, so it can go into whatever monitoring you already pay for. Usually a QA lead or a developer runs it, and compliance and product read the results. Nothing is filed for you, so no finding becomes a ticket without someone deciding it should.
