import json
import tempfile
import unittest
from pathlib import Path

from observe import parse_events


class ObserveTests(unittest.TestCase):
    def parse(self, events, tail=b""):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "events.jsonl"
            path.write_bytes(b"".join(json.dumps(e).encode() + b"\n" for e in events) + tail)
            return parse_events(path)

    def completed(self, input_tokens=10, output_tokens=5, cached=0):
        return {"type": "turn.completed", "usage": {"input_tokens": input_tokens, "output_tokens": output_tokens, "cached_input_tokens": cached}}

    def test_zero_is_observed_not_missing(self):
        result = self.parse([self.completed(0, 0)])
        self.assertEqual(result["total_tokens"], 0)
        self.assertEqual(result["cached_input_tokens"], 0)
        self.assertIsNone(result["cost_usd"])

    def test_cache_and_reasoning_are_subsets(self):
        event = self.completed(100, 20, 80)
        event["usage"]["reasoning_output_tokens"] = 12
        result = self.parse([event, self.completed(10, 2, 4)])
        self.assertEqual(result["total_tokens"], 132)
        self.assertEqual(result["cached_input_tokens"], 84)
        self.assertIsNone(result["reasoning_output_tokens"])
        self.assertEqual(result["known_reasoning_output_tokens"], 12)

    def test_missing_partial_and_truncated_keep_known_spend(self):
        for events, tail in [
            ([self.completed(), {"type": "turn.completed"}], b""),
            ([self.completed(), {"type": "turn.started"}], b""),
            ([self.completed()], b'{"type":'),
            ([self.completed()], b"invalid\n"),
        ]:
            with self.subTest(events=events, tail=tail):
                result = self.parse(events, tail)
                self.assertIsNone(result["total_tokens"])
                self.assertEqual(result["known_total_tokens"], 15)

    def test_failed_attempt_preserves_prior_cached_usage(self):
        result = self.parse([self.completed(10, 5, 7), {"type": "turn.failed", "error": {"message": "private"}}])
        self.assertTrue(result["failed"])
        self.assertIsNone(result["total_tokens"])
        self.assertEqual(result["known_total_tokens"], 15)
        self.assertEqual(result["known_cached_input_tokens"], 7)
        self.assertNotIn("private", json.dumps(result))

    def test_subagent_usage_unknown_and_shell_actions_not_human_rework(self):
        result = self.parse([
            {"type": "item.completed", "item": {"type": "command_execution", "exit_code": 1}},
            {"type": "item.completed", "item": {"type": "collab_tool_call", "tool": "spawn_agent"}},
            self.completed(),
        ])
        self.assertIsNone(result["total_tokens"])
        self.assertEqual(result["command_actions"], 1)
        self.assertEqual(result["known_total_tokens"], 15)
        self.assertIn("subagent_usage_unattributed", result["incomplete_reasons"])
        self.assertNotIn("human_rework", result)

    def test_invalid_usage_is_not_coerced(self):
        for invalid in [True, -1, 1.5, "10", None]:
            event = self.completed()
            event["usage"]["input_tokens"] = invalid
            result = self.parse([event])
            self.assertIsNone(result["total_tokens"])
            self.assertEqual(result["known_total_tokens"], 0)
        event = self.completed(10, 5, 11)
        self.assertIsNone(self.parse([event])["total_tokens"])

    def test_runtime_error_and_duplicate_json_fields(self):
        result = self.parse([self.completed(), {"type": "error", "message": "private"}])
        self.assertTrue(result["failed"])
        self.assertIsNone(result["total_tokens"])
        result = self.parse([], b'{"type":"turn.completed","type":"error"}\n')
        self.assertIsNone(result["total_tokens"])
        self.assertIn("malformed_json", result["incomplete_reasons"])



class ReportTests(unittest.TestCase):
    def record(self, arm, tokens, accepted=True, failed=False):
        return {"task_id": "task-a", "arm": arm, "accepted": accepted,
                "timed_out": failed, "elapsed_seconds": 2.0,
                "operational_error": "timeout" if failed else None,
                "observation": {"schema_version": "codex_exec_observation.v1",
                    "total_tokens": tokens, "known_total_tokens": tokens if tokens is not None else 7,
                    "cached_input_tokens": min(2, tokens) if tokens is not None else None,
                    "known_cached_input_tokens": min(2, tokens) if tokens is not None else 2, "command_actions": 1,
                    "failed": failed, "cost_usd": None}}

    def test_report_includes_failure_spend_and_unknowns(self):
        from report import build_report, to_markdown
        rows = [self.record("native", 10), self.record("reduced", None, False, True), self.record("current", 30)]
        result = build_report(rows)
        reduced = next(a for a in result["arms"] if a["arm"] == "reduced")
        self.assertEqual(reduced["accepted"], 0)
        self.assertEqual(reduced["timed_out"], 1)
        self.assertEqual(reduced["known_total_tokens"], 7)
        self.assertIsNone(reduced["total_tokens"])
        pair = next(p for p in result["paired_tasks"] if p["baseline"] == "native" and p["candidate"] == "reduced")
        self.assertIsNone(pair["token_delta"])
        self.assertEqual(pair["elapsed_seconds_delta"], 0)
        self.assertIn("single-agent", to_markdown(result))
        self.assertIsNone(result["cost_usd"])

    def test_report_keeps_missing_arm_visible(self):
        from report import build_report
        result = build_report([self.record("native", 10)])
        current = next(a for a in result["arms"] if a["arm"] == "current")
        self.assertEqual(current["missing_tasks"], ["task-a"])
        self.assertIsNone(current["total_tokens"])
        self.assertEqual(len(result["paired_tasks"]), 3)

    def test_report_rejects_duplicates_bad_numbers_and_unknown_usage(self):
        from report import build_report
        row = self.record("native", 10)
        for rows in ([row, row], [{**row, "elapsed_seconds": float("nan")}], [{**row, "accepted": 1}]):
            with self.assertRaises(ValueError):
                build_report(rows)



    def test_report_zero_usage_and_cli_artifacts(self):
        import subprocess
        import sys
        from report import build_report
        row = self.record("native", 0)
        self.assertEqual(build_report([row])["arms"][0]["total_tokens"], 0)
        row["metadata"] = {"prompt": "private-not-in-report"}
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "records.json").write_text(json.dumps([row]))
            completed = subprocess.run([sys.executable, str(Path(__file__).with_name("report.py")),
                "--records", str(root / "records.json"), "--json", str(root / "report.json"),
                "--markdown", str(root / "report.md")], capture_output=True, text=True, timeout=5)
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertNotIn("private-not-in-report", (root / "report.json").read_text())
            self.assertIn("explicit", (root / "report.json").read_text().lower())
            self.assertIn("not a whole-workflow", (root / "report.md").read_text())


if __name__ == "__main__":
    unittest.main()
