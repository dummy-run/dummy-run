---
title: Get started
lead: Install Dummy Run, run one persona against a site without changing anything, and look at the run in Grafana.
description: Get started with Dummy Run. Install the dummyrun command, run one persona against a site in read-only mode, then export the run to a local Grafana.
next:
  title: Pick personas to run
  text: The persona library holds ready-made personas for the users every site has.
  label: Browse the persona library
  href: /personas/
---

{{< terminal >}}

## What you need

- Go 1.27.1 or later
- Firefox, or Chrome with a matching chromedriver
- A model provider. The steps below use an Azure OpenAI deployment signed in with `az login`. Other providers also work, and [the provider guide](../../docs/PROVIDERS.md) lists what each needs
- Docker, for the local Grafana
- A clone of the repository, for the example cast

```sh
git clone https://github.com/dummy-run/dummy-run.git
cd dummy-run
```

## 1. Install the commands {#install}

`dummyrun` plays a persona. `dummyrun-otel` sends the run to an OpenTelemetry backend.

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

The example cast visits dummyrun.dev. The target file sets `"read_only": true`, so the browser blocks every request that could change data and every request to another host.

```sh
export DUMMYRUN_AZURE_ENDPOINT=https://<resource>.openai.azure.com
dummyrun run --cast casts/dummyrun-dev --persona tom-developer --scenario find-the-quickstart \
  --target examples/targets/dummyrun-dev.json --out ~/.local/share/dummyrun/runs/dummyrun-dev \
  --browser firefox --provider azure --model <deployment> --region uksouth
```

The run ends with a JSON record, the persona's own report and a screenshot for each step. To test your own site, copy `examples/targets/dummyrun-dev.json` and change `base_url` and `allowed_hosts`. To try a persona on an app you are free to break, start [Inkwell](../../docs/INKWELL.md) with `go run ./examples/inkwell -test-login`. To use another provider, change `--provider`, `--model` and the sign-in. The [command line reference](../../docs/CLI.md) lists every flag.

Every finding starts as `pending`. Check each one against its screenshots, then confirm or reject it with `dummyrun review ~/.local/share/dummyrun/runs/dummyrun-dev`. Confirmed findings count in a report by default. The option `--confirmed-only=false` also counts pending ones and marks the report as a demonstration (code: `internal/coverage/report.go`). The Review section of the [engine guide](../../docs/ENGINE.md) has the keys and the flags for scripts.

## 3. Look at it in Grafana {#grafana}

```sh
docker run --rm -p 127.0.0.1:4318:4318 -p 127.0.0.1:3000:3000 grafana/otel-lgtm:0.35.0
dummyrun-otel export ~/.local/share/dummyrun/runs/dummyrun-dev
```

Open `http://localhost:3000` and sign in as `admin` with the password `admin`. Pick Explore, choose Loki and query `{service_name="dummyrun"}` over the last 7 days. Each line links to its trace. To see Grafana without a model deployment, export the example records with `dummyrun-otel export examples/runs`.

## 4. Read the record {#read}

Every record follows [the run record schema](docs/run-record.md). The page lists each field and shows a full example. The [Grafana guide](../../docs/integrations/grafana.md) covers the dashboards and alerts.

To hand the runs to someone else, run `dummyrun report --runs examples/runs --build a865ffd --include pending --out report.html`. It writes one HTML file with the findings, the scope and what the report does not claim. The [report guide](../../docs/REPORT.md) explains each part.

## Sources

The install and run commands are in `docs/ENGINE.md`, code. The export commands are in `integrations/grafana/exporter/README.md`, code. The read-only behaviour is the engine's request interception, code.
