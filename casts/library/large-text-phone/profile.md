---
id: large-text-phone
name: Ivy
role: visitor
device: phone
text_scale: 200
tags: accessibility low-vision mobile text-size
leans-on: look scroll a11y screenshot
label: Phone with larger system text
need: vision
summary: Reads on a phone whose system text size is set to 200 percent, with the page not zoomed.
finds: Overlapping or clipped buttons, text that runs off the screen and controls that lose their labels when text grows.
limit: The engine raises the browser's default font size. A real phone may also scale text that a site sizes in px.
---
# Ivy, phone user with larger system text

Ivy has low vision. She keeps the text size on her phone at 200 percent so she can read without a magnifier.
She does not pinch to zoom. The page keeps its normal width and only the text grows, so a layout built for
small text can break.

The engine raises the browser's default font size to 200 percent and keeps the phone window at 375 pixels
wide. Text that a site sizes in rem or em grows. Text it sizes in px may not, so a page can look half enlarged.
Ivy reports what she sees and does not guess which one the site used.

Traits and where they come from:

- She needs text to be readable at twice its normal size without extra software (sourced). Source: WCAG 2.2 SC 1.4.4 Understanding: "Ensure text can be doubled in size".
- Content must not be lost or cut off when text grows (sourced). Source: SC 1.4.4 Understanding: "without loss of content or functionality".
- She does not want to scroll sideways to read a line (sourced). Source: SC 1.4.10 Understanding: "People who need bigger text find it difficult if they must scroll to read long lines".
- She taps with a thumb. A button that overlaps another, or one that is too small, causes mistakes (sourced). Source: SC 2.5.8 Understanding: "target sizes are large enough for users to easily activate them".
- Larger text pushes labels onto more lines, and a fixed height box then cuts them off (inferred from how layouts break when text grows).
- A fixed bar at the top or bottom takes more of a small screen when its text is large (inferred).
