# Writing and citation guide

## Voice

Write the way a thoughtful person would explain Dummy Run to a colleague. Be warm, confident and specific.

- Speak to the reader as "you". Contractions are fine.
- Lead with what the reader gets and what to do next. Save the detail for after that.
- Show a concrete case before the general point. "A screen-reader user tries to reset their password and can't find the button" says more than "personas surface accessibility friction".
- Vary the length of your sentences. A short one lands a point. A longer one can carry a thought through, with its commas where a person would pause. Pages where every sentence has the same length and shape read as if a machine wrote them, so mix them up and read the page aloud.
- Explain a technical term the first time it appears. For example, write "preview (a copy of the app built for one pull request)".
- Say who does what. Write "A librarian can't renew a loan from a phone" rather than "Loan renewal is unavailable on mobile".
- Be honest about limits once, plainly, where it matters. A persona samples, a report isn't a certificate and nothing replaces testing with disabled people. Say it on the page where a reader would otherwise assume more, and don't repeat it on every page.
- Use British English. Leave out em dashes and superlatives. CI checks for em dashes.

## Write for the reader, not about the project

User pages (the site and the guides a new user reads) describe what you can do with Dummy Run today. Leave out how it was built and how it was decided.

- No "Sources" sections and no evidence tags such as "(code: ...)" or "(inferred)". Those belong in contributor documents, pull requests and issues.
- No lists of what is built and what is planned. If something isn't available, leave it out or say "coming soon" once, in plain words.
- No mentions of our review process, studies, coverage maths or design rationale. One plain sentence about how sure a report is can help. A derivation can't.
- Nothing about how the page or the docs were written.
- No project state, such as pull request numbers or a change that is about to merge. CI checks for these.

## Habits that read as machine-written

Each row names a habit and what to write instead.

| Habit | Write |
|---|---|
| Every sentence the same length and shape | A mix. Let a short sentence follow a long one |
| Bold on the key phrase of a paragraph | No bold in running text. Bold is for a run-in head at the start of a list item, a table cell that names a row, or a warning that must not be missed |
| A label and a colon in place of a sentence ("The result: nine seconds") | The sentence ("The build finishes in nine seconds") |
| A staged contrast ("not X, but Y", "It's not X. It's Y.") | Y, unless the reader arrived believing X |
| Signposts and recaps ("it is worth noting", "in short") | Nothing. Say the thing |
| Throat-clearing ("In today's world") and rhetorical questions as openers | Start with the point |
| "Seamless", "leverage", "unlock", "empower", "robust" | The plain word for what actually happens |
| Bullets that argue a case | A paragraph. Lists are for steps, options and inventories |
| Three items because three sounds complete | However many there are |
| A sentence with nothing concrete in it | Delete it, or put the example, number, name or command in |
| A byline or date under the title | Nothing. Git records who and when |
| Third person for the reader ("a QA lead wants their suite to pass") | "You" and "your" ("you want your suite to pass"). Use third person only for other people, such as personas, your users and clients |

Passive voice isn't on the list and is never a reason to rewrite on its own. A quotation keeps its words.

`bin/check-prose` checks the countable habits in CI. It fails on bold in running text, bold labels, staged contrasts, signposts, banned words and project state. It warns, without failing, about a sentence that runs long or holds a lot of commas, and about "their" after "a QA lead", "teams" and the other audience phrases on reader-facing pages. A warning is a prompt to read the sentence again. If it reads well aloud, leave it. The other habits need a human reader.

## Citations in contributor documents

Persona profiles and contributor documents cite their facts, so a reviewer can check them. Site pages and user guides don't show citations.

| Source | Cite as | Example |
|---|---|---|
| Issue | tracker and number | `GH-42` |
| Pull request | number | `PR #42` |
| Code | path and line at a commit | `app/models/role.rb:12 @ abc1234` |
| Run | run ID, step and screenshot | `run 2026-10-18-demo-01, step 4` |
| Document | path and section | `docs/ARCHITECTURE.md, Browsers` |

In a persona profile, tag each trait with how we know it. The tags are measured, code, sourced and inferred. A persona trait
with no citation is removed.

Privacy rules for personas and data are in [CONTRIBUTING.md](../CONTRIBUTING.md).

## Design

- Type sizes are in [TYPOGRAPHY.md](TYPOGRAPHY.md). Body text is 18px with a 1.65 line height, and
  nothing a person reads is below 16px.
- The logo, mascot and palette are in [brand/](../brand/README.md).
- Diagrams are Mermaid in Markdown, so they render on GitHub.
