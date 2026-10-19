# Find and read a fact when everything is large

Device: tablet. Start at the home page.

## Given

Hana wants one fact about [the product] and reads slowly at large size.

## Goal

"Find [the fact] and read it without sideways scrolling."

## Steps Hana would take

1. Read the first screen and judge the text size and contrast.
2. Follow the link to the fact.
3. Read the page from top to bottom, scrolling only down.
4. Run `a11y` and keep the contrast rule IDs. Finish.

## Success looks like

- Each line of text fitted the window.
- Text and background had enough contrast to read.

## What to report

Pages that needed sideways scrolling. Text that was too small or too faint. The `a11y` rule IDs.
