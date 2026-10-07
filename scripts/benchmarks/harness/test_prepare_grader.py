"""Module cache verification, two-direction calibration and calibration document contracts."""
import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import unittest
import zipfile

import grader
import prepare_grader as pg

CHECKOUT = Path(__file__).resolve().parents[3]


def h1(files):
    """Go dirhash Hash1 of name -> bytes, the go.sum hash format."""
    summary = ''.join(f'{hashlib.sha256(data).hexdigest()}  {name}\n' for name, data in sorted(files.items()))
    return 'h1:' + base64.b64encode(hashlib.sha256(summary.encode()).digest()).decode()


def consumer_module(base):
    """A module requiring example.com/dep v1.0.0 from a local file proxy; returns (source, proxy, go.sum)."""
    proxy = base / 'proxy'
    versions = proxy / 'example.com' / 'dep' / '@v'
    versions.mkdir(parents=True)
    gomod = b'module example.com/dep\n\ngo 1.21\n'
    files = {'example.com/dep@v1.0.0/go.mod': gomod,
             'example.com/dep@v1.0.0/dep.go': b'package dep\n\nfunc Value() int { return 7 }\n'}
    with zipfile.ZipFile(versions / 'v1.0.0.zip', 'w') as archive:
        for name, data in files.items():
            archive.writestr(name, data)
    (versions / 'v1.0.0.mod').write_bytes(gomod)
    (versions / 'v1.0.0.info').write_text('{"Version":"v1.0.0","Time":"2026-01-01T00:00:00Z"}')
    (versions / 'list').write_text('v1.0.0\n')
    source = base / 'source'
    source.mkdir()
    (source / 'go.mod').write_text('module example.com/consumer\n\ngo 1.21\n\nrequire example.com/dep v1.0.0\n')
    (source / 'consumer.go').write_text('package consumer\n\nimport "example.com/dep"\n\n'
                                        'func Seven() int { return dep.Value() }\n')
    (source / 'consumer_test.go').write_text('package consumer\n\nimport "testing"\n\n'
                                             'func TestSeven(t *testing.T) {\n\tif Seven() != 7 {\n'
                                             '\t\tt.Fatal("Seven() != 7")\n\t}\n}\n')
    go_sum = f'example.com/dep v1.0.0 {h1(files)}\nexample.com/dep v1.0.0/go.mod {h1({"go.mod": gomod})}\n'
    return source, proxy.as_uri(), go_sum


@unittest.skipUnless(shutil.which('go'), 'requires the Go toolchain')
class PrepareTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.base = Path(directory.name).resolve()
        self.addCleanup(directory.cleanup)
        self.addCleanup(pg.remove_tree, self.base)
        self.source, self.proxy, self.go_sum = consumer_module(self.base)

    def test_module_cache_is_go_sum_verified_read_only_and_build_cache_warm(self):
        (self.source / 'go.sum').write_text(self.go_sum)
        prepared = pg.prepare(self.source, self.base / 'grader', ['./...'], self.proxy)
        self.assertEqual(prepared['modules'], 1)
        self.assertEqual((self.source / 'go.sum').read_text(), self.go_sum)
        modcache = Path(prepared['modcache'])
        self.assertTrue((modcache / 'example.com' / 'dep@v1.0.0' / 'dep.go').is_file())
        entries = [modcache, *modcache.rglob('*')]
        self.assertEqual([str(path) for path in entries if os.lstat(path).st_mode & 0o222], [])
        self.assertTrue(any(Path(prepared['warm_cache']).rglob('*-d')))
        self.assertEqual(Path(prepared['go']).name, 'go')

    def test_unpinned_or_mismatched_module_hash_refuses_preparation(self):
        zip_line, mod_line = self.go_sum.splitlines()
        tampered = zip_line[:-6] + 'AAAAA=' + '\n' + mod_line + '\n'
        cases = {'zip hash missing': (mod_line + '\n', 'not pinned in go.sum: example.com/dep@v1.0.0'),
                 'zip hash mismatched': (tampered, 'checksum mismatch')}
        for index, (name, (go_sum, message)) in enumerate(cases.items()):
            with self.subTest(name):
                (self.source / 'go.sum').write_text(go_sum)
                with self.assertRaisesRegex(pg.PrepareError, message):
                    pg.prepare(self.source, self.base / f'grader-{index}', ['./...'], self.proxy)


def task(task_id, after, expected):
    return {'id': task_id, 'expected_tests': expected, 'corpus': {
        'id': task_id.lower(), 'mutation': {'path': 'consumer.go', 'before': 'return dep.Value()', 'after': after},
        'oracle': {'command': ['go', 'test', '-p', '1', '.', '-run', '^TestSeven$', '-count=1']}}}


@unittest.skipUnless(sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and shutil.which('go'),
                     'requires macOS sandbox-exec and the Go toolchain')
class CalibrationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        directory = tempfile.TemporaryDirectory()
        cls.addClassCleanup(directory.cleanup)
        cls.base = Path(directory.name).resolve()
        cls.addClassCleanup(pg.remove_tree, cls.base)
        cls.source, proxy, go_sum = consumer_module(cls.base)
        (cls.source / 'go.sum').write_text(go_sum)
        cls.prepared = pg.prepare(cls.source, cls.base / 'grader', ['./...'], proxy)

    def calibrate(self, name, tasks, prepared=None):
        return pg.calibrate(tasks, self.source, prepared or self.prepared, self.base / name, self.base / name / 'logs',
                            timeout=120)

    def test_clean_accepted_and_mutation_detected_passes(self):
        result = self.calibrate('good', [task('GT-AG-001', 'return dep.Value() + 1', ['TestSeven'])])
        self.assertEqual((result['status'], result['tasks']), ('passed', [
            {'task_id': 'GT-AG-001', 'clean_accepted': True, 'mutated_accepted': False}]))
        self.assertEqual([run['signal'] for run in result['runs']], ['accepted', 'oracle_failed'])
        self.assertEqual(sorted(os.listdir(self.base / 'good')), ['logs'])

    def test_weak_mutation_broken_oracle_and_empty_module_cache_each_fail(self):
        weak = self.calibrate('weak', [task('GT-AG-002', 'return (dep.Value())', ['TestSeven'])])
        self.assertEqual((weak['status'], weak['tasks']), ('failed', [
            {'task_id': 'GT-AG-002', 'clean_accepted': True, 'mutated_accepted': True}]))
        broken = self.calibrate('broken', [task('GT-AG-003', 'return dep.Value() + 1', ['TestSeven', 'TestMissing'])])
        self.assertEqual((broken['status'], broken['tasks']), ('failed', [
            {'task_id': 'GT-AG-003', 'clean_accepted': False, 'mutated_accepted': False}]))
        self.assertEqual(broken['runs'][0]['signal'], 'oracle_output_invalid')
        empty = self.base / 'empty-modcache'
        empty.mkdir()
        result = self.calibrate('empty', [task('GT-AG-001', 'return dep.Value() + 1', ['TestSeven'])],
                                {**self.prepared, 'modcache': str(empty)})
        self.assertEqual(result['status'], 'failed')
        self.assertEqual(result['runs'][0]['oracle'], {'ran': False, 'build_failed': True, 'expected_passed': 0,
                                                       'expected_failed': 0})
        self.assertNotIn(b'"Action":"pass"', (self.base / 'empty' / 'logs' / 'GT-AG-001-clean.jsonl').read_bytes())


class CalibrationDocumentTests(unittest.TestCase):
    def test_before_is_created_exclusively_and_after_recorded_once(self):
        before = {'status': 'failed', 'tasks': [{'task_id': 'GT-AG-001', 'clean_accepted': False,
                                                  'mutated_accepted': False}], 'runs': [{'signal': 'oracle_failed'}]}
        after = {'status': 'passed', 'tasks': [{'task_id': 'GT-AG-001', 'clean_accepted': True,
                                                 'mutated_accepted': False}], 'runs': []}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'calibration.json'
            pg.write_calibration(path, 'ab' * 16, before)
            with self.assertRaises(FileExistsError):
                pg.write_calibration(path, 'cd' * 16, after)
            self.assertEqual(json.loads(path.read_text()), {'session_id': 'ab' * 16, 'before': {
                'status': 'failed', 'tasks': before['tasks']}})
            pg.record_after(path, after)
            with self.assertRaises(ValueError):
                pg.record_after(path, after)
            self.assertEqual(json.loads(path.read_text()), {'session_id': 'ab' * 16, 'before': {
                'status': 'failed', 'tasks': before['tasks']}, 'after': {'status': 'passed', 'tasks': after['tasks']}})
            self.assertEqual(sorted(os.listdir(directory)), ['calibration.json'])


class AgentTaskTests(unittest.TestCase):
    def test_committed_agent_tasks_resolve_to_their_pinned_corpus_entries(self):
        manifest, tasks = pg.agent_tasks(CHECKOUT)
        self.assertEqual(manifest['schema_version'], 'harness_golden_set.v1')
        self.assertEqual([task['id'] for task in tasks],
                         [f'GT-AGENT-{group}0{index}' for group in 'AB' for index in range(1, 7)])
        for task in tasks:
            committed = json.loads((CHECKOUT / 'evals/harness/tasks/agent' / (task['id'] + '.json')).read_text())
            self.assertEqual(task['corpus']['id'], committed['corpus_ref']['task_id'])
            self.assertEqual(task['expected_tests'], committed['expected_tests'])

    def test_corpus_digest_mismatch_is_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ('evals/harness/manifest.json', 'evals/harness/tasks/agent/GT-AGENT-B02.json',
                         'scripts/benchmarks/harness/corpus_b.json'):
                (root / name).parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(CHECKOUT / name, root / name)
            self.assertEqual([task['id'] for task in pg.agent_tasks(root)[1]], ['GT-AGENT-B02'])
            with (root / 'scripts/benchmarks/harness/corpus_b.json').open('ab') as corpus:
                corpus.write(b' ')
            with self.assertRaisesRegex(ValueError, 'corpus_digest_mismatch: GT-AGENT-B02'):
                pg.agent_tasks(root)


if __name__ == '__main__':
    unittest.main()
