"""Pilot C runner contracts: schedule, report, surface config edit and the deadline kill."""
import os
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

import run_c
import surface_c
import trial_c


def row(arm, passed, review, tokens, timed_out=False, error=None, blocks=0, scope=True):
    return {'task_id': 'c01', 'arm': arm, 'replicate': 1, 'operational_error': error,
            'agent': {'timed_out': timed_out, 'elapsed_seconds': 10.0, 'exit_code': 0},
            'observation': {'total_tokens': tokens, 'stop_hook_blocks': [{}] * blocks},
            'scope': {'accepted_scope': scope},
            'oracle': {'passed': passed, 'total': 4, 'review_passed': review, 'review_total': 2}}


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


class RevisionTests(unittest.TestCase):
    def test_harness_revision_resolves_to_a_commit_sha(self):
        with tempfile.TemporaryDirectory() as d:
            env = {'PATH': '/usr/bin:/bin', 'HOME': d, 'GIT_CONFIG_GLOBAL': os.devnull, 'GIT_AUTHOR_NAME': 't',
                   'GIT_AUTHOR_EMAIL': 't@example.invalid', 'GIT_COMMITTER_NAME': 't',
                   'GIT_COMMITTER_EMAIL': 't@example.invalid'}
            for args in (['init', '-q', '-b', 'main'], ['commit', '-q', '--allow-empty', '-m', 'one']):
                subprocess.run(['git', *args], cwd=d, env=env, check=True)
            head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=d, env=env, text=True).strip()
            self.assertEqual(run_c.resolve_revision(Path(d), 'main'), head)
            with self.assertRaises(subprocess.CalledProcessError):
                run_c.resolve_revision(Path(d), 'no-such-branch')


class ReportTests(unittest.TestCase):
    def test_unknown_usage_is_counted_apart_and_failures_stay(self):
        records = [row('full', 4, 2, 100, blocks=2), row('full', 1, 0, None, timed_out=True, error='agent_timeout'),
                   row('native', 2, 0, 50, scope=False)]
        summary = run_c.summarize(records)
        self.assertEqual(summary['full']['known_tokens'], 100)
        self.assertEqual(summary['full']['usage_unknown'], 1)
        self.assertEqual(summary['full']['timed_out'], 1)
        self.assertEqual(summary['full']['cases_passed'], 5)
        self.assertEqual(summary['full']['stop_hook_blocks'], 2)
        self.assertEqual(summary['native']['scope_violations'], 1)
        text = run_c.to_markdown(summary, records)
        self.assertIn('| full | 2 | 5/8 | 2/4 | 1 | 1 | 0 |', text)
        self.assertIn('agent_timeout', text)


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

    def test_stop_hooks_are_read_from_generated_settings(self):
        with tempfile.TemporaryDirectory() as d:
            settings = Path(d) / '.claude' / 'settings.json'
            settings.parent.mkdir()
            settings.write_text('{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "auto verify stop"}]}]}}')
            self.assertEqual(surface_c.stop_hooks(Path(d)), ['auto verify stop'])
            self.assertEqual(surface_c.stop_hooks(Path(d) / 'none'), [])


class DeadlineTests(unittest.TestCase):
    def test_deadline_kills_the_whole_group_and_keeps_output(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            marker = root / 'child-survived'
            script = 'sleep 3 && touch %s & echo started; sleep 30' % marker
            started = time.monotonic()
            result = trial_c.execute_exact(['/bin/sh', '-c', script], root, {'PATH': '/usr/bin:/bin'},
                                           root / 'out', root / 'err', 0.5)
            self.assertTrue(result['timed_out'])
            self.assertLess(time.monotonic() - started, 15)
            self.assertIn('started', (root / 'out').read_text())
            time.sleep(3.5)
            self.assertFalse(marker.exists())

    def test_prompt_names_allowed_paths_but_no_oracle(self):
        prompt = trial_c.prompt_for({'prompt': 'Do X.', 'allowed_paths': ['internal/cli/*']})
        self.assertIn('internal/cli/*', prompt)
        self.assertNotIn('oracle', prompt.lower())


if __name__ == '__main__':
    unittest.main()
