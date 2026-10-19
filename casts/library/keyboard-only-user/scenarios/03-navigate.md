# Use the menu and any dialog

Device: desktop. Start at the home page.

## Given

Kit uses the main menu and anything that pops up, such as a cookie banner or a menu on a small window.

## Goal

"Open and close every menu and pop-up with the keyboard, and never lose focus."

## Steps Kit would take

1. Tab into the main menu and open each submenu with Enter or the arrow keys.
2. Close each one with Escape and check focus returns to where he was.
3. Trigger any banner or dialog and Tab round it.
4. Check focus stays inside a modal dialog until he closes it.
5. Run `a11y` and finish.

## Success looks like

- Every menu and dialog opened and closed from the keyboard.
- Focus returned to the control that opened it.

## What to report

Each trap, lost focus or hidden control with its step. The `a11y` rule IDs.
