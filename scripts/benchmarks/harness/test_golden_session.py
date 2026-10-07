"""Whole golden sessions on the fixture world under the real grader sandbox, judged by the built `auto`.

macOS only: calibration, warmup and grading run inside grader.sb. The agent is the fake codex of
test_golden_fixture, so no model is called. Each session ends in `auto eval harness report`.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import golden
import golden_agent as ga
import golden_protocol as gp
import golden_trial as gt
import grader
from test_golden_fixture import PIN, argv, build_world, calls

CANARIES = {'GITHUB_TOKEN': 'leak', 'ACTIONS_RUNTIME_TOKEN': 'leak', 'RUNNER_TEMP': 'leak', 'HARNEVAL_CANARY': 'leak'}
# The fake codex is a Python script: its interpreter adds these two keys itself on macOS (PEP 538, CoreFoundation).
INTERPRETER_KEYS = {'LC_CTYPE', '__CF_USER_TEXT_ENCODING'}
NONE_RAN = {'ran': False, 'build_failed': False, 'expected_passed': 0, 'expected_failed': 0}


def report(auto: str, session: Path) -> tuple:
    completed = subprocess.run([auto, 'eval', 'harness', 'report', '--input', str(session), '--format', 'json'],
                               capture_output=True, text=True, timeout=120)
    return completed.returncode, (json.loads(completed.stdout) if completed.returncode == 0 else completed.stderr)


def records(session: Path) -> list:
    return [json.loads(line) for line in (session / 'records.jsonl').read_text().splitlines()]


@unittest.skipUnless(sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and shutil.which('go'),
                     'requires macOS sandbox-exec and the Go toolchain')
class GoldenSessionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        directory = tempfile.TemporaryDirectory()
        cls.addClassCleanup(directory.cleanup)
        cls.tools = Path(directory.name)
        cls.auto = golden.build_auto(cls.tools)

    def world(self, names, **options):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        return build_world(Path(directory.name), names, **options)

    def run_golden(self, world, steps=None, *extra):
        options = golden.parse(argv(world, '--auto', self.auto, '--credential-env', 'HARNEVAL_AGENT_CREDENTIAL', *extra))
        with mock.patch.dict(os.environ, {**CANARIES, 'HARNEVAL_AGENT_CREDENTIAL': 'agent-only'}):
            return golden.run_session(options, steps or golden.Steps())

    def test_s8_frozen_protocol_balanced_order_no_retry_and_hard_flip_report(self):
        world = self.world(['alpha', 'beta'])
        warmups = []

        def warmup(*args):
            warmups.append(args)
            return len(warmups) != 3 and gt.warmup(*args)
        summary = self.run_golden(world, golden.Steps(warmup=warmup))
        self.assertEqual((summary['records'], summary['calibration_after']), (8, 'passed'))
        protocol = json.loads((world.session / 'protocol.json').read_text())
        order = [(row['task_id'][-3:], row['arm'][0], row['trial']) for row in protocol['order']]
        self.assertEqual(order, [('X01', 'b', 0), ('X01', 'c', 0), ('X02', 'c', 0), ('X02', 'b', 0),
                                 ('X01', 'c', 1), ('X01', 'b', 1), ('X02', 'b', 1), ('X02', 'c', 1)])
        rows = records(world.session)
        self.assertEqual([(row['task_id'], row['arm'], row['trial']) for row in rows],
                         [(row['task_id'], row['arm'], row['trial']) for row in protocol['order']])
        self.assertEqual((rows[2]['outcome'], rows[2]['signal'], rows[2]['oracle']), ('error', 'warmup_failed', NONE_RAN))
        for row in rows[:2] + rows[3:]:
            expected = ('pass', 'accepted') if row['arm'] == 'baseline' else ('fail', 'oracle_failed')
            self.assertEqual((row['outcome'], row['signal'], row['oracle']['ran']), expected + (True,))
        made = calls(world)
        self.assertEqual(len(made), 7)
        for call in made:
            self.assertEqual(set(call['env']) - INTERPRETER_KEYS, set(ga.AGENT_KEYS) | {'HARNEVAL_AGENT_CREDENTIAL'})
            self.assertEqual(call['env']['HARNEVAL_AGENT_CREDENTIAL'], 'agent-only')
            self.assertIn('--strict-config', call['argv'])
            self.assertNotIn('HARNEVAL_AGENT_CREDENTIAL', ' '.join(call['argv']))
        live = world.manifest['live']
        self.assertEqual([protocol[key] for key in ('workspace_revision', 'baseline_ref', 'model', 'cli_version')],
                         [live['workspace_revision'], live['baseline_ref'], live['model'], PIN])
        self.assertEqual(protocol['runner_sha256'], gp.runner_digest())
        self.assertEqual(protocol['grader_profile_sha256'], hashlib.sha256(grader.PROFILE.read_bytes()).hexdigest())
        self.assertEqual(protocol['candidate_surface_digest'], gp.surface_digest(world.surfaces / 'candidate'))
        trials = [json.loads(path.read_text()) for path in sorted((world.session / 'trials').glob('*/trial.json'))]
        self.assertEqual(len({trial['snapshot_tree_sha256'] for trial in trials}), 1)
        self.assertFalse((world.session / 'scratch').exists())
        code, document = report(self.auto, world.session)
        self.assertEqual(code, 0, document)
        self.assertEqual((document['verdict'], document['reason'], document['hard_flips']),
                         ('regression', 'hard_flip', ['GT-AGENT-X01']))
        self.assertEqual((document['arms']['baseline']['passes'], document['arms']['baseline']['valid'],
                          document['arms']['candidate']['passes'], document['arms']['candidate']['valid']), (4, 4, 0, 3))
        self.assertAlmostEqual(document['completeness'], 0.875, delta=1e-9)
        self.assertEqual((document['advisory'], document['calibration']['status'], document['runner_sha256'],
                          document['agent_set_digest']), (True, 'passed', gp.runner_digest(), protocol['agent_set_digest']))

    def test_s10_failed_calibration_refuses_with_protocol_and_calibration_only(self):
        world = self.world(['alpha'], expected={'alpha': ['TestValue', 'TestMissing']})
        with self.assertRaises(golden.Refusal) as caught:
            self.run_golden(world)
        self.assertEqual((caught.exception.reason, caught.exception.detail), ('oracle_calibration_failed', 'GT-AGENT-X01'))
        self.assertEqual(calls(world), [])
        self.assertEqual(sorted(os.listdir(world.session)), ['calibration.json', 'logs', 'protocol.json'])
        protocol = json.loads((world.session / 'protocol.json').read_text())
        calibration = json.loads((world.session / 'calibration.json').read_text())
        self.assertEqual((len(protocol['order']), protocol['calibration']['status']), (4, 'failed'))
        self.assertEqual(calibration['before'], {'status': 'failed', 'tasks': [
            {'task_id': 'GT-AGENT-X01', 'clean_accepted': False, 'mutated_accepted': False}]})
        self.assertNotIn('after', calibration)
        code, document = report(self.auto, world.session)
        self.assertEqual((code, document['verdict'], document['reason']), (0, 'vacuous', 'oracle_calibration_failed'))

    def test_s11_signals_scope_audit_and_process_cleanup(self):
        # The agent timeout is covered on the agent runner alone (test_golden_runner); a session adds nothing.
        behaviors = {'alpha/alpha.go': 'forbidden', 'beta/beta.go': 'scope', 'gamma/gamma.go': 'exit',
                     'delta/delta.go': 'background'}
        world = self.world(['alpha', 'beta', 'gamma', 'delta'], behaviors=behaviors, k=1, refuse_candidate=True)
        self.run_golden(world)
        by_key = {(row['task_id'], row['arm']): row for row in records(world.session)}
        expected = {'GT-AGENT-X01': ('forbidden_construct', False), 'GT-AGENT-X02': ('scope_violation', False),
                    'GT-AGENT-X03': ('agent_exit_nonzero', True), 'GT-AGENT-X04': ('accepted', True)}
        for task, (signal, ran) in expected.items():
            with self.subTest(task):
                baseline, candidate = by_key[(task, 'baseline')], by_key[(task, 'candidate')]
                self.assertEqual((baseline['signal'], baseline['oracle']['ran']), (signal, ran))
                self.assertEqual((candidate['outcome'], candidate['signal'], candidate['oracle']['ran']),
                                 ('fail', 'agent_launch_failed', True))
        self.assertEqual(by_key[('GT-AGENT-X03', 'baseline')]['oracle']['expected_passed'], 2)
        trial = json.loads(next((world.session / 'trials').glob('*-GT-AGENT-X04-baseline-0/trial.json')).read_text())
        self.assertEqual((trial['agent']['stragglers'], trial['agent']['leftover']), (True, False))
        child = int(next(world.calls.glob('child-*.pid')).read_text())
        state = subprocess.run(['/bin/ps', '-o', 'stat=', '-p', str(child)], capture_output=True, text=True).stdout
        self.assertTrue(not state.strip() or state.strip().startswith('Z'), state)
        code, document = report(self.auto, world.session)
        self.assertEqual((code, document['verdict'], document['reason']), (0, 'regression', 'hard_flip'))


if __name__ == '__main__':
    unittest.main()
