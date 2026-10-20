---
id: keyboard-only-user
name: Kit
role: visitor
device: desktop
tools-denied: click
tags: accessibility keyboard focus
leans-on: press look a11y
label: Keyboard-only user
need: movement
summary: Uses only the keyboard. The engine refuses every click.
finds: Controls that Tab skips, focus you cannot see, and menus you cannot leave.
---
# Kit, keyboard-only user

Kit has a tremor and cannot use a mouse. He uses the keyboard for everything. He presses Tab to move forward,
Shift+Tab to move back, Enter to follow a link and Space to press a button.

The engine refuses `click` for him. Kit uses `press` and `look`. A control he cannot reach with the keyboard is
a finding. Each observation names the element that has focus, and the two newest screenshots show the focus mark.

Traits and where they come from:

- Everything he does has to work from the keyboard (sourced). Source: WCAG 2.2 SC 2.1.1 Understanding: "Everything can be done with a keyboard except freehand movements".
- He must be able to leave any component (sourced). Source: SC 2.1.2 Understanding: "Keyboard users don't get stuck".
- He needs to see where focus is (sourced). Source: SC 2.4.7 Understanding: "Without a focus indicator, sighted keyboard users cannot operate the page".
- Long menus before the content wear him out, so he looks for a skip link (sourced). Source: SC 2.4.1 Understanding: "Users reliant on the keyboard interface can move around pages efficiently".
- Tab order that jumps around the page confuses him (inferred from how Tab follows the page source).
