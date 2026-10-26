# Tools we build on

Dummy Run stands on other people's open-source work. Here are the tools the accessibility checks bundle or run, and
the data taken from them, with what each does for you and its licence. The full licence texts are in
[LICENSES](../LICENSES) and next to each bundled copy, and [NOTICE](../NOTICE) has the copyright lines.

## Rule engines

These run inside the page you test, so their results describe the page as your visitors' browsers build it.

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [axe-core](https://github.com/dequelabs/axe-core) by Deque Systems | 4.13.0 | The main accessibility rules, in the sweep and the `a11y` tool | MPL-2.0 |
| [IBM Equal Access accessibility-checker-engine](https://github.com/IBMa/equal-access) | 4.0.34 | A second set of WCAG rules, with `--engines` | Apache-2.0 |
| [HTML_CodeSniffer](https://github.com/squizlabs/HTML_CodeSniffer) by Squiz | 2.5.1 | WCAG 2 AA rules on the markup, with `--engines` | BSD-3-Clause |
| [Siteimprove Alfa](https://github.com/Siteimprove/alfa) | 0.120.0 | Rules written against the W3C ACT test cases, with `--engines` | MIT |
| [QualWeb](https://github.com/qualweb/qualweb) by the University of Lisbon (FCUL) | act-rules 0.8.6, wcag-techniques 0.4.9, qw-page 0.3.8, util 0.7.4 | ACT rules and WCAG techniques, with `--engines` | ISC |

Alfa is bundled into one script with [esbuild](https://github.com/evanw/esbuild) (MIT), which is used only to
build it. The bundle includes [Color.js](https://github.com/color-js/color.js) (MIT), which Alfa uses for its colour
calculations. `internal/page/engines/alfa/build.sh` rebuilds it from the pinned versions.

## Cookie banners

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [DuckDuckGo autoconsent](https://github.com/duckduckgo/autoconsent) | 16.61.0 | Answers cookie banners by rejecting, with `--consent`, and sorts their buttons so a banner with no reject button can be reported | MPL-2.0 |

The compact rule set is used as published. The bundle has one change. A pattern that spots "close" buttons listed
"des cookies" and "de cookies" twice: once on their own and once as an article followed by "cookie". The overlap
let a page make the browser backtrack for a very long time on a crafted button label, so the two extra entries are
removed. When the change was made, the old and new patterns were run side by side on 12 real phrases, such as
"fermer des cookies", and 20,000 random ones. They gave the same answer every time, except on strings like "des de
cookies", an article followed by "de cookies", which only the old one accepts. The change is marked in the file, and
a test checks the bundle still carries it.

## Screen reader view

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [guidepup virtual screen reader](https://github.com/guidepup/virtual-screen-reader) by Craig Morten | 0.33.0 | Reads the page aloud as text for a persona that gets no screenshots | MIT |

It's bundled into one script with esbuild. Its browser build already includes other packages, and NOTICE has their
copyright lines. aria-query is under Apache-2.0. The rest are under MIT: @testing-library/dom,
@testing-library/user-event, dom-accessibility-api, html-aria, lz-string, pretty-format, react-is, ansi-regex,
ansi-styles and dequal.

`internal/page/screenreader/build/build.sh` rebuilds it from the pinned versions.

## Browser accessibility tree

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [Chrome DevTools Protocol](https://chromedevtools.github.io/devtools-protocol/), reached through [chromedriver](https://developer.chrome.com/docs/chromedriver)'s `goog:cdp` commands | the Chrome you run | In Chrome, `Accessibility.getFullAXTree` gives the role, name and states of each control in Chrome's tree. It also says whether a screen reader can reach the control. A control missing from the tree, and every control in Firefox, gets the engine's own name script. Firefox refuses `goog:cdp` commands | Part of Chrome; no code of it is bundled |

## Lighthouse

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [Lighthouse](https://github.com/GoogleChrome/lighthouse) by Google | 13.5.0, which you install | Its accessibility and best-practice audits, with `--lighthouse`. It runs as its own program and isn't bundled | Apache-2.0 |

## Link checking

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [lychee](https://github.com/lycheeverse/lychee) by Matthias Endler and the lychee contributors | 0.24.2 in the container image | Checks the links on each page with `--links`. With preview access on, the browser checks the links on the sites you sweep, and lychee only those to other sites. It runs as its own program, found on `PATH` or through `DUMMYRUN_LYCHEE` | MIT or Apache-2.0 |

## Plain language

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [Vale](https://github.com/vale-cli/vale) by Joseph Kato | 3.24.0 in the container image | Checks the main content of each page with `--plain-language`. It runs as its own program | MIT |
| [Vale Readability style](https://github.com/vale-cli/Readability) | as published | The Flesch reading ease and Flesch-Kincaid grade rules, used unchanged | MIT |

## HTML checking

| Tool | Version | What it does in Dummy Run | Licence |
|---|---|---|---|
| [Nu HTML Checker](https://github.com/validator/validator) (vnu) | 26.10.12 in the container image | Checks each page's HTML with `--html`. It runs as its own program | MIT |
| [OpenJDK](https://openjdk.org/) from Debian | 21, cut down with jlink, in the container image | The Java runtime vnu runs on | GPL-2.0 with the Classpath Exception |

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
| [Leonardo](https://github.com/adobe/leonardo) by Adobe | Suggesting a colour that meets a contrast ratio by changing its lightness, so the hue stays, for the colour suggestions on contrast failures. When the new lightness at that chroma is a colour the screen can't show, the chroma comes down too. The colour maths is OKLab, written again from the formulas in [CSS Color Module Level 4](https://www.w3.org/TR/css-color-4/#ok-lab) | Apache-2.0 |
| [Sa11y](https://github.com/ryersondmp/sa11y) from Toronto Metropolitan University and [Editoria11y](https://github.com/itmaybejj/editoria11y), which is developed with it | The idea of checking what authors write, such as vague link names, file names as alt text and bold text standing in for a heading, for `--content`. The checks, thresholds and wording are our own, written from the public descriptions of the problems. No code from either is used | GPL-2.0 |
| [LayoutLens](https://github.com/gojiplus/layoutlens) by goji+ | Measuring layout from the browser's rectangles and what is painted on top at a point: outermost elements past the window, clipped text, controls under other controls, focus hidden behind opaque content, and the measurable target size exceptions. Used by `--layout` | MIT |

## Data

The list of WCAG 2.2 success criteria at levels A and AA, with their titles and links, was generated from the
criteria data in Siteimprove Alfa (MIT).

The test fixtures for checking the sweep against real markup come from the
[AccessGuru dataset](https://github.com/NadeenAhmad/AccessGuruLLM) (MIT). The fixtures use the rule names and the markup of
the elements it lists, and none of its code.

## Mutation testing

It runs in the project's own weekly workflow, and CONTRIBUTING.md shows how to run it on your machine. It isn't part of the program you install.

| Tool | Version | What it does for Dummy Run | Licence |
|---|---|---|---|
| [gremlins](https://github.com/go-gremlins/gremlins) | 0.6.0 | Changes the engine's code one small step at a time and runs the unit tests against each change, to show where the tests would miss a bug | Apache-2.0 |
