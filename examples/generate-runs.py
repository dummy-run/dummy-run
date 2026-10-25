#!/usr/bin/env python3
"""Write synthetic run records for Inkwell, the fictional publishing app used in examples.

Usage: examples/generate-runs.py [--runs N] [--days D] [--end YYYY-MM-DD] [--seed S] [--out DIR]

The output is deterministic for a given seed and end date. The defaults write the eight records in
examples/runs/. Larger sets (for example --runs 400 --days 28) give dashboards some history.
Every record validates against schema/run-record.schema.json (version 2).

Fingerprints follow internal/identity (route|locus|symptom, SHA-256, first 16 hex). Costs come from
internal/price/prices.json. Each environment has one commit, so the runs of one environment can be pooled
by "dummyrun coverage". The records that use the same persona, scenario and model in a batch get a
repeat_index of 0, 1, 2.
"""
import argparse
import hashlib
import json
import random
from datetime import datetime, timedelta, timezone
from pathlib import Path
from urllib.parse import urlsplit

PERSONAS = [
    {"id": "ada-editor", "role": "editor", "device": "desktop"},
    {"id": "sam-author", "role": "author", "device": "phone", "text_scale": 200},
    {"id": "ravi-reviewer", "role": "reviewer", "device": "desktop"},
    {"id": "lin-contributor", "role": "author", "device": "tablet", "zoom": 200},
]
SCENARIOS = {
    "ada-editor": [("schedule-issue", "Schedule next week's issue"), ("reassign-draft", "Reassign a stalled draft")],
    "sam-author": [("submit-draft", "Submit a draft from a phone"), ("reply-to-review", "Reply to review comments")],
    "ravi-reviewer": [("review-queue", "Clear the review queue"), ("approve-piece", "Approve a piece for publishing")],
    "lin-contributor": [("first-pitch", "Send a first pitch"), ("check-payment", "Check a payment status")],
}
# Findings each persona can hit, with the step they surface on and how often.
# Each finding also has the route it appears on, the locus (control or region) and the symptom.
FINDINGS = {
    "ravi-reviewer": [("reviewer-sees-unassigned-drafts", "permission", "blocker",
                       "A reviewer can open drafts that are not assigned to them", "measured", 0.6,
                       "/drafts", "region:draft list", "access-wrongly-allowed")],
    "sam-author": [("submit-button-below-keyboard", "friction", "major",
                    "On a phone the Submit button sits under the keyboard", "measured", 0.5,
                    "/drafts", "button:submit", "obscured"),
                   ("draft-title-field-unlabelled", "accessibility", "minor",
                    "The draft title field has no accessible name", "measured", 0.7,
                    "/drafts", "textbox:title", "a11y:label")],
    "ada-editor": [("schedule-500-on-past-date", "bug", "major",
                    "Scheduling an issue for a past date returns a server error", "measured", 0.3,
                    "/issues", "button:schedule", "error-shown")],
    "lin-contributor": [("payment-status-wording", "question", "info",
                         "The payment status 'settled' is unclear to a contributor", "inferred", 0.4,
                         "/payments", "region:payment status", "unclear-label"),
                        ("author-edits-others-draft", "permission", "major",
                         "An author can open another author's draft editor by its address", "code", 0.25,
                         "/drafts/:id/edit", "page", "access-wrongly-allowed")],
}
DEVICES = {"desktop": (1400, 1000), "tablet": (820, 1180), "phone": (390, 844)}
TAXONOMY_VERSION = 1
PRICES = {m["model"]: m for m in json.loads(
    (Path(__file__).parent.parent / "internal" / "price" / "prices.json").read_text())["models"]}
ACTIONS = ["look", "visit", "click", "fill", "click", "look", "select", "click", "type", "scroll", "a11y", "look"]
PROVIDERS = [
    {"kind": "vertex-gemini", "model": "gemini-3.5-flash-lite", "region": "europe-west2"},
    {"kind": "anthropic", "model": "claude-sonnet-5-5", "region": "global"},
]
BROWSERS = [{"name": "chrome", "version": "148.0"}, {"name": "firefox", "version": "146.0"}]


def iso(t):
    return t.astimezone(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def route_of(url):
    segs = [s for s in urlsplit(url).path.split("/") if s]
    return "/" + "/".join(segs)


def fingerprint(route, locus, symptom):
    return hashlib.sha256(f"{route}|{locus}|{symptom}".encode()).hexdigest()[:16]


def commit_for(env):
    return hashlib.sha256(f"inkwell-{env}".encode()).hexdigest()[:7]


def make_run(rng, start, n, seen):
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
        step["route"] = route_of(step["url"])
        if action == "a11y":
            step["a11y_violations"] = rng.randint(0, 3)
        if action == "look":
            step["screenshot"] = f"screens/{n:03d}-{i:02d}.png"
        steps.append(step)
        t += timedelta(milliseconds=dur + rng.randint(1500, 6000))

    findings = []
    for fid, kind, sev, ftitle, evidence, p, froute, flocus, symptom in FINDINGS.get(persona["id"], []):
        if rng.random() < p:
            at = rng.randint(2, count - 1)
            steps[at - 1]["url"] = f"https://{env}.inkwell.example{froute.replace(':id', '1')}"
            steps[at - 1]["route"] = froute
            if kind == "bug":
                steps[at - 1]["status"] = 500
                steps[at - 1]["console_errors"] = 2
            # The engine records what each finding rests on. A record keeps only the count of violations an a11y step
            # found, not which rules, so a generated finding can never show that axe reported its rule and every one
            # is the persona's own report. The check is the axe rule for an a11y symptom and the symptom otherwise.
            check = symptom.split(":", 1)[1] if symptom.startswith("a11y:") else symptom.split(":")[0]
            inputs = {"evidence_kind": "model-only", "engines": [], "check": check, "on_journey": True}
            findings.append({"id": fid, "kind": kind, "severity": sev, "title": ftitle, "steps": [at],
                             "evidence": evidence, "verdict": rng.choices(["confirmed", "pending", "rejected"], [7, 2, 1])[0],
                             "class": "problem", "route": froute, "locus": flocus, "symptom": symptom,
                             "taxonomy_version": TAXONOMY_VERSION, "fingerprint": fingerprint(froute, flocus, symptom),
                             "inputs": inputs})

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
    rng.getrandbits(28)  # keeps the random stream of earlier versions of this script
    pr = PRICES.get(provider["model"])
    cost = round((pr["input"] * inp + pr["output"] * out) / 1e6, 4) if pr else None
    batch = f"{start:%Y-%m-%d}-{env}"
    key = (batch, persona["id"], sid, provider["model"])
    repeat = seen.get(key, 0)
    seen[key] = repeat + 1
    width, height = DEVICES[persona["device"]]
    zoom = persona.get("zoom", 100)
    goal = {"completed": "yes", "blocked": "no", "errored": "no", "budget_exhausted": "partly"}[outcome]
    if goal == "yes" and any(f["severity"] in ("blocker", "major") for f in findings):
        goal = "partly"
    for f in findings:
        if f["verdict"] != "pending":
            f["reviewer"] = "example-reviewer"
            f["reviewed_at"] = (t + timedelta(hours=20)).strftime("%Y-%m-%dT%H:%M:%SZ")
    return {
        "schema_version": 2,
        "id": f"{start:%Y-%m-%d}-{persona['id']}-{n:03d}",
        "batch": batch,
        "repeat_index": repeat,
        "seed": None,
        "started_at": iso(start), "ended_at": iso(t),
        "persona": {**persona, "dimensions": {
            "viewport": {"width": width * 100 // zoom, "height": height * 100 // zoom},
            "locale": None, "at": "none", "connection": "unthrottled", "zoom": zoom / 100, "motion": None}},
        "scenario": {"id": sid, "title": title, "goal_met": goal},
        "target": {"adapter": "url", "environment": env, "base_url": f"https://{env}.inkwell.example",
                   "commit": commit_for(env)},
        "provider": provider,
        "browser": rng.choice(BROWSERS),
        "outcome": outcome,
        "steps": steps,
        "findings": findings,
        "usage": {"model_calls": calls, "input_tokens": inp, "output_tokens": out, "cost_usd": cost},
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
    seen = {}
    for n, start in enumerate(starts, 1):
        run = make_run(rng, start, n, seen)
        (out / f"{run['id']}.json").write_text(json.dumps(run, indent=2) + "\n")
    print(f"wrote {a.runs} runs to {out}")


if __name__ == "__main__":
    main()
