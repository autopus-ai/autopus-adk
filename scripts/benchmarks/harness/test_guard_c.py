"""Pilot C preflight refusals: harness ancestry, parent instructions, gates, frozen inputs, hook pinning."""
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import guard_c
import sources_c
import surface_c
from gate_c import load_corpus
from oracle_c import frozen_hash

GIT_ENV = {'PATH': '/usr/bin:/bin', 'GIT_CONFIG_GLOBAL': os.devnull, 'GIT_AUTHOR_NAME': 't',
           'GIT_AUTHOR_EMAIL': 't@example.invalid', 'GIT_COMMITTER_NAME': 't', 'GIT_COMMITTER_EMAIL': 't@example.invalid'}


def git(repo, *args):
    return subprocess.run(['git', *args], cwd=repo, env={**GIT_ENV, 'HOME': str(repo)}, check=True,
                          capture_output=True, text=True).stdout.strip()


class AncestryTests(unittest.TestCase):
    def test_a_harness_that_contains_a_task_head_is_refused(self):
        with tempfile.TemporaryDirectory() as d:
            git(d, 'init', '-q', '-b', 'main')
            git(d, 'commit', '-q', '--allow-empty', '-m', 'base')
            base = git(d, 'rev-parse', 'HEAD')
            git(d, 'checkout', '-q', '-b', 'harness')
            git(d, 'commit', '-q', '--allow-empty', '-m', 'harness fix')
            harness = git(d, 'rev-parse', 'HEAD')
            git(d, 'checkout', '-q', '-b', 'feature', base)
            git(d, 'commit', '-q', '--allow-empty', '-m', 'pre')
            pre = git(d, 'rev-parse', 'HEAD')
            git(d, 'commit', '-q', '--allow-empty', '-m', 'fixed')
            fixed = git(d, 'rev-parse', 'HEAD')
            task = {'id': 't', 'heads': {'base': base, 'pre_review': pre, 'fixed': fixed}}
            self.assertEqual(guard_c.contains_task_head(Path(d), harness, [task]), [])
            git(d, 'checkout', '-q', 'main')
            git(d, 'merge', '-q', '--no-ff', '-m', 'merge', 'feature')
            merged = git(d, 'rev-parse', 'HEAD')
            self.assertEqual(len(guard_c.contains_task_head(Path(d), merged, [task])), 2)
            self.assertEqual(sources_c.resolve_revision(Path(d), 'harness'), harness)
            needs = dict(task, harness_requires=[harness])
            self.assertEqual(guard_c.missing_requirements(Path(d), harness, [needs]), [])
            self.assertEqual(len(guard_c.missing_requirements(Path(d), base, [needs])), 1)
            unknown = dict(task, heads=dict(task['heads'], fixed='0' * 40))
            self.assertIn('unknown', guard_c.contains_task_head(Path(d), harness, [unknown])[0])


class OutputTests(unittest.TestCase):
    def test_parent_instructions_are_found(self):
        with tempfile.TemporaryDirectory() as d:
            out = Path(d) / 'a' / 'b' / 'out'
            self.assertEqual(guard_c.parent_instructions(out), [])
            (Path(d) / 'a' / '.claude').mkdir(parents=True)
            self.assertEqual(guard_c.parent_instructions(out), [os.path.realpath(Path(d) / 'a' / '.claude')])
            (Path(d) / 'a' / 'b').mkdir()
            (Path(d) / 'a' / 'b' / 'CLAUDE.md').write_text('x')
            self.assertEqual(len(guard_c.parent_instructions(out)), 2)


    # /tmp and /var/tmp resolve into /private only on macOS, the one host pilot C runs on.
    @unittest.skipUnless(sys.platform == 'darwin', 'macOS temporary-root layout')
    def test_shared_temporary_roots_are_refused(self):
        self.assertEqual(guard_c.shared_temp(Path('/private/tmp/claude-502/run')), ['/private/tmp'])
        self.assertEqual(guard_c.shared_temp(Path('/tmp/run')), ['/private/tmp'])
        self.assertEqual(guard_c.shared_temp(Path('/private/var/tmp/x')), ['/private/var/tmp'])
        self.assertEqual(guard_c.shared_temp(Path('/Users/Shared/pilot-c/run')), [])


class GatesTests(unittest.TestCase):
    def test_gates_must_pass_for_this_corpus(self):
        tasks = load_corpus()
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'gates.json'
            good = {'passed': True, 'frozen_hash': frozen_hash(tasks), 'tasks': ['c01', 'c02']}
            path.write_text(json.dumps(good))
            guard_c.check_gates(path, tasks, guard_c.HERE / 'oracles')
            for bad in (dict(good, passed=False), dict(good, frozen_hash='0'), dict(good, tasks=['c01'])):
                path.write_text(json.dumps(bad))
                with self.assertRaises(SystemExit):
                    guard_c.check_gates(path, tasks, guard_c.HERE / 'oracles')

    def test_freeze_copies_profiles_corpus_and_oracles(self):
        with tempfile.TemporaryDirectory() as d:
            paths = guard_c.freeze(Path(d), guard_c.HERE / 'corpus_c.json')
            for key in ('agent_profile', 'grader_profile', 'corpus'):
                self.assertTrue(paths[key].is_file())
            self.assertEqual(frozen_hash(load_corpus()), frozen_hash(load_corpus(paths['corpus']), paths['oracles']))


class PinTests(unittest.TestCase):
    def test_hook_commands_call_the_pinned_binary(self):
        with tempfile.TemporaryDirectory() as d:
            settings = Path(d) / '.claude' / 'settings.json'
            settings.parent.mkdir()
            hooks = {'Stop': [{'hooks': [{'type': 'command', 'command': 'auto verify stop'}]}],
                     'PreToolUse': [{'matcher': 'Bash', 'hooks': [
                         {'type': 'command', 'command': 'out=$(auto disk gate) && echo "$out"; exit 0'},
                         {'type': 'command', 'command': 'autopus-other run'}]}]}
            settings.write_text(json.dumps({'hooks': hooks}))
            (Path(d) / '.mcp.json').write_text('{}')
            pinned = surface_c.pin_hooks(Path(d), Path('/run dir/bin/auto'))
            self.assertTrue(pinned['mcp_json_removed'])
            self.assertFalse((Path(d) / '.mcp.json').exists())
            data = json.loads(settings.read_text())['hooks']
            self.assertEqual(data['Stop'][0]['hooks'][0]['command'], "'/run dir/bin/auto' verify stop")
            self.assertEqual(data['PreToolUse'][0]['hooks'][0]['command'],
                             'out=$(\'/run dir/bin/auto\' disk gate) && echo "$out"; exit 0')
            self.assertEqual(data['PreToolUse'][0]['hooks'][1]['command'], 'autopus-other run')


if __name__ == '__main__':
    unittest.main()
