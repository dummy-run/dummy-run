# Licensing

Dummy Run is licensed under the [Apache License 2.0](../LICENSE). The licence covers the code. The
[trademark policy](../TRADEMARKS.md) covers the name and logo.

## What you can do

| You want to | Allowed? | Conditions |
|---|---|---|
| Run it on your laptop, in CI or on your own cluster | Yes | None |
| Change it for your own use | Yes | None |
| Run it for clients | Yes | None |
| Ship it inside your own product, open or closed | Yes | Include LICENSE and NOTICE, and mark changed files (section 4) |
| Offer it as a hosted service | Yes | As above, under a name other than "Dummy Run" |

The [LICENSE](../LICENSE) text governs. This table is not legal advice.

## Why Apache-2.0

| | MIT | **Apache-2.0** | MPL-2.0 | AGPL-3.0 | FSL-1.1 | BSL 1.1 | Elastic 2.0 |
|---|---|---|---|---|---|---|---|
| OSI-approved open source | yes | **yes** | yes | yes | no | no | no |
| Friction for organisations | none | **none** | low | high (Google bans it) | high | high | high |
| Explicit patent grant | no | **yes** | yes | yes | limited | no | yes |
| A closed hosted copy must share its changes | no | **no** | only changed MPL files | yes, for changed versions | barred for 2 years | barred until a set date | barred |
| Compatible with bundled axe-core (MPL-2.0) | yes | **yes** | yes | yes | yes | yes | yes |

1. **Adoption.** QA teams, agencies and compliance staff run Dummy Run against their own apps. Their
   legal teams approve Apache-2.0 routinely and copyleft slowly.
2. **Patents.** Section 3 grants a patent licence from every contributor. MIT has no patent grant.
3. **Open source.** NLnet NGI Zero funds only software "published under a recognised free and open
   source license in its entirety". FSL, BSL and the Elastic License are not OSI-approved.
4. **No contributor agreement.** Contributions arrive under Apache-2.0, so a Developer Certificate of
   Origin sign-off is enough. The maintainer gets no rights that contributors lack.

## Why not AGPL-3.0

AGPL-3.0 was the runner-up. Its section 13 makes anyone who runs a changed version as a network service
share their changes, which is why Plausible, Elastic and Redis adopted it. We rejected it for adoption.
Google's policy says AGPL code "MUST NOT be used at Google", and other organisations ban it or send it to
legal review. Selling commercial licences alongside it would also need a contributor agreement. The cost
of Apache-2.0 is that anyone may build Dummy Run into a closed hosted product without sharing changes.
The trademark policy stops them calling it Dummy Run.

## Sources

- Apache License 2.0: https://www.apache.org/licenses/LICENSE-2.0
- SPDX licence list ("OSI Approved?" column): https://spdx.org/licenses/
- AGPL-3.0 section 13: https://spdx.org/licenses/AGPL-3.0-only.html
- Google AGPL policy: https://opensource.google/documentation/reference/using/agpl-policy
- NLnet: https://nlnet.nl/propose/ and https://nlnet.nl/NGI0/
- Plausible (2020): https://plausible.io/blog/open-source-licenses
- Elastic (2024): https://www.elastic.co/blog/elasticsearch-is-open-source-again
- Redis (2025): https://redis.io/blog/agplv3/
- Developer Certificate of Origin 1.1: https://developercertificate.org/
