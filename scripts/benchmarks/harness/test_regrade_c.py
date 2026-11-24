"""Pilot C re-grade: task compatibility, output placement, tree rebuild, row re-grade and the report's
saturated-task exclusion, without Go (run_graded is replaced)."""
import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import regrade_c
import report_c
from gate_c import load_corpus


def _row(task_id, arm, oracle):
    return {'task_id': task_id, 'arm': arm, 'replicate': 1, 'operational_error': None, 'oracle': oracle,
            'agent': {'elapsed_seconds': 1.0}, 'observation': {'total_tokens': 10}, 'scope': {'accepted_scope': True}}


class CompatibilityTests(unittest.TestCase):
    def test_same_task_passes_and_a_changed_prompt_is_refused(self):
        tasks = load_corpus(ids=['c03'])
        regrade_c.check_compatible(json.loads(json.dumps(tasks)), tasks)
        changed = json.loads(json.dumps(tasks))
        changed[0]['prompt'] += ' more'
        with self.assertRaisesRegex(SystemExit, 'prompt'):
            regrade_c.check_compatible(changed, tasks)
        with self.assertRaisesRegex(SystemExit, 'not in the source run'):
            regrade_c.check_compatible([], tasks)

    def test_oracle_cases_may_change(self):
        tasks = load_corpus(ids=['c06'])
        frozen = json.loads(json.dumps(tasks))
        frozen[0]['oracle']['cases'] = frozen[0]['oracle']['cases'][:1]
        regrade_c.check_compatible(frozen, tasks)

    def test_output_must_be_new_and_outside_the_run(self):
        with tempfile.TemporaryDirectory() as tmp:
            run = Path(tmp) / 'run'
            (run / 'measure').mkdir(parents=True)
            for bad in (run, run / 'regrade', Path(tmp)):
                with self.assertRaises(SystemExit):
                    regrade_c.check_output(run, bad)
            used = Path(tmp) / 'used'
            used.mkdir()
            (used / 'x').write_text('x')
            with self.assertRaisesRegex(SystemExit, 'new or empty'):
                regrade_c.check_output(run, used)
            regrade_c.check_output(run, Path(tmp) / 'new')


class RebuildTests(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.base = self.tmp / 'base'
        (self.base / 'pkg').mkdir(parents=True)
        (self.base / 'pkg' / 'a.go').write_text('base a')
        (self.base / 'pkg' / 'gone.go').write_text('base gone')
        self.measure = self.tmp / 'run' / 'measure' / 'c03-full-k1'
        (self.measure / 'candidate' / 'pkg').mkdir(parents=True)
        (self.measure / 'candidate' / 'pkg' / 'a.go').write_text('new a')
        (self.measure / 'candidate' / 'pkg' / 'b.go').write_text('new b')
        self.overlay = {'copied': ['pkg/a.go', 'pkg/b.go'], 'deleted': ['pkg/gone.go']}

    def test_base_plus_kept_files_minus_deletions(self):
        dest = self.tmp / 'out' / 'grading' / 'tree'
        regrade_c.rebuild(self.base, self.measure, self.overlay, dest)
        self.assertEqual((dest / 'pkg' / 'a.go').read_text(), 'new a')
        self.assertEqual((dest / 'pkg' / 'b.go').read_text(), 'new b')
        self.assertFalse((dest / 'pkg' / 'gone.go').exists())
        self.assertEqual((self.base / 'pkg' / 'a.go').read_text(), 'base a')

    def test_a_candidate_that_does_not_match_its_overlay_is_refused(self):
        with self.assertRaisesRegex(ValueError, 'differ'):
            regrade_c.rebuild(self.base, self.measure, {'copied': ['pkg/a.go']}, self.tmp / 'x' / 'tree')

    def test_row_is_regraded_into_the_output_and_ungraded_rows_stay_ungraded(self):
        task = load_corpus(ids=['c03'])[0]
        names = [c['name'] for c in task['oracle']['cases']]
        output = self.tmp / 'out'
        ctx = {'run': self.tmp / 'run', 'output': output, 'oracles': Path('/unused'),
               'bases': {task['heads']['base']: self.base}, 'caches': {task['heads']['base']: self.tmp / 'cache'},
               'base_passing': {}, 'keep': False}
        verdicts = {'build': {}, 'builds': {}, 'cases': {n: 'pass' for n in names}}
        source = _row('c03', 'full', {'passed': 1, 'total': 17, 'overlay': self.overlay})
        with mock.patch.object(regrade_c.grade_c, 'run_graded', return_value=verdicts) as graded:
            row = regrade_c.regrade_row(source, self.measure, task, ctx)
        tree = graded.call_args.args[0]
        self.assertTrue(str(tree).startswith(str(output)))
        self.assertEqual(row['oracle']['passed'], len(names))
        self.assertEqual(row['regrade']['source_passed'], 1)
        self.assertEqual(row['regrade']['status'], 'regraded')
        self.assertIsNone(row['oracle']['review_new_total'])
        self.assertFalse((output / 'grading' / 'c03-full-k1').exists())
        ungraded = regrade_c.regrade_row(_row('c03', 'full', None), self.measure, task, ctx)
        self.assertIsNone(ungraded['oracle'])
        self.assertEqual(ungraded['regrade']['status'], 'ungraded_in_source')


class ReportTests(unittest.TestCase):
    def test_saturated_tasks_leave_the_arm_comparison_but_not_the_trial_table(self):
        oracle = {'passed': 2, 'total': 2, 'review_passed': 1, 'review_total': 1, 'feature_passed': 1,
                  'feature_total': 1, 'guard_passed': 0, 'guard_total': 0, 'review_new_passed': 1,
                  'review_new_total': 1}
        records = [_row('c02', 'full', oracle), _row('c03', 'full', oracle)]
        self.assertEqual([r['task_id'] for r in report_c.without(records, ['c02'])], ['c03'])
        text = report_c.to_markdown(report_c.summarize(records), records, ['c02'])
        self.assertIn('without saturated tasks (c02)', text)
        self.assertIn('| full | 1 |', text)
        self.assertIn('| c02-full-k1 |', text)
        self.assertNotIn('saturated', report_c.to_markdown(report_c.summarize(records), records))


if __name__ == '__main__':
    unittest.main()
