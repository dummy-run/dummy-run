---
title: Free scan
lead: A quick accessibility check and one persona on the public pages of a site, with a short report by email.
description: The Dummy Run free scan will check the public pages of a site for accessibility problems and send one persona through them. It is read-only and it honours robots.txt.
next:
  title: Do not want to wait?
  text: The engine is open source. Run an accessibility sweep or a persona against your own site today.
  label: Get started
  href: /start/
---

The free scan is not open yet. To hear when it opens, [watch the releases on GitHub](https://github.com/dummy-run/dummy-run/releases).

## What it will do

You give it the address of a public page. You pass a bot check and give an email address. Then it does three things.

- It sweeps the public pages it finds, up to a page limit. It takes them from the sitemap, the list of pages a site publishes for crawlers. It tests each page in a real browser with axe-core, an open-source accessibility checker. The rules are WCAG 2.2 levels A and AA. WCAG is the Web Content Accessibility Guidelines.
- It sends at least one persona through the site. The persona reads, follows links and tries to do a first-time visitor's job.
- It emails you a link to a short report. The report is private to the link.

## Who it is for

Anyone who wants a first look at a public site. That includes owners, developers, designers and testers. You do not need an account.

## What it is not

It is not an audit. It is not a statement that your site meets WCAG or any law. It samples pages, so a clean report does not mean the rest of the site is clean.

A persona is a model. It finds likely problems and a person should check each one. [Ethics and limits](../../docs/ETHICS.md) says what a persona can and cannot tell you.

## How it protects your site

- It loads public pages only. It never signs in.
- It is read-only. Only GET, HEAD and OPTIONS requests reach your site. The browser blocks every other request.
- It is rate limited, with caps for each domain and for each email address, and a limit on pages for each scan.
- It identifies itself in the user agent. [The scanner page](scanner.md) shows the string.
- It honours robots.txt. If your file disallows `DummyRun`, no scan starts.
- It stops when you object. Write to abuse@dummyrun.dev and scans of your domain stop.

## What happens to the data

The text and screenshots of the pages it visits go to the model that plays the persona. The privacy page covers the waitlist only. [Read the privacy page](privacy.md).

{{< waitlist >}}
