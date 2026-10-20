---
title: That did not work
lead: The audit request did not go through.
description: The audit request did not go through.
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
    text: The form was missing something or had a field that was too long. Check every field.
  - code: invalid_email
    text: The email address did not look right. Check it for typos.
  - code: consent_required
    text: The consent box was not ticked. We need it to reply to you.
  - code: turnstile
    text: The human check did not pass. Reload the page and try again.
  - code: rate_limited
    text: You or your network tried too many times. Wait an hour and try again.
  - code: unavailable
    text: The form is not taking requests right now. Try again later.
---

Find the likely cause below and try again from the [audit request page](_index.md). If it keeps failing, write to hello@dummyrun.dev with what you need.
