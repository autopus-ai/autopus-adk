"""Pilot C runner contracts: schedule, report, surface config edit, the deadline kill and overlay grading."""
import os
import sys
import tempfile
import time
import unittest
from pathlib import Path

import grade_c
import report_c
import run_c
import surface_c
import trial_c


def row(arm, review, tokens, timed_out=False, error=None, blocks=0, scope=True, oracle=True, contaminated=False):
    graded = {'review_passed': review, 'review_total': 2, 'feature_passed': 1, 'feature_total': 1,
              'guard_passed': 1, 'guard_total': 1} if oracle else None
    return {'task_id': 'c01', 'arm': arm, 'replicate': 1, 'operational_error': error, 'arm_valid': True,
            'agent': {'timed_out': timed_out, 'elapsed_seconds': 10.0, 'exit_code': 0},
            'observation': {'total_tokens': tokens, 'stop_hook_blocks': [{}] * blocks,
                            'contamination': ['gh pr view'] if contaminated else []},
            'scope': {'accepted_scope': scope}, 'oracle': graded}


class ScheduleTests(unittest.TestCase):
    def test_each_arm_takes_each_position_equally_over_six_tasks(self):
        tasks = [{'id': 'c%02d' % i} for i in range(6)]
        plan = run_c.schedule(tasks, ('native', 'instructions', 'full'), 1)
        self.assertEqual(len(plan), 18)
        for position in range(3):
            arms = [plan[i * 3 + position][1] for i in range(6)]
            self.assertEqual({a: arms.count(a) for a in set(arms)}, {'native': 2, 'instructions': 2, 'full': 2})

    def test_replicates_continue_the_rotation(self):
        plan = run_c.schedule([{'id': 'c01'}, {'id': 'c02'}], ('a', 'b', 'c'), 2)
        self.assertEqual([k for _, _, k in plan], [1] * 6 + [2] * 6)
        self.assertNotEqual([a for _, a, _ in plan[:3]], [a for _, a, _ in plan[6:9]])


class ReportTests(unittest.TestCase):
    def test_unknown_usage_ungraded_and_failures_stay(self):
        records = [row('full', 2, 100, blocks=2), row('full', 0, None, timed_out=True, error='agent_timeout'),
                   row('native', 1, 50, scope=False, oracle=False, contaminated=True)]
        summary = report_c.summarize(records)
        self.assertEqual((summary['full']['known_tokens'], summary['full']['usage_unknown']), (100, 1))
        self.assertEqual((summary['full']['review_passed'], summary['full']['review_total']), (2, 4))
        self.assertEqual(summary['full']['stop_hook_blocks'], 2)
        self.assertEqual((summary['native']['ungraded'], summary['native']['contaminated']), (1, 1))
        self.assertEqual(summary['native']['scope_violations'], 1)
        text = report_c.to_markdown(summary, records)
        self.assertIn('| full | 2 | 2/4 | 2/2 | 2/2 | 0 | 1 | 1 | 0 | 0 | 0 |', text)
        self.assertIn('| c01-native-k1 | - | - | - |', text)


class SurfaceTests(unittest.TestCase):
    def test_language_stop_is_disabled_in_place(self):
        text = 'mode: full\nhooks:\n    pre_commit_lore: true\n    language_stop: true\nlanguage:\n  x: 1\n'
        out = surface_c.disable_language_stop(text)
        self.assertIn('    language_stop: false\n', out)
        self.assertNotIn('language_stop: true', out)
        self.assertTrue(out.endswith('language:\n  x: 1\n'))
        added = surface_c.disable_language_stop('hooks:\n  verify_stop: true\nother: 1\n')
        self.assertEqual(added, 'hooks:\n  language_stop: false\n  verify_stop: true\nother: 1\n')
        with self.assertRaises(ValueError):
            surface_c.disable_language_stop('mode: full\n')


class DeadlineTests(unittest.TestCase):
    def test_deadline_kills_setsid_descendants_and_keeps_output(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            marker = root / 'detached-survived'
            # A tool shell in its own session, as Claude Code's Bash tool starts it.
            agent = [sys.executable, '-c', (
                'import subprocess, sys, time\n'
                'subprocess.Popen(["/bin/sh", "-c", "sleep 3; touch %s"], start_new_session=True)\n'
                'print("started", flush=True)\n'
                'time.sleep(60)\n') % marker]
            started = time.monotonic()
            result = trial_c.execute_tree(agent, root, {'PATH': '/usr/bin:/bin'}, root / 'out', root / 'err', 1.0)
            self.assertTrue(result['timed_out'])
            self.assertTrue(result['stragglers'])
            # No sandbox instance to count: an empty sweep cannot prove the tree empty.
            self.assertIsNone(result['leftover'])
            self.assertLess(time.monotonic() - started, 20)
            self.assertIn('started', (root / 'out').read_text())
            time.sleep(3.5)
            self.assertFalse(marker.exists())

    def test_prompt_names_allowed_paths_and_the_test_reset_but_no_oracle(self):
        prompt = trial_c.prompt_for({'prompt': 'Do X.', 'allowed_paths': ['internal/cli/*']})
        self.assertIn('internal/cli/*', prompt)
        self.assertIn('existing tests are restored', prompt)
        self.assertNotIn('oracle', prompt.lower())


class OverlayTests(unittest.TestCase):
    def test_only_allowed_regular_production_files_reach_the_graded_tree(self):
        with tempfile.TemporaryDirectory() as d:
            base, work, dest = Path(d) / 'base', Path(d) / 'work', Path(d) / 'dest'
            for root in (base, work):
                (root / 'internal' / 'cli').mkdir(parents=True)
                (root / 'docs').mkdir()
                (root / 'internal' / 'cli' / 'a.go').write_text('base a')
                (root / 'internal' / 'cli' / 'gone.go').write_text('base gone')
                (root / 'internal' / 'cli' / 'dropped.go').write_text('base dropped')
                (root / 'internal' / 'cli' / 'a_test.go').write_text('base test')
                (root / 'docs' / 'x.md').write_text('base doc')
            (work / 'internal' / 'cli' / 'a.go').write_text('candidate a')
            (work / 'internal' / 'cli' / 'gone.go').unlink()
            (work / 'internal' / 'cli' / 'dropped.go').unlink()
            (work / 'internal' / 'cli' / 'new.go').write_text('candidate new')
            (work / 'internal' / 'cli' / 'a_test.go').write_text('agent test edit')
            (work / 'internal' / 'cli' / 'b_test.go').write_text('agent test')
            (work / 'internal' / 'cli' / 'link.go').symlink_to('/etc/hosts')
            os.mkfifo(work / 'internal' / 'cli' / 'gone.go')  # a FIFO where a base file was
            (work / 'docs' / 'x.md').write_text('agent doc')
            (work / 'CLAUDE.md').write_text('harness')
            record = grade_c.overlay(base, work, ['internal/cli/*'], dest)
            self.assertEqual(sorted(record['copied']), ['internal/cli/a.go', 'internal/cli/new.go'])
            self.assertEqual(record['deleted'], ['internal/cli/dropped.go'])
            self.assertEqual(record['rejected_symlinks'], ['internal/cli/link.go'])
            self.assertEqual(record['rejected_special'], ['internal/cli/gone.go'])
            self.assertEqual((dest / 'internal' / 'cli' / 'gone.go').read_text(), 'base gone')
            self.assertEqual((dest / 'internal' / 'cli' / 'a_test.go').read_text(), 'base test')
            self.assertFalse((dest / 'internal' / 'cli' / 'b_test.go').exists())
            self.assertFalse((dest / 'internal' / 'cli' / 'link.go').exists())
            self.assertEqual((dest / 'docs' / 'x.md').read_text(), 'base doc')
            self.assertFalse((dest / 'CLAUDE.md').exists())


if __name__ == '__main__':
    unittest.main()
