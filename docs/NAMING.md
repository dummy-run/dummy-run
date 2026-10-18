# How the name was chosen

**Decision record** · 18 October 2026 · maintainers

> - Name: Dummy Run. British English for a trial run, and literally what happens: dummies run your app.
> - Chosen after an evidence-based screen of 88 candidates against the rubric below, with registry, domain, clash and language checks on the top 31.
> - The crash-test metaphor gives a mascot; the cautions are at the end.

## Rubric

Scores are 1-5 per criterion, multiplied by the weight. Maximum 100.

| # | Criterion | Weight | What a 5 looks like | Source and verbatim quote |
|---|-----------|-------:|---------------------|---------------------------|
| 1 | Trademark distinctiveness (spectrum position) | 20 | Suggestive, arbitrary or fanciful; nothing that names the category ("persona", "test", "QA", "agent") | USPTO TMEP 1209.01: "Fanciful, arbitrary, and suggestive marks, often referred to as 'inherently distinctive' marks, are registrable on the Principal Register without proof of acquired distinctiveness." 1209.01(a): suggestive marks "require imagination, thought, or perception to reach a conclusion as to the nature of those goods or services"; "Incongruity is a strong indication that a mark is suggestive rather than merely descriptive." https://tmep.uspto.gov/RDMS/TMEP/print?href=TMEP-1200d1e6993.html&version=current . USPTO "Strong trademarks": "Strong trademarks are suggestive, fanciful, or arbitrary. Weak trademarks are descriptive or generic." https://www.uspto.gov/trademarks/basics/strong-trademarks |
| 2 | Processing fluency and pronounceability | 15 | One obvious pronunciation in UK and US English; no consonant clusters a non-native speaker trips on | Alter and Oppenheimer, PNAS 2006: "fluently named stocks robustly outperformed stocks with disfluent names in the short term"; "companies with names like Barnings Incorporated will initially outperform companies with names like Aegeadux Incorporated." https://www.pnas.org/doi/10.1073/pnas.0601071103 |
| 3 | Semantic appositeness (suggests what it does) | 15 | A QA lead and a product person both get a hint of "someone plays a user and reports back" | Lowrey, Shrum and Dubitsky, J. Advertising 2003: "three linguistic variables were positively related to brand-name memory (semantic appositeness, paranomasia, initial plosives), but only for less familiar brands." https://doi.org/10.1080/00913367.2003.10639137 . Also TMEP 1209.01(a) above: suggestive beats arbitrary for recall while staying registrable. |
| 4 | Sound symbolism | 10 | Sounds that suggest sharp, quick, light and precise (front vowels, fricatives); initial plosive helps recall; avoid sounds that suggest slow, heavy, dull | Klink, Marketing Letters 2000: "the sound of a brand name can communicate information about the product, e.g. its size, speed, strength, weight, etc." https://doi.org/10.1023/A:1008184423824 . Yorkston and Menon, JCR 2004: "the [i] sound ... is associated with perceptions of the object being smaller, lighter, livelier, and sharper"; the effect is "automatic in as much as it is uncontrollable, outside awareness and effortless." https://doi.org/10.1086/383422 . Vanden Bergh et al. 1984: "more respondents recalled and recognized words beginning with plosives than those beginning with other sounds." https://eric.ed.gov/?id=ED243117 |
| 5 | Length and memorability | 10 | 1-3 syllables, roughly 4-7 phonemes, 4-10 letters; not a morpheme blend | Vanden Bergh et al. 1984: "Recall was significantly greater for one syllable words than for two and three syllable words." Zee et al. (VUB) 2026: "names shorter than three phonemes lead to less positive attitudes"; "Longer non-word brand names are recognised more accurately than shorter alternatives." https://smit.research.vub.be/en/policy-brief-94-rightsizing-brand-names-the-effects-of-brand-name-length-on-brand-name-attitude-and . Lowrey et al. 2003: "unusual spelling (positive) and blending (negative)" as main effects on memory, so portmanteaux are penalised here. |
| 6 | CLI command fitness | 10 | 4-10 lowercase letters, no hyphen, typeable with alternating hands, no collision with POSIX/GNU tools or Homebrew formulae | Practical criterion. Checked with `command -v`, the local `compgen -c` list and https://formulae.brew.sh/api/formula/<name>.json |
| 7 | Unique searchability and namespace availability | 10 | Searching "<name> software" finds nothing in QA, testing, AI or dev tools; GitHub top hit has few stars; npm/PyPI/crates/gems/Go names unregistered | Practical criterion; checks in checks/*.json. "Unregistered" is not "free": formal trademark databases (USPTO TESS, EUIPO, UKIPO) were not searched. |
| 8 | Cross-audience and cross-language safety | 10 | Reads well to a non-technical executive and to a developer; no rude or unfortunate meaning in EN-UK, EN-US, DE, FR, ES, PT, IT, NL, PL, JA, ZH, HI, AR | Practical criterion, informed by Klink 2000 (names carry meaning before any marketing). Verified where a dictionary or search result could be found; otherwise marked "not verified". |

## Notes on applying the rubric

- The "theatre" metaphor is kept as a family, not a requirement. Rejected names (Understudy, Stagehand, StandIn, Callboard, Dress Rehearsal) show the obvious theatre words are crowded.
- A morpheme blend (e.g. "Personaut") loses points on criterion 5 because Lowrey et al. found blending had a negative main effect on brand-name memory.
- A pun or wordplay (paranomasia) gains points on criterion 3 (Lowrey et al.).
- Descriptive words for the category ("persona", "test", "QA", "agent", "user") score 1-2 on criterion 1 because TMEP 1209.01(b) bars merely descriptive marks without acquired distinctiveness.

## Shortlist and checks

Raw outputs (registry probes, DNS, per-name clash searches, language notes) are kept by the maintainers; the summary is below. The private-deployment context mentioned in the research is omitted here.


### 1. Pretaster (82)
- Method: metaphor, tasting family. The person who tastes the dish before it reaches the table.
- Suggests: someone tries the release before your customers do; by ear, US listeners also hear "pre-tester".
- CLI: `pretaster` (9 chars, no collision locally or in Homebrew).
- Availability: npm, PyPI, RubyGems, crates.io unregistered; no pkg.go.dev hits; brew 404; GitHub only "Zero1624/Pretasters" (0 stars); pretaster.dev/.com/.io/.ai all have no NS (unregistered as of the check).
- Closest clash: Provide Foundry's internal test suite "PRETASTER" (documentation page only).
- Linguistics: transparent English compound; initial plosive, front vowels (Yorkston and Menon's "lighter, livelier, sharper"); nothing found in the other 11 languages.
- QA lead: "the pre-taster runs on every preview deploy" is immediately legible. Executive: "someone tastes it before the customer does" needs no explanation.
- Weakness: three syllables and slightly playful; "taster" is a food word, so the compliance audience may find it light.

### 2. Figurant (80)
- Method: metaphor, theatre family but oblique: the extra who plays a part without lines, in French, Dutch, Italian, Spanish and Portuguese theatre vocabulary and as a rare English word.
- Suggests: a person who takes a role in the scene without being the star; fits the existing cast and scenarios vocabulary without colliding with Stunt Double's actors.
- CLI: `figurant` (8 chars, no collision).
- Availability: npm, PyPI, RubyGems, crates.io unregistered; brew 404; GitHub best exact match 0 stars; figurant.dev, .io, .ai have no NS; figurant.com is registered (Gandi NS).
- Closest clash: none in software. A 2019 Czech short film. Figure AI is a different word.
- Linguistics: FR "figurant = extra, walk-on" (Cambridge); PONS also gives "n'être qu'un figurant = token role" and "potiche = puppet". PL "figurant = figurehead" (Cambridge). RU фигурант is also "a person named in a case". Pronunciation differs EN/FR but both are easy.
- QA lead: "run the figurants against the PR" reads as "run the extras through it". Executive: needs one line ("a figurant is a background actor; ours play your users"), then it sticks.
- Weakness: figurehead sense in PL/FR; not instantly transparent in English.

### 3. Walkon (83 on the rubric, ranked third on judgement)
- Method: suggestive compound from "a walk-on part"; also literal: the persona walks on to your app.
- Suggests: a small, unscripted part played in your product.
- CLI: `walkon` (6 chars, no collision).
- Availability: npm, PyPI, crates.io unregistered; RubyGems "walkon" taken; brew 404; GitHub exact match 1 star; walkon.dev (Vercel NS), .com, .io (Afternic, for sale), .ai all registered.
- Closest clash: WalkOn, a Korean step-tracking app with 1M+ downloads (Swallaby). Different category, but it owns search for "walkon app".
- Linguistics: US "walk-on" also means an unrecruited athlete; nothing rude found. One-word spelling is a coinage, so other languages read it as English.
- QA lead: fine. Executive: "walk-on" is understood in UK/US; weaker outside English.
- Weakness: searchability (common phrase plus the Korean app) and all four domains gone. Ranked below Figurant for that reason despite the higher rubric score.

### 4. Lodger (73)
- Method: metaphor, other. Someone who lives in your house for a while and tells you what is wrong with the room.
- Suggests: a temporary resident of your app with their own key (login) and habits.
- CLI: `lodger` (6 chars, no collision).
- Availability: PyPI, RubyGems, crates.io unregistered; npm "lodger" taken; brew 404; GitHub exact 4 stars; all four domains registered.
- Closest clash: Lodger, a French proptech (2025) and an abandoned landlord SaaS of the same name.
- Linguistics: everyday UK word; US readers understand but say "boarder". Nothing rude.
- QA lead: fine. Executive: friendly and plain, slightly quaint.
- Weakness: proptech clash, npm taken, domains taken, US register.

### 5. Guiser (73)
- Method: metaphor, other. Scots: a person in disguise who goes from house to house (guising).
- Suggests: disguise, playing a part, visiting doors; fits permission-leak testing ("what can a guiser see?").
- CLI: `guiser` (6 chars, no collision).
- Availability: all four registries unregistered; brew 404; GitHub exact 2 stars; guiser.dev/.io/.ai no NS; guiser.com registered.
- Closest clash: guiser.org, a non-KYC crypto exchange aggregator and payment processor. Unwelcome neighbour in search results.
- Linguistics: pronunciation not obvious from spelling (GUY-zer); echoes "geezer".
- QA lead: needs the one-line story. Executive: likewise; the crypto neighbour hurts.

### 6. Houseguest (72)
- Method: metaphor, other.
- Suggests: a guest who stays and notices what is wrong with the house; warmer than "tester".
- CLI: `houseguest` (10 chars, at the limit; no collision).
- Availability: all four registries unregistered; brew 404; GitHub exact 0 stars; all four domains registered.
- Closest clash: Amazon Alexa "Houseguest" blueprint; a New Zealand Airbnb manager.
- Linguistics: transparent English; no issues found.
- QA lead and executive: both get it instantly; the plainest name on the list.
- Weakness: length, domains, and it says nothing about verification.

### 7. Phersu (71)
- Method: coined/fanciful. The Etruscan masked figure whose name (via phersuna) became Latin persona.
- Suggests: nothing until told; then it is the best story on the list: "the word persona comes from this mask".
- CLI: `phersu` (6 chars, no collision).
- Availability: all four registries unregistered; brew 404; GitHub exact 0 stars; phersu.dev/.com/.io no NS; phersu.ai registered.
- Closest clash: Phersu Interactive (Brazilian indie game studio), Phersu Atlas (historical atlas).
- Linguistics: no living-language meaning; spelling not recoverable from sound; the Phersu in the frescoes is "a man-hunter with a mask", so the story has an edge.
- QA lead: neutral. Executive: needs the etymology, then memorable.
- Weakness: processing fluency (Alter and Oppenheimer): "ph" and the final "u" make people hesitate.

### 8. Manikin (70)
- Method: metaphor, crash-test family: the correct word for a crash-test dummy.
- Suggests: a stand-in body that takes the hit so a person does not; also an anatomical model.
- CLI: `manikin` (7 chars, no collision).
- Availability: PyPI, RubyGems, crates.io unregistered; npm taken; brew 404; GitHub exact 2 stars; all four domains registered.
- Closest clash: medical simulation manikins own the search results (Laerdal, SimVox); odipar/manikin library.
- Linguistics: benign everywhere checked; JA マネキン means shop mannequin.
- QA lead: good fit. Executive: "crash-test dummy for your software" is a strong one-liner, but the word itself reads as a shop dummy.

## Wildcards (lower score, worth a look)

- **Dummyrun** (75, descriptive risk). UK idiom for a trial run plus a crash-test-dummy pun. Everything unregistered except dummyrun.com (parked for sale). Weak as a trademark (TMEP 1209.01(b): "dummy run" describes what a test tool does) and "dummy" grates on executives, but as a CLI verb it is perfect: `dummyrun ./preview`.
- **Comparsa** (70). Spanish and Italian for a stage extra and for the troupe that moves together through a carnival: "Not one star, a company of people" (comparsa.ai's own tagline, which is the clash: a pre-launch event-talent marketplace). comparsa.dev and .io have no NS; registries clean.
- **Vedette** (knocked out on language, 71 before). Military English for the advance sentry who sees the enemy first; French for a star. Beautiful fit for "sees the leak first", but in Spanish it is the cabaret showgirl (RAE, es.wikipedia) and in Mexico "an arrogant show-off". Only usable if Spanish markets are out of scope, which they are not.

## Not checked
- Formal trademark registers (USPTO TESS, EUIPO, UKIPO, WIPO). Required before committing; "unregistered on npm" is not "free".
- Company registers beyond what search surfaced (Companies House, Handelsregister, etc.).
- Native-speaker review for Hindi, Arabic, Mandarin and Japanese beyond dictionary transliterations.
- App stores (Apple, Google) except where search surfaced them (WalkOn).
- Social handles (GitHub org, X, Bluesky, LinkedIn).
- DNS was checked with `dig NS` only; a domain with no NS can still be registered but undelegated, and a registered domain may be for sale.

## Addendum (2026-10-18): Dummy Run, deeper check at the author's request

Handles and packages: GitHub org `dummyrun` free, user `dummyrun` taken (an account exists), org and user `dummy-run` free; npm `dummyrun` and `dummy-run` unregistered, PyPI both unregistered, RubyGems/crates `dummyrun` unregistered, brew 404. GitHub repos: "Ritchie333/dummyrun" (4 stars, disassembly of the 1985 ZX Spectrum game "Herbert's Dummy Run"), a handful of 0-star hobby projects.
Domains: dummyrun.dev, .io, .ai, .app, .sh have no NS. dummyrun.com registered 2012, parked at HugeDomains (for sale). dummy-run.com registered (Namecheap).
Other uses found: "Herbert's Dummy Run" (Mikro-Gen, 1985, defunct); DUMMY RUN, a Japanese anti-fashion clothing brand (dummyrun.jp); "Dummy Run" indie racing game teaser (IndieDB, 2018, never shipped as far as found); Dummy Run Pty Ltd (Australia, PPE supplier). Nothing in software, QA or AI.
Trademark position: the phrase is a common idiom for a trial run, so for a testing tool it sits near the descriptive end of TMEP 1209.01; the double meaning (dummies that literally run your app) is the kind of incongruity TMEP 1209.01(a) treats as a sign of suggestiveness, and Lowrey et al. 2003 found wordplay helps memory. Still needs a proper search in classes 9 and 42.
Mascot: "Vince and Larry" is a live US registered mark (Reg. 1801195, NHTSA, class 42, renewed 2024) - do not reference those characters. Humanetics holds patents on dummy hardware, not on the look. The generic yellow-and-black target-disc dummy is widely used (toys, films, ads) and an original drawing is low risk; avoid copying Hybrid III photographs or the Vince/Larry costume design. "Crash Test Dummies" is also a band (music classes).
Accessibility: "dummy" as an insult is mild in UK/US English but some style guides flag it; "dummy data" and "dummy run" are standard technical English, which helps.
