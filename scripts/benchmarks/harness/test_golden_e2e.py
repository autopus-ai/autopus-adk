"""End-to-end golden session on two real corpus tasks through `run.py --mode golden` and `auto eval harness report`.

Opt-in with HARNEVAL_GOLDEN_E2E=1 on a macOS host: it snapshots the committed live.workspace_revision
of this checkout, fills the session module cache from the local module cache, calibrates
GT-AGENT-A01 and GT-AGENT-A05 inside grader.sb and runs their 8 trials. No model is called: the agent
is the fake codex of test_golden_fixture, which repairs the seeded regression only under the
baseline surface, so the session must be judged a hard-flip regression.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest

import golden
import golden_protocol as gp
import grader
from test_golden_fixture import calls, write_stub, write_surfaces

HERE = Path(__file__).resolve().parent
CHECKOUT = HERE.parents[2]
TASKS = ('GT-AGENT-A01', 'GT-AGENT-A05')
COPIED = ('evals/harness/manifest.json', 'evals/harness/fixtures/codex-models.json',
          'scripts/benchmarks/harness/corpus_a.json', *(f'evals/harness/tasks/agent/{task}.json' for task in TASKS))


@unittest.skipUnless(os.environ.get('HARNEVAL_GOLDEN_E2E') == '1' and sys.platform == 'darwin'
                     and os.path.exists(grader.SANDBOX) and shutil.which('go'),
                     'opt-in (HARNEVAL_GOLDEN_E2E=1) macOS end-to-end session on real corpus tasks')
class GoldenEndToEndTests(unittest.TestCase):
    def test_two_corpus_tasks_end_in_a_hard_flip_advisory_report(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            root = base / 'set'
            for rel in COPIED:
                (root / rel).parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(CHECKOUT / rel, root / rel)
            shutil.copytree(CHECKOUT / 'evals/harness/tasks/surface', root / 'evals/harness/tasks/surface')
            corpus = [row for row in json.loads((root / COPIED[2]).read_text()) if row['id'] in ('a01', 'a05')]
            revision = json.loads((root / COPIED[0]).read_text())['live']['workspace_revision']
            # The fake agent writes the clean file back: reversing the a05 mutation text would hit an earlier
            # `if !ok {` of decode.go and break the build.
            repairs = {row['allowed_paths'][0]: subprocess.run(
                ['git', 'show', revision + ':' + row['allowed_paths'][0]], cwd=CHECKOUT, check=True,
                capture_output=True, text=True).stdout for row in corpus}
            stub = write_stub(base / 'bin' / 'codex', base / 'calls', corpus, repairs=repairs)
            surfaces = write_surfaces(base / 'surfaces')
            auto = golden.build_auto(base)
            session = base / 'session'
            completed = subprocess.run(
                [sys.executable, str(HERE / 'run.py'), '--mode', 'golden', '--output', str(session), '--surfaces',
                 str(surfaces), '--dir', str(root), '--codex', str(stub), '--auto', auto,
                 '--credential-env', 'HARNEVAL_AGENT_CREDENTIAL'],
                capture_output=True, text=True, timeout=3600,
                env={**os.environ, 'PYTHONDONTWRITEBYTECODE': '1', 'HARNEVAL_AGENT_CREDENTIAL': 'agent-only',
                     'GITHUB_TOKEN': 'leak'})
            self.assertEqual(completed.returncode, 0, completed.stderr[-3000:])
            summary = json.loads(completed.stdout)
            self.assertEqual((summary['records'], summary['outcomes'], summary['calibration_after']),
                             (8, {'pass': 4, 'fail': 4}, 'passed'))
            made = calls(SimpleNamespace(calls=base / 'calls'))
            self.assertEqual(len(made), 8)
            self.assertTrue(all(call['env']['HARNEVAL_AGENT_CREDENTIAL'] == 'agent-only'
                                and 'GITHUB_TOKEN' not in call['env'] for call in made))
            reported = subprocess.run([auto, 'eval', 'harness', 'report', '--input', str(session), '--format', 'json'],
                                      capture_output=True, text=True, timeout=120)
            self.assertEqual(reported.returncode, 0, reported.stderr)
            report = json.loads(reported.stdout)
            protocol = json.loads((session / 'protocol.json').read_text())
            print('\nGOLDEN-E2E ' + json.dumps({key: report[key] for key in (
                'session_id', 'verdict', 'reason', 'hard_flips', 'arms', 'completeness', 'calibration',
                'runner_sha256', 'grader_profile_sha256', 'agent_set_digest', 'workspace_revision')}), file=sys.stderr)
            self.assertEqual((report['verdict'], report['reason'], report['hard_flips']),
                             ('regression', 'hard_flip', list(TASKS)))
            self.assertEqual((report['arms']['baseline']['passes'], report['arms']['candidate']['passes'],
                              report['completeness'], report['calibration']['status']), (4, 0, 1.0, 'passed'))
            self.assertEqual(report['workspace_revision'], revision)
            self.assertEqual((report['runner_sha256'], report['grader_profile_sha256']),
                             (gp.runner_digest(), hashlib.sha256(grader.PROFILE.read_bytes()).hexdigest()))
            self.assertEqual(protocol['baseline_surface_digest'], gp.surface_digest(surfaces / 'baseline'))
            self.assertEqual(sorted(os.listdir(session)), ['calibration.json', 'logs', 'protocol.json',
                                                           'records.jsonl', 'trials'])


if __name__ == '__main__':
    unittest.main()
