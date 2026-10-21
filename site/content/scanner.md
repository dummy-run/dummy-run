---
title: Dummy Run visited my site
lead: If you've spotted DummyRun in your logs, here's who we are, what the visit was and how to stop it.
description: Dummy Run is an open-source tool that plays fictional users on a website. This page explains its user agent, how to block it with robots.txt and how to object.
next:
  title: What does the free scan do?
  text: The free scan reads the public pages of a site and checks them for accessibility problems. It never changes anything.
  label: About the free scan
  href: /scan/
---

## Who we are

Dummy Run is an open-source tool that plays fictional users on a website. A persona tries to get something done, like finding the pricing or signing up, and reports what got in its way. Most people use it on their own sites before a release. The [source is on GitHub](https://github.com/dummy-run/dummy-run) if you'd like to see exactly what it does.

## What you saw

The visit ended its user agent with this.

```text
DummyRun/<version> (+https://dummyrun.dev/scanner/)
```

The browser's own user agent comes first, so your site renders just as it would for any other visitor using that browser.

## Why it came

Someone ran the open-source tool against your site. When our hosted free scan opens, a visit may also mean someone asked it to check one of your public pages.

The hosted scan only loads public pages, and it only sends GET, HEAD and OPTIONS requests. It's rate limited and it follows robots.txt. [The free scan page](scan.md) has the detail.

Because the engine is open source, anyone can run it, and they can change the user agent. The identifying string is a courtesy that our hosted scan and our own runs always keep. We can't promise the same for copies other people run.

## Block it with robots.txt

Add this to your site's robots.txt and our hosted scan and our own runs will stay away.

```text
User-agent: DummyRun
Disallow: /
```

You can disallow single paths instead. People running the tool themselves only follow robots.txt if they turn on `--respect-robots`, so a self-run visit can still happen. If one does, email abuse@dummyrun.dev with the domain and we'll help where we can.

## Ask us to stop

Write to abuse@dummyrun.dev with your domain and we'll stop scanning it. If you can tell us roughly when the visit happened, we'll also tell you what we find out about it.
