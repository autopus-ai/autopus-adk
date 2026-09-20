"""Conservative, body-free observations from Codex exec --json JSONL logs."""
import json
from pathlib import Path

MAX_BYTES = 64 * 1024 * 1024
MAX_LINE_BYTES = 1024 * 1024
MAX_EVENTS = 100000
MAX_COUNT = 10**15


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def reject_json_constant(_):
    raise ValueError("nonfinite JSON number")


def count(value):
    return type(value) is int and 0 <= value <= MAX_COUNT


def _collaboration(event):
    item = event.get("item", {})
    item = item if isinstance(item, dict) else {}
    kinds = (event.get("type", ""), item.get("type", ""))
    if any(isinstance(k, str) and ("collab" in k.lower().replace("_", "").replace("-", "") or "subagent" in k.lower().replace("_", "").replace("-", "")) for k in kinds):
        return True
    tool = item.get("tool", "")
    return isinstance(tool, str) and tool.replace("_", "").lower() in ("spawnagent", "subagent")


def parse_events(path):
    """Sum valid completed-turn input+output; unknown is never imputed as zero.

    Cache and reasoning tokens are subsets, not additional tokens. Known totals
    are lower bounds from valid completed-turn usage, including rejected runs.
    """
    result = {
        "schema_version": "codex_exec_observation.v1",
        "usage_scope": "single_agent_transcript",
        "total_tokens": None,
        "known_total_tokens": 0,
        "input_tokens": None,
        "output_tokens": None,
        "known_input_tokens": 0,
        "known_output_tokens": 0,
        "cached_input_tokens": None,
        "known_cached_input_tokens": 0,
        "reasoning_output_tokens": None,
        "known_reasoning_output_tokens": 0,
        "cost_usd": None,
        "completed_turns": 0,
        "command_actions": 0,
        "failed": False,
        "failure_codes": [],
        "incomplete_reasons": [],
    }
    reasons, failures = set(), set()
    cached_complete, reasoning_complete = True, True
    outstanding, event_count, consumed = 0, 0, 0
    with Path(path).open("rb") as source:
        while True:
            line = source.readline(MAX_LINE_BYTES + 1)
            if not line:
                break
            consumed += len(line)
            event_count += 1
            if len(line) > MAX_LINE_BYTES or consumed > MAX_BYTES or event_count > MAX_EVENTS:
                reasons.add("transcript_limit_exceeded")
                break
            if not line.endswith(b"\n"):
                reasons.add("truncated_transcript")
            if not line.strip():
                reasons.add("malformed_json")
                continue
            try:
                event = json.loads(line, object_pairs_hook=unique_object, parse_constant=reject_json_constant)
            except (ValueError, UnicodeDecodeError, RecursionError):
                reasons.add("malformed_json")
                continue
            if not isinstance(event, dict) or not isinstance(event.get("type"), str):
                reasons.add("invalid_event_schema")
                continue
            if _collaboration(event):
                reasons.add("subagent_usage_unattributed")
                result["usage_scope"] = "primary_transcript_only_subagent_detected"
            kind = event["type"]
            if kind == "turn.started":
                outstanding += 1
            if kind in ("error", "turn.failed"):
                failures.add("runtime_error" if kind == "error" else "turn_failed")
                reasons.add("failed_attempt_usage_incomplete")
            if kind == "turn.failed":
                outstanding = max(0, outstanding - 1)
            item = event.get("item", {})
            if kind == "item.completed" and isinstance(item, dict) and item.get("type") in ("command_execution", "commandExecution"):
                result["command_actions"] += 1
            if kind != "turn.completed":
                continue
            result["completed_turns"] += 1
            outstanding = max(0, outstanding - 1)
            usage = event.get("usage")
            if not isinstance(usage, dict) or not all(count(usage.get(key)) for key in ("input_tokens", "output_tokens")):
                reasons.add("missing_or_invalid_usage")
                cached_complete = reasoning_complete = False
                continue
            incoming, outgoing = usage["input_tokens"], usage["output_tokens"]
            cached, reasoning = usage.get("cached_input_tokens"), usage.get("reasoning_output_tokens")
            invalid_subset = (cached is not None and (not count(cached) or cached > incoming)) or (reasoning is not None and (not count(reasoning) or reasoning > outgoing))
            if invalid_subset:
                reasons.add("invalid_usage_subset")
                cached_complete = reasoning_complete = False
                continue
            result["known_input_tokens"] += incoming
            result["known_output_tokens"] += outgoing
            result["known_total_tokens"] += incoming + outgoing
            if cached is None:
                cached_complete = False
            else:
                result["known_cached_input_tokens"] += cached
            if reasoning is None:
                reasoning_complete = False
            else:
                result["known_reasoning_output_tokens"] += reasoning
    if outstanding:
        reasons.add("incomplete_turn")
    if result["completed_turns"] == 0:
        reasons.add("no_completed_turn_usage")
    if not reasons:
        for field in ("total_tokens", "input_tokens", "output_tokens"):
            result[field] = result["known_" + field]
        if cached_complete:
            result["cached_input_tokens"] = result["known_cached_input_tokens"]
        if reasoning_complete:
            result["reasoning_output_tokens"] = result["known_reasoning_output_tokens"]
    result["failed"] = bool(failures)
    result["failure_codes"] = sorted(failures)
    result["incomplete_reasons"] = sorted(reasons)
    return result
