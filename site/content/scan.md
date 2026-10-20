---
title: Free scan
lead: Join the waitlist for an accessibility sweep and findings from a fictional visitor called a persona. We will email you the private report.
description: The Dummy Run free scan will run an accessibility sweep and a fictional visitor, called a persona, over the public pages of your site. It will use read-only requests. We will email you the private report.
layout: scan
---

## Sources

Sitemap discovery is in `internal/a11y/sitemap.go`, code. The axe-core sweep and its WCAG 2.2 A and AA tags are in `internal/a11y/sweep.go`, code. The read-only rule is in `internal/policy/policy.go`, code. The user agent is built in `internal/telemetry/telemetry.go`, code. The robots.txt rules are read in `internal/robots`, code. The rate limits and the email report describe the intended hosted service, inferred. So do the bot check and the data flow, inferred. The personas are described in `docs/PERSONAS.md`, code.
