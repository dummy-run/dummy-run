## What changed

<!-- One or two sentences. The PR title becomes the squash-commit message, so make it a good one. -->

## Why

<!-- Link the issue or explain the problem. -->

## How I checked it

<!-- Commands run, screenshots, run IDs. -->

## Checklist

- [ ] No real customer, company or person data, and nothing copied from a target environment
- [ ] No identifiers from any customer deployment (the "Tree is clean" check passes)
- [ ] Documentation updated where behaviour changed
- [ ] Docs and site: the pages in `docs/`, the READMEs and `site/content/` match the code, `go generate ./cmd/dummyrun` leaves no diff and `go run ./internal/tools/doctest` passes. Or the change is not visible to a user
- [ ] CHANGELOG entry under Unreleased, if a user would notice the change
