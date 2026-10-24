# Tools we build on

Dummy Run stands on other people's open-source work. This page names the tools the accessibility checks bundle or
run, and the data taken from them, with what each does for you and its licence. The full licence texts are in [LICENSES](../LICENSES) and next to each bundled copy,
and [NOTICE](../NOTICE) has the copyright lines.

## Rule engines

These run inside the page you test, so their results describe the page as your visitors' browsers build it.

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [axe-core](https://github.com/dequelabs/axe-core) by Deque Systems | 4.13.0 | The main accessibility rules, in the sweep and the `a11y` tool | MPL-2.0 |
| [IBM Equal Access accessibility-checker-engine](https://github.com/IBMa/equal-access) | 4.0.34 | A second set of WCAG rules, with `--engines` | Apache-2.0 |
| [HTML_CodeSniffer](https://github.com/squizlabs/HTML_CodeSniffer) by Squiz | 2.5.1 | WCAG 2 AA rules on the markup, with `--engines` | BSD-3-Clause |
| [Siteimprove Alfa](https://github.com/Siteimprove/alfa) | 0.120.0 | Rules written against the W3C ACT test cases, with `--engines` | MIT |

Alfa is bundled into one script with [esbuild](https://github.com/evanw/esbuild) (MIT), which is used only to
build it. The bundle includes [Color.js](https://github.com/color-js/color.js) (MIT), which Alfa uses for its colour
calculations. `internal/page/engines/alfa/build.sh` rebuilds it from the pinned versions.

## Cookie banners

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [DuckDuckGo autoconsent](https://github.com/duckduckgo/autoconsent) | 16.61.0 | Answers cookie banners by rejecting, with `--consent`, and sorts their buttons so a banner with no reject button can be reported | MPL-2.0 |

The bundle and its compact rule set are used as published, unchanged.

## Screen reader view

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [guidepup virtual screen reader](https://github.com/guidepup/virtual-screen-reader) by Craig Morten | 0.33.0 | Reads the page aloud as text for a persona that gets no screenshots | MIT |

It's bundled into one script with esbuild. Its browser build already includes these packages, and NOTICE has their
copyright lines:

- under MIT: @testing-library/dom, @testing-library/user-event, dom-accessibility-api, html-aria, lz-string,
  pretty-format, react-is, ansi-regex, ansi-styles and dequal;
- under Apache-2.0: aria-query.

`internal/page/screenreader/build/build.sh` rebuilds it from the pinned versions.

## Plain language

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [Vale](https://github.com/vale-cli/vale) by Joseph Kato | 3.24.0 in the container image | Checks the main content of each page with `--plain-language`. It runs as its own program | MIT |
| [Vale Readability style](https://github.com/vale-cli/Readability) | as published | The Flesch reading ease and Flesch-Kincaid grade rules, used unchanged | MIT |

The long sentence, words to avoid and Latin abbreviation rules are our own, written from the public
[GOV.UK A to Z style guide](https://guidance.publishing.service.gov.uk/writing-to-gov-uk-standards/style-guides/a-to-z-style-guide/).

## Technology detection

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [webappanalyzer](https://github.com/enthec/webappanalyzer) by Enthec | your copy, with `DUMMYRUN_WEBAPPANALYZER` | Its technology fingerprints, read from your copy at run time to fill `target.tech`. Dummy Run doesn't bundle or copy them, and its own fingerprints are written separately | GPL-3.0 |

## Methods we learned from

Some checks follow a method another project worked out, written again in our own code.

| Project | What we took | Licence of the project |
|---|---|---|
| [contrastcap](https://github.com/ICJIA/contrastcap-mcp) by ICJIA | Measuring text contrast over an image by hiding the text, taking a picture of what is behind it and comparing every pixel, for `--image-contrast` | MIT |

## Data

The list of WCAG 2.2 success criteria at levels A and AA, with their titles and links, was generated from the
criteria data in Siteimprove Alfa (MIT).
