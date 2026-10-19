# Changelog

All notable changes are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- `dummyrun a11y` sweeps pages with axe-core for WCAG 2.2 A and AA on Chrome and Firefox, with no model calls. The site's accessibility check now uses it in place of pa11y-ci, which removes the extract-zip alerts.
- The project is licensed under Apache-2.0, with a trademark policy for the name and logo.
- Contributors sign off commits under the Developer Certificate of Origin, and CI checks every commit.
- CI also blocks credentials and known customer identifiers, broken links and em dashes.
- `bin/check-prose` enforces the plain-wording rules in `docs/STYLE.md`.
- The docs cover the plan and the architecture. Others explain the licence, the name and the type scale.
- `brand/` holds the Viewfinder logo, the Dummy mascot and the colour palette.
