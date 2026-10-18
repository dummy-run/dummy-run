# Contributing to Dummy Run

Thank you for looking. The engine has not landed yet, so the most useful help for now is an issue. Tell
us about a persona you want, a target adapter you need, or a problem with the plan. This guide will grow
once there is code.

## Ground rules

1. **Evidence or nothing.** A claim about what a persona found needs a run ID and a step number. It also
   needs a screenshot or a log line. We remove any finding that has no evidence.
2. **No real people.** Personas are fictional composites. Never commit the name or contact details of a
   real customer, company or person. Never commit data copied from a target environment either. Preview
   environments can hold real production data, and none of it comes into this repository.
3. **No private-deployment identifiers.** This project was extracted from a private deployment. CI fails
   if an identifier from that deployment appears anywhere in the tree. The `clean-tree` job in
   `.github/workflows/ci.yml` runs the check, and its pattern list is kept as a repository secret. If you
   spot one that the check missed, report it through a private security advisory.
4. **No secrets.** Tokens, passwords and keys belong in your keychain or in CI secrets.
5. **Plain English, British spelling.** Write short sentences with one idea each. Explain a term the
   first time you use it. Leave out em dashes and superlatives. [docs/STYLE.md](docs/STYLE.md) has the
   rest, and `bin/check-prose` checks the main pages.

## Workflow

- Fork the repository, branch from `main` and open a pull request. We squash-merge, so the PR title
  becomes the commit message.
- Sign off every commit with `git commit -s`. The `Signed-off-by:` line certifies the
  [Developer Certificate of Origin](https://developercertificate.org/). It says you wrote the change or
  have the right to submit it. CI checks every commit. Contributions are licensed under
  [Apache-2.0](LICENSE) like the rest of the code, and there is no separate agreement to sign.
- Keep each pull request to one change. A persona change and a harness change are two pull requests.
- CI runs the clean-tree check, the docs checks and the sign-off check on every pull request. Tests and
  lint will join them when the engine lands.
- Say what you changed and how you checked it. The template prompts you.

## Proposing a persona

Open an issue with the "Persona proposal" template. A persona needs a role and the jobs that role comes
to do. It also needs a device, its usual conditions and what it may see. Add three to six scenarios.
Each trait should say where it comes from. That can be a public source, a reasonable inference or your
own experience. We do not accept personas built from a real customer's data.

## Proposing a target adapter

Open an issue with the "Feature or adapter" template. Tell us how the platform exposes a preview for a
pull request. Then tell us how to create a test user with a role, and how to load test data. We do not
accept adapters that need production credentials.

## Code of conduct

This project follows the [Code of Conduct](CODE_OF_CONDUCT.md), which also explains how to report a
problem.
