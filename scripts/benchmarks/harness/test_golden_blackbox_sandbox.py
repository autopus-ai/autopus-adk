"""artifact.sb, oracle.sb and the sibling stage runner (SPEC-HARNEVAL-003 REQ-HR-08, REQ-HR-10, probe A3).

The sandbox classes are macOS only. No credential file is read: the protected expected outputs are
fixture files below a fake golden set root, and the real checkout is probed on its committed
evals/harness manifest and .git only. The stage runner class needs only POSIX processes.
"""
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

import golden_blackbox as gb
import golden_sandbox as gs
import grader
import prepare_grader
import process_tree

PROBE = '''package main

import (
\t"fmt"
\t"net"
\t"os"
\t"sort"
\t"strings"
\t"time"
)

func main() {
\tfor index, arg := range os.Args[1:] {
\t\top, target, _ := strings.Cut(arg, "=")
\t\tvar err error
\t\tswitch op {
\t\tcase "read":
\t\t\t_, err = os.ReadFile(target)
\t\tcase "stat":
\t\t\t_, err = os.Stat(target)
\t\tcase "list":
\t\t\t_, err = os.ReadDir(target)
\t\tcase "write":
\t\t\terr = os.WriteFile(target, []byte("x"), 0o644)
\t\tcase "link":
\t\t\tfrom, to, _ := strings.Cut(target, ">")
\t\t\terr = os.Link(from, to)
\t\tcase "dial":
\t\t\tvar conn net.Conn
\t\t\tconn, err = net.DialTimeout("tcp", target, time.Second)
\t\t\tif conn != nil {
\t\t\t\tconn.Close()
\t\t\t}
\t\tcase "env":
\t\t\tvar keys []string
\t\t\tfor _, pair := range os.Environ() {
\t\t\t\tkeys = append(keys, strings.SplitN(pair, "=", 2)[0])
\t\t\t}
\t\t\tsort.Strings(keys)
\t\t\terr = fmt.Errorf("%s", strings.Join(keys, ","))
\t\t}
\t\tfmt.Printf("PROBE %d %v\\n", index, err)
\t}
}
'''
DENIED = 'operation not permitted'
SANDBOXED = sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and shutil.which('go')


def probe_results(stdout: bytes) -> list:
    return [line.split(' ', 2)[2] for line in stdout.decode().splitlines() if line.startswith('PROBE ')]


@unittest.skipUnless(SANDBOXED, 'requires macOS sandbox-exec and the Go toolchain')
class ProfileTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        directory = tempfile.TemporaryDirectory()
        cls.addClassCleanup(directory.cleanup)
        cls.base = Path(directory.name).resolve()
        cls.addClassCleanup(prepare_grader.remove_tree, cls.base)
        source = cls.base / 'probe-src'
        source.mkdir()
        (source / 'go.mod').write_text('module probe\n\ngo 1.21\n')
        (source / 'main.go').write_text(PROBE)
        cls.probe = cls.base / 'bin' / 'probe'
        subprocess.run(['go', 'build', '-o', str(cls.probe), '.'], cwd=source, check=True, capture_output=True,
                       env={**os.environ, 'CGO_ENABLED': '0'})
        cls.oracle = gs.build_oracle(cls.base / 'bin')
        cls.set_root = cls.base / 'set'
        cls.expected = cls.set_root / 'evals/harness/oracles/X01/stdout.txt'
        cls.expected.parent.mkdir(parents=True)
        cls.expected.write_text('3\n')
        cls.params = gs.guard(cls.set_root)
        cls.prepared = prepare_grader.prepare(source, cls.base / 'grader', ['./...'], 'off')
        cls.source = source

    def stage_dirs(self, name: str) -> tuple:
        work = self.base / name
        for sub in ('input', 'run/out', 'run/home', 'run/tmp', 'logs', 'oracle', 'outside'):
            (work / sub).mkdir(parents=True)
        (work / 'input' / 'in.txt').write_text('input\n')
        return work, work / 'input', work / 'run'

    def run_probe(self, name: str, ops: list) -> tuple:
        work, inputs, run_root = self.stage_dirs(name)
        argv = gs.run_argv(self.probe, inputs, run_root, [self.probe, *ops], self.params)
        run = gs.run_stage(argv, run_root / 'out', work / 'logs/out', work / 'logs/err', 60)
        return run, probe_results((work / 'logs/out').read_bytes()), work

    def test_run_mode_reads_only_its_inputs_and_writes_only_its_scratch(self):
        listener = socket.create_server(('127.0.0.1', 0))
        self.addCleanup(listener.close)
        listener.settimeout(0.5)
        work = self.base / 'probe-run'
        ops = [f'read={work}/input/in.txt', f'write={work}/run/out/x.txt', f'write={work}/outside/x.txt',
               f'read={self.expected}', f'stat={self.expected}', f'list={self.expected.parent}',
               f'link={self.expected}>{work}/run/out/hl', f'read={gs.CHECKOUT}/evals/harness/manifest.json',
               f'stat={gs.CHECKOUT}/.git', f'write={work}/input/in.txt', f'dial=127.0.0.1:{listener.getsockname()[1]}',
               'env=']
        run, results, _ = self.run_probe('probe-run', ops)
        self.assertEqual((run['exit_code'], run['leftover']), (0, False))
        self.assertEqual(results[:2], ['<nil>', '<nil>'])
        for index, result in enumerate(results[2:11], 2):
            self.assertIn(DENIED, result, ops[index])
        self.assertEqual(results[11], 'HOME,PATH,PWD,TMPDIR,TZ')
        with self.assertRaises(OSError):
            listener.accept()
        self.assertFalse((work / 'outside/x.txt').exists())
        self.assertFalse((work / 'run/out/hl').exists())
        self.assertEqual(self.expected.read_text(), '3\n')

    def test_build_mode_builds_offline_with_the_read_only_module_cache(self):
        import golden_blackbox_trial as gbt
        root = gbt.new_build_root(self.base / 'build', self.source, Path(self.prepared['warm_cache']))
        argv = gs.build_argv(self.prepared, root, '.', self.params)
        run = gs.run_stage(argv, root / 'ws', self.base / 'build.out', self.base / 'build.err', 300)
        self.assertEqual(run['exit_code'], 0, (self.base / 'build.err').read_text())
        self.assertTrue((root / 'bin' / 'artifact').is_file())
        with self.assertRaisesRegex(ValueError, 'protected'):
            gs.check_roots({'SCRATCH': self.base}, self.params)

    def judge(self, name: str, setup, path: str = 'result.txt') -> dict:
        work, _, run_root = self.stage_dirs(name)
        out = run_root / 'out'
        setup(out, work / 'outside')
        oracle = gb.definition({'oracle_mode': 'black_box', 'black_box_oracle': {
            'build': './cmd/x', 'command': ['{artifact}'], 'inputs': [],
            'assertions': [{'id': 'result', 'kind': 'file', 'path': path,
                            'expected': {'path': 'evals/harness/oracles/X01/stdout.txt',
                                         'sha256': '0' * 64}}]}})
        data = gb.bundle('GT-AGENT-X01', oracle, {'result': b'3\n'}, {'exit_code': 0, 'timed_out': False,
                                                                      'overflow': False}, b'')
        started = time.monotonic()
        argv = gs.oracle_argv(self.oracle, out, work / 'oracle', 'GT-AGENT-X01', self.params)
        done = gs.run_stage(argv, work / 'oracle', work / 'logs/o.out', work / 'logs/o.err', 60, data)
        self.assertEqual(done['exit_code'], 0, (work / 'logs/o.err').read_text())
        self.assertLess(time.monotonic() - started, 30)
        return gb.decode_result((work / 'oracle' / gb.RESULT_FILE).read_bytes(), 'GT-AGENT-X01')

    def test_output_self_check_under_oracle_sb_rejects_every_threat_type(self):
        def regular(out, _):
            (out / 'result.txt').write_text('3\n')

        def inside(out, _):
            (out / 'real.txt').write_text('3\n')
            (out / 'result.txt').symlink_to('real.txt')

        def outside(out, _):
            (out / 'result.txt').symlink_to(self.expected)

        def directory(out, _):
            (out / 'real').mkdir()
            (out / 'real/result.txt').write_text('3\n')
            (out / 'sub').symlink_to('real')

        def hard(out, other):
            (other / 'copy.txt').write_text('3\n')
            os.link(other / 'copy.txt', out / 'result.txt')

        def fifo(out, _):
            os.mkfifo(out / 'result.txt')

        def large(out, _):
            (out / 'result.txt').write_bytes(b'3\n' * gb.OUTPUT_LIMIT)
        cases = [(regular, 'ok'), (inside, 'link_rejected'), (outside, 'link_rejected'), (directory, 'link_rejected'),
                 (hard, 'link_rejected'), (fifo, 'link_rejected'), (large, 'too_large')]
        for setup, check in cases:
            with self.subTest(setup.__name__):
                path = 'sub/result.txt' if setup is directory else 'result.txt'
                result = self.judge('judge-' + setup.__name__, setup, path)
                self.assertEqual(result['output_check'], check)
                passed = [item['passed'] for item in result['assertions']]
                self.assertEqual(passed, [True] if check == 'ok' else [])

    def test_oracle_sb_cannot_read_expected_outputs_or_change_artifact_files(self):
        work, _, run_root = self.stage_dirs('oracle-probe')
        (run_root / 'out/result.txt').write_text('4\n')
        ops = [f'read={self.expected}', f'read={run_root}/out/result.txt', f'write={run_root}/out/result.txt',
               f'write={work}/oracle/note.txt', f'read={gs.CHECKOUT}/evals/harness/manifest.json']
        argv = gs.oracle_argv(self.probe, run_root / 'out', work / 'oracle', 'GT-AGENT-X01', self.params)
        argv[argv.index(str(self.probe), argv.index('-f')):] = [str(self.probe), *ops]
        run = gs.run_stage(argv, work / 'oracle', work / 'logs/out', work / 'logs/err', 60)
        results = probe_results((work / 'logs/out').read_bytes())
        self.assertEqual(run['exit_code'], 0)
        self.assertIn(DENIED, results[0])
        self.assertEqual(results[1], '<nil>')
        self.assertIn(DENIED, results[2])
        self.assertEqual(results[3], '<nil>')
        self.assertIn(DENIED, results[4])
        self.assertEqual((run_root / 'out/result.txt').read_text(), '4\n')


class StageRunnerTests(unittest.TestCase):
    """run_stage alone: rlimits, capped capture, timeout, stdin and the session sweep (no sandbox needed)."""

    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()

    def stage(self, argv, timeout=30, stdin=None):
        result = gs.run_stage(argv, self.base, self.base / 'out', self.base / 'err', timeout, stdin)
        return result, (self.base / 'out').read_bytes()

    def test_stdin_bytes_and_files_reach_the_stage(self):
        result, out = self.stage(['/bin/cat'], stdin=b'piped bundle')
        self.assertEqual((result['exit_code'], out), (0, b'piped bundle'))
        (self.base / 'in.txt').write_text('from a file')
        self.assertEqual(self.stage(['/bin/cat'], stdin=self.base / 'in.txt')[1], b'from a file')

    def test_timeout_overflow_and_session_members_are_handled(self):
        slow, _ = self.stage(['/bin/sleep', '30'], timeout=1)
        self.assertEqual((slow['timed_out'], slow['leftover']), (True, False))
        flood, out = self.stage([sys.executable, '-c', 'import sys; sys.stdout.write("x" * (3 << 20))'])
        self.assertTrue(flood['overflow'])
        self.assertLessEqual(len(out), grader.OUTPUT_LIMIT)
        background, out = self.stage(['/bin/sh', '-c', '/bin/sleep 300 >/dev/null 2>&1 & echo $!'])
        self.assertEqual((background['stragglers'], background['leftover']), (True, False))
        state = subprocess.run(['/bin/ps', '-o', 'stat=', '-p', out.decode().strip()], capture_output=True, text=True)
        self.assertTrue(not state.stdout.strip() or state.stdout.strip().startswith('Z'))

    def test_a_process_that_escaped_the_session_with_the_pipe_is_a_leftover(self):
        # No walk runs while the stage lives (process_tree.POLL is an hour), so the setsid child, orphaned
        # when its parent exits, is out of reach; still holding the stdout pipe makes it a leftover.
        script = ('import pathlib, subprocess, sys; child = subprocess.Popen(["/bin/sleep", "60"], '
                  'start_new_session=True); pathlib.Path(sys.argv[1]).write_text(str(child.pid))')
        with mock.patch.object(process_tree, 'POLL', 3600):
            escaped, _ = self.stage([sys.executable, '-c', script, str(self.base / 'pid')])
        pid = int((self.base / 'pid').read_text())
        self.addCleanup(lambda: subprocess.run(['/bin/kill', '-9', str(pid)], capture_output=True))
        self.assertTrue(escaped['leftover'])
        self.assertEqual(json.loads(json.dumps(escaped))['exit_code'], 0)


if __name__ == '__main__':
    unittest.main()
