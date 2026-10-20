# Security

## Reporting a vulnerability in Dummy Run

To report a vulnerability, use [Report a vulnerability](https://github.com/dummy-run/dummy-run/security/advisories/new) on this
repository's Security tab. If you can't use that, email
[security@dummyrun.dev](mailto:security@dummyrun.dev). Please don't open a public issue. We'll reply within five working days and tell you what
happens next.

We follow coordinated disclosure. We fix the problem in a private fork, then publish a GitHub security
advisory with a CVE where one applies. The advisory credits you unless you ask us not to. We publish when
the fix is released, or 90 days after your report if that comes first. If you need a different date, tell us.

Everything in this organisation is in scope. The apps Dummy Run tests and third-party model providers
aren't.

## Findings about apps you do not own

A finding in someone else's app belongs to its owner. Report it through their disclosure route, and keep
it out of this project's issues, discussions and example data.

## How Dummy Run treats hostile pages

A page under test is untrusted input to the model. [Threat model](docs/THREAT-MODEL.md) lists the threats,
the defences against each and the gaps that remain.

## What this repository holds

This repository holds no credentials and no run results or screenshots from a real app. The run records
in `examples/runs/` are synthetic, and no persona is drawn from a real customer. Run data stays with whoever runs Dummy Run.

## Verifying a release

Every release is built by the release workflow of this repository. The checksums file and the container image
carry cosign signatures made with GitHub's OIDC identity, and GitHub holds SLSA build provenance for the
archives and the image. Accept only the identity `https://github.com/dummy-run/dummy-run/.github/workflows/release.yml@refs/tags/v<version>` with the issuer `https://token.actions.githubusercontent.com`. A signature that names any other workflow, repository or issuer means the file did not come from us. [docs/INSTALL.md](docs/INSTALL.md) has the commands.

## Test credentials

A target file can name test accounts, so the engine signs in before a persona starts. Follow these rules for them.

- Use accounts made for testing, with no multi-factor step and no access to real data. Never use a person's account.
- The target file holds the names of environment variables, never the values. Put the values in your CI secret store or your shell, and don't commit them.
- The engine reads the values when the run starts and types them into the login form. The model never sees them. The engine keeps them out of the run record, the log, the screenshots and the telemetry. It also scrubs them from errors. That holds unless your app shows a credential on a page after sign-in, so don't let a test account's page display its password or token.
- The engine takes no screenshot while it signs in, so the password field is never captured.
- A saved session file holds live session cookies and is as sensitive as a password. The engine writes it with mode 0600. Keep it out of version control and rotate it as you would a token.
- A page can show the signed-in user's own name or email, and the engine doesn't edit what a site displays. The model may see those details and repeat them in a report, so give test users fake ones.
- The read-only policy has one exception: the sign-in form's own request, and only while the engine signs in. See "Sign in" in the [engine guide](docs/ENGINE.md).

## Supported versions

Only `main`, until the first release.
