# Writing and citation guide

## Plain language

- Write short sentences with one idea each.
- Explain a technical term the first time it appears. For example, write "preview (a copy of the app
  built for one pull request)".
- Say who does what. Write "A librarian cannot renew a loan from a phone" rather than "Loan renewal is
  unavailable on mobile".
- Use British English. Leave out em dashes and superlatives. CI checks for em dashes.

## Plain wording

These rules come from reader research into what people object to in a text. Each row names a habit and
what to write instead.

| Habit | Write |
|---|---|
| Long sentences strung together with commas | One idea per sentence, with at most two commas. Split a long list into separate sentences rather than tidy groups of three |
| Bold on the key phrase of a paragraph | No bold in running text. Bold is for a run-in head at the start of a list item, a table cell that names a row, or a warning that must not be missed |
| A label and a colon in place of a sentence ("The result: nine seconds") | The sentence ("The build finishes in nine seconds") |
| A staged contrast ("not X, but Y" or "X, not Y") | Y, unless the reader arrived believing X |
| Signposts and recaps ("it is worth noting", "in short") | Nothing. Say the thing |
| Bullets that argue a case | A paragraph. Lists are for steps, options and inventories |
| Three items because three sounds complete | However many there are |
| A sentence with no number, name, path, command or date in it | Delete it, or put the fact in |
| A byline or date under the title | Nothing. Git records who and when |
| Third person for the reader ("a QA lead wants their suite to pass") | "You" and "your" ("you want your suite to pass"). Use third person only for other people, such as personas, your users and clients |
| Project state in user docs (pull request numbers, "in review", "once it merges") | What you can do today. Work in progress belongs in pull requests and issues |

Passive voice is not on the list and is never a reason to rewrite. A quotation keeps its words.

`bin/check-prose` checks the main pages for the countable habits in CI. Those are commas, bold, staged
contrasts and signposts. It also warns about "their" after "a QA lead", "teams" and the other listed audience phrases on reader-facing pages. The other habits need a human reader.

## Citations

Every fact about users, the app under test or the code carries a citation.

| Source | Cite as | Example |
|---|---|---|
| Issue | tracker and number | `GH-42` |
| Pull request | number | `PR #42` |
| Code | path and line at a commit | `app/models/role.rb:12 @ abc1234` |
| Run | run ID, step and screenshot | `run 2026-10-18-demo-01, step 4` |
| Document | path and section | `docs/ARCHITECTURE.md, Browsers` |

Tag each claim with how we know it. The tags are measured, code, sourced and inferred. A persona trait
with no citation is removed.

Privacy rules for personas and data are in [CONTRIBUTING.md](../CONTRIBUTING.md).

## Design

- Type sizes are in [TYPOGRAPHY.md](TYPOGRAPHY.md). Body text is 18px with a 1.65 line height, and
  nothing a person reads is below 16px.
- The logo, mascot and palette are in [brand/](../brand/README.md).
- Diagrams are Mermaid in Markdown, so they render on GitHub.
