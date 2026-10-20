---
title: Free scan
lead: An accessibility sweep and the findings of a persona, a fictional visitor, in a private report by email. Join the waitlist.
description: The Dummy Run free scan will run an accessibility sweep and a persona, a fictional visitor, over the public pages of a site. It is read-only and the report comes by email.
layout: scan
---

## Sources

Sitemap discovery is in `internal/a11y/sitemap.go`, code. The axe-core sweep and its WCAG 2.2 A and AA tags are in `internal/a11y/sweep.go`, code. The read-only rule is in `internal/policy/policy.go`, code. The user agent is built in `internal/telemetry/telemetry.go`, code. The robots.txt rules are read in `internal/robots`, code. The rate limits and the email report describe the intended hosted service, inferred. So do the bot check and the data flow, inferred. The personas are described in `docs/PERSONAS.md`, code.
