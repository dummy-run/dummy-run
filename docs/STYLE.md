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

## Plain wording

These rules come from novibes research into what readers object to in a text. It found three things:
sameness with every other document of the same kind, sentences that carry nothing, and a known set of
wording habits. Each row names the habit and then what to write instead.

| Habit | Write |
|---|---|
| Long sentences strung together with commas | One idea per sentence, with at most two commas. A list of four or more items becomes a bulleted list or two sentences |
| Bold on the key phrase of a paragraph | No bold in running text. Bold is for a run-in head at the start of a list item, a table cell that names a row, or a warning that must not be missed |
| A label and a colon in place of a sentence ("The result: nine seconds") | The sentence ("The build finishes in nine seconds") |
| A staged contrast ("not X, but Y" or "X, not Y") | Y, unless the reader arrived believing X |
| Signposts and recaps ("it is worth noting", "in short") | Nothing. Say the thing |
| Bullets that argue a case | A paragraph. Lists are for steps, options and inventories |
| Three items because three sounds complete | However many there are |
| A sentence with no number, name, path, command or date in it | Delete it, or put the fact in |

Passive voice is not on the list and is never a reason to rewrite. A quotation keeps its words.

`bin/check-prose` checks the user-facing pages for the countable habits (commas, bold, staged contrasts
and signposts) and runs in CI. The other habits need a human reader.

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
