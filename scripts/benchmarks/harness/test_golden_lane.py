"""Signed-lane inputs: the lane file, task selection, refusals and the evals/harness exclusion (cross-platform)."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

import golden
import golden_lane as gl
from test_golden_fixture import argv, build_world, calls, git
from test_golden_runner import fake_digests

LANE = {'run_id': 4242, 'run_attempt': 2, 'binding_digest': 'b' * 64, 'baseline_commit': 'c' * 40,
        'runner_tree_digest': 'd' * 64}


def write_lane(path: Path, **changes) -> Path:
    path.write_text(json.dumps({**LANE, **changes}))
    return path


class LaneFileTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()

    def test_the_lane_file_holds_exactly_the_five_protocol_fields(self):
        self.assertEqual(gl.load(write_lane(self.base / 'lane.json')), LANE)
        self.assertEqual(list(gl.load(write_lane(self.base / 'lane.json'))), list(gl.FIELDS))
        bad = {'missing': {k: v for k, v in LANE.items() if k != 'runner_tree_digest'}, 'extra': {**LANE, 'model': 'x'},
               'zero run': {**LANE, 'run_id': 0}, 'bool attempt': {**LANE, 'run_attempt': True},
               'string run': {**LANE, 'run_id': '4242'}, 'short digest': {**LANE, 'binding_digest': 'b' * 63},
               'upper digest': {**LANE, 'runner_tree_digest': 'D' * 64}, 'short commit': {**LANE, 'baseline_commit': 'c' * 39}}
        for name, document in bad.items():
            with self.subTest(name), self.assertRaises(gl.LaneError):
                (self.base / 'bad.json').write_text(json.dumps(document))
                gl.load(self.base / 'bad.json')
        (self.base / 'dup.json').write_text('{"run_id": 1, "run_id": 2}')
        with self.assertRaises(gl.LaneError):
            gl.load(self.base / 'dup.json')
        with self.assertRaises(gl.LaneError):
            gl.load(self.base / 'absent.json')

    def test_exclude_harness_removes_the_golden_set_from_the_snapshot_only(self):
        source = self.base / 'source'
        (source / 'evals/harness/oracles').mkdir(parents=True)
        (source / 'evals/harness/oracles/expected.txt').write_text('3\n')
        (source / 'evals/other.txt').write_text('kept\n')
        gl.exclude_harness(source)
        self.assertEqual(sorted(path.name for path in (source / 'evals').iterdir()), ['other.txt'])
        (source / 'evals/harness').symlink_to(self.base)
        gl.exclude_harness(source)
        self.assertFalse((source / 'evals/harness').exists() or (source / 'evals/harness').is_symlink())
        self.assertTrue(self.base.is_dir())
        gl.exclude_harness(source)


class LaneRefusalTests(unittest.TestCase):
    """A signed-lane input the runner cannot trust refuses the session before any agent call."""

    def world(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        world = build_world(Path(directory.name), ['alpha'])
        git(world.repo, 'tag', 'v0.50.123')
        return world

    def refuse(self, world, lane: Path) -> golden.Refusal:
        options = golden.parse(argv(world, '--auto', 'unused', '--signed-lane', str(lane)))
        with self.assertRaises(golden.Refusal) as caught:
            golden.run_session(options, golden.Steps(platform='darwin', set_digests=fake_digests))
        self.assertEqual(calls(world), [])
        self.assertFalse(world.session.exists() and any(world.session.iterdir()))
        return caught.exception

    def test_an_invalid_lane_file_a_moved_baseline_tag_or_no_black_box_task_is_refused(self):
        world = self.world()
        commit = subprocess.run(['git', 'rev-parse', 'v0.50.123^{commit}'], cwd=world.repo, capture_output=True,
                                text=True, check=True).stdout.strip()
        refused = self.refuse(world, write_lane(world.base / 'lane.json', run_id=-1))
        self.assertEqual((refused.reason, refused.detail.split(':')[0]), ('invalid', 'signed lane file'))
        refused = self.refuse(world, write_lane(world.base / 'lane.json'))
        self.assertEqual(refused.reason, 'invalid')
        self.assertIn('baseline_commit ' + 'c' * 40 + ' is not v0.50.123 (' + commit + ')', refused.detail)
        refused = self.refuse(world, write_lane(world.base / 'lane.json', baseline_commit=commit))
        self.assertEqual((refused.reason, refused.detail), ('invalid', 'no active agent task has a black-box oracle'))
        task = world.root / 'evals/harness/tasks/agent/GT-AGENT-X01.json'
        task.write_text(json.dumps({**json.loads(task.read_text()), 'oracle_mode': 'black_box'}))
        refused = self.refuse(world, write_lane(world.base / 'lane.json', baseline_commit=commit))
        self.assertTrue(refused.detail.startswith('GT-AGENT-X01: oracle_mode black_box needs'), refused.detail)


if __name__ == '__main__':
    unittest.main()
