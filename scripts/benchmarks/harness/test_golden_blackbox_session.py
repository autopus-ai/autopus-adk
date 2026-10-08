"""Whole signed-lane sessions on the fixture world: black-box tasks judged only by the oracle harness (S8).

macOS only: every build and artifact run goes under artifact.sb and the harness under oracle.sb, each
a sibling process the runner starts. The agent is the fake codex of test_golden_fixture, so no model
is called. Each package gets a cmd/<name> main whose stdout and result.txt the task pins.
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
import golden_blackbox as gb
import golden_protocol as gp
import golden_sandbox as gs
import golden_trial as gt
import grader
from test_golden_fixture import PIN, argv, build_world, calls, git, write_stub
from test_golden_lane import LANE
from test_golden_runner import fake_digests

MAIN = ('package main\n\nimport (\n\t"fmt"\n\t"os"\n\n\t"example.com/golden/{name}"\n)\n\nfunc main() {{\n'
        '\tvalue := {name}.Value(2)\n\tfmt.Println(value)\n'
        '\tif err := os.WriteFile("result.txt", []byte(fmt.Sprintln(value)), 0o644); err != nil {{\n\t\tos.Exit(3)\n\t}}\n}}\n')
REPAIRS = {
    'beta': 'package beta\n\nfunc Value(x int) int { return "x" }\n',
    'gamma': ('package gamma\n\nimport (\n\t"fmt"\n\t"time"\n)\n\nfunc Value(x int) int {\n\tfmt.Println(x + 1)\n'
              '\tfor {\n\t\ttime.Sleep(time.Hour)\n\t}\n}\n'),
    'delta': ('package delta\n\nimport "os"\n\nfunc Value(x int) int {\n\t_ = os.WriteFile("real.txt", []byte("3\\n"), 0o644)\n'
              '\t_ = os.Symlink("real.txt", "result.txt")\n\treturn x + 1\n}\n'),
    # epsilon reads EXPECTED, the pinned expected stdout in the golden set root, and forges with it if it can.
    'epsilon': ('package epsilon\n\nimport (\n\t"fmt"\n\t"os"\n)\n\nfunc Value(x int) int {\n'
                '\tdata, err := os.ReadFile(EXPECTED)\n\tif err == nil && string(data) == "3\\n" {\n\t\treturn 3\n\t}\n'
                '\t_ = os.WriteFile("probe.txt", []byte(fmt.Sprint(err)), 0o644)\n\treturn x + 2\n}\n'),
    'zeta': 'package zeta\n\nimport (\n\t"fmt"\n\t"os"\n)\n\nfunc Value(x int) int {\n\tfmt.Println("PASS")\n\tos.Exit(0)\n\treturn x\n}\n',
    'theta': 'package theta\n\nfunc Value(x int) int { return "x" }\n',
}
BEHAVIORS = {'eta/eta.go': 'scope', 'theta/theta.go': 'exit', 'jota/jota.go': 'slowrepair'}
NAMES = ['alpha', 'beta', 'gamma', 'delta', 'epsilon', 'zeta', 'eta', 'theta', 'jota', 'kappa', 'lam', 'mu', 'nu']


def black_box(world, names: list, assertions=None) -> None:
    """Commit a cmd/<name> main per package (and a decoy golden set), tag the baseline, and make every task
    but the last black-box with pinned stdout and result.txt."""
    for name in names:
        (world.repo / 'cmd' / name).mkdir(parents=True)
        (world.repo / 'cmd' / name / 'main.go').write_text(MAIN.format(name=name))
    (world.repo / 'evals/harness/oracles').mkdir(parents=True)
    (world.repo / 'evals/harness/oracles/decoy.txt').write_text('visible only to a leaky snapshot\n')
    git(world.repo, 'add', '--all')
    git(world.repo, 'commit', '--quiet', '-m', 'black-box mains')
    git(world.repo, 'tag', 'v0.50.123')
    world.manifest['live']['workspace_revision'] = git(world.repo, 'rev-parse', 'HEAD')
    (world.root / 'evals/harness/manifest.json').write_text(json.dumps(world.manifest, indent=2))
    for index, name in enumerate(names[:-1] if len(names) > 1 else names, 1):
        task_id = 'GT-AGENT-X%02d' % index
        folder = world.root / 'evals/harness/oracles' / task_id
        folder.mkdir(parents=True)
        pins = {}
        for file in ('stdout.txt', 'result.txt'):
            (folder / file).write_text('3\n')
            pins[file] = {'path': f'evals/harness/oracles/{task_id}/{file}', 'sha256': hashlib.sha256(b'3\n').hexdigest()}
        oracle = {'build': './cmd/' + name, 'command': ['{artifact}'], 'inputs': [], 'assertions': assertions or [
            {'id': 'exit', 'kind': 'exit_code', 'exit_code': 0},
            {'id': 'stdout', 'kind': 'stdout', 'expected': pins['stdout.txt']},
            {'id': 'result', 'kind': 'file', 'path': 'result.txt', 'expected': pins['result.txt']}]}
        path = world.root / 'evals/harness/tasks/agent' / (task_id + '.json')
        path.write_text(json.dumps({**json.loads(path.read_text()), 'oracle_mode': 'black_box', 'black_box_oracle': oracle}))


def lane(world) -> Path:
    path = world.base / 'lane.json'
    path.write_text(json.dumps({**LANE, 'baseline_commit': git(world.repo, 'rev-parse', 'v0.50.123^{commit}')}))
    return path


def records(session: Path) -> list:
    return [json.loads(line) for line in (session / 'records.jsonl').read_text().splitlines()]


@unittest.skipUnless(sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and shutil.which('go'),
                     'requires macOS sandbox-exec and the Go toolchain')
class SignedLaneSessionTests(unittest.TestCase):
    def world(self, names, **options):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        world = build_world(Path(directory.name), names, **options)
        self.addCleanup(subprocess.run, ['chmod', '-R', 'u+w', str(world.base)], capture_output=True)
        return world

    def run_signed(self, world, steps, *extra):
        options = golden.parse(argv(world, '--auto', 'unused', '--signed-lane', str(lane(world)), *extra))
        with mock.patch.object(gs, 'ARTIFACT_TIMEOUT', 5):
            return golden.run_session(options, steps)

    def test_s8_every_trial_is_judged_by_the_black_box_table_and_re_derives(self):
        world = self.world(NAMES, behaviors=BEHAVIORS, k=1, timeout=8)
        expected_file = world.root / 'evals/harness/oracles/GT-AGENT-X05/stdout.txt'
        repairs = {f'{name}/{name}.go': text for name, text in REPAIRS.items()}
        repairs['epsilon/epsilon.go'] = REPAIRS['epsilon'].replace('EXPECTED', json.dumps(str(expected_file)))
        write_stub(world.stub, world.calls, world.corpus, BEHAVIORS, PIN, repairs)
        black_box(world, NAMES)
        real_run, real_argv, oracle_runs = gs.run_stage, gs.oracle_argv, []

        def run_stage(argv_, cwd, stdout_path, stderr_path, timeout, stdin=None):
            result = real_run(argv_, cwd, stdout_path, stderr_path, timeout, stdin)
            if '/trials/' in str(stdout_path) and 'GT-AGENT-X11' in str(stdout_path) and 'MODE=run' in argv_:
                result['leftover'] = True
            if str(stdout_path).endswith('oracle.stdout') and '/trials/' in str(stdout_path):
                oracle_runs.append(str(stdout_path))
            return result

        def oracle_argv(oracle, output, result_dir, task_id, params):
            broken = task_id == 'GT-AGENT-X12' and '/trials/' in str(result_dir)
            return real_argv(Path('/usr/bin/true') if broken else oracle, output, result_dir, task_id, params)

        def warmup(root, command, prepared, logs):
            return 'GT-AGENT-X10' not in str(logs) and gt.warmup(root, command, prepared, logs)
        steps = golden.Steps(set_digests=fake_digests, warmup=warmup)
        with mock.patch.object(gs, 'run_stage', run_stage), mock.patch.object(gs, 'oracle_argv', oracle_argv):
            summary = self.run_signed(world, steps, '--keep-scratch')
        self.assertEqual((summary['records'], summary['calibration_after']), (24, 'passed'))
        protocol = json.loads((world.session / 'protocol.json').read_text())
        self.assertEqual({key: protocol[key] for key in LANE},
                         {**LANE, 'baseline_commit': git(world.repo, 'rev-parse', 'v0.50.123^{commit}')})
        self.assertEqual(list(protocol)[-5:], list(LANE))
        self.assertEqual((protocol['calibration']['status'], protocol['runner_sha256']), ('passed', gp.runner_digest()))
        self.assertNotIn('GT-AGENT-X13', {row['task_id'] for row in protocol['order']})
        want = {
            'X01': [('pass', 'accepted', True, False), ('fail', 'expectation_mismatch', True, False)],
            'X02': [('fail', 'artifact_build_failed', False, True), ('fail', 'expectation_mismatch', True, False)],
            'X03': [('fail', 'artifact_timeout', False, False), ('fail', 'expectation_mismatch', True, False)],
            'X04': [('fail', 'output_link_rejected', False, False), ('fail', 'expectation_mismatch', True, False)],
            'X05': [('fail', 'expectation_mismatch', True, False)] * 2,
            'X06': [('fail', 'expectation_mismatch', True, False)] * 2,
            'X07': [('fail', 'scope_violation', False, False)] * 2,
            'X08': [('fail', 'agent_exit_nonzero', False, True)] * 2,
            'X09': [('fail', 'agent_timeout', True, False)] * 2,
            'X10': [('error', 'warmup_failed', False, False)] * 2,
            'X11': [('fail', 'observation_failed', False, False)] * 2,
            'X12': [('fail', 'oracle_harness_error', False, False)] * 2,
        }
        rows = records(world.session)
        documents = {path.stem: json.loads(path.read_text())
                     for path in (world.root / 'evals/harness/tasks/agent').glob('*.json')}
        for row in rows:
            got = (row['outcome'], row['signal'], row['oracle']['ran'], row['oracle']['build_failed'])
            with self.subTest(task=row['task_id'], arm=row['arm']):
                self.assertEqual(got, want[row['task_id'][-3:]][row['arm'] == 'candidate'])
                digest = row['oracle_result_sha256']
                data = (world.session / 'oracle-results' / (digest + '.json')).read_bytes() if digest else None
                if data is not None:
                    self.assertEqual(hashlib.sha256(data).hexdigest(), digest)
                ids = [item['id'] for item in documents[row['task_id']]['black_box_oracle']['assertions']]
                # The signer's re-derivation: the same table over the record fields and the stored result bytes.
                self.assertEqual(gb.derive(row['stage_reached'], row['signal'], row['agent_termination'], data,
                                           row['task_id'], ids), (row['outcome'], row['signal'], row['oracle']))
        by_key = {(row['task_id'][-3:], row['arm']): row for row in rows}
        self.assertEqual(by_key[('X09', 'baseline')]['agent_termination']['timed_out'], True)
        self.assertIsNone(by_key[('X09', 'baseline')]['agent_termination']['exit_code'])
        self.assertEqual(by_key[('X08', 'baseline')]['agent_termination'], {'launched': True, 'exit_code': 3,
                                                                            'os_signal': None, 'timed_out': False})
        self.assertEqual((by_key[('X10', 'baseline')]['stage_reached'], by_key[('X11', 'baseline')]['stage_reached'],
                          by_key[('X07', 'baseline')]['stage_reached']), ('setup', 'run', 'agent'))
        self.assertIsNone(by_key[('X12', 'baseline')]['oracle_result_sha256'])
        self.assertFalse(any('GT-AGENT-X11' in path or 'GT-AGENT-X07' in path for path in oracle_runs))
        made = calls(world)
        self.assertEqual(len(made), 22)
        self.assertFalse(any(call['harness_files'] for call in made), 'an agent saw evals/harness')
        probe = next((world.session / 'scratch/trials').glob('*-GT-AGENT-X05-baseline-0')) / 'run/out/probe.txt'
        self.assertIn('operation not permitted', probe.read_text())
        calibration = json.loads((world.session / 'calibration.json').read_text())
        self.assertEqual((calibration['before']['status'], calibration['after']['status']), ('passed', 'passed'))
        self.assertEqual(len(calibration['before']['tasks']), 12)

    def test_s8_a_weak_oracle_fails_calibration_before_any_agent_call(self):
        world = self.world(['alpha'])
        black_box(world, ['alpha'], assertions=[{'id': 'exit', 'kind': 'exit_code', 'exit_code': 0}])
        with self.assertRaises(golden.Refusal) as caught:
            self.run_signed(world, golden.Steps(set_digests=fake_digests))
        self.assertEqual((caught.exception.reason, caught.exception.detail), ('oracle_calibration_failed', 'GT-AGENT-X01'))
        self.assertEqual(calls(world), [])
        self.assertFalse((world.session / 'records.jsonl').exists())
        calibration = json.loads((world.session / 'calibration.json').read_text())
        self.assertEqual(calibration['before'], {'status': 'failed', 'tasks': [
            {'task_id': 'GT-AGENT-X01', 'clean_accepted': True, 'mutated_accepted': True}]})


if __name__ == '__main__':
    unittest.main()
