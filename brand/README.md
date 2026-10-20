# Brand

## Logo

The Viewfinder mark is a crash-test target framed by camera corners. Keep the corners: without them the
target is a generic crash-test symbol.

| File | Use |
|---|---|
| `logo/mark.svg` | The mark on light backgrounds |
| `logo/mark-mono-black.svg` and `logo/mark-mono-white.svg` | One-colour printing and embossing |
| `logo/lockup-horizontal.svg` and `logo/lockup-stacked.svg` | The mark with the name, on light backgrounds |
| `logo/lockup-horizontal-dark.svg` and `logo/lockup-stacked-dark.svg` | The same on dark backgrounds |
| `logo/readme-header-light.svg` and `logo/readme-header-dark.svg` | The README header |
| `logo/favicon.svg` and `logo/favicon-*.png` | Favicons at 16, 32, 180 and 512 pixels |
| `logo/social-card.png` | Link previews at 1280 by 640 pixels |

The wordmark is Space Grotesk 600, converted to outlines. Space Grotesk is licensed under the SIL Open
Font License 1.1.

## Mascot

Dummy is a friendly amber crash-test dummy with a quartered target on its temple, the same target as the
Viewfinder mark. Dummy is the persona's stand-in: curious, a little clumsy and always willing to try again.

Use Dummy in docs, empty states and error pages. Keep it out of the logo. The files are in
`mascot/`. The cautions about real crash-test dummies and their trademarks are in
[docs/NAMING.md](../docs/NAMING.md).

`docs/images/brand/mascot-sheet.png` is a contact sheet of every pose on light and dark backgrounds.

### Poses

| File | Use it for |
|---|---|
| `mascot/dummy-friendly.svg` | The default. Welcomes, home page, About |
| `mascot/dummy-thinking.svg` | Loading, a run in progress, a question |
| `mascot/dummy-found-it.svg` | A finding, a search result, a success after looking |
| `mascot/dummy-oops.svg` | Errors and the 404 page. Sheepish, never hurt |
| `mascot/dummy-thumbs-up.svg` | Success, such as a confirmed sign-up |
| `mascot/dummy-reading.svg` | Reports, review and documentation |
| `mascot/dummy-keyboard.svg` | Keyboard-only personas and loading states |
| `mascot/dummy-head.svg` | Avatars, list bullets and anything under 64 pixels tall |
| `mascot/dummy-friendly-mono-black.svg` and `-mono-white.svg` | One-colour printing and embossing |
| `mascot/dummy-head-mono-black.svg` and `-mono-white.svg` | The head in one colour |
| `mascot/dummy.svg`, `dummy-mono-black.svg`, `dummy-mono-white.svg` | Legacy. The first, deadpan head. Do not use in new work |

The poses share one body, so they sit at the same size and can swap in place. Use one pose per page and
keep Dummy beside the text and never behind it.

### Sizes and clear space

- Full-body poses: at least 96 pixels tall. Below that, use `dummy-head.svg`.
- Head: at least 24 pixels wide. It is drawn to stay readable at that size.
- Leave clear space on every side equal to the width of Dummy's hand, about a tenth of the figure's width.
  Keep text and other graphics out of it. The sparkles and thought dots count as part of the figure.

### Accessibility

Dummy is decoration in nearly every use, so give the image an empty `alt=""`. Write alt text only when the
picture adds meaning the surrounding text does not, for example "Dummy shrugging" next to a 404 message
that says nothing of the sort. Each file has a `<title>` for when it is opened on its own.

- As an image, `<img src="dummy-oops.svg" alt="">`, or a described `alt` when it adds meaning.
- Inline, keep `role="img"` and `aria-labelledby` pointing at the `<title>`, and change the `id`
  (`t` in the files) so two inlined copies do not clash. For decoration, drop the `<title>` and use
  `aria-hidden="true"` instead.

### Do and don't

- Do keep the colours: amber `#fbad18`, its one darker shade `#db8a06`, ink `#24221e` and paper
  `#f3f1ec`. The darker shade is for depth only.
- Do keep the target on the temple. It is what makes Dummy a crash-test dummy.
- Do tilt the head a few degrees for expression, as the poses do.
- Don't make Dummy look injured, hurt or in pain. No bandages, cracks, bruises or tears. Oops is
  sheepish and undamaged.
- Don't use Dummy for anything that suggests a person was harmed.
- Don't use Dummy for real accidents, injuries or crashes.
- Don't change the colours, add outlines or gradients, or recolour Dummy to match a theme. Use a
  one-colour file when you need one colour.
- Don't stretch, rotate the whole figure, or redraw the face.

Dummy is a fictional character and stands for the personas. It does not stand for disabled people.

## Colour

Amber is for graphics on light backgrounds. Its contrast against white is 1.89:1, too low for text, so
light-theme text uses the darker text-amber instead.

| Role | Light theme | Dark theme |
|---|---|---|
| Amber (graphics) | `#fbad18` | `#fbad18` |
| Text-amber | `#8a5a00` (5.93:1) | `#fbad18` (9.56:1) |
| Ink | `#1d1d1b` (16.88:1) | `#f3f1ec` (16.03:1) |
| Paper | `#ffffff` | `#161616` |
| Pass | `#1a7f37` (5.08:1) | `#57d273` (9.37:1) |
| Blocker | `#cf222e` (5.36:1) | `#ff7b72` (7.18:1) |

Ratios are WCAG 2 contrast against the paper of the same theme. Every text colour meets AA.

## Type

Sizes and line heights are in [docs/TYPOGRAPHY.md](../docs/TYPOGRAPHY.md).
