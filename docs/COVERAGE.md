# Coverage

`dummyrun coverage` reads a set of run records and tells you how much ground they covered. It keeps three
questions apart, so you can see which one is weak.

1. Reach. How much of the app did the personas touch?
2. Diversity. How different were the personas that touched it?
3. Completeness. How many distinct problems are probably still out there?

Each figure says what it's a share of. If the records don't say how big the whole is, the figure isn't printed.

## Count what a set of runs covers

Point the command at a folder of run records and name the build:

<!-- doctest -->
```sh
dummyrun coverage --runs examples/runs --build a865ffd
```

Problems found on one build say nothing about another, so the command won't mix builds. If the folder holds
more than one, it asks you for `--build`.

To get JSON for other tools, add `--format json`:

<!-- doctest -->
```sh
dummyrun coverage --runs examples/runs --build a865ffd --format json | head -n 20
```

The example records are synthetic runs against a made-up app. They show you the shape of the output and
nothing about a real product.

## What each number means

| Number | What it tells you | Share of | Watch out for |
|---|---|---|---|
| Runs by outcome | How many runs completed, were blocked, errored or ran out of budget | The runs of one build | Only completed runs count as samples. The errored share is how often the harness itself failed |
| Route coverage | The share of the app's pages that at least one persona opened | The routes in `target.route_universe`, from a sitemap, a crawl or the scenarios | Not printed when the records list no routes. Query strings are ignored when routes are matched |
| Persona dimensions | How many values of each dimension the personas used, and how many exist | The values the record format allows, such as 3 devices and 2 browsers | Locale, zoom, viewport and model are open-ended, so you only see the used count. The engine emulates device, zoom and tool limits. The rest is recorded as the browser had it |
| Problems observed | How many distinct problems the counted findings describe | Reviewer-confirmed problems in completed runs | Depends on how findings are matched. See [Problem identity](#problem-identity) |
| Chao2 at least N | A lower bound on the number of distinct problems | The same problems | It's a floor, never a count of what's left. It rises above the observed count when many problems were seen only once. Below 5 runs its interval means little |
| Sample coverage | The share of the chance of finding a problem that sits in problems you've already found | The same problems | Reads as not estimable when only one problem was seen once and none twice. Below 0.50, don't extrapolate |
| New problems per extra run | How many new problems the last run added, as an expected count from the rarefaction curve | The same problems | It isn't the chance that the next run finds something |
| Jackknife-2 | A second estimate to compare with Chao2 | The same problems | Use it as a cross-check only. It behaves badly when most problems were seen once or twice |
| Stopping rule | Whether every group of personas has stopped finding new problems | Completed runs in each group | The defaults are starting points. Change them with `--threshold` |
| Band | The severity a confirmed problem is listed under | One problem | Pending and rejected findings get no band |

## Before you trust an estimate of what's left

The estimates of unfound problems rest on three things. When one is missing, the command still prints, but it
labels the figures as a demonstration and warns you.

- Reviewer-confirmed findings. You set the verdict with `dummyrun review` (see Review in [ENGINE.md](ENGINE.md)).
  A pending finding is a claim nobody has checked. By default only confirmed findings count. With
  `--confirmed-only=false` pending ones count too, and the report calls itself a demonstration on unreviewed
  findings. Rejected findings and observations never count.
- Enough runs in each group. The stopping rule needs 4 completed runs per group, and Chao2 needs at least 5
  to be useful. The report warns about every group below the minimum.
- A checked matching rule. Every count assumes two findings are the same problem when their fingerprints
  match. To check that people agree, give two reviewers' groupings with `--clusters`. When their agreement
  (Cohen's kappa) is below 0.6, only the observed count is printed.

## The stopping rule

The stopping rule tells you when another run is unlikely to turn up anything new. It works per group of
personas. By default a group is a device, and `--strata persona` makes each persona its own group.

A group has stopped when all three of these hold:

- it has at least 4 completed runs
- its sample coverage is at least 0.90
- its last 3 completed runs found no new confirmed problem

The batch stops only when every group has stopped. A completed run with no findings still counts as evidence.
Blocked, errored and budget-exhausted runs count for nothing.

To change the numbers, use `--threshold`. The names and defaults are `coverage` (0.90) and `min-runs` (4),
then `no-new` (3) and `kappa` (0.6).
The report echoes the values it used. In CI, add `--fail-unless-stopped` and the command exits with 3 until the
rule is met.

## Bands

A confirmed problem's band is the severity the reviewer gave it. It drops one step only when the problem was
seen in a single run and its evidence is inferred. Info stays info. There's no score or weighting, and a rarely
seen problem keeps its severity.

Next to every band you'll see k of n. Here k is the number of runs that saw the problem, and n is the number of
completed runs that reached its page. Problems are listed by band, then by evidence, then by the lower end of
the Wilson interval for k of n. When runs disagree, the highest severity and the strongest evidence win.

A finding that depends on a `state` is marked. That's because n counts every run that reached the page, and it
can't tell whether those runs were in the same state.

## Problem identity

Counting problems only works if the same problem gets the same identity every time a run finds it. The engine
works that identity out. The model never writes it.

The fingerprint is the first 16 hex characters of the SHA-256 of `route|locus|symptom`.

- Route is the path of the page where the problem shows. The scheme, host, fragment and trailing slash are
  dropped. A path segment that's all digits, a UUID, or 12 or more hex characters becomes `:id`. The query
  becomes the sorted names of its parameters. Case is kept.
- Locus is where on the page. For a control it's `role:accessible name`, such as `button:open menu`. The
  persona names the control by its `@12` number, and the engine reads the role and name from the page
  outline. Without a control it's `region:` plus the nearest heading or landmark, or just `page`.
- Symptom is one value from the list below. The persona picks it in the `finish` tool. When the value isn't
  valid, the engine asks the model to call `finish` again. After two retries it drops the finding.

Kind and severity aren't part of the identity, because one persona's bug can be another's friction.

| Symptom | Meaning |
|---|---|
| `not-reachable` | The persona couldn't reach the page or control by any route it tried |
| `off-screen` | The control exists but sits outside the viewport and can't be scrolled to |
| `obscured` | Covered by something else, such as a keyboard, banner, overlay or sticky bar |
| `not-operable` | Visible but doesn't respond to the input mode |
| `wrong-outcome` | The action runs and gives a different result from the one the page promises |
| `missing-information:<topic>` | Something the persona needed isn't stated. The topic is a slug of at most four words |
| `misleading-copy` | Text that leads the reader to a wrong conclusion |
| `unclear-label` | A control or heading whose name doesn't say what it does |
| `layout-broken` | Overlap, clipping or a reflow failure that doesn't hide a control |
| `slow` | A response or render slower than a person would wait for |
| `error-shown` | An error page, console error or failed request the persona saw |
| `dead-link` | A link whose target returns an error status |
| `focus-problem` | Focus order, lost focus or a missing focus indicator |
| `access-wrongly-allowed` | A role did or saw something the policy denies |
| `access-wrongly-denied` | A role was stopped from something the policy allows |
| `content-inconsistent` | Two places in the app say different things |
| `a11y:<rule>` | An automated accessibility failure, with the axe rule id |

For an `a11y` symptom, the engine checks that the rule id is lower-case letters, digits and hyphens. It
doesn't check the id against the bundled axe version. Every finding records a `taxonomy_version`, which changes
whenever the list, the route rules or the fingerprint change.

`dummyrun coverage` recomputes each fingerprint from its route, locus and symptom, and refuses a record whose
fingerprint doesn't match.

Two findings with different fingerprints can still be one problem. A reviewer can say so in a cluster file,
or by setting `cluster_id` on the findings. The report then shows the estimates under each way of grouping, in
the identity band. Read the spread across that band as your uncertainty. It tells you more than any single
interval.

## Flags

| Flag | Meaning |
|---|---|
| `--runs DIR` | A folder or file of run records. Repeat it to read several |
| `--build SHA` | The commit or build hash to count, at least 7 characters. Needed when the records come from more than one build |
| `--strata device\|persona` | What the stopping rule treats as separate groups. The default is `device` |
| `--confirmed-only` | On by default. `--confirmed-only=false` also counts pending findings and labels the report a demonstration |
| `--threshold NAME=VALUE` | A stopping-rule number: `coverage`, `min-runs`, `no-new` or `kappa`. Repeat it to set several |
| `--clusters FILE` | A reviewer's grouping of findings into problems. One file compares it with the fingerprints. Two files give kappa |
| `--format json\|markdown` | The default is `markdown` |
| `--out FILE` | Write the report to a file |
| `--fail-unless-stopped` | Exit with 3 when the stopping rule isn't met |

A cluster file looks like `{"clusters": [["<run id>/<finding id>", "<run id>/<finding id>"], ...]}`. Any finding
the file doesn't list is treated as a problem of its own. So if you cluster without looking at the fingerprints,
list every finding that belongs with another.

The exit codes are 0 for a report, 1 for a read or write failure, 2 for a refusal and 3 for a stopping rule that
isn't met. A refusal prints its reason on standard error. The command refuses to pool runs from more than one
build, a record whose fingerprint doesn't recompute, and a build with no completed run. Schema version 1
records are listed as not fingerprinted and left out of every count.

## What the engine records

Run records of schema version 2 carry everything the command needs. [The schema](../schema/run-record.schema.json)
lists every field.

- Each step has a `route`.
- Each finding has a `route`, a `locus` and a `symptom`, plus the `fingerprint` the engine computes from them.
  It also has a `class` and a `taxonomy_version`. The engine writes `class: problem` for every finding.
- A record can have an `observations` list of `text` and `steps`. These are things that worked, kept apart from
  the findings. They're never counted and have no fingerprint. Older records may hold a finding with
  `class: observation`, and the estimators skip those too.
- `scenario.goal_met` says whether the persona did what it came to do. The persona reports it when it finishes.
- `persona.dimensions` records the viewport, the assistive technology and the connection. It also
  records the zoom, the locale and the motion setting. The assistive technology is `keyboard-only` when the persona can't click, and
  `screen-reader-view` when it gets no screenshots.
- `repeat_index` counts earlier runs of the same persona, scenario and model in the batch. `seed` is the
  sampling seed when the provider takes one. Set them with `--repeat-index` and `--seed`.
- `target.commit` is the build. A URL target with no commit gets the first 12 hex characters of a hash of its
  home page body and its `Last-Modified` and `ETag` headers. If your home page changes on every request, every
  run becomes a build of its own, so give that target a commit.
- `target.route_universe` lists the routes that exist, when the run starts with `--universe sitemap` or
  `--universe URL`.
- `usage.cost_usd` comes from the price table in `internal/price/prices.json`. Each entry names the vendor's
  price page and the date it was read. A model that isn't in the table gets null.

## Limits

- The bands and the stopping rule haven't yet been shown to work on a wide range of real apps. Use them as
  guidance alongside your own judgement.
- When personas describe one problem in different words, fingerprints can split it in two, which pushes the
  Chao2 figure up. Merging findings in a cluster file pushes it down.
- The engine doesn't suggest which findings to merge. You join them yourself with a cluster file.
- Dimension coverage only measures against the values the record format allows. It can't take your own list
  of the dimensions that matter to you.
- Runs share a model and a prompt style, so they aren't fully independent and the intervals are optimistic.
  You can spot exact repeats through `repeat_index`.

## Method

The completeness figures borrow from species estimation in ecology, where you count the species you've seen
and estimate how many you haven't. Each completed run is a sample, and each distinct problem is a species.
Chao2 uses how many problems were seen exactly once and exactly twice, with a log-normal interval. Sample
coverage follows Chao and others (2014), and Jackknife-2 is the second-order jackknife. The k of n intervals are
95 percent Wilson score intervals. The
code is in `internal/coverage/` and the fingerprint is in `internal/identity/`.
