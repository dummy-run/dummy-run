---
title: Dummy Run visited my site
lead: Who we are, what the visit was and how to stop it.
description: Dummy Run is an open-source tool that plays fictional users on a website. This page explains its user agent, how to block it with robots.txt and how to object.
next:
  title: What does the free scan do?
  text: The free scan checks the public pages of a site for accessibility problems, read-only.
  label: About the free scan
  href: /scan/
---

## Who we are

Dummy Run is an open-source tool that plays fictional users on a website. It tries to do their jobs and reports what got in the way. The [source is on GitHub](https://github.com/dummy-run/dummy-run).

## What you saw

The visit carried this at the end of the user agent.

```text
DummyRun/<version> (+https://dummyrun.dev/scanner/)
```

The browser's own user agent comes first, so your site renders as it does for any visitor of that browser.

## Why it came

A visit comes from someone who ran the open-source tool themselves. Once it opens, it will also come from someone who asked the hosted free scan to check a public page.

The hosted scan loads public pages only and sends only GET, HEAD and OPTIONS requests. It is rate limited and it honours robots.txt. [The free scan page](scan.md) has the detail.

Anyone can run the open-source engine with any user agent. The identifying string is a courtesy of the hosted service and of our own runs. It is not a guarantee about other people's copies.

## Block it with robots.txt

Add this to the robots.txt of your site. Runs that honour robots.txt then stay away. That includes the hosted scan and our own runs.

```text
User-agent: DummyRun
Disallow: /
```

You can also disallow single paths. The open-source tool honours robots.txt only when its user turns on `--respect-robots`, so a self-run visit can still happen. Object as below if one does.

## Object

Write to abuse@dummyrun.dev with your domain. We stop scans of that domain. Tell us the time of the visit if you can, and we will tell you what we find.
