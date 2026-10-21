---
title: That did not work
lead: Your audit request didn't go through.
description: Your audit request didn't go through.
layout: audit-result
mascot: dummy-oops
noindex: true
sitemap:
  disable: true
back:
  label: Back to the form
  href: /audit/
codes:
  - code: invalid_request
    text: Something was missing from the form, or a field was too long. Check each field and try again.
  - code: invalid_email
    text: The email address didn't look right. Check it for typos.
  - code: consent_required
    text: The consent box wasn't ticked. We need it before we can reply to you.
  - code: turnstile
    text: The human check didn't pass. Reload the page and try again.
  - code: rate_limited
    text: You or your network tried too many times. Wait an hour and try again.
  - code: unavailable
    text: The form isn't taking requests right now. Try again later.
---

The likely cause is below. Fix it and try again from the [audit request page](_index.md). If it keeps failing, just email hello@dummyrun.dev and tell us what you need.
