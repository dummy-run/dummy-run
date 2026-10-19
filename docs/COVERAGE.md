# Coverage

`dummyrun coverage` reads a set of run records and says how much they cover. It answers three questions
separately, because one blended number would hide which of them failed.

1. Reach. How much of the app did the personas touch?
2. Diversity. How different were the personas that did the touching?
3. Completeness. How many distinct problems are probably still unfound?

Every figure names its universe, the total it is a share of. A figure with no stated universe is not printed.
The code is in `internal/coverage/` and `internal/identity/`.

## Count what a set of runs covers

The command reads the run records in a folder. Runs of one build only: problems found on one build say
nothing about another, so the command refuses to mix builds and asks for `--build`.

<!-- doctest -->
```sh
dummyrun coverage --runs examples/runs --build a865ffd
```

For JSON, which the other tools read, add `--format json`.

<!-- doctest -->
```sh
dummyrun coverage --runs examples/runs --build a865ffd --format json | head -n 20
```

The example records are synthetic runs against a made-up app. The report on them shows the shape of the
output and nothing about a real product.

## What each number means

| Number | In one sentence | Its universe | Its limits |
|---|---|---|---|
| Runs by outcome | How many runs completed, were blocked, errored or ran out of budget | The runs of one build | Only completed runs count as samples. The errored share is the harness failure rate |
| Route coverage | The share of the pages that exist that at least one persona opened | The routes in `target.route_universe`, from a sitemap, a crawl or the scenarios | Not printed when the records state no universe. Query strings are ignored when matching routes |
| Persona dimensions | How many values of each dimension the personas used, and how many exist | The values the record format allows, such as 3 devices and 2 browsers | Locale, zoom, viewport and model are open-ended, so only the used count is shown. The engine emulates device, zoom and tool limits. The rest is recorded as the browser had it |
| Problems observed | How many distinct problems the counted findings describe | The fingerprints of reviewer-confirmed problems in completed runs | Depends on the identity rule. See problem identity below |
| Chao2 at least N | A lower bound on the number of distinct problems, which is more than the number seen when many were seen once | The same problems | It is a floor and never a count of what remains. The form is always named. Below 5 runs the interval is decorative |
| Sample coverage | The share of the chance of finding a problem that sits in problems already found | The same problems | Reads as not estimable when only one problem was seen once and none twice. Below 0.50 do not extrapolate |
| New problems per extra run | The number of new problems the last run added, which is the singletons divided by the runs | The same problems | An expected count and not a probability that the next run finds something |
| Jackknife-2 | A second estimate for comparison | The same problems | A check only. It works badly when most problems were seen once or twice |
| Stopping rule | Whether every group of personas has stopped finding new problems | Completed runs in each stratum | Defaults are policy numbers that no study has calibrated |
| Band | The reviewer's severity for a confirmed problem, lowered one step only when it was seen once and its evidence is inferred | One problem | Pending and rejected findings get no band |

## Before you trust a remaining-problem estimate

The estimates of unfound problems need three things. Without them the command still prints, but it labels the
figures as a demonstration and warns.

- Reviewer-confirmed findings. A pending finding is a claim nobody has checked. The default is `--confirmed-only`, which counts only confirmed ones. With `--confirmed-only=false` pending findings count too and the report says it is a demonstration on unreviewed findings. Rejected findings and observations never count.
- Enough runs per stratum. The stopping rule needs 4 completed runs in each stratum, and Chao advises at least 5 samples for the estimator. The report warns for every stratum below the minimum.
- A measured identity rule. Every count assumes two findings are the same problem when their fingerprints match. Give two reviewers' groupings with `--clusters` to measure how far people agree with each other. Below a kappa of 0.6 only the observed count is printed.

## Problem identity

A problem needs the same identity every time a run finds it, or no count of problems means anything. The engine
computes it. The model never writes it.

The fingerprint is the first 16 hex characters of the SHA-256 of `route|locus|symptom`.

- Route. The path of the page where the problem appears. The scheme, host, fragment and trailing slash are dropped. A path segment that is all digits, a UUID or 12 or more hex characters becomes `:id`. The query is reduced to the sorted names of its parameters. The path keeps its case.
- Locus. Where on the page. For a control it is `role:accessible name`, such as `button:open menu`. The persona names the control by its `@12` number from the results and the engine reads the role and name from the page outline. Without a control it is `region:` and the nearest heading or landmark, or `page`.
- Symptom. One value from the closed list below. The persona picks it from an enum in the `finish` tool. If the value is not valid the engine asks the model to call `finish` again, twice at most, and then drops the finding.

Kind and severity are not part of the identity. The same problem can be a bug to one persona and friction to another.

| Symptom | Meaning |
|---|---|
| `not-reachable` | The needed page or control cannot be reached by any route the persona tried |
| `off-screen` | The control exists but sits outside the viewport and cannot be scrolled to |
| `obscured` | Covered by another element such as a keyboard, banner, overlay or sticky bar |
| `not-operable` | Visible but does not respond to the input mode |
| `wrong-outcome` | The action runs and gives a result other than the one the page promises |
| `missing-information:<topic>` | Content the persona needed is not stated. The topic is a slug of at most four words |
| `misleading-copy` | Text that points the reader to a wrong conclusion |
| `unclear-label` | A control or heading whose name does not say what it does |
| `layout-broken` | Overlap, clipping or reflow failure that does not hide a control |
| `slow` | Response or render time beyond what a person would wait |
| `error-shown` | An error page, console error or failed request the persona saw |
| `dead-link` | A link target returns an error status |
| `focus-problem` | Focus order, focus loss or a missing focus indicator |
| `access-wrongly-allowed` | A role did or saw what the policy denies |
| `access-wrongly-denied` | A role was stopped from what the policy allows |
| `content-inconsistent` | Two places on the app say different things |
| `a11y:<rule>` | An automated accessibility rule failure, with the axe rule id |

The engine checks that the rule id of an `a11y` symptom is lower-case letters, digits and hyphens. It does not
check it against the rules of the bundled axe version. Changing the list, the route rules or the fingerprint
is a new `taxonomy_version`, which every finding records. The Go code reproduces the reference vectors in
`internal/identity/testdata/fingerprint_vectors.json`.

`dummyrun coverage` recomputes each fingerprint from its route, locus and symptom and refuses a record whose
fingerprint does not match.

Two findings with different fingerprints may still be one problem. A reviewer can say so in a cluster file, or
set `cluster_id` on the findings. The report then shows the estimates under each way of deciding, in the
identity band. The spread across the band is the uncertainty to report, more than any interval.

## The stopping rule

A stratum is a group of personas. The default stratum is the device, and `--strata persona` uses each persona.
A stratum has stopped when it has at least 4 completed runs, its sample coverage is at least 0.90 and its last 3
completed runs added no new confirmed problem. The batch stops only when every stratum has stopped. A
completed run with no findings counts as evidence. Blocked, errored and budget-exhausted runs count for nothing.

`--threshold` changes the numbers: `coverage` (0.90), `min-runs` (4), `no-new` (3) and `kappa` (0.6). The report
echoes the values it used. `--fail-unless-stopped` exits with 3 when the rule is not satisfied, so a CI job can wait.

## Bands

A confirmed problem gets the reviewer's severity as its band. The band drops one step only when the problem was
seen in one run and its evidence is inferred. Info stays info. There is no score and no weighting, and a
rarely seen problem keeps its severity. The report shows k of n beside every band: k is the runs that saw the
problem and n is the completed runs that reached its route. Problems are listed by band, then evidence, then the
lower end of the Wilson interval for k of n. When runs disagree, the highest severity and the strongest evidence
among them are used. A finding with a `state` is marked, because n counts the runs that reached the route and
cannot see the state.

## The command

| Flag | Meaning |
|---|---|
| `--runs DIR` | A folder or file of run records. Repeat the flag to read several |
| `--build SHA` | The commit or build hash to count, at least 7 characters. Needed when the records come from more than one build |
| `--strata device\|persona` | What the stopping rule treats as separate groups. The default is `device` |
| `--confirmed-only` | On by default. `--confirmed-only=false` also counts pending findings and labels the report a demonstration |
| `--threshold NAME=VALUE` | A policy number: `coverage`, `min-runs`, `no-new` or `kappa`. Repeat the flag for several |
| `--clusters FILE` | A reviewer's grouping of findings into problems. Give one file for a comparison with the fingerprints, or two for kappa |
| `--format json\|markdown` | The default is `markdown` |
| `--out FILE` | Write the report to a file |
| `--fail-unless-stopped` | Exit with 3 when the stopping rule is not satisfied |

A cluster file is `{"clusters": [["<run id>/<finding id>", "<run id>/<finding id>"], ...]}`. A finding the file
does not list is a problem of its own, so a reviewer who clusters blind lists every finding that belongs with another.

Exit codes: 0 for a report, 1 for a failure while reading or writing, 2 for a refusal and 3 for a stopping rule
that is not satisfied. A refusal prints its reason on standard error. The command refuses to pool runs from
more than one build, a record whose fingerprint does not recompute, and a build with no completed run.
Schema version 1 records are listed as not fingerprinted and left out of every count.

## What the engine records

Run records of schema version 2 carry what the command needs. [The schema](../schema/run-record.schema.json)
lists every field.

- Each step has a `route`.
- Each finding has a `route`, a `locus` and a `symptom`.
- Each finding also has the `fingerprint` the engine computes from them, a `class` of problem or observation, and a `taxonomy_version`.
- `scenario.goal_met` says whether the persona did what it came to do. The persona reports it when it finishes.
- `persona.dimensions` records six things: the viewport, the assistive technology and the connection. It also holds the zoom, the locale and the motion setting. The assistive technology is `keyboard-only` when the persona cannot click and `screen-reader-view` when it is sent no screenshots.
- `repeat_index` counts earlier runs of the same persona, scenario and model in the batch, and `seed` is the sampling seed when the provider takes one. `--repeat-index` and `--seed` set them.
- `target.commit` is the build. A URL target with no commit gets the first 12 hex characters of a hash of its home page body and its `Last-Modified` and `ETag` headers. A home page that changes on every request makes every run a build of its own, so give such a target a commit.
- `target.route_universe` lists the routes that exist when a run starts with `--universe sitemap` or `--universe URL`.
- `usage.cost_usd` comes from the price table in `internal/price/prices.json`. Each entry has the vendor page and the date it was read. A model that is not in the table gets null.

## Limits

- The model is built from 19 real runs on two sites and from simulation. No study yet shows that the bands order problems correctly or that the stopping rule works on real apps.
- Fingerprint identity splits one problem into several when personas describe it differently, which pushes the Chao2 figure up. Merging by cluster pushes it down.
- Similarity candidates, which would suggest findings on one route for a reviewer to join, are not built. A reviewer joins findings with a cluster file.
- The dimensions the format allows are the only universe for dimension coverage. The command cannot yet read a list of the dimension values a team says matter.
- Runs share a model and a prompt style, so they are not fully independent and the intervals are optimistic. Exact repeats are visible through `repeat_index`.
