# Find and read a fact with larger text

Device: phone. Start at the home page.

## Given

Ivy wants one fact about [the product] and reads slowly at 200 percent text size.

## Goal

"Find [the fact] and read it without sideways scrolling or cut-off text."

## Steps Ivy would take

1. Read the first screen. Say whether the text is large and whether any control overlaps another.
2. Open the menu if there is one and say whether every item is readable.
3. Follow the link to the fact.
4. Read the page from top to bottom, scrolling only down.
5. Run `a11y` and keep the layout rule IDs. Finish.

## Success looks like

- Each line of text fitted the screen.
- No button, link or menu item was cut off or covered by another.

## What to report

Pages that needed sideways scrolling. Controls that overlapped or lost words. The `layout:` lines and the `a11y` rule IDs.
