"""Golden-mode documents, digests, trial classification and agent confinement contracts (cross-platform)."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import golden_agent as ga
import golden_protocol as gp
import golden_trial as gt

HERE = Path(__file__).resolve().parent
REFERENCE = HERE.parents[2] / 'pkg/harneval/testdata/live-session'
CANARIES = {'GITHUB_TOKEN': 'leak', 'ACTIONS_ID_TOKEN_REQUEST_TOKEN': 'leak', 'RUNNER_TEMP': 'leak',
            'OPENAI_API_KEY': 'leak', 'HARNEVAL_CANARY': 'leak'}


class ScheduleAndDocumentTests(unittest.TestCase):
    def test_balanced_order_matches_the_scenario_and_the_reference_session(self):
        short = [(row['task_id'][-3:], row['arm'], row['trial']) for row in gp.schedule(['GT-AG-002', 'GT-AG-001'], 2)]
        self.assertEqual(short, [('001', 'baseline', 0), ('001', 'candidate', 0), ('002', 'candidate', 0),
                                 ('002', 'baseline', 0), ('001', 'candidate', 1), ('001', 'baseline', 1),
                                 ('002', 'baseline', 1), ('002', 'candidate', 1)])
        reference = json.loads((REFERENCE / 'protocol.json').read_text())
        self.assertEqual(gp.schedule(['GT-AGENT-A03', 'GT-AGENT-A01', 'GT-AGENT-A02'], 2), reference['order'])

    def test_protocol_and_record_carry_exactly_the_reference_session_fields(self):
        reference = json.loads((REFERENCE / 'protocol.json').read_text())
        manifest = {'live': reference['policy'], 'pins': reference['pins']}
        protocol = gp.protocol_document(reference['session_id'], reference['started_at'], manifest,
                                        {'baseline': 'b' * 64, 'candidate': 'c' * 64}, 'a' * 64,
                                        reference['corpus_digests'], 'd' * 64, 'e' * 64,
                                        {**reference['calibration'], 'runs': []}, reference['cli_version'],
                                        reference['order'])
        self.assertEqual(list(protocol), list(reference))
        for key in ('workspace_revision', 'baseline_ref', 'policy', 'pins', 'model', 'calibration', 'prompt_layers'):
            self.assertEqual(protocol[key], reference[key], key)
        line = (REFERENCE / 'records.jsonl').read_text().splitlines()[1]
        expected = json.loads(line)
        record = gp.record_document(expected['session_id'], expected, expected['signal'], expected['oracle'],
                                    expected['duration_s'])
        self.assertEqual(record, expected)

    def test_signal_table_is_the_requirement_table(self):
        self.assertEqual({signal for signal, outcome in gp.SIGNAL_OUTCOMES.items() if outcome == 'error'},
                         {'workspace_setup_failed', 'mutation_failed', 'warmup_failed'})
        self.assertEqual([signal for signal, outcome in gp.SIGNAL_OUTCOMES.items() if outcome == 'pass'], ['accepted'])
        self.assertEqual(len(gp.SIGNAL_OUTCOMES), 13)

    def test_frozen_documents_are_exclusive_and_records_append(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'protocol.json'
            gp.write_exclusive(path, {'a': 1})
            with self.assertRaises(FileExistsError):
                gp.write_exclusive(path, {'a': 2})
            self.assertEqual(json.loads(path.read_text()), {'a': 1})
            records = Path(directory) / 'records.jsonl'
            gp.append_record(records, {'n': 1})
            gp.append_record(records, {'n': 2})
            self.assertEqual(records.read_text(), '{"n":1}\n{"n":2}\n')


class DigestTests(unittest.TestCase):
    def test_runner_digest_covers_every_runner_file_and_nothing_else(self):
        # The copy keeps the checkout layout: ORACLE_FILES are read at their checkout path (SPEC-HARNEVAL-003).
        with tempfile.TemporaryDirectory() as directory:
            checkout, copy = Path(directory), Path(directory) / 'scripts/benchmarks/harness'
            files = [copy / name for name in gp.RUNNER_FILES + ('README.md', 'test_golden.py')]
            files += [checkout / name for name in gp.ORACLE_FILES + ('cmd/harneval-oracle/main_test.go',)]
            for path in files:
                path.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(HERE.parents[2] / path.relative_to(checkout), path)
            original = gp.runner_digest(copy)
            self.assertEqual(original, gp.runner_digest(HERE))
            for extra in (copy / 'README.md', copy / 'test_golden.py', checkout / 'cmd/harneval-oracle/main_test.go'):
                with extra.open('a') as prose:
                    prose.write('more prose\n')
            self.assertEqual(gp.runner_digest(copy), original)
            for path in [copy / name for name in gp.RUNNER_FILES] + [checkout / name for name in gp.ORACLE_FILES]:
                with self.subTest(str(path.relative_to(checkout))):
                    data = path.read_bytes()
                    path.write_bytes(data + b'\n')
                    self.assertNotEqual(gp.runner_digest(copy), original)
                    path.write_bytes(data)

    def test_oracle_files_are_the_sources_of_the_oracle_harness(self):
        sources = sorted(path.relative_to(HERE.parents[2]).as_posix()
                         for path in (HERE.parents[2] / 'cmd/harneval-oracle').glob('*.go')
                         if not path.name.endswith('_test.go'))
        self.assertEqual(sorted(gp.ORACLE_FILES), sources)

    def test_runner_files_are_the_modules_the_runner_loads_plus_the_profile_and_the_driver(self):
        # grader.sb goes to sandbox-exec and surface_driver/main.go into every arm build (T13).
        probe = ('import sys, golden; from pathlib import Path; here = Path(golden.__file__).parent; '
                 'print("\\n".join(sorted(Path(m.__file__).name for m in list(sys.modules.values()) '
                 'if getattr(m, "__file__", None) and Path(m.__file__).parent == here)))')
        loaded = subprocess.run([sys.executable, '-c', probe], cwd=HERE, capture_output=True, text=True, check=True,
                                env={**os.environ, 'PYTHONDONTWRITEBYTECODE': '1'}).stdout.split()
        self.assertIn('golden_surface.py', loaded)
        self.assertIn('golden_blackbox_trial.py', loaded)
        profiles = ['artifact.sb', 'grader.sb', 'oracle.sb', 'surface_driver/main.go']
        self.assertEqual(sorted(gp.RUNNER_FILES), sorted(loaded + profiles))

    def test_surface_digest_is_the_go_surface_digest(self):
        # The literal is harneval.SurfaceDigest of this tree, printed by a Go test on 2026-10-07.
        files = {'AGENTS.md': '# fixture surface\n', '.codex/config.toml': 'model = "gpt-6-astra"\n',
                 '.codex/skills/alpha/SKILL.md': 'alpha skill\n', '.autopus/txns/0001.json': '{"at": "2026-10-07T00:00:00Z"}\n',
                 '.autopus/codex-manifest.json': '{"generated_at": 1}\n', '.autopus/omp-manifest.json': '{"generated_at": 2}\n',
                 '.autopus/other-manifest.json': '{"kept": true}\n', 'nested/über/notes.txt': 'unicode path\n',
                 'nested/a.txt': ''}
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for rel, content in files.items():
                (root / rel).parent.mkdir(parents=True, exist_ok=True)
                (root / rel).write_text(content)
            self.assertEqual(gp.surface_digest(root), 'e0d4279ac1a8f36635b855134ccf4d53e55c5c5fb201c9419fe924b48edca35a')
            (root / 'link.md').symlink_to('AGENTS.md')
            with self.assertRaisesRegex(ValueError, 'link.md is not a regular file'):
                gp.surface_digest(root)


class TrialClassificationTests(unittest.TestCase):
    RUN = {'launched': True, 'exit_code': 0, 'timed_out': False, 'leftover': False, 'transcript_bytes': 120}
    SEEN = {'failed': False}

    def test_agent_stage_signals_in_order(self):
        cases = [({**self.RUN, 'launched': False}, None, 'agent_launch_failed'),
                 ({**self.RUN, 'exit_code': 1, 'transcript_bytes': 0}, None, 'agent_launch_failed'),
                 ({**self.RUN, 'timed_out': True, 'exit_code': -15, 'transcript_bytes': 0}, None, 'agent_timeout'),
                 ({**self.RUN, 'exit_code': 3}, self.SEEN, 'agent_exit_nonzero'),
                 (self.RUN, None, 'observation_failed'), (self.RUN, {'failed': True}, 'observation_failed'),
                 ({**self.RUN, 'leftover': True}, self.SEEN, 'observation_failed'), (self.RUN, self.SEEN, None)]
        for run, observation, expected in cases:
            with self.subTest(expected=expected, run=run):
                self.assertEqual(gt.agent_signal(run, observation), expected)

    def test_decide_orders_agent_scope_forbidden_then_oracle(self):
        self.assertEqual(gt.decide('agent_timeout', False, ['os.Exit'], 'accepted'), 'agent_timeout')
        self.assertEqual(gt.decide(None, False, ['os.Exit'], None), 'scope_violation')
        self.assertEqual(gt.decide(None, True, ['os.Exit'], None), 'forbidden_construct')
        for oracle in ('accepted', 'oracle_failed', 'oracle_timeout', 'oracle_output_invalid'):
            self.assertEqual(gt.decide(None, True, [], oracle), oracle)
            self.assertEqual(gp.SIGNAL_OUTCOMES[gt.decide('agent_exit_nonzero', True, [], oracle)], 'fail')

    def test_only_newly_added_forbidden_literals_count(self):
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            original = b'package p\n\nimport "os"\n\nfunc main() { os.Exit(1) }\n'
            (work / 'p.go').write_bytes(original)
            self.assertEqual(gt.new_forbidden({'p.go': original}, work, ['p.go']), [])
            for literal in gt.FORBIDDEN:
                with self.subTest(literal):
                    (work / 'p.go').write_bytes(original + b'// ' + literal.encode() + b'\n')
                    self.assertEqual(gt.new_forbidden({'p.go': original}, work, ['p.go']), [literal])
            (work / 'p.go').unlink()
            self.assertEqual(gt.new_forbidden({'p.go': original}, work, ['p.go']), [])

    def test_warm_command_runs_no_test(self):
        command = ['go', 'test', '-p', '1', './pkg/a', '-run', '^TestA$', '-count=1']
        self.assertEqual(gt.warm_command(command), ['go', 'test', '-p', '1', './pkg/a', '-run', '^$', '-count=1'])
        self.assertEqual(gt.warm_command(['go', 'test', './a']), ['go', 'test', './a', '-run', '^$'])


class AgentConfinementTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()
        self.root = self.base / 'agent'
        for name in ('ws', 'gocache', 'tmp', 'home'):
            (self.root / name).mkdir(parents=True)
        self.prepared = {'go': str(self.base / 'go/bin/go'), 'modcache': str(self.base / 'modcache')}
        self.codex = str(self.base / 'tools/codex')

    def test_agent_environment_is_the_allowlist_plus_credentials_only(self):
        with mock.patch.dict(os.environ, CANARIES):
            env = ga.agent_env(self.root, self.prepared, self.codex, {'HARNEVAL_AGENT_CREDENTIAL': 'secret'})
        self.assertEqual(set(env), set(ga.AGENT_KEYS) | {'HARNEVAL_AGENT_CREDENTIAL'})
        self.assertEqual(env['PATH'].split(os.pathsep), [str(self.base / 'tools'), str(self.base / 'go/bin'),
                                                         '/usr/bin', '/bin', '/usr/sbin', '/sbin'])
        self.assertEqual((env['HOME'], env['GOMODCACHE'], env['GOPROXY']), (str(self.root / 'home'),
                                                                            self.prepared['modcache'], 'off'))
        self.assertNotIn('leak', json.dumps(env))
        with self.assertRaisesRegex(ValueError, 'PATH'):
            ga.agent_env(self.root, self.prepared, self.codex, {'PATH': '/evil'})

    @unittest.skipUnless(shutil.which('go'), 'profile_args asks go for GOPATH')
    def test_codex_commands_inherit_only_the_allowlist_under_the_pilot_profile(self):
        argv = ga.agent_argv(self.codex, 'gpt-6-astra', self.root, Path(self.prepared['modcache']))
        self.assertEqual(argv[:5], [self.codex, 'exec', '--ephemeral', '--ignore-user-config', '--strict-config'])
        self.assertIn('shell_environment_policy.inherit="all"', argv)
        self.assertIn('shell_environment_policy.include_only=' + json.dumps(list(ga.AGENT_KEYS)), argv)
        filesystem = next(value for value in argv if value.startswith('permissions.bench.filesystem='))
        self.assertIn(json.dumps(str(self.base / 'modcache')) + '="read"', filesystem)
        for name in ('ws', 'gocache', 'tmp'):
            self.assertIn(json.dumps(str(self.root / name)) + '="write"', filesystem)
        self.assertEqual(argv[-5:], ['--skip-git-repo-check', '-C', str(self.root / 'ws'), '--json', '-'])
        self.assertEqual(argv[argv.index('-m') + 1], 'gpt-6-astra')
        self.assertIn('approval_policy="never"', argv)


if __name__ == '__main__':
    unittest.main()
