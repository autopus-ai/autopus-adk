"""The signed-lane session the Go signer verifies: the runner/signer wire cross-check (SPEC-HARNEVAL-003).

pkg/harneval/testdata/signed-lane holds a session this runner wrote with --signed-lane on the fixture
world, K=2, the fake codex (no model is called): twelve black-box tasks and one white-box task. The
golden set is committed as written, with oracle_mode, black_box_oracle and the oracle fixtures, so Go
decodes the T15 schema with LoadSet and takes the trusted assertion ids from OracleAssertionIDs. Go's
TestSignedLaneFixture_* judge it with RebuildTrustedProtocol, VerifySignedSession and ComputeVerdict.
This module checks on every host that the runner's own table re-derives every committed record from the
committed oracle result bytes, and regenerates the fixture on macOS with the Go toolchain when
HARNEVAL_REGENERATE_WIRE_FIXTURE=1.

The REQ-HR-08 rows the fixture reaches, by task: 10 accepted (X01 baseline); 9 expectation_mismatch (X01
candidate, X07 whose fix refuses every input and so fails its positive control); 8 output_link_rejected
(X04) and output_too_large (X11); 7 artifact_timeout (X03); 5 artifact_build_failed (X05); 4
scope_violation (X06); 3 observation_failed (X10, a transcript whose turn failed); 2 agent_exit_nonzero (X02),
agent_timeout (X08) and agent_launch_failed (X09); 1 warmup_failed (X12, through the runner's
Steps.warmup seam). Not reached, by design of an honest runner on a sound host:
  * workspace_setup_failed and mutation_failed: the session's calibration already copied the same
    snapshot and applied the same mutation, so only a host fault between the two fails them;
  * observation_failed from a process: process_tree kills every descendant and every process the
    stage's profile confines, so no fake agent or artifact leaves one the runner cannot end;
  * oracle_harness_error: the trusted harness always writes a schema-valid result naming exactly the
    bundle's assertions, which the runner builds from the same definition; only a broken or replaced
    harness binary gives it (derive-table.json and the Go derive tests cover that row).
"""
from datetime import datetime, timezone
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
import golden_lane
import golden_protocol as gp
import golden_sandbox as gs
import golden_trial as gt
from test_golden_blackbox_session import MAIN, REPAIRS
from test_golden_fixture import CHECKOUT, PIN, argv, build_world, git, write_stub

FIXTURE = CHECKOUT / 'pkg/harneval/testdata/signed-lane'
NAMES = ['alpha', 'beta', 'gamma', 'delta', 'epsilon', 'eta', 'theta', 'iota', 'kappa', 'lam', 'mu', 'nu', 'zeta']
BEHAVIORS = {'beta/beta.go': 'exit', 'gamma/gamma.go': 'forbidden', 'delta/delta.go': 'forbidden',
             'epsilon/epsilon.go': 'forbidden', 'eta/eta.go': 'scope', 'theta/theta.go': 'forbidden',
             'iota/iota.go': 'slowrepair', 'kappa/kappa.go': 'refuse', 'lam/lam.go': 'turnfailed', 'mu/mu.go': 'forbidden'}
FIXTURE_REPAIRS = {
    'gamma/gamma.go': REPAIRS['gamma'], 'delta/delta.go': REPAIRS['delta'],
    'epsilon/epsilon.go': REPAIRS['theta'].replace('theta', 'epsilon'),
    # Over-corrects: refuses every request, which only the positive control tells from the right fix.
    'theta/theta.go': 'package theta\n\nimport "os"\n\nfunc Value(x int) int {\n\tos.Exit(1)\n\treturn x\n}\n',
    'mu/mu.go': ('package mu\n\nimport (\n\t"os"\n\t"strings"\n)\n\nfunc Value(x int) int {\n'
                 '\t_, _ = os.Stdout.WriteString(strings.Repeat("x", 3<<20))\n\treturn x + 1\n}\n'),
}
# theta refuses a request that is not "valid\n" (exit 1, no stdout) and prints theta.Value(2) for one that is.
THETA_MAIN = ('package main\n\nimport (\n\t"fmt"\n\t"os"\n\n\t"example.com/golden/theta"\n)\n\nfunc main() {\n'
              '\tdata, err := os.ReadFile(os.Args[1])\n\tif err != nil || string(data) != "valid\\n" {\n\t\tos.Exit(1)\n'
              '\t}\n\tfmt.Println(theta.Value(2))\n}\n')
STARTED = datetime(2026, 10, 9, 0, 0, 0, tzinfo=timezone.utc)
SESSION_FILES = ('protocol.json', 'records.jsonl', 'calibration.json')
SIGNALS = {'accepted', 'expectation_mismatch', 'output_link_rejected', 'output_too_large', 'artifact_timeout',
           'artifact_build_failed', 'scope_violation', 'observation_failed', 'agent_exit_nonzero', 'agent_timeout',
           'agent_launch_failed', 'warmup_failed'}


def _digest(label: str) -> str:
    return hashlib.sha256(label.encode()).hexdigest()


def _pin(folder: Path, task_id: str, name: str, data: bytes) -> dict:
    (folder / name).parent.mkdir(parents=True, exist_ok=True)
    (folder / name).write_bytes(data)
    return {'path': f'evals/harness/oracles/{task_id}/{name}', 'sha256': hashlib.sha256(data).hexdigest()}


def fixture_black_box(world) -> None:
    """Commit a cmd/<name> main per package, tag the baseline, and make every task but the last black-box:
    stdout and result.txt pinned, except theta, which refuses one request and has a positive control."""
    for name in NAMES:
        (world.repo / 'cmd' / name).mkdir(parents=True)
        (world.repo / 'cmd' / name / 'main.go').write_text(THETA_MAIN if name == 'theta' else MAIN.format(name=name))
    git(world.repo, 'add', '--all')
    git(world.repo, 'commit', '--quiet', '-m', 'black-box mains')
    git(world.repo, 'tag', 'v0.50.123')
    world.manifest['live']['workspace_revision'] = git(world.repo, 'rev-parse', 'HEAD')
    (world.root / 'evals/harness/manifest.json').write_text(json.dumps(world.manifest, indent=2))
    for index, name in enumerate(NAMES[:-1], 1):
        task_id = 'GT-AGENT-X%02d' % index
        folder = world.root / 'evals/harness/oracles' / task_id
        if name == 'theta':
            oracle = {'build': './cmd/theta', 'command': ['{artifact}', '{input}/request.txt'],
                      'inputs': [_pin(folder, task_id, 'request.txt', b'invalid\n')],
                      'assertions': [{'id': 'exit', 'kind': 'exit_code', 'exit_code': 1},
                                     {'id': 'stdout', 'kind': 'stdout', 'expected': _pin(folder, task_id, 'empty.txt', b'')}],
                      'positive_control': {'inputs': [_pin(folder, task_id, 'control/request.txt', b'valid\n')],
                                           'stdout': _pin(folder, task_id, 'control/stdout.txt', b'3\n')}}
        else:
            oracle = {'build': './cmd/' + name, 'command': ['{artifact}'], 'inputs': [], 'assertions': [
                {'id': 'exit', 'kind': 'exit_code', 'exit_code': 0},
                {'id': 'stdout', 'kind': 'stdout', 'expected': _pin(folder, task_id, 'stdout.txt', b'3\n')},
                {'id': 'result', 'kind': 'file', 'path': 'result.txt',
                 'expected': _pin(folder, task_id, 'result.txt', b'3\n')}]}
        path = world.root / 'evals/harness/tasks/agent' / (task_id + '.json')
        path.write_text(json.dumps({**json.loads(path.read_text()), 'oracle_mode': 'black_box',
                                    'black_box_oracle': oracle}, indent=2) + '\n')


def regenerate(target: Path) -> None:
    """Run one signed-lane session on a fresh fixture world and replace target with its wire documents."""
    with tempfile.TemporaryDirectory() as directory:
        base = Path(directory)
        world = build_world(base, NAMES, behaviors=BEHAVIORS, k=2, timeout=8)
        write_stub(world.stub, world.calls, world.corpus, BEHAVIORS, PIN, FIXTURE_REPAIRS)
        fixture_black_box(world)
        lane = {'run_id': 18300000001, 'run_attempt': 1, 'binding_digest': _digest('harneval wire fixture binding'),
                'baseline_commit': git(world.repo, 'rev-parse', 'v0.50.123^{commit}'),
                'runner_tree_digest': _digest('harneval wire fixture runner tree')}
        (base / 'lane.json').write_text(json.dumps(lane))
        auto = base / 'auto'
        subprocess.run(['go', 'build', '-o', str(auto), './cmd/auto'], cwd=CHECKOUT, check=True, capture_output=True)

        def warmup(root, command, prepared, logs):
            return 'GT-AGENT-X12' not in str(logs) and gt.warmup(root, command, prepared, logs)
        steps = golden.Steps(now=lambda: STARTED, warmup=warmup,
                             set_digests=lambda _auto, _root: gp.set_digests(str(auto), world.root))
        options = golden.parse(argv(world, '--auto', 'unused', '--signed-lane', str(base / 'lane.json')))
        with mock.patch.object(gs, 'ARTIFACT_TIMEOUT', 5):
            golden.run_session(options, steps)
        shutil.rmtree(target, ignore_errors=True)
        (target / 'session').mkdir(parents=True)
        for name in SESSION_FILES:
            shutil.copyfile(world.session / name, target / 'session' / name)
        shutil.copytree(world.session / 'oracle-results', target / 'session' / 'oracle-results')
        shutil.copytree(world.root, target / 'set')
        shutil.copytree(world.surfaces, target / 'surfaces')
        meta = {'run_id': lane['run_id'], 'run_attempt': lane['run_attempt'], 'run_created_at': '2026-10-08T23:50:00Z',
                'attempt_started_at': '2026-10-08T23:50:04Z'}
        for name, document in (('lane.json', lane), ('run-meta.json', meta)):
            (target / name).write_text(json.dumps(document, indent=2, sort_keys=True) + '\n')


def committed(name: str):
    return json.loads((FIXTURE / name).read_text())


def assertion_ids() -> dict:
    """{task_id: trusted assertion ids} of the committed set's black-box tasks, as the signer reads them."""
    manifest = committed('set/evals/harness/manifest.json')
    tasks = [{'id': path.stem} for path in sorted((FIXTURE / 'set/evals/harness/tasks/agent').glob('*.json'))]
    return {task['id']: gb.assertion_ids(task['black_box'])
            for task in golden_lane.black_box_tasks(FIXTURE / 'set', manifest, tasks)}


class WireFixtureTests(unittest.TestCase):
    """The committed fixture is the runner's own output: its table re-derives every record."""

    @unittest.skipUnless(os.environ.get('HARNEVAL_REGENERATE_WIRE_FIXTURE') == '1' and sys.platform == 'darwin'
                         and shutil.which('go'), 'opt-in: regenerates pkg/harneval/testdata/signed-lane')
    def test_00_regenerate(self):
        regenerate(FIXTURE)

    def test_every_record_re_derives_from_its_committed_oracle_result(self):
        ids = assertion_ids()
        self.assertEqual(len(ids), 12)
        self.assertEqual(ids['GT-AGENT-X07'], ['exit', 'stdout', 'positive_control.exit', 'positive_control.stdout'])
        lines = (FIXTURE / 'session/records.jsonl').read_text().splitlines()
        self.assertEqual(len(lines), 2 * 2 * len(ids))
        signals = set()
        for line in lines:
            row = json.loads(line)
            digest = row['oracle_result_sha256']
            data = (FIXTURE / 'session/oracle-results' / (digest + '.json')).read_bytes() if digest else None
            if data is not None:
                self.assertEqual(hashlib.sha256(data).hexdigest(), digest)
            got = gb.derive(row['stage_reached'], row['signal'], row['agent_termination'], data, row['task_id'],
                            ids[row['task_id']])
            self.assertEqual(got, (row['outcome'], row['signal'], row['oracle']), row['task_id'])
            signals.add(row['signal'])
        self.assertEqual(signals, SIGNALS)

    def test_protocol_carries_the_lane_fields_order_and_surface_digests(self):
        protocol, lane = committed('session/protocol.json'), committed('lane.json')
        self.assertEqual({key: protocol[key] for key in golden_lane.FIELDS}, lane)
        self.assertEqual(protocol['order'], gp.schedule(sorted(assertion_ids()), protocol['policy']['k']))
        for arm in gp.ARMS:
            self.assertEqual(protocol[arm + '_surface_digest'], gp.surface_digest(FIXTURE / 'surfaces' / arm))
        self.assertEqual(protocol['started_at'], STARTED.strftime('%Y-%m-%dT%H:%M:%SZ'))


if __name__ == '__main__':
    unittest.main()
