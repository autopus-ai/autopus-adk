"""Trusted parser, grader allowlist and grader.sb confinement contracts."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import socket
import sys
import tempfile
import threading
import unittest
from unittest import mock

import grader
import prepare_grader

PASSED = {'ran': True, 'build_failed': False, 'expected_passed': 2, 'expected_failed': 0}
NOTHING = {'ran': False, 'build_failed': False, 'expected_passed': 0, 'expected_failed': 0}
CANARIES = {'HARNEVAL_CANARY': 'leak', 'GITHUB_TOKEN': 'leak', 'GITHUB_ENV': '/tmp/leak',
            'ACTIONS_ID_TOKEN_REQUEST_TOKEN': 'leak', 'RUNNER_TEMP': 'leak', 'CODEX_API_KEY': 'leak',
            'OPENAI_API_KEY': 'leak'}


def stream(*rows):
    """test2json stdout for (action, test) pairs; an empty test is a package-level event."""
    events = [{'Action': action, 'Package': 'p', **({'Test': test} if test else {})} for action, test in rows]
    return ''.join(json.dumps(event) + '\n' for event in events).encode()


class JudgeTests(unittest.TestCase):
    def verdict(self, stdout, exit_code=0, timed_out=False, overflow=False):
        return grader.judge(stdout, exit_code, timed_out, overflow, ['TestA', 'TestB'])

    def test_accepts_only_exit_zero_with_every_expected_top_level_pass(self):
        clean = stream(('start', ''), ('run', 'TestA'), ('pass', 'TestA'), ('run', 'TestB'),
                       ('run', 'TestB/sub'), ('pass', 'TestB/sub'), ('pass', 'TestB'), ('pass', ''))
        self.assertEqual(self.verdict(clean), {'signal': 'accepted', 'oracle': PASSED})
        self.assertEqual(self.verdict(clean, exit_code=1), {'signal': 'oracle_failed', 'oracle': PASSED})
        subtest_only = stream(('run', 'TestA'), ('pass', 'TestA'), ('run', 'TestB/sub'), ('pass', 'TestB/sub'))
        self.assertEqual(self.verdict(subtest_only), {'signal': 'oracle_output_invalid', 'oracle': {
            'ran': True, 'build_failed': False, 'expected_passed': 1, 'expected_failed': 0}})
        skipped = stream(('run', 'TestA'), ('pass', 'TestA'), ('run', 'TestB'), ('skip', 'TestB'))
        self.assertEqual(self.verdict(skipped)['signal'], 'oracle_output_invalid')

    def test_expected_fail_and_build_failure_are_oracle_failures(self):
        failed = stream(('run', 'TestA'), ('fail', 'TestA'), ('run', 'TestB'), ('pass', 'TestB'), ('fail', ''))
        self.assertEqual(self.verdict(failed, exit_code=1), {'signal': 'oracle_failed', 'oracle': {
            'ran': True, 'build_failed': False, 'expected_passed': 1, 'expected_failed': 1}})
        forged = stream(('pass', 'TestA'), ('fail', 'TestA'), ('pass', 'TestB'))
        self.assertEqual(self.verdict(forged)['signal'], 'oracle_failed')
        build = (b'{"ImportPath":"p [p.test]","Action":"build-output","Output":"# p\\n"}\n'
                 b'{"ImportPath":"p [p.test]","Action":"build-fail"}\n'
                 b'{"Time":"2026-10-07T00:00:00Z","Action":"fail","Package":"p","Elapsed":0,"FailedBuild":"p [p.test]"}\n')
        self.assertEqual(self.verdict(build, exit_code=1), {'signal': 'oracle_failed', 'oracle': {
            'ran': False, 'build_failed': True, 'expected_passed': 0, 'expected_failed': 0}})
        self.assertEqual(self.verdict(b'', exit_code=1), {'signal': 'oracle_failed', 'oracle': NOTHING})

    def test_malformed_oversized_or_unterminated_output_is_invalid(self):
        clean = stream(('run', 'TestA'), ('pass', 'TestA'), ('run', 'TestB'), ('pass', 'TestB'))
        cases = {
            'plain text line': clean + b'PASS\n',
            'unknown action': clean + b'{"Action":"won","Test":"TestA"}\n',
            'duplicate key': clean + b'{"Action":"pass","Action":"pass"}\n',
            'wrong field type': clean + b'{"Action":"pass","Test":1}\n',
            'unknown field': clean + b'{"Action":"pass","Test":"TestA","Verdict":"ok"}\n',
            'boolean number': clean + b'{"Action":"pass","Elapsed":true}\n',
            'not an object': clean + b'["pass"]\n',
            'blank line': clean + b'\n',
            'missing final newline': clean[:-1],
            'over one MiB': clean + b'{"Action":"output","Output":"' + b'x' * grader.OUTPUT_LIMIT + b'"}\n',
        }
        for name, stdout in cases.items():
            with self.subTest(name):
                self.assertEqual(self.verdict(stdout), {'signal': 'oracle_output_invalid', 'oracle': NOTHING})
        self.assertEqual(self.verdict(clean, overflow=True), {'signal': 'oracle_output_invalid', 'oracle': NOTHING})
        self.assertEqual(self.verdict(clean, exit_code=-9, timed_out=True)['signal'], 'oracle_timeout')
        with self.assertRaises(ValueError):
            grader.judge(clean, 0, False, False, [])


class GraderArgvTests(unittest.TestCase):
    COMMAND = ['go', 'test', '-p', '1', './pkg/x', '-run', '^TestA$', '-count=1']

    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()
        self.root = self.base / 'session' / 'grade'
        self.prepared = {'go': str(self.base / 'go/bin/go'), 'goroot': str(self.base / 'go'),
                         'modcache': str(self.base / 'session/grader/modcache'),
                         'warm_cache': str(self.base / 'session/grader/warm/gocache')}
        for path in (self.root, self.prepared['goroot'], self.prepared['modcache'], self.prepared['warm_cache']):
            Path(path).mkdir(parents=True, exist_ok=True)

    def test_grader_environment_is_exactly_the_allowlist(self):
        roots = ('/Users/maintainer', '/Users/maintainer/Library/codex home')
        with mock.patch.dict(os.environ, CANARIES), mock.patch.object(grader, 'credential_roots', return_value=roots):
            argv = grader.grader_argv(self.root, self.COMMAND, self.prepared)
        self.assertEqual(argv[:2], ['/usr/bin/env', '-i'])
        split = argv.index(grader.SANDBOX)
        self.assertEqual(dict(item.split('=', 1) for item in argv[2:split]), {
            'PATH': str(self.base / 'go/bin'), 'HOME': str(self.root / 'home'), 'TMPDIR': str(self.root / 'tmp'),
            'GOPATH': str(self.root / 'gopath'), 'GOCACHE': str(self.root / 'gocache'),
            'GOMODCACHE': self.prepared['modcache'], 'GOFLAGS': '-mod=mod', 'GOPROXY': 'off', 'GOSUMDB': 'off',
            'GOTOOLCHAIN': 'local', 'PWD': str(self.root / 'ws')})
        self.assertEqual(argv[split:], [grader.SANDBOX, '-f', str(grader.PROFILE), '-D', 'GRADE_ROOT=' + str(self.root),
                                        '-D', 'MODCACHE=' + self.prepared['modcache'],
                                        '-D', 'GOROOT=' + self.prepared['goroot'],
                                        '-D', 'ACCOUNT_HOME=/Users/maintainer',
                                        '-D', 'CODEX_HOME_DIR=/Users/maintainer/Library/codex home',
                                        self.prepared['go'], 'test', '-json', '-p', '1', './pkg/x', '-run',
                                        '^TestA$', '-count=1'])
        self.assertNotIn('leak', ' '.join(argv))

    def test_rejects_roots_that_would_open_protected_paths_and_odd_commands(self):
        link = self.base / 'link'
        link.symlink_to(self.root)
        for root in (Path('/'), self.base / 'session', link, Path('session/grade'), self.base / 'missing'):
            with self.subTest(root=str(root)), self.assertRaises(ValueError):
                grader.grader_argv(root, self.COMMAND, self.prepared)
        for command in (['go', 'build', './...'], self.COMMAND + ['-json'], ['go', 'test', '-exec=/bin/sh', '.'],
                        ['go', 'test', '-toolexec', '/bin/sh', '.'], ['sh', '-c', 'go test']):
            with self.subTest(command=command), self.assertRaises(ValueError):
                grader.grader_argv(self.root, command, self.prepared)

    def test_rejects_a_module_cache_or_toolchain_that_would_reopen_a_home(self):
        # grader.sb opens reads below the homes for these two roots, so neither may be or hold a home.
        for key, value in (('goroot', '/'), ('goroot', str(Path.home())), ('modcache', '/private/tmp'),
                           ('modcache', str(self.base / 'missing')), ('goroot', '')):
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                grader.grader_argv(self.root, self.COMMAND, {**self.prepared, key: value})


PROBE = '''package probe

import (
\t"net"
\t"os"
\t"os/exec"
\t"path/filepath"
\t"sort"
\t"strings"
\t"testing"
\t"time"
)

func target(t *testing.T, name string) string {
\tdata, err := os.ReadFile(name + ".target")
\tif err != nil {
\t\tt.Fatal(err)
\t}
\treturn strings.TrimSpace(string(data))
}

func TestProbe(t *testing.T) {
\tt.Logf("PROBE outside=%v", os.WriteFile(target(t, "outside"), []byte("x"), 0o644))
\tt.Logf("PROBE modcache=%v", os.WriteFile(filepath.Join(os.Getenv("GOMODCACHE"), "planted"), []byte("x"), 0o644))
\tt.Logf("PROBE inside=%v", os.WriteFile(filepath.Join(os.Getenv("HOME"), "inside"), []byte("x"), 0o644))
\tt.Logf("PROBE cache=%v", os.WriteFile(filepath.Join(os.Getenv("GOCACHE"), "planted"), []byte("x"), 0o644))
\tconn, err := net.DialTimeout("tcp", target(t, "address"), 2*time.Second)
\tif conn != nil {
\t\tconn.Close()
\t}
\tt.Logf("PROBE dial=%v", err)
\tvar keys []string
\tfor _, pair := range os.Environ() {
\t\tkeys = append(keys, strings.SplitN(pair, "=", 2)[0])
\t}
\tsort.Strings(keys)
\tt.Logf("PROBE env=%s", strings.Join(keys, ","))
\tsleeper := exec.Command(os.Args[0], "-test.run=^TestSleep$")
\tsleeper.Env = append(os.Environ(), "PROBE_SLEEP=1")
\tt.Logf("PROBE spawn=%v", sleeper.Start())
}

func TestSleep(t *testing.T) {
\tif os.Getenv("PROBE_SLEEP") != "" {
\t\ttime.Sleep(time.Minute)
\t}
}

func TestFlood(t *testing.T) {
\tfor i := 0; i < 2048; i++ {
\t\tos.Stdout.WriteString(strings.Repeat("x", 1023) + "\\n")
\t}
}
'''


def listing(root):
    """SHA-256 over every entry's relative path, mode, size and mtime below root."""
    rows = sorted(f'{path.relative_to(root)}\0{path.lstat().st_mode}\0{path.lstat().st_size}\0'
                  f'{path.lstat().st_mtime_ns}' for path in [root, *root.rglob('*')])
    return hashlib.sha256('\n'.join(rows).encode()).hexdigest()


@unittest.skipUnless(sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and shutil.which('go'),
                     'requires macOS sandbox-exec and the Go toolchain')
class SandboxTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        directory = tempfile.TemporaryDirectory()
        cls.addClassCleanup(directory.cleanup)
        cls.base = Path(directory.name).resolve()
        cls.addClassCleanup(prepare_grader.remove_tree, cls.base)
        cls.listener = socket.create_server(('127.0.0.1', 0))
        cls.addClassCleanup(cls.listener.close)
        cls.accepted = []

        def accept():
            try:
                cls.accepted.append(cls.listener.accept())
            except OSError:
                pass
        threading.Thread(target=accept, daemon=True).start()
        cls.source = cls.base / 'source'
        cls.source.mkdir()
        (cls.source / 'go.mod').write_text('module probe\n\ngo 1.21\n')
        (cls.source / 'probe_test.go').write_text(PROBE)
        (cls.source / 'outside.target').write_text(str(cls.base / 'outside.txt'))
        (cls.source / 'address.target').write_text('127.0.0.1:%d' % cls.listener.getsockname()[1])
        cls.prepared = prepare_grader.prepare(cls.source, cls.base / 'grader', ['./...'], 'off')

    def run_probe(self, name, test):
        root = prepare_grader.new_grade(self.base / name, self.source, Path(self.prepared['warm_cache']))
        result = grader.grade(root, ['go', 'test', '-count=1', '.', '-run', '^%s$' % test], [test], self.prepared,
                              self.base / (name + '.jsonl'), self.base / (name + '.stderr'), timeout=120)
        return root, result

    def test_grader_writes_only_its_root_and_cannot_connect_or_inherit(self):
        modcache = Path(self.prepared['modcache'])
        before = listing(modcache)
        with mock.patch.dict(os.environ, CANARIES):
            root, result = self.run_probe('g1', 'TestProbe')
        lines = (self.base / 'g1.jsonl').read_text().splitlines()
        output = ''.join(json.loads(line).get('Output', '') for line in lines)
        probe = dict(re.findall(r'PROBE (\w+)=(.*)', output))
        self.assertEqual(result['signal'], 'accepted', output)
        self.assertIn('operation not permitted', probe['outside'])
        self.assertFalse((self.base / 'outside.txt').exists())
        self.assertNotEqual(probe['modcache'], '<nil>')
        self.assertEqual(listing(modcache), before)
        self.assertEqual((probe['inside'], probe['cache']), ('<nil>', '<nil>'))
        self.assertIn('operation not permitted', probe['dial'])
        self.assertEqual(self.accepted, [])
        expected = set(grader.allowlist(root, Path(self.prepared['go']), modcache)) | {'PWD'}
        self.assertEqual(set(probe['env'].split(',')), expected)
        self.assertEqual(probe['spawn'], '<nil>')
        self.assertEqual((result['stragglers'], result['leftover']), (True, False))
        self.assertTrue((root / 'gocache' / 'planted').is_file())
        fresh = prepare_grader.new_grade(self.base / 'g2', self.source, Path(self.prepared['warm_cache']))
        self.assertFalse((fresh / 'gocache' / 'planted').exists())
        self.assertFalse((Path(self.prepared['warm_cache']) / 'planted').exists())

    def test_output_over_the_limit_is_cut_and_invalid(self):
        _, result = self.run_probe('g3', 'TestFlood')
        self.assertTrue(result['overflow'])
        self.assertEqual(result['signal'], 'oracle_output_invalid')
        self.assertLessEqual((self.base / 'g3.jsonl').stat().st_size, grader.OUTPUT_LIMIT)
        self.assertFalse(result['leftover'])


if __name__ == '__main__':
    unittest.main()
