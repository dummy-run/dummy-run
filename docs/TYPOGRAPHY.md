# Type scale and minimum sizes

**Design note** · 18 October 2026 · maintainers

> - Body text 17px with a 1.65 line height; nothing a person is expected to read is below 15px.
> - Headings scale with the viewport through clamp(), not breakpoints.
> - The typeface is open; the sizes are not. Small type is the most common tell of an unreviewed interface.


## Tokens (`:root`, 1rem = 16px)

| Token | Value | At 16px root |
|---|---|---|
| --type-display | clamp(2.5rem, 2.05rem + 2.2vw, 3.75rem) | 40px to 60px |
| --type-display-sm | clamp(2.25rem, 1.9rem + 1.75vw, 3.25rem) | 36px to 52px |
| --type-title | clamp(1.875rem, 1.66rem + 1.05vw, 2.5rem) | 30px to 40px |
| --type-title-sm | clamp(1.5rem, 1.38rem + 0.6vw, 1.875rem) | 24px to 30px |
| --type-subtitle | clamp(1.25rem, 1.18rem + 0.34vw, 1.4375rem) | 20px to 23px |
| --type-lead | clamp(1.125rem, 1.08rem + 0.22vw, 1.25rem) | 18px to 20px |
| --type-body | 1.0625rem | 17px |
| --type-small | 0.9375rem | 15px |
| --type-caption | 0.8125rem | 13px |
| --type-eyebrow | 0.75rem (uppercase, 0.12em tracking, 600 weight) | 12px, the only sub-13px size and only for all-caps labels |

`--font-size-base: 1rem` is set and never shrunk. `html` keeps `-webkit-text-size-adjust: 100%`.

## Composed classes

| Class | Font | Size | Line height | Tracking | Weight |
|---|---|---|---|---|---|
| .marketing (page wrapper) | Lexend, system fallback | --type-body | 1.65 | | antialiased, optimizeLegibility |
| .u-display | Poppins | --type-display | 1.02 | -0.035em | 700 |
| .u-display-sm | Poppins | --type-display-sm | 1.06 | -0.03em | 700 |
| .u-title | Poppins | --type-title | 1.14 | -0.022em | 700 |
| .u-title-sm | Poppins | --type-title-sm | 1.22 | -0.016em | 600 |
| .u-subtitle | Poppins | --type-subtitle | 1.32 | -0.012em | 600 |
| .u-lead | body font, muted ink | --type-lead | 1.55 | | text-wrap: pretty |
| .u-eyebrow | Poppins | --type-eyebrow | 1.4 | +0.12em | 600, uppercase |
| .text-body | | --type-body | 1.65 | | |
| .text-small | | --type-small | | | |
| .text-caption | | --type-caption | 1.5 | | |

Fonts in the reference scale: Lexend for running text, Poppins for headings and labels, with fallback metrics. Small UI chrome (buttons, navigation) may use 14px; reading copy never does.

## Rules to carry over
0. The typeface is free to change (author's instruction, 2026-10-18). What must carry over is the sizing, line heights and the minimums below, not Lexend or Poppins.
1. Body copy 17px / 1.65, never 14px or 16px by default.
2. Smallest reading size 15px; 13px only for captions and metadata; 12px only for uppercase eyebrows.
3. Headings scale with the viewport via clamp(), not breakpoints, with tight negative tracking and line-height between 1.02 and 1.32.
4. Lead paragraphs are a step up from body (18 to 20px) and muted, not bold.
5. Do not set a root font-size below 16px and do not use `text-size-adjust: none`.
