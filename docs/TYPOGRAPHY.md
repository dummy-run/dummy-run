# Type scale and minimum sizes

Body text is 18px with a 1.65 line height. Nothing a person reads is below 16px. Headings scale with the
viewport through `clamp()`.


## Tokens (`:root`, 1rem = 16px)

| Token | Value | At 16px root |
|---|---|---|
| --type-display | clamp(2.5rem, 2.05rem + 2.2vw, 3.75rem) | 40px to 60px |
| --type-display-sm | clamp(2.25rem, 1.9rem + 1.75vw, 3.25rem) | 36px to 52px |
| --type-title | clamp(1.875rem, 1.66rem + 1.05vw, 2.5rem) | 30px to 40px |
| --type-title-sm | clamp(1.5rem, 1.38rem + 0.6vw, 1.875rem) | 24px to 30px |
| --type-subtitle | clamp(1.25rem, 1.18rem + 0.34vw, 1.4375rem) | 20px to 23px |
| --type-lead | clamp(1.25rem, 1.2rem + 0.25vw, 1.375rem) | 20px to 22px |
| --type-body | 1.125rem | 18px |
| --type-small | 1rem | 16px |
| --type-caption | 0.8125rem | 13px |
| --type-eyebrow | 0.75rem (uppercase, 0.12em tracking, 600 weight) | 12px, the only sub-13px size and only for all-caps labels |

`--font-size-base: 1rem` is set and never shrunk. `html` keeps `-webkit-text-size-adjust: 100%`.

## Composed classes

| Class | Font | Size | Line height | Tracking | Weight |
|---|---|---|---|---|---|
| body (page wrapper) | body face | --type-body | 1.65 | | antialiased, optimizeLegibility |
| .u-display | heading face | --type-display | 1.02 | -0.035em | 700 |
| .u-display-sm | heading face | --type-display-sm | 1.06 | -0.03em | 700 |
| .u-title | heading face | --type-title | 1.14 | -0.022em | 700 |
| .u-title-sm | heading face | --type-title-sm | 1.22 | -0.016em | 600 |
| .u-subtitle | heading face | --type-subtitle | 1.32 | -0.012em | 600 |
| .u-lead | body font, muted ink | --type-lead | 1.55 | | text-wrap: pretty |
| .u-eyebrow | heading face | --type-eyebrow | 1.4 | +0.12em | 600, uppercase |
| .text-body | | --type-body | 1.65 | | |
| .text-small | | --type-small | | | |
| .text-caption | | --type-caption | 1.5 | | |

Buttons and navigation may use 14px. Text people read never does.

## Rules

1. Body text is 18px with a 1.65 line height.
2. The smallest reading size is 16px. Use 13px only for captions and metadata, and 12px only for
   uppercase labels.
3. Headings scale with the viewport through `clamp()`, with tight negative tracking. Their line height
   is between 1.02 and 1.32.
4. Lead paragraphs are 20 to 22px and set in a muted colour.
5. Never set the root font size below 16px, and never use `text-size-adjust: none`.
6. The typeface may change. These sizes and line heights may not.
