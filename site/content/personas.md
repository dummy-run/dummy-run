---
title: Persona library
wide: true
lead: Ready-made personas for the users every site has. Start from these, then add the personas that are specific to your app.
description: The Dummy Run persona library. Ready-made personas for the users every site has, from a screen-reader user to the viewer, editor and admin pattern.
next:
  title: Use them
  text: The library lives in the repository under casts/library, one folder for each persona.
  label: Open the library on GitHub
  repo: casts/library
library:
  - name: Screen-reader user
    does: Moves by headings, landmarks and link text and never looks at the picture.
    finds: Controls with no name, headings out of order, and focus lost after a dialog closes.
  - name: Keyboard-only user
    does: Uses only the keyboard, with no mouse and no touch.
    finds: Controls that Tab skips, focus you cannot see, and menus you cannot leave.
  - name: Phone on a slow connection
    does: Works on a 390 pixel screen with touch and a slow network.
    finds: Buttons under the keyboard, small tap targets and pages that time out.
  - name: 200 percent zoom
    does: Browses a laptop screen at 200 percent browser zoom.
    finds: Cut-off text, sideways scrolling, overlapping controls and sticky bars that cover the page.
  - name: First-time visitor
    does: Knows nothing about your product and gives it five minutes.
    finds: Jargon, no clear first step, and empty screens that do not say what to do.
  - name: Non-native English reader
    does: Reads English as a second language and translates long words.
    finds: Idioms, long sentences, unexplained terms and dates in a local format.
  - name: Older adult
    does: Reads at a larger text size, takes time over each screen and avoids surprises.
    finds: Small text, low contrast, time limits and controls that appear only on hover.
  - name: Viewer, editor and admin
    does: Three personas on the same app, one for each permission level.
    finds: A viewer who can edit, an editor who can reach admin pages, and controls that show and then refuse.
---

Every site has screen-reader users, phone users and people who have never seen it before. The library gives you a persona for each, so a first run covers them without writing a cast from scratch.

## The library

{{< library >}}

## How to use a library persona

A cast is a folder of personas. Copy a persona into your own cast and write it a scenario for a job in your app, such as "renew a loan" or "change a password". The persona brings its way of using a page and your scenario brings the job.

## What it does not replace

A persona is a model. The screen-reader persona is told to ignore the picture and work from the page's text and controls. It catches missing names and a broken reading order. It does not replace a session with a real screen reader and a person who uses one every day.

## The permission pattern

Viewer, editor and admin are three personas rather than one. Run all three through the same scenario, and any difference in what each can reach is a [permission finding](for/compliance-and-security.md).
