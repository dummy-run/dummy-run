# Why Apache-2.0, and what it means for you

**Decision record** · 18 October 2026 · maintainers

> - Dummy Run is licensed under the **Apache License 2.0** (`Apache-2.0`).
> - You can use, modify and redistribute it, in open or closed products, at no cost. Keep the licence
>   and notices, and say what you changed.
> - The licence covers the code. The name and logo are covered by the [trademark policy](../TRADEMARKS.md).
> - Contributors sign off each commit under the [Developer Certificate of Origin](https://developercertificate.org/).
>   There is no contributor licence agreement.

## What you can do

| You want to | Can you? | What it asks of you |
|---|---|---|
| Run Dummy Run on your laptop, in CI or on your own cluster | Yes | Nothing |
| Modify it for your own use | Yes | Nothing |
| Run it for clients as a consultant or agency | Yes | Nothing |
| Ship it inside your own product, open or closed | Yes | Include the licence and the NOTICE file, and mark the files you changed (section 4) |
| Offer it as a hosted service | Yes | The same; and you may not call your service "Dummy Run" (see [TRADEMARKS.md](../TRADEMARKS.md)) |

This table is a plain-English summary, not legal advice. The [LICENSE](../LICENSE) text governs.

## How we chose

We compared seven licences against five criteria: friction for organisations adopting the tool, an
explicit patent grant, approval by the Open Source Initiative (OSI), protection against a closed hosted
copy, and compatibility with what we bundle.

| | MIT | **Apache-2.0** | MPL-2.0 | AGPL-3.0 | FSL-1.1 | BSL 1.1 | Elastic 2.0 |
|---|---|---|---|---|---|---|---|
| OSI-approved open source | yes | **yes** | yes | yes | no | no | no |
| Friction for organisations | none | **none** | low | high (Google bans it) | high | high | high |
| Explicit patent grant | no | **yes** | yes | yes | limited | no | yes |
| A closed hosted copy must share its changes | no | **no** | only changed MPL files | yes, for modified versions | barred for 2 years | barred until a change date | barred |
| Fits axe-core (MPL-2.0), which we bundle | yes | **yes** | yes | yes | yes | yes | yes |

Four reasons decided it:

1. **Adoption matters most.** Dummy Run is a tool people run against their own apps, on their own
   machines and in their own CI. The people we want to reach (QA teams, agencies, compliance staff in
   larger organisations) are the ones whose legal teams are slowest to approve copyleft. Apache-2.0 is
   on almost every organisation's pre-approved list.
2. **It has an explicit patent grant.** Section 3 grants a patent licence from every contributor, which
   ends for anyone who "institutes patent litigation" over the work. MIT says nothing about patents.
3. **It is open source.** Grants such as NLnet NGI Zero require software "published under a recognised
   free and open source license in its entirety". FSL, BSL and the Elastic License are not OSI-approved
   (SPDX licence list), so they would rule those out, and the words "open source" with them.
4. **No contributor agreement is needed.** Because contributions arrive under Apache-2.0, they can
   already be used in any future work. A sign-off under the Developer Certificate of Origin, which
   confirms the contributor has the right to submit the code, is enough. That keeps contributing quick
   and fair: nobody grants the maintainer rights they do not get themselves.

## What we gave up, and why

**AGPL-3.0 was the runner-up.** It is the licence Plausible moved to in 2020, after "a corporation has
taken parts of our code, made it closed source and started selling it as a direct competitor", and the
one Elastic (2024) and Redis (2025) came back to after trying source-available licences. Its section 13
makes anyone running a modified version as a network service share their changes.

We rejected it because:

- Many organisations ban it outright. Google's policy is that AGPL code "MUST NOT be used at Google",
  and others send it to legal review, which stops a quick trial.
- What counts as "interacting remotely" and as a modified version is argued about, so cautious lawyers
  assume the worst.
- Selling commercial licences alongside AGPL needs every contributor to grant the maintainer extra
  rights through a contributor agreement. Some contributors see that as one-sided, and some will not
  contribute.
- The case it protects against, a competitor running a modified copy as a closed hosted service, is
  rare for a tool most people run themselves. Our protection there is the name (see below), and the
  people and services behind the project.

**The cost we accept.** Anyone may fold Dummy Run into a closed hosted product without sharing their
changes. They may not call it Dummy Run or suggest we endorse it.

## Names and logos

Apache-2.0 section 6 does not grant permission to use the licensor's trade names or marks. The
[trademark policy](../TRADEMARKS.md) says what you can do with the name "Dummy Run" and the logo.

## Commercial services

The maintainer offers paid services around the project, such as fixed-price audits of an app and support.
Ask through the contact on the [organisation profile](https://github.com/dummy-run). The open-source
edition is the full product.

## For contributors

Sign off every commit with `git commit -s`. That adds a `Signed-off-by:` line, which certifies the
[Developer Certificate of Origin](https://developercertificate.org/). CI checks that every commit in a
pull request has one.

## Sources

- Apache License 2.0: https://www.apache.org/licenses/LICENSE-2.0
- SPDX licence list, "OSI Approved?" column: https://spdx.org/licenses/
- AGPL-3.0 section 13: https://spdx.org/licenses/AGPL-3.0-only.html
- Google AGPL policy: https://opensource.google/documentation/reference/using/agpl-policy
- NLnet: https://nlnet.nl/propose/ and https://nlnet.nl/NGI0/
- Plausible, 2020: https://plausible.io/blog/open-source-licenses
- Elastic, 2024: https://www.elastic.co/blog/elasticsearch-is-open-source-again
- Redis, 2025: https://redis.io/blog/agplv3/
- GitHub Open Source Guides, Legal: https://opensource.guide/legal/
- Developer Certificate of Origin 1.1: https://developercertificate.org/
- MPL-2.0 sections 3.3 and 1.12: https://www.mozilla.org/en-US/MPL/2.0/
