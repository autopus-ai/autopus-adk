"""Pilot C corpus, oracle installation, grading-tree reset, case verdicts and scope audit."""
import json
import re
import tempfile
import unittest
from pathlib import Path

import oracle_c
from gate_c import judge, load_corpus

HERE = Path(__file__).resolve().parent


class CorpusTests(unittest.TestCase):
    def test_every_case_is_a_test_in_its_oracle_and_hidden_from_the_prompt(self):
        tasks = load_corpus()
        self.assertEqual([t['id'] for t in tasks], ['c01', 'c02', 'c03', 'c04', 'c05', 'c06'])
        for task in tasks:
            source = ''.join(p.read_text() for p in oracle_c.oracle_files(task))
            defined = set(re.findall(r'^func (Test\w+)\(t \*testing\.T\)', source, re.M))
            names = [case['name'] for case in task['oracle']['cases']]
            self.assertEqual(len(names), len(set(names)))
            self.assertEqual(set(names), defined, task['id'])
            self.assertTrue({c['kind'] for c in task['oracle']['cases']} <= {'review', 'feature', 'guard'})
            self.assertTrue(any(c['kind'] == 'review' for c in task['oracle']['cases']))
            self.assertNotIn('Oracle', task['prompt'])
            self.assertNotIn('oracle', task['prompt'].lower())
            self.assertEqual(set(task['heads']), {'base', 'pre_review', 'fixed'})
            for head in task['heads'].values():
                self.assertRegex(head, r'^[0-9a-f]{40}$')

    def test_oracle_files_are_invisible_to_go(self):
        for path in (HERE / 'oracles').rglob('*'):
            if path.is_file():
                self.assertFalse(path.name.endswith('.go'), path)


class InstallTests(unittest.TestCase):
    def test_install_strips_suffix_and_never_overwrites(self):
        task = load_corpus(ids=['c02'])[0]
        with tempfile.TemporaryDirectory() as d:
            tree = Path(d)
            (tree / 'internal' / 'cli').mkdir(parents=True)
            installed = oracle_c.install_oracle(tree, task)
            self.assertEqual([p.name for p in installed], ['zz_oracle_c02_test.go'])
            with self.assertRaises(ValueError):
                oracle_c.install_oracle(tree, task)

    def test_files_and_cases_can_name_other_packages(self):
        with tempfile.TemporaryDirectory() as d:
            oracles, tree = Path(d) / 'oracles', Path(d) / 'tree'
            (oracles / 't').mkdir(parents=True)
            for name in ('a_test.go.oracle', 'b_test.go.oracle', 'stub.go.oracle'):
                (oracles / 't' / name).write_text('package x\n')
            task = {'id': 't', 'oracle': {'package': 'internal/cli', 'files': [
                'a_test.go.oracle', {'name': 'b_test.go.oracle', 'package': 'pkg/x'}],
                'base_stub': {'name': 'stub.go.oracle', 'package': 'pkg/x'},
                'cases': [{'name': 'TestA', 'kind': 'review'}, {'name': 'TestB', 'kind': 'review', 'package': 'pkg/x'}]}}
            self.assertEqual(oracle_c.packages(task), ['internal/cli', 'pkg/x'])
            self.assertEqual([oracle_c.case_package(task, c) for c in task['oracle']['cases']], ['internal/cli', 'pkg/x'])
            self.assertEqual([p for _, p in oracle_c.stub_specs(task, oracles)], ['pkg/x'])
            (tree / 'internal' / 'cli').mkdir(parents=True)
            self.assertEqual(oracle_c.present_packages(tree, task), ['internal/cli'], 'pkg/x is new')
            installed = oracle_c.install_oracle(tree, task, oracles)
            self.assertEqual([p.relative_to(tree).as_posix() for p in installed],
                             ['internal/cli/a_test.go', 'pkg/x/b_test.go'])
            before = oracle_c.frozen_hash([task], oracles)
            (oracles / 't' / 'stub.go.oracle').write_text('package y\n')
            self.assertNotEqual(before, oracle_c.frozen_hash([task], oracles), 'a stub edit changes the hash')


class VerdictTests(unittest.TestCase):
    def test_case_verdicts(self):
        name = 'TestOracleX_A'
        self.assertEqual(oracle_c.case_verdict('--- PASS: TestOracleX_A (0.1s)\nPASS\n', name, 0, False), 'pass')
        self.assertEqual(oracle_c.case_verdict('--- FAIL: TestOracleX_A (0.1s)\n', name, 1, False), 'fail')
        self.assertEqual(oracle_c.case_verdict('--- SKIP: TestOracleX_A (0.0s)\n', name, 0, False), 'skip')
        self.assertEqual(oracle_c.case_verdict('testing: warning: no tests to run\n', name, 0, False), 'no_result')
        self.assertEqual(oracle_c.case_verdict('panic: boom\n', name, 2, False), 'fail')
        self.assertEqual(oracle_c.case_verdict('', name, -9, True), 'timeout')
        # A prefix-sharing test name is a different case.
        self.assertEqual(oracle_c.case_verdict('--- PASS: TestOracleX_AB (0.1s)\n', name, 0, False), 'no_result')

    def test_scope_uses_directory_globs_and_reports_harness_apart(self):
        before = {'internal/cli/a.go': '1', 'docs/x.md': '1', 'internal/cli/a_test.go': '1', 'CLAUDE.md': '1'}
        after = {'internal/cli/a.go': '2', 'internal/cli/sub/new.go': '1', 'docs/x.md': '2',
                 'internal/cli/a_test.go': '2', 'CLAUDE.md': '2', '.claude/state.json': '1'}
        scope = oracle_c.audit_globs(before, after, ['internal/cli/*'])
        self.assertEqual(scope['outside_allowed'], ['docs/x.md'])
        self.assertEqual(scope['harness_changes'], ['.claude/state.json', 'CLAUDE.md'])
        self.assertEqual(scope['changed_existing_tests'], ['internal/cli/a_test.go'])
        self.assertFalse(scope['accepted_scope'])
        self.assertTrue(oracle_c.audit_globs(before, before, ['internal/cli/*'])['accepted_scope'])


class GateTests(unittest.TestCase):
    task = {'id': 't', 'oracle': {'cases': [{'name': 'R1', 'kind': 'review'}, {'name': 'F1', 'kind': 'feature'}]}}

    def runs(self, fixed, pre, base):
        names = ('R1', 'F1')
        return {head: {'build': {}, 'cases': dict(zip(names, values))}
                for head, values in (('fixed', fixed), ('pre_review', pre), ('base', base))}

    def test_valid_oracle_passes_all_gates(self):
        verdict = judge(self.task, self.runs(('pass', 'pass'), ('fail', 'pass'), ('fail', 'fail')))
        self.assertTrue(verdict['gate1'] and verdict['gate2'] and verdict['gate3'])
        self.assertEqual(verdict['drop'], [])

    def test_non_discriminating_cases_are_marked_for_dropping(self):
        verdict = judge(self.task, self.runs(('pass', 'pass'), ('pass', 'pass'), ('pass', 'pass')))
        self.assertFalse(verdict['gate2'])
        self.assertFalse(verdict['gate3'])
        self.assertEqual(verdict['drop'], ['R1', 'F1'])
        unsatisfiable = judge(self.task, self.runs(('fail', 'pass'), ('fail', 'pass'), ('fail', 'fail')))
        self.assertFalse(unsatisfiable['gate1'])
        self.assertEqual(unsatisfiable['drop'], ['R1'])

    def test_base_that_cannot_compile_without_a_stub_is_not_a_gate3_pass(self):
        verdict = judge(self.task, self.runs(('pass', 'pass'), ('fail', 'pass'),
                                             ('build_failed', 'build_failed')))
        self.assertFalse(verdict['gate3'])
        self.assertIn('base_stub', verdict['gate3_note'])
        guard = {'id': 't', 'oracle': {'cases': [{'name': 'G1', 'kind': 'guard'}]}}
        runs = lambda base: {h: {'build': {}, 'cases': {'G1': v}} for h, v in
                             (('fixed', 'pass'), ('pre_review', 'pass'), ('base', base))}
        self.assertEqual(judge(guard, runs('pass'))['drop'], [])
        self.assertEqual(judge(guard, runs('fail'))['drop'], ['G1'])
        stubbed = {'id': 't', 'oracle': dict(self.task['oracle'], base_stub='stub.go.oracle')}
        self.assertTrue(judge(stubbed, self.runs(('pass', 'pass'), ('fail', 'pass'),
                                                 ('build_failed', 'build_failed')))['gate3'])


if __name__ == '__main__':
    unittest.main()
