---
title: For product and UX
role: Product and UX
icon: cursor
lead_short: Friction, with the screenshot
weight: 40
lead: See where people get stuck or confused, each moment tied to the step and the screenshot where it happened.
description: What you get from Dummy Run in product and UX. Friction, confusion and accessibility problems tied to a step and a screenshot, from personas with their own habits and devices.
next:
  title: Start from personas that already exist
  text: The library has a screen-reader user, someone on a phone, a first-time visitor and seven more.
  label: Browse the persona library
  href: /personas/
---

## Friction, with the screenshot

{{< graphic name="finding-card" >}}

Sam, an author on a phone, tried to reply to an editor's comments and found the Submit button hidden under the keyboard. That's a friction finding: nothing crashed, but a real author would have given up or gone looking for a laptop. It points at step 6 of run [`2026-10-17-sam-author-006`](../../../examples/runs/2026-10-17-sam-author-006.json), so you can open the screenshot and see it for yourself. Findings come in five kinds. Friction, question and accessibility are about the experience, while bug and permission are about the code.

## Personas have devices and habits

{{< graphic name="persona-card" who="sam-author" >}}

The device sets the window size, so the same job on a phone and on a desktop can turn up quite different problems. A persona can also read at 200 percent zoom, which narrows the page, or at a larger system text size, which leaves the window as it is and makes the text bigger. It can use only the keyboard, or have very little patience with long pages. Write your own personas in Markdown, or start from the library and adjust them to your product.

## Questions are findings too

{{< graphic name="board" >}}

Sometimes the most useful thing a persona says is that it didn't understand. In run [`2026-10-16-lin-contributor-002`](../../../examples/runs/2026-10-16-lin-contributor-002.json), Lin, a contributor, couldn't work out what the payment status "settled" meant. Nothing was broken, but it's exactly the sort of label that fills a support inbox. The personas are fictional, and no real user is watched or recorded.
