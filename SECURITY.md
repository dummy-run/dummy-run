# Security

## Reporting a vulnerability in Dummy Run

Use [Report a vulnerability](https://github.com/dummy-run/dummy-run/security/advisories/new) on this
repository's Security tab. Do not open a public issue. We reply within five working days and say what
happens next.

We follow coordinated disclosure. We fix the problem in a private fork and then publish a GitHub
security advisory with a CVE where one applies. The advisory credits you unless you ask us not to. We
publish when the fix is released, or 90 days after your report if that comes first. Tell us if you need
a different date.

Everything in this organisation is in scope. The apps Dummy Run tests and third-party model providers are
not.

## Findings about apps you do not own

A finding in someone else's app belongs to its owner. Report it through their disclosure route, and keep
it out of this project's issues, discussions and example data.

## What this repository holds

This repository holds no run results, screenshots, credentials or personas drawn from real customers.
Run data stays with whoever runs Dummy Run.

## Test credentials

A target file can name test accounts so the engine signs in before a persona starts. The rules for them:

- Use accounts made for testing, with no multi-factor step and no access to real data. Never use a person's account.
- The target file holds the names of environment variables and never the values. Put the values in your CI secret store or your shell. Do not commit them.
- The engine reads them when the run starts and types them into the login form. The model never sees them. The run record, the log, the screenshots and the telemetry do not hold them. Errors are scrubbed of both values.
- The engine takes no screenshot while it signs in, so the password field is never captured.
- A saved session file holds live session cookies and is as sensitive as a password. The engine writes it with mode 0600. Keep it out of version control and rotate it as you would a token.
- A page can show the signed-in user's own name or email, since the engine does not edit what a site displays. The model may see that and may repeat it in a report. Use test users whose details are fake.
- The read-only policy has one exception, the sign-in form's own request, and only while the engine signs in. See [docs/ENGINE.md](docs/ENGINE.md#sign-in).

## Supported versions

Only `main`, until the first release.
