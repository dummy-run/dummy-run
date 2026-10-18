# Writing, citation and design guide

Everything in this repository is read by QA, UX, developers and executives. Write so each can find what
they need in under a minute.

## Shape of every document

1. **Summary** (3 to 5 lines, top of page). What we found, why it matters, what happens next. No jargon.
   An executive should be able to stop here.
2. **Detail**, grouped by audience need: what to test (QA), what confused people (UX), what is broken in
   code (developers).
3. **Evidence and sources** at the end, with every citation.

## Plain language

- Short sentences. One idea each.
- Explain a technical term the first time it appears, or avoid it. Example: "preview (a temporary copy
  of the app built for one pull request)".
- Say who does what: "A site manager cannot close a job from a phone", not "Job closure is unavailable
  on mobile".
- Concise. No em dashes (CI checks). No superlatives.
- British English.

## Citations

Every fact about users, the product under test or the code carries a citation.

| Source | Cite as | Example |
|---|---|---|
| Ticket | tracker and ID | `GH-42` |
| Pull request | PR number | `PR #42` |
| Code | path and line at a commit | `app/models/role.rb:12 @ abc1234` |
| Run | run ID, step and screenshot | `run 2026-10-18-ops-01, step 4` |
| Document | path and claim | `docs/research/reviewer-design.md, C7` |

Each claim is tagged by how we know it: **measured**, **code**, **sourced** or **inferred**. A persona
trait with no citation is removed, not kept as a guess.

## Privacy

Personas are fictional composites. Never include real customer, company or person names, contact details,
or data copied from a target environment. Preview environments may hold unmasked production data.

## Design

- Type scale and minimum sizes: [TYPOGRAPHY.md](TYPOGRAPHY.md). Body text is 17px with a 1.65 line
  height on every surface; nothing a person is expected to read is below 15px.
- Palette and mascot: to be defined in `brand/`. Until then, the board uses the crash-test amber
  `#fbad18` for the agent stage, near-black `#1d1d1b` for ink, green `#15803d` for passes and
  `#9c1924` for blockers. These are working values, not a brand.
- Diagrams are Mermaid in Markdown so they render on GitHub and can be re-themed.
