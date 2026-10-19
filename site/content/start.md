---
title: Get started
lead: Install the telemetry exporter, start a local Grafana and look at what a run produces.
description: Get started with Dummy Run. An install command and a local Grafana you can run today, the example run records, and what the persona runner will add.
next:
  title: Pick personas to run
  text: The persona library holds ready-made personas for the users every site has.
  label: Browse the persona library
  href: /personas/
---

Dummy Run is pre-release. You can install the exporter and read real run records today. The runner that plays a persona against your own app is not packaged yet, so this page uses the example records.

## What you need

- Go 1.27 or later
- Docker, for the local Grafana
- A clone of the repository, for the example records

```sh
git clone https://github.com/dummy-run/dummy-run.git
cd dummy-run
```

## 1. Install the exporter {#install}

`dummyrun-otel` reads run records and sends them to any OpenTelemetry backend.

```sh
go install github.com/dummy-run/dummy-run/integrations/grafana/exporter/cmd/dummyrun-otel@latest
```

## 2. Start a local Grafana {#grafana}

This runs Grafana with Tempo, Loki and Prometheus in one container. It is for local use only.

```sh
docker run --rm -p 4318:4318 -p 3000:3000 grafana/otel-lgtm:0.35.0
```

## 3. Export the example runs {#export}

The eight records in `examples/runs/` come from Inkwell, a made-up publishing app.

```sh
dummyrun-otel export examples/runs
```

Open Grafana at `http://localhost:3000` and sign in as `admin` with the password `admin`. Pick Explore, choose Loki and query `{service_name="dummyrun"}` over the last 7 days. Each line links to its trace.

## 4. Read a record {#read}

Every record follows [the run record schema](docs/run-record.md). The page lists each field and shows a full example. The four Grafana dashboards are on the [Grafana page](docs/grafana.md), and the [Grafana guide](../../docs/integrations/grafana.md) covers the setup for each audience.

## The persona runner

The command that plays a persona is `dummyrun run`. It signs one persona in to a preview, works through one scenario and writes a run record. The [architecture](../../docs/ARCHITECTURE.md) describes the browsers, adapters and model providers it uses. The [plan](../../docs/PLAN.md) lists what comes with it.

## Sources

The install and run commands are in `integrations/grafana/exporter/README.md`, code. The example records are synthetic, measured.
