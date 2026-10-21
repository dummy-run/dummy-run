---
title: Get started
lead: Download Dummy Run, set one API key and point it at your site. A first-time visitor plays through it in read-only mode. You can be reading what it found in a few minutes.
description: Get started with Dummy Run in a few minutes. Download the dummyrun command and fetch a browser. Then set one API key and run dummyrun try against your site in read-only mode.
next:
  title: Pick personas to run
  text: The library has ready-made personas for the people most sites have, from screen-reader users to someone on a phone.
  label: Browse the persona library
  href: /personas/
---

{{< terminal >}}

{{< try-scan >}}

## What you need

- A Mac, Linux or Windows machine. You don't need Go or a clone of the repository
- One way to reach a model: an OpenAI or Anthropic API key, or [Ollama](https://ollama.com/) running on your own machine. Azure OpenAI, Gemini and Bedrock work too, and [the provider guide](../../docs/PROVIDERS.md) shows how to set each one up
- About 400 MB of room for the browser Dummy Run fetches for you
- Docker, if you want to see the run in a local Grafana later

## 1. Download Dummy Run {#install}

{{< mascot "dummy-keyboard" >}}

Get the archive for your machine from [GitHub Releases](https://github.com/dummy-run/dummy-run/releases), unpack
it and put `dummyrun` on your path. On a Mac with Apple silicon, that looks like this:

```sh
VERSION=...   # the newest release number on the releases page
curl -fsSLO "https://github.com/dummy-run/dummy-run/releases/download/v${VERSION}/dummyrun_${VERSION}_darwin_arm64.tar.gz"
tar -xzf "dummyrun_${VERSION}_darwin_arm64.tar.gz"
```

Every release is signed. [The install guide](../../docs/INSTALL.md) shows how to check a download, and has the other
archives and the container image. If you already have Go 1.27.1 or later, this works as well:

```sh
go install github.com/dummy-run/dummy-run/cmd/dummyrun@latest
```

Check that it's on your path.

<!-- doctest -->
```sh
dummyrun version
```

## 2. Fetch a browser {#browsers}

Dummy Run drives a real browser. This downloads the Chrome for Testing it was tested with and the chromedriver that
matches it, so you never match the two by hand. Each file is checked against a hash that ships with Dummy Run.

```sh
dummyrun browsers install
```

You usually do this once. It lands in your user cache, and Dummy Run finds it from there. A later release may pin a newer browser. It keeps using the one you have until you run the command again. If you're offline, the
command tells you so and you can run it again later.

## 3. Give it a model {#model}

Set one key. Dummy Run looks for `OPENAI_API_KEY` first and `ANTHROPIC_API_KEY` second. A running Ollama comes
third. If you have set `DUMMYRUN_AZURE_ENDPOINT`, Azure goes first, and you then pass your own deployment as `--model` and `--region`. Otherwise it uses the first it finds, with a small, cheap model that can read screenshots and call tools.

```sh
export ANTHROPIC_API_KEY=...
```

The first line of every run says which provider and model it chose. [The provider guide](../../docs/PROVIDERS.md)
lists the defaults and how to pick your own.

## 4. Try your site {#try}

```sh
dummyrun try https://your-site
```

A first-time visitor called Alex lands on your page with no idea what it is. Alex works out what it is for,
what it costs and how to start, for a few minutes and about 15 steps. The browser blocks every request that could
change data and every request to another host.

When it's done you get the findings in your terminal, each with its step number. You also get Alex's own report in
plain words, a screenshot of every step and the run record, with the path to each.

Every finding starts as `pending`, because a model can be wrong. Look at each one next to its screenshots, then
confirm or reject it with `dummyrun review`. The command prints the exact line to run. Reports count only the
findings you've confirmed unless you ask for the rest. The [engine guide](../../docs/ENGINE.md) explains the review
keys and the flags for scripting it.

## 5. Pick the persona yourself {#run}

`dummyrun try` is a shortcut. To choose the persona, the scenario and the pages the browser may visit, use
`dummyrun run`. This sends Tom, a developer looking for the quickstart, to dummyrun.dev. You'll need a copy of the
repository for its example personas:

```sh
git clone https://github.com/dummy-run/dummy-run.git
cd dummy-run
```

```sh
dummyrun run --cast casts/dummyrun-dev --persona tom-developer --scenario find-the-quickstart \
  --target examples/targets/dummyrun-dev.json --out ~/.local/share/dummyrun/runs/dummyrun-dev
```

The target file sets `"read_only": true`, so the browser blocks anything that could change data and won't wander off to other sites. To try your own site, copy `examples/targets/dummyrun-dev.json` and
change `base_url` and `allowed_hosts`. To practise on something you're free to break, run [Inkwell](../../docs/INKWELL.md). It is a small made-up app,
and `go run ./examples/inkwell -test-login` starts it. The
terminal at the top of this page is a real run like this one, on Azure OpenAI. The [command line
reference](../../docs/CLI.md) has every flag, and Azure is one choice among several in the provider guide.

## 6. Look at it in Grafana {#grafana}

`dummyrun-otel` sends a finished run to Grafana or any other OpenTelemetry backend. It is a second command, with its own
release archive, or you can install it with Go:

```sh
go install github.com/dummy-run/dummy-run/integrations/grafana/exporter/cmd/dummyrun-otel@latest
```

Then start a local Grafana and export the run.

```sh
docker run --rm -p 127.0.0.1:4318:4318 -p 127.0.0.1:3000:3000 grafana/otel-lgtm:0.35.0
dummyrun-otel export ~/.local/share/dummyrun/runs/dummyrun-dev
```

Open `http://localhost:3000` and sign in as `admin` with the password `admin`. Pick Explore, choose Loki and query `{service_name="dummyrun"}` over the last 7 days. Each line links to the run's trace. No model set up yet? Export the example records with `dummyrun-otel export examples/runs` and you'll still have something to look at.

## 7. Read the record {#read}

Every record follows [the run record schema](docs/run-record.md), which explains each field and shows a full example. The [Grafana guide](../../docs/integrations/grafana.md) covers the dashboards and alerts.

When you want to show someone else, run `dummyrun report --runs examples/runs --build a865ffd --include pending --out report.html`. You get one HTML file with the findings, what was tested and what the report doesn't claim, ready to send or print. The [report guide](../../docs/REPORT.md) walks through each part.

{{< waitlist >}}

