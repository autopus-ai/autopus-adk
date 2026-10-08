"""The signed-lane session the Go signer verifies: the runner/signer wire cross-check (SPEC-HARNEVAL-003).

pkg/harneval/testdata/signed-lane holds a session this runner wrote with --signed-lane on the fixture
world: four black-box tasks (accepted and expectation_mismatch, agent_exit_nonzero with a compared
artifact, artifact_timeout, artifact_build_failed) and one white-box task, K=2, the fake codex, so no
model is called. Go's TestSignedLaneFixture_* decode it with LoadSignerInput and judge it with
RebuildTrustedProtocol, VerifySignedSession and ComputeVerdict. This module checks on every host that
the runner's own table re-derives every committed record from the committed oracle result bytes, and
regenerates the fixture on macOS with the Go toolchain when HARNEVAL_REGENERATE_WIRE_FIXTURE=1.

The golden set is committed as the strict Go loader reads it today: task documents without
oracle_mode and black_box_oracle, which the 001 task schema gains in T15. black_box.json keeps each
black-box task's assertion ids, the trusted values the signer takes from main's task definitions.
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
from test_golden_blackbox_session import REPAIRS, black_box
from test_golden_fixture import CHECKOUT, PIN, argv, build_world, git, write_stub

FIXTURE = CHECKOUT / 'pkg/harneval/testdata/signed-lane'
NAMES = ['alpha', 'beta', 'gamma', 'epsilon', 'zeta']
BEHAVIORS = {'beta/beta.go': 'exit', 'gamma/gamma.go': 'forbidden', 'epsilon/epsilon.go': 'forbidden'}
FIXTURE_REPAIRS = {'gamma/gamma.go': REPAIRS['gamma'], 'epsilon/epsilon.go': REPAIRS['theta'].replace('theta', 'epsilon')}
STARTED = datetime(2026, 10, 9, 0, 0, 0, tzinfo=timezone.utc)
SESSION_FILES = ('protocol.json', 'records.jsonl', 'calibration.json')
STRIPPED = ('oracle_mode', 'black_box_oracle')


def _digest(label: str) -> str:
    return hashlib.sha256(label.encode()).hexdigest()


def go_view(root: Path, target: Path) -> dict:
    """Copy the golden set as the Go loader reads it and return {task_id: assertion ids} of its black-box tasks."""
    assertions = {}
    for name in ('evals/harness/manifest.json', 'evals/harness/fixtures/codex-models.json',
                 'scripts/benchmarks/harness/corpus_fixture.json'):
        (target / name).parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(root / name, target / name)
    (target / 'evals/harness/tasks/agent').mkdir(parents=True)
    for path in sorted((root / 'evals/harness/tasks/agent').glob('*.json')):
        document = json.loads(path.read_text())
        if document.get('oracle_mode') == 'black_box':
            assertions[document['id']] = [item['id'] for item in document['black_box_oracle']['assertions']]
        kept = {key: value for key, value in document.items() if key not in STRIPPED}
        (target / 'evals/harness/tasks/agent' / path.name).write_text(json.dumps(kept, indent=2) + '\n')
    return assertions


def regenerate(target: Path) -> None:
    """Run one signed-lane session on a fresh fixture world and replace target with its wire documents."""
    with tempfile.TemporaryDirectory() as directory:
        base = Path(directory)
        world = build_world(base, NAMES, behaviors=BEHAVIORS, k=2, timeout=60)
        write_stub(world.stub, world.calls, world.corpus, BEHAVIORS, PIN, FIXTURE_REPAIRS)
        black_box(world, NAMES)
        lane = {'run_id': 18300000001, 'run_attempt': 1, 'binding_digest': _digest('harneval wire fixture binding'),
                'baseline_commit': git(world.repo, 'rev-parse', 'v0.50.123^{commit}'),
                'runner_tree_digest': _digest('harneval wire fixture runner tree')}
        (base / 'lane.json').write_text(json.dumps(lane))
        view = base / 'go-view'
        assertions = go_view(world.root, view)
        auto = base / 'auto'
        subprocess.run(['go', 'build', '-o', str(auto), './cmd/auto'], cwd=CHECKOUT, check=True, capture_output=True)
        steps = golden.Steps(now=lambda: STARTED, set_digests=lambda _auto, _root: gp.set_digests(str(auto), view))
        options = golden.parse(argv(world, '--auto', 'unused', '--signed-lane', str(base / 'lane.json')))
        with mock.patch.object(gs, 'ARTIFACT_TIMEOUT', 5):
            golden.run_session(options, steps)
        shutil.rmtree(target, ignore_errors=True)
        (target / 'session').mkdir(parents=True)
        for name in SESSION_FILES:
            shutil.copyfile(world.session / name, target / 'session' / name)
        shutil.copytree(world.session / 'oracle-results', target / 'session' / 'oracle-results')
        shutil.copytree(view, target / 'set')
        shutil.copytree(world.surfaces, target / 'surfaces')
        meta = {'run_id': lane['run_id'], 'run_attempt': lane['run_attempt'], 'run_created_at': '2026-10-08T23:50:00Z',
                'attempt_started_at': '2026-10-08T23:50:04Z'}
        for name, document in (('lane.json', lane), ('run-meta.json', meta), ('black_box.json', assertions)):
            (target / name).write_text(json.dumps(document, indent=2, sort_keys=True) + '\n')


def committed(name: str):
    return json.loads((FIXTURE / name).read_text())


class WireFixtureTests(unittest.TestCase):
    """The committed fixture is the runner's own output: its table re-derives every record."""

    @unittest.skipUnless(os.environ.get('HARNEVAL_REGENERATE_WIRE_FIXTURE') == '1' and sys.platform == 'darwin'
                         and shutil.which('go'), 'opt-in: regenerates pkg/harneval/testdata/signed-lane')
    def test_00_regenerate(self):
        regenerate(FIXTURE)

    def test_every_record_re_derives_from_its_committed_oracle_result(self):
        assertions = committed('black_box.json')
        lines = (FIXTURE / 'session/records.jsonl').read_text().splitlines()
        self.assertEqual(len(lines), 2 * 2 * len(assertions))
        signals = set()
        for line in lines:
            row = json.loads(line)
            digest = row['oracle_result_sha256']
            data = (FIXTURE / 'session/oracle-results' / (digest + '.json')).read_bytes() if digest else None
            if data is not None:
                self.assertEqual(hashlib.sha256(data).hexdigest(), digest)
            got = gb.derive(row['stage_reached'], row['signal'], row['agent_termination'], data, row['task_id'],
                            assertions[row['task_id']])
            self.assertEqual(got, (row['outcome'], row['signal'], row['oracle']), row['task_id'])
            signals.add(row['signal'])
        self.assertEqual(signals, {'accepted', 'expectation_mismatch', 'agent_exit_nonzero', 'artifact_timeout',
                                   'artifact_build_failed'})

    def test_protocol_carries_the_lane_fields_order_and_surface_digests(self):
        protocol, lane = committed('session/protocol.json'), committed('lane.json')
        self.assertEqual({key: protocol[key] for key in golden_lane.FIELDS}, lane)
        self.assertEqual(protocol['order'], gp.schedule(sorted(committed('black_box.json')), protocol['policy']['k']))
        for arm in gp.ARMS:
            self.assertEqual(protocol[arm + '_surface_digest'], gp.surface_digest(FIXTURE / 'surfaces' / arm))
        self.assertEqual(protocol['started_at'], STARTED.strftime('%Y-%m-%dT%H:%M:%SZ'))



if __name__ == '__main__':
    unittest.main()
