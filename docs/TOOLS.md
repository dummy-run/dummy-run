# Tools we build on

Dummy Run stands on other people's open-source work. This page names each tool the engine ships or runs, what it
does for you and its licence. The full licence texts are in [LICENSES](../LICENSES) and next to each bundled copy,
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

## Data

The list of WCAG 2.2 success criteria at levels A and AA, with their titles and links, was generated from the
criteria data in Siteimprove Alfa (MIT).
