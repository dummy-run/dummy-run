#!/usr/bin/env python3
"""Write synthetic run records for Inkwell, the fictional publishing app used in examples.

Usage: examples/generate-runs.py [--runs N] [--days D] [--end YYYY-MM-DD] [--seed S] [--out DIR]

The output is deterministic for a given seed and end date. The defaults write the eight records in
examples/runs/. Larger sets (for example --runs 400 --days 28) give dashboards some history.
Every record validates against schema/run-record.schema.json.
"""
import argparse
import json
import random
from datetime import datetime, timedelta, timezone
from pathlib import Path

PERSONAS = [
    {"id": "ada-editor", "role": "editor", "device": "desktop"},
    {"id": "sam-author", "role": "author", "device": "phone"},
    {"id": "ravi-reviewer", "role": "reviewer", "device": "desktop"},
    {"id": "lin-contributor", "role": "contributor", "device": "tablet", "zoom": 200},
]
SCENARIOS = {
    "ada-editor": [("schedule-issue", "Schedule next week's issue"), ("reassign-draft", "Reassign a stalled draft")],
    "sam-author": [("submit-draft", "Submit a draft from a phone"), ("reply-to-review", "Reply to review comments")],
    "ravi-reviewer": [("review-queue", "Clear the review queue"), ("approve-piece", "Approve a piece for publishing")],
    "lin-contributor": [("first-pitch", "Send a first pitch"), ("check-payment", "Check a payment status")],
}
# Findings each persona can hit, with the step they surface on and how often.
FINDINGS = {
    "ravi-reviewer": [("reviewer-sees-unassigned-drafts", "permission", "blocker",
                       "A reviewer can open drafts that are not assigned to them", "measured", 0.6)],
    "sam-author": [("submit-button-below-keyboard", "friction", "major",
                    "On a phone the Submit button sits under the keyboard", "measured", 0.5),
                   ("draft-title-field-unlabelled", "accessibility", "minor",
                    "The draft title field has no accessible name", "measured", 0.7)],
    "ada-editor": [("schedule-500-on-past-date", "bug", "major",
                    "Scheduling an issue for a past date returns a server error", "measured", 0.3)],
    "lin-contributor": [("payment-status-wording", "question", "info",
                         "The payment status 'settled' is unclear to a contributor", "inferred", 0.4),
                        ("contributor-sees-rates-table", "permission", "major",
                         "A contributor can open the rates table for all writers", "code", 0.25)],
}
ACTIONS = ["look", "visit", "click", "fill", "click", "look", "select", "click", "type", "scroll", "a11y", "look"]
PROVIDERS = [
    {"kind": "vertex-gemini", "model": "gemini-3.5-flash", "region": "europe-west2"},
    {"kind": "anthropic", "model": "claude-sonnet-5-5", "region": "global"},
]
BROWSERS = [{"name": "chrome", "version": "148.0"}, {"name": "firefox", "version": "146.0"}]


def iso(t):
    return t.astimezone(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def make_run(rng, start, n):
    persona = rng.choice(PERSONAS)
    sid, title = rng.choice(SCENARIOS[persona["id"]])
    env = f"pr-{rng.choice([41, 42, 43, 44])}"
    provider = rng.choice(PROVIDERS)
    steps, t = [], start
    count = rng.randint(6, 18)
    for i in range(1, count + 1):
        action = "finish" if i == count else rng.choice(ACTIONS)
        dur = rng.randint(300, 4500) if action != "a11y" else rng.randint(800, 2000)
        step = {"n": i, "at": iso(t), "action": action, "duration_ms": dur,
                "url": f"https://{env}.inkwell.example/{rng.choice(['drafts', 'issues', 'reviews', 'payments'])}",
                "status": 200, "console_errors": 0}
        if action in ("click", "fill", "select", "type"):
            step["target"] = rng.choice(["Submit", "Title", "Assign to", "Schedule", "Approve", "Comment"])
        if action == "a11y":
            step["a11y_violations"] = rng.randint(0, 3)
        if action == "look":
            step["screenshot"] = f"screens/{n:03d}-{i:02d}.png"
        steps.append(step)
        t += timedelta(milliseconds=dur + rng.randint(1500, 6000))

    findings = []
    for fid, kind, sev, ftitle, evidence, p in FINDINGS.get(persona["id"], []):
        if rng.random() < p:
            at = rng.randint(2, count - 1)
            if kind == "bug":
                steps[at - 1]["status"] = 500
                steps[at - 1]["console_errors"] = 2
            findings.append({"id": fid, "kind": kind, "severity": sev, "title": ftitle, "steps": [at],
                             "evidence": evidence, "verdict": rng.choices(["confirmed", "pending", "rejected"], [7, 2, 1])[0]})

    roll = rng.random()
    outcome = "completed"
    if any(f["kind"] == "bug" for f in findings) or roll < 0.08:
        outcome = "blocked"
    elif roll < 0.12:
        outcome = "budget_exhausted"
    elif roll < 0.15:
        outcome = "errored"
    calls = count + rng.randint(0, 4)
    inp, out = calls * rng.randint(5000, 9000), calls * rng.randint(250, 600)
    price = 0.30e-6 * inp + 2.5e-6 * out if provider["kind"] == "vertex-gemini" else 3e-6 * inp + 15e-6 * out
    return {
        "schema_version": 1,
        "id": f"{start:%Y-%m-%d}-{persona['id']}-{n:03d}",
        "batch": f"{start:%Y-%m-%d}-{env}",
        "started_at": iso(start), "ended_at": iso(t),
        "persona": persona,
        "scenario": {"id": sid, "title": title},
        "target": {"adapter": "url", "environment": env, "base_url": f"https://{env}.inkwell.example",
                   "commit": f"{rng.getrandbits(28):07x}"},
        "provider": provider,
        "browser": rng.choice(BROWSERS),
        "outcome": outcome,
        "steps": steps,
        "findings": findings,
        "usage": {"model_calls": calls, "input_tokens": inp, "output_tokens": out, "cost_usd": round(price, 4)},
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--runs", type=int, default=8)
    ap.add_argument("--days", type=int, default=2)
    ap.add_argument("--end", default="2026-10-17")
    ap.add_argument("--seed", type=int, default=2)
    ap.add_argument("--out", default=str(Path(__file__).parent / "runs"))
    a = ap.parse_args()
    rng = random.Random(a.seed)
    end = datetime.fromisoformat(a.end).replace(hour=18, tzinfo=timezone.utc)
    out = Path(a.out)
    out.mkdir(parents=True, exist_ok=True)
    starts = sorted(end - timedelta(seconds=rng.randint(0, a.days * 86400)) for _ in range(a.runs))
    for n, start in enumerate(starts, 1):
        run = make_run(rng, start, n)
        (out / f"{run['id']}.json").write_text(json.dumps(run, indent=2) + "\n")
    print(f"wrote {a.runs} runs to {out}")


if __name__ == "__main__":
    main()
