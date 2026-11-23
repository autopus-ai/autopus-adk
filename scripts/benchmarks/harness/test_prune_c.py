"""Pilot C trial pruning: the graded candidate survives under measure/, the trial and grading trees do not."""
import os
import tempfile
import unittest
from pathlib import Path

import run_c


class PruneTests(unittest.TestCase):
    def test_keeps_the_candidate_and_drops_the_trees(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            trial, measure, grading = root / 'trials' / 't', root / 'measure' / 't', root / 'grading' / 't'
            (trial / 'workspace' / 'pkg').mkdir(parents=True)
            (trial / 'workspace' / 'pkg' / 'a.go').write_text('agent copy')
            locked = trial / 'gomodcache' / 'mod'
            locked.mkdir(parents=True)
            (locked / 'x').write_text('x')
            locked.chmod(0o500)  # go makes module cache directories read-only
            (grading / 'tree' / 'pkg').mkdir(parents=True)
            (grading / 'tree' / 'pkg' / 'a.go').write_text('graded copy')
            measure.mkdir(parents=True)
            (measure / 'result.json').write_text('{}')
            row = {'oracle': {'overlay': {'copied': ['pkg/a.go', 'pkg/missing.go']}}}

            run_c.prune_trial(row, trial, measure, grading)

            self.assertEqual((measure / 'candidate' / 'pkg' / 'a.go').read_text(), 'graded copy')
            self.assertFalse((measure / 'candidate' / 'pkg' / 'missing.go').exists())
            self.assertTrue((measure / 'result.json').is_file())
            self.assertFalse(trial.exists())
            self.assertFalse(grading.exists())

    def test_a_symlink_in_the_grading_tree_is_not_followed(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            secret = root / 'secret'
            secret.write_text('outside')
            trial, measure, grading = root / 'trials' / 't', root / 'measure' / 't', root / 'grading' / 't'
            trial.mkdir(parents=True)
            (grading / 'tree').mkdir(parents=True)
            os.symlink(secret, grading / 'tree' / 'a.go')
            measure.mkdir(parents=True)

            run_c.prune_trial({'oracle': {'overlay': {'copied': ['a.go']}}}, trial, measure, grading)

            self.assertFalse((measure / 'candidate' / 'a.go').exists())
            self.assertEqual(secret.read_text(), 'outside')

    def test_no_oracle_prunes_without_copying(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            trial, measure, grading = root / 'trials' / 't', root / 'measure' / 't', root / 'grading' / 't'
            trial.mkdir(parents=True)
            measure.mkdir(parents=True)
            run_c.prune_trial({'oracle': None}, trial, measure, grading)
            self.assertFalse(trial.exists())
            self.assertFalse((measure / 'candidate').exists())


if __name__ == '__main__':
    unittest.main()
