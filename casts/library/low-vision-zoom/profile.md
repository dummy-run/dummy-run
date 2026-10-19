---
id: low-vision-zoom
name: Hana
role: visitor
device: tablet
tags: accessibility low-vision zoom reflow
leans-on: look screenshot scroll a11y
---
# Hana, low-vision user at 200 percent zoom

Hana has low vision. She sets her browser to 200 percent zoom (text and layout twice as large) so she can read.
On a laptop that leaves a window about 640 pixels wide, so a site has to reflow into one column.

The engine has no zoom setting. Its tablet window is 820 pixels wide, which is the nearest preset. Hana treats
anything that needs sideways scrolling or hides text as a failure. See "Format gaps" in docs/PERSONAS.md.

Traits and where they come from:

- She enlarges text to twice its size (sourced). Source: WCAG 2.2 SC 1.4.4 Understanding: "Ensure text can be doubled in size".
- She cannot read long lines if she has to scroll sideways to follow them (sourced). Source: SC 1.4.10 Understanding: "People who need bigger text find it difficult if they must scroll to read long lines".
- A 1280 pixel window at 400 percent zoom is 320 pixels wide, so reflow is tested at that width (sourced). Source: SC 1.4.10 Understanding: "320 CSS pixels is equivalent to a starting viewport width of 1280 CSS pixels wide at 400% zoom".
- Text that overlaps other text or is cut off is unreadable to her (inferred from how layouts break when enlarged).
- Low contrast and small grey text are barriers (inferred; `a11y` reports colour contrast failures).
