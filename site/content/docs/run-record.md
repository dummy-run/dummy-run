---
title: Run record
weight: 11
group: understand
layout: run-record
summary: The JSON a run writes, field by field, with an example
lead: One persona working through one scenario against one preview. The runner writes it and the reviewer completes it.
description: The Dummy Run run record format. Every field of schema/run-record.schema.json explained, with one of the synthetic example records.
---

The run record is the source of truth for everything else. The results board is built from run records. The [telemetry](../../../docs/TELEMETRY.md) exporter reads run records and maps them to traces, metrics and logs. The schema is `schema/run-record.schema.json` in the repository, with its `$id` at `https://dummyrun.dev/schema/run-record/v1.json`. The eight synthetic examples in `examples/runs/` validate against it in CI, and `examples/generate-runs.py` writes more for dashboards that need history.
