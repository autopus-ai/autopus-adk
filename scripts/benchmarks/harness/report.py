"""Descriptive single-agent instruction/skill-exposure ablation reporting.

Usage: python3 report.py --records records.json --json report.json --markdown report.md
"""
import argparse
import itertools
import json
import math
import re
from pathlib import Path

from observe import unique_object, reject_json_constant

ARMS = ("native", "reduced", "current")
ARM_DEFINITIONS = {
    "native": "No project harness instructions or catalog",
    "reduced": "Existing compact split default configuration",
    "current": "Explicit full-catalog configuration; not the default",
}
IDENTIFIER = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,127}\Z")


def _integer(value):
    return type(value) is int and value >= 0


def _elapsed(value):
    return value is None or (type(value) in (int, float) and math.isfinite(value) and value >= 0)


def _validate(records):
    if not isinstance(records, list) or not 0 < len(records) <= 3000:
        raise ValueError("records must be a nonempty bounded list")
    seen = set()
    for row in records:
        if not isinstance(row, dict) or not isinstance(row.get("task_id"), str) or not IDENTIFIER.fullmatch(row["task_id"]):
            raise ValueError("invalid task_id")
        if row.get("arm") not in ARMS:
            raise ValueError("unknown benchmark arm")
        key = (row["task_id"], row["arm"])
        if key in seen:
            raise ValueError("duplicate task/arm observation; one trial per task/arm is supported")
        seen.add(key)
        if type(row.get("accepted")) is not bool or type(row.get("timed_out")) is not bool:
            raise ValueError("accepted and timed_out must be explicit booleans")
        if "elapsed_seconds" not in row or not _elapsed(row["elapsed_seconds"]):
            raise ValueError("invalid elapsed_seconds")
        error = row.get("operational_error")
        if error is not None and (not isinstance(error, str) or not IDENTIFIER.fullmatch(error)):
            raise ValueError("operational_error must be a body-free code")
        observation = row.get("observation")
        if not isinstance(observation, dict) or observation.get("schema_version") != "codex_exec_observation.v1":
            raise ValueError("missing or unknown observation schema")
        for name in ("known_total_tokens", "known_cached_input_tokens", "command_actions"):
            if not _integer(observation.get(name)):
                raise ValueError("missing or invalid observation count: " + name)
        for name in ("total_tokens", "cached_input_tokens"):
            if name not in observation or (observation[name] is not None and not _integer(observation[name])):
                raise ValueError("missing or invalid nullable observation: " + name)
        if type(observation.get("failed")) is not bool:
            raise ValueError("missing observation failure status")
        if observation["total_tokens"] is not None and observation["total_tokens"] != observation["known_total_tokens"]:
            raise ValueError("complete total disagrees with observed subtotal")
        if observation["cached_input_tokens"] is not None and observation["cached_input_tokens"] != observation["known_cached_input_tokens"]:
            raise ValueError("complete cached total disagrees with observed subtotal")
        if observation["known_cached_input_tokens"] > observation["known_total_tokens"]:
            raise ValueError("cached token subset exceeds known token total")


def _nullable_sum(values, complete):
    return sum(values) if complete and all(v is not None for v in values) else None


def build_report(records):
    _validate(records)
    tasks = sorted({row["task_id"] for row in records})
    indexed = {(r["task_id"], r["arm"]): r for r in records}
    result = {
        "schema_version": "harness_ablation_report.v1",
        "scope": "single_agent_instruction_and_skill_exposure_ablation",
        "arm_definitions": ARM_DEFINITIONS,
        "task_ids": tasks,
        "trial_design": "one observation per task and arm; descriptive only",
        "automatic_winner": None,
        "statistical_significance": "not_assessed",
        "cost_usd": None,
        "human_corrections": 0,
        "human_corrections_basis": "no human interventions by experiment design; not model rework measurement",
        "arms": [],
        "paired_tasks": [],
    }
    for arm in ARMS:
        rows = [indexed[(task, arm)] for task in tasks if (task, arm) in indexed]
        observations = [r["observation"] for r in rows]
        complete = len(rows) == len(tasks)
        result["arms"].append({
            "arm": arm, "expected_tasks": len(tasks), "observed_tasks": len(rows),
            "missing_tasks": [task for task in tasks if (task, arm) not in indexed],
            "accepted": sum(r["accepted"] for r in rows),
            "not_accepted": sum(not r["accepted"] for r in rows),
            "timed_out": sum(r["timed_out"] for r in rows),
            "operational_failures": sum(r.get("operational_error") is not None for r in rows),
            "runtime_failures": sum(o["failed"] for o in observations),
            "elapsed_seconds": _nullable_sum([r["elapsed_seconds"] for r in rows], complete),
            "known_elapsed_seconds": sum(r["elapsed_seconds"] for r in rows if r["elapsed_seconds"] is not None),
            "total_tokens": _nullable_sum([o["total_tokens"] for o in observations], complete),
            "known_total_tokens": sum(o["known_total_tokens"] for o in observations),
            "cached_input_tokens": _nullable_sum([o["cached_input_tokens"] for o in observations], complete),
            "known_cached_input_tokens": sum(o["known_cached_input_tokens"] for o in observations),
            "command_actions_observed": sum(o["command_actions"] for o in observations),
            "cost_usd": None,
        })
    for baseline, candidate in itertools.combinations(ARMS, 2):
        for task in tasks:
            left, right = indexed.get((task, baseline)), indexed.get((task, candidate))
            pair = {"task_id": task, "baseline": baseline, "candidate": candidate,
                    "baseline_accepted": left["accepted"] if left else None,
                    "candidate_accepted": right["accepted"] if right else None,
                    "token_delta": None, "cached_input_delta": None, "elapsed_seconds_delta": None,
                    "complete": False, "reason": "missing_record"}
            if left is not None and right is not None:
                for output, key in (("token_delta", "total_tokens"), ("cached_input_delta", "cached_input_tokens")):
                    a, b = left["observation"][key], right["observation"][key]
                    if a is not None and b is not None:
                        pair[output] = b - a
                a, b = left["elapsed_seconds"], right["elapsed_seconds"]
                if a is not None and b is not None:
                    pair["elapsed_seconds_delta"] = b - a
                pair["complete"] = pair["token_delta"] is not None and pair["elapsed_seconds_delta"] is not None
                pair["reason"] = "observed_including_failures" if pair["complete"] else "incomplete_usage_or_elapsed"
            result["paired_tasks"].append(pair)
    return result


def _display(value):
    if value is None:
        return "unknown"
    return f"{value:.3f}" if isinstance(value, float) else str(value)


def to_markdown(report):
    lines = ["# Harness instruction/exposure ablation", "",
             "Scope: single-agent instruction/skill-exposure ablation, not a whole-workflow or multi-agent study.", "",
             "One trial per task/arm. No automatic winner or statistical significance claim. Failures and timeouts remain in every total and paired comparison.", ""]
    for arm in ARMS:
        lines.append(f"- **{arm}**: {ARM_DEFINITIONS[arm]}.")
    lines += ["", "| Arm | Observed/expected | Accepted | Timeouts | Operational failures | Runtime failures | Elapsed (s) | Tokens | Known token subtotal | Cached input | Known cached subtotal | Command actions |",
              "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"]
    for arm in report["arms"]:
        values = [arm["arm"], f'{arm["observed_tasks"]}/{arm["expected_tasks"]}', arm["accepted"], arm["timed_out"], arm["operational_failures"], arm["runtime_failures"], arm["elapsed_seconds"], arm["total_tokens"], arm["known_total_tokens"], arm["cached_input_tokens"], arm["known_cached_input_tokens"], arm["command_actions_observed"]]
        lines.append("| " + " | ".join(_display(v) for v in values) + " |")
    lines += ["", "Tokens = input + output. Cached input and reasoning output are subsets and are not added again. Unknown totals retain the observed subtotal; unknown does not mean zero cost. USD cost is unknown.", "",
              "Human corrections: 0 by the no-intervention design. Command actions are shell execution counts, not human corrections or proof of model rework.", "",
              "## Paired task differences", "", "Deltas are candidate minus baseline; failed tasks are not excluded.", "",
              "| Task | Baseline | Candidate | Acceptance (baseline/candidate) | Token delta | Cached delta | Elapsed delta (s) | Evidence |",
              "| --- | --- | --- | --- | --- | --- | --- | --- |"]
    for pair in report["paired_tasks"]:
        accepted = f'{_display(pair["baseline_accepted"])}/{_display(pair["candidate_accepted"])}'
        values = [pair["task_id"], pair["baseline"], pair["candidate"], accepted, pair["token_delta"], pair["cached_input_delta"], pair["elapsed_seconds_delta"], pair["reason"]]
        lines.append("| " + " | ".join(_display(v) for v in values) + " |")
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--records", type=Path, required=True)
    parser.add_argument("--json", type=Path, required=True)
    parser.add_argument("--markdown", type=Path, required=True)
    args = parser.parse_args()
    if args.records.stat().st_size > 10 * 1024 * 1024:
        parser.error("record file exceeds 10 MiB")
    records = json.loads(args.records.read_text(), object_pairs_hook=unique_object, parse_constant=reject_json_constant)
    report = build_report(records)
    for path, body in ((args.json, json.dumps(report, indent=2, allow_nan=False) + "\n"), (args.markdown, to_markdown(report))):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)
    print(f"JSON: {args.json}\nMarkdown: {args.markdown}")


if __name__ == "__main__":
    main()
