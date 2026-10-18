# Contributing to Dummy Run

Thank you for looking. Until the engine lands, the most useful contributions are issues: persona ideas,
target adapters you would want, and problems with the plan. Once code is here, this file grows.

## Ground rules

1. **Evidence or nothing.** Any claim about what a persona found carries a run ID, a step number and a
   screenshot or log line. A finding with no evidence is removed, not kept as a guess.
2. **No real people.** Personas are fictional composites. Never commit real customer, company or person
   names, contact details, or data copied from a target environment. Preview environments may hold
   unmasked production data; nothing from them comes into this repository.
3. **No private-deployment identifiers.** This project was extracted from a private deployment. CI fails
   if an identifier from that deployment appears anywhere in the tree (`.github/workflows/ci.yml`, job
   `clean-tree`; the pattern list is a repository secret so it does not publish what it protects). If you
   see one that the gate missed, open a security advisory, not a public issue.
4. **No secrets.** Tokens, passwords and keys live in your keychain or CI secrets, never in files.
5. **Plain English, British spelling.** Short sentences, one idea each, explain a term the first time it
   appears or avoid it. No em dashes, no superlatives. See [docs/STYLE.md](docs/STYLE.md).

## Workflow

- Fork, branch from `main`, open a pull request. Squash merges only; the PR title becomes the commit.
- Keep PRs focused. A persona change and a harness change are two PRs.
- Every PR runs the clean-tree gate and the docs checks. Code PRs will also run tests and lint when
  the engine lands.
- Describe what you changed and how you checked it. The template asks.

## Proposing a persona

Open an issue with the "Persona proposal" template. A persona needs a role, the jobs that role comes to
do, its device and typical conditions, what it is allowed to see, and three to six scenarios. Each trait
should say where it comes from: a public source, a reasonable inference, or your own experience. We do not
accept personas built from a specific real customer's data.

## Proposing a target adapter

Open an issue with the "Feature" template and say how the platform exposes a preview for a pull
request, how a test user with a role can be created, and how fixtures are seeded. Adapters that need
production credentials will not be accepted.

## Code of conduct

This project follows the [Code of Conduct](CODE_OF_CONDUCT.md). It says how to report a problem.
