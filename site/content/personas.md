---
title: Persona library
wide: true
mascot: dummy-friendly
lead: Eleven ready-made personas for the people most sites have. Pick the ones you're most worried about and run them today.
description: The Dummy Run persona library. Eleven ready-made personas for the people most sites have. Filter them by need and copy the command for each one.
next:
  title: Adapt one to your app
  text: Copy a persona into your cast and write a scenario for a job in your product.
  label: Read the persona guide
  href: /docs/personas/
---

Most sites have screen-reader users, people on phones and people who have never seen them before. Each persona below brings one way of using a page, and you bring the job, such as "renew a loan" or "change a password". Run the keyboard-only user through your checkout, and you'll find out quickly whether the checkout works without a mouse.

A persona simulates a barrier so you can catch likely problems early. It doesn't replace testing with disabled people, and [Ethics and limits](../../docs/ETHICS.md) explains what it can and can't tell you.

{{< try-scan >}}

## Pick a persona

{{< library >}}

## Make one your own

1. Copy the persona's folder from [casts/library](../../casts/library/README.md) into your own cast.
2. Write a scenario for a job in your app.
3. Start from the command on its card. Change `casts/library` to your own cast's folder, and for the viewer, editor and admin point `--target` at a test environment with sign-in details. If you haven't installed Dummy Run yet, [get started](start.md) first. The [persona guide](../../docs/PERSONAS.md) covers the setup.

The viewer, editor and admin work best together. Run all three through the same job, and anything one of them can reach that it shouldn't is a [permission finding](for/compliance-and-security.md).
