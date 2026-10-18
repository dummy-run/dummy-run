# Changelog

All notable changes are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Repository scaffold: README, contributing guide, code of conduct, security policy, issue and PR
  templates, Dependabot, CI with a clean-tree gate.
- Documentation carried over from the naming research: naming decision, type scale, extraction plan,
  architecture outline, house style, free-for-open-source programmes.

### Changed
- User-facing pages rewritten in plain wording: shorter sentences, fewer commas, no bold in running
  text. `bin/check-prose` enforces the countable rules in CI (docs/STYLE.md, "Plain wording").

### Decided
- Name: **Dummy Run**. GitHub organisation `dummy-run` (the unhyphenated name is held by a dormant
  account that GitHub policy does not release); CLI command `dummyrun`.
- Licence: Apache-2.0, with a trademark policy for the name and logo and DCO sign-off for contributions. See docs/LICENSING.md.
- The engine is extracted from a private deployment with a fresh history; no run data, personas or
  branding from that deployment are carried over.
