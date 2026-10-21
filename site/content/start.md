---
title: Get started
lead: Install Dummy Run, send one persona to a website in read-only mode, and read what it found. If the site needs a sign-in, use a test account. You can be looking at your first run in a few commands.
description: Get started with Dummy Run. Install the dummyrun command, run one persona against a site in read-only mode, then export the run to a local Grafana.
next:
  title: Pick personas to run
  text: The library has ready-made personas for the people most sites have, from screen-reader users to someone on a phone.
  label: Browse the persona library
  href: /personas/
---

{{< terminal >}}

{{< try-scan >}}

## What you need

- Go 1.27.1 or later
- Firefox, or Chrome with a matching chromedriver
- A model to play the persona. The steps below use an Azure OpenAI deployment signed in with `az login`, but Anthropic, Gemini, Bedrock and local models work too. [The provider guide](../../docs/PROVIDERS.md) shows how to set each one up
- Docker, if you want to see the run in a local Grafana
- A copy of the repository, which has the example personas in it

```sh
git clone https://github.com/dummy-run/dummy-run.git
cd dummy-run
```

## 1. Install the commands {#install}

{{< mascot "dummy-keyboard" >}}

There are two commands. `dummyrun` plays the persona, and `dummyrun-otel` sends the finished run to Grafana or any other OpenTelemetry backend.

```sh
go install github.com/dummy-run/dummy-run/cmd/dummyrun@latest
go install github.com/dummy-run/dummy-run/integrations/grafana/exporter/cmd/dummyrun-otel@latest
```

Check that both are on your path.

<!-- doctest -->
```sh
dummyrun version
dummyrun-otel version
```

## 2. Run one persona {#run}

This sends Tom, a developer looking for the quickstart, to dummyrun.dev. The target file sets `"read_only": true`, so the browser blocks anything that could change data and won't wander off to other sites.

```sh
export DUMMYRUN_AZURE_ENDPOINT=https://<resource>.openai.azure.com
dummyrun run --cast casts/dummyrun-dev --persona tom-developer --scenario find-the-quickstart \
  --target examples/targets/dummyrun-dev.json --out ~/.local/share/dummyrun/runs/dummyrun-dev \
  --browser firefox --provider azure --model <deployment> --region uksouth
```

When it's done you'll have a JSON record of the run, Tom's own report in plain words and a screenshot of every step. To try your own site, copy `examples/targets/dummyrun-dev.json` and change `base_url` and `allowed_hosts`. If you'd rather start somewhere you're free to break things, run [Inkwell](../../docs/INKWELL.md), a small made-up app, with `go run ./examples/inkwell -test-login`. For another provider, change `--provider`, `--model` and the sign-in. The [command line reference](../../docs/CLI.md) has every flag.

Every finding starts as `pending`, because a model can be wrong. Look at each one next to its screenshots, then confirm or reject it with `dummyrun review ~/.local/share/dummyrun/runs/dummyrun-dev`. Reports count only the findings you've confirmed unless you ask for the rest. The [engine guide](../../docs/ENGINE.md) explains the review keys and the flags for scripting it.

## 3. Look at it in Grafana {#grafana}

```sh
docker run --rm -p 127.0.0.1:4318:4318 -p 127.0.0.1:3000:3000 grafana/otel-lgtm:0.35.0
dummyrun-otel export ~/.local/share/dummyrun/runs/dummyrun-dev
```

Open `http://localhost:3000` and sign in as `admin` with the password `admin`. Pick Explore, choose Loki and query `{service_name="dummyrun"}` over the last 7 days. Each line links to the run's trace. No model set up yet? Export the example records with `dummyrun-otel export examples/runs` and you'll still have something to look at.

## 4. Read the record {#read}

Every record follows [the run record schema](docs/run-record.md), which explains each field and shows a full example. The [Grafana guide](../../docs/integrations/grafana.md) covers the dashboards and alerts.

When you want to show someone else, run `dummyrun report --runs examples/runs --build a865ffd --include pending --out report.html`. You get one HTML file with the findings, what was tested and what the report doesn't claim, ready to send or print. The [report guide](../../docs/REPORT.md) walks through each part.

{{< waitlist >}}

