# Why Apache-2.0, and what it means for you

**Decision record** · 18 October 2026 · maintainers

> - Dummy Run is licensed under the Apache License 2.0 (`Apache-2.0`).
> - You can use, modify and share it at no cost. That includes using it inside closed products. Keep the
>   licence and the notices, and mark what you changed.
> - The licence covers the code. The [trademark policy](../TRADEMARKS.md) covers the name and the logo.
> - Contributors sign off each commit under the [Developer Certificate of Origin](https://developercertificate.org/).
>   There is no contributor licence agreement.

## What you can do

| You want to | Can you? | What it asks of you |
|---|---|---|
| Run Dummy Run on your laptop, in CI or on your own cluster | Yes | Nothing |
| Change it for your own use | Yes | Nothing |
| Run it for clients as a consultant or agency | Yes | Nothing |
| Ship it inside your own product, open or closed | Yes | Include the licence and the NOTICE file, and mark the files you changed (section 4) |
| Offer it as a hosted service | Yes | The same, and your service may not be called "Dummy Run" ([TRADEMARKS.md](../TRADEMARKS.md)) |

This table summarises the licence in plain English and is no substitute for legal advice. The
[LICENSE](../LICENSE) text is what counts.

## How we chose

We compared seven licences against five tests. The first was friction for organisations that adopt the
tool. The others were an explicit patent grant and approval by the Open Source Initiative (OSI). We also
looked at protection against a closed hosted copy and at fit with the code we bundle.

| | MIT | **Apache-2.0** | MPL-2.0 | AGPL-3.0 | FSL-1.1 | BSL 1.1 | Elastic 2.0 |
|---|---|---|---|---|---|---|---|
| OSI-approved open source | yes | **yes** | yes | yes | no | no | no |
| Friction for organisations | none | **none** | low | high (Google bans it) | high | high | high |
| Explicit patent grant | no | **yes** | yes | yes | limited | no | yes |
| A closed hosted copy must share its changes | no | **no** | only changed MPL files | yes, for changed versions | barred for 2 years | barred until a set date | barred |
| Fits axe-core (MPL-2.0), which we bundle | yes | **yes** | yes | yes | yes | yes | yes |

Four reasons decided it.

1. **Adoption matters most.** People run Dummy Run against their own apps on their own machines and in
   their own CI. We most want to reach QA teams, agencies and compliance staff in larger organisations.
   Their legal teams are the slowest to approve copyleft licences. Apache-2.0 is on almost every
   organisation's approved list.
2. **It grants patents explicitly.** Section 3 gives a patent licence from every contributor. That
   licence ends for anyone who "institutes patent litigation" over the work. MIT says nothing about
   patents.
3. **It is open source.** NLnet NGI Zero only funds software "published under a recognised free and open
   source license in its entirety". FSL, BSL and the Elastic License are missing from the OSI list
   (SPDX licence list). Choosing one would rule out grants like that one. It would also stop us calling
   Dummy Run open source.
4. **Contributors sign off, with no agreement to sign.** Contributions arrive under Apache-2.0, so they
   can already be used in any future work. A sign-off under the Developer Certificate of Origin confirms
   that the contributor has the right to submit the code. That keeps contributing quick and fair, because
   the maintainer gets no rights that contributors lack.

## What we gave up

AGPL-3.0 came second. Plausible moved to it in 2020 after "a corporation has taken parts of our code,
made it closed source and started selling it as a direct competitor". Elastic came back to it in 2024
after trying a source-available licence, and Redis did the same in 2025. Its section 13 makes anyone who
runs a changed version as a network service share their changes.

We turned it down for four reasons.

- Many organisations ban it outright. Google's policy says AGPL code "MUST NOT be used at Google". Others
  send it to legal review, which stops a quick trial.
- Lawyers disagree about what "interacting remotely" and a changed version mean. Cautious ones assume the
  worst.
- Selling commercial licences alongside AGPL means every contributor must grant the maintainer extra
  rights through an agreement. Some contributors find that one-sided and stay away.
- It protects against a competitor who runs a changed copy as a closed hosted service. That case is rare
  for a tool most people run themselves. The name protects us there, together with the people and
  services behind the project.

We accept one cost. Anyone may build Dummy Run into a closed hosted product without sharing their changes.
They may not call that product Dummy Run or suggest that we endorse it.

## Names and logos

Section 6 of Apache-2.0 grants no permission to use the licensor's trade names or marks. The
[trademark policy](../TRADEMARKS.md) sets out what you can do with the name "Dummy Run" and the logo.

## Commercial services

The maintainer offers paid services around the project. They include fixed-price audits of an app and
support. Ask through the contact on the [organisation profile](https://github.com/dummy-run). The
open-source edition is the full product.

## For contributors

Sign off every commit with `git commit -s`. That adds a `Signed-off-by:` line, which certifies the
[Developer Certificate of Origin](https://developercertificate.org/). CI checks that every commit in a
pull request has one.

## Sources

- Apache License 2.0: https://www.apache.org/licenses/LICENSE-2.0
- SPDX licence list ("OSI Approved?" column): https://spdx.org/licenses/
- AGPL-3.0 section 13: https://spdx.org/licenses/AGPL-3.0-only.html
- Google AGPL policy: https://opensource.google/documentation/reference/using/agpl-policy
- NLnet: https://nlnet.nl/propose/ and https://nlnet.nl/NGI0/
- Plausible (2020): https://plausible.io/blog/open-source-licenses
- Elastic (2024): https://www.elastic.co/blog/elasticsearch-is-open-source-again
- Redis (2025): https://redis.io/blog/agplv3/
- GitHub Open Source Guides, Legal: https://opensource.guide/legal/
- Developer Certificate of Origin 1.1: https://developercertificate.org/
- MPL-2.0 sections 3.3 and 1.12: https://www.mozilla.org/en-US/MPL/2.0/
