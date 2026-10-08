"""Process isolation of artifact.sb, oracle.sb and grader.sb (SPEC-HARNEVAL-003 S1, REQ-HR-08).

On the live runner the golden step's environment holds the Codex credential, and a same-user
process can read another one's environment through the kern.procargs2 sysctl. A probe built here
tries exactly that against a sibling that holds a canary variable, and also signals it and reads its
kern.proc entry. Outside any profile it succeeds, so the probe is not vacuous; under each profile all
three are refused while the probe's own signals still work, and `go test` still grades under
grader.sb. macOS only; no credential file is read (CODEX_HOME is an empty directory).
"""
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import tempfile
import unittest

import golden_sandbox as gs
import grader
import prepare_grader

PROBE = '''package main

import (
\t"bytes"
\t"errors"
\t"fmt"
\t"os"
\t"strconv"
\t"strings"
\t"syscall"
\t"time"

\t"golang.org/x/sys/unix"
)

func main() {
\tfor index, arg := range os.Args[1:] {
\t\top, target, _ := strings.Cut(arg, "=")
\t\tpid, _ := strconv.Atoi(target)
\t\tvar err error
\t\tswitch op {
\t\tcase "hold":
\t\t\ttime.Sleep(time.Duration(pid) * time.Second)
\t\tcase "environ":
\t\t\tvar data []byte
\t\t\tdata, err = unix.SysctlRaw("kern.procargs2", pid)
\t\t\tif err == nil && bytes.Contains(data, []byte(os.Getenv("PROBE_CANARY_NAME"))) {
\t\t\t\terr = errors.New("canary read")
\t\t\t}
\t\tcase "signal":
\t\t\terr = syscall.Kill(pid, 0)
\t\tcase "self":
\t\t\terr = syscall.Kill(os.Getpid(), 0)
\t\tcase "kinfo":
\t\t\t_, err = unix.SysctlKinfoProc("kern.proc.pid", pid)
\t\t}
\t\tfmt.Printf("PROBE %d %v\\n", index, err)
\t}
}
'''
TEST = 'package main\n\nimport "testing"\n\nfunc TestRuns(t *testing.T) { t.Log("graded") }\n'
DENIED = 'operation not permitted'
SANDBOXED = sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and shutil.which('go')


def results(stdout: bytes) -> list:
    return [line.split(' ', 2)[2] for line in stdout.decode().splitlines() if line.startswith('PROBE ')]


@unittest.skipUnless(SANDBOXED, 'requires macOS sandbox-exec and the Go toolchain')
class ProcessIsolationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        directory = tempfile.TemporaryDirectory()
        cls.addClassCleanup(directory.cleanup)
        cls.base = Path(directory.name).resolve()
        cls.addClassCleanup(prepare_grader.remove_tree, cls.base)
        source = cls.base / 'probe-src'
        source.mkdir()
        (source / 'go.mod').write_text('module probe\n\ngo 1.25.0\n\nrequire golang.org/x/sys v0.45.0\n')
        sums = [line for line in (gs.CHECKOUT / 'go.sum').read_text().splitlines() if line.startswith('golang.org/x/sys v0.45.0')]
        (source / 'go.sum').write_text('\n'.join(sums) + '\n')
        (source / 'main.go').write_text(PROBE)
        (source / 'probe_test.go').write_text(TEST)
        cls.probe = cls.base / 'bin' / 'probe'
        subprocess.run(['go', 'build', '-o', str(cls.probe), '.'], cwd=source, check=True, capture_output=True,
                       env={**os.environ, 'CGO_ENABLED': '0', 'GOFLAGS': '-mod=mod', 'GOPROXY': 'off'})
        cls.canary = 'HARNEVAL_PROBE_CANARY_' + secrets.token_hex(8)
        cls.holder = subprocess.Popen([str(cls.probe), 'hold=600'], env={'PATH': '/usr/bin:/bin', cls.canary: '1'},
                                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        cls.addClassCleanup(cls.holder.wait)
        cls.addClassCleanup(cls.holder.kill)
        cls.params = gs.guard(cls.base / 'set')
        (cls.base / 'set').mkdir()
        cls.prepared = prepare_grader.prepare(source, cls.base / 'grader', ['./...'])
        cls.source = source

    def ops(self) -> list:
        pid = str(self.holder.pid)
        return ['environ=' + pid, 'signal=' + pid, 'kinfo=' + pid, 'self=']

    def run_stage(self, name: str, argv: list, cwd: Path) -> list:
        logs = self.base / 'logs' / name
        logs.mkdir(parents=True)
        run = gs.run_stage(argv, cwd, logs / 'out', logs / 'err', 60)
        self.assertEqual((run['exit_code'], run['leftover']), (0, False), (logs / 'err').read_text())
        return results((logs / 'out').read_bytes())

    def test_the_probe_reads_the_canary_outside_every_profile(self):
        done = subprocess.run([str(self.probe), *self.ops()], capture_output=True, timeout=60,
                              env={'PATH': '/usr/bin:/bin', 'PROBE_CANARY_NAME': self.canary})
        self.assertEqual(results(done.stdout), ['canary read', '<nil>', '<nil>', '<nil>'])

    def assert_isolated(self, got: list):
        self.assertEqual(len(got), 4, got)
        for result in got[:3]:
            self.assertIn(DENIED, result)
        self.assertEqual(got[3], '<nil>', 'a process still signals itself')

    def test_artifact_run_mode_cannot_read_signal_or_list_a_sibling(self):
        run_root = self.base / 'run'
        for sub in ('out', 'home', 'tmp'):
            (run_root / sub).mkdir(parents=True)
        (self.base / 'input').mkdir()
        argv = gs.run_argv(self.probe, self.base / 'input', run_root, [self.probe, *self.ops()], self.params)
        argv.insert(2, 'PROBE_CANARY_NAME=' + self.canary)
        self.assert_isolated(self.run_stage('artifact', argv, run_root / 'out'))

    def test_oracle_profile_cannot_read_signal_or_list_a_sibling(self):
        (self.base / 'outputs').mkdir()
        (self.base / 'oracle').mkdir()
        argv = gs.oracle_argv(self.probe, self.base / 'outputs', self.base / 'oracle', 'GT-AGENT-X01', self.params)
        argv[argv.index(str(self.probe), argv.index('-f')):] = [str(self.probe), *self.ops()]
        argv.insert(2, 'PROBE_CANARY_NAME=' + self.canary)
        self.assert_isolated(self.run_stage('oracle', argv, self.base / 'oracle'))

    def test_grader_profile_isolates_and_go_test_still_grades(self):
        root = prepare_grader.new_grade(self.base / 'grade', self.source, Path(self.prepared['warm_cache']))
        shutil.copy2(self.probe, root / 'probe')
        env = {**grader.allowlist(root, Path(self.prepared['go']), Path(self.prepared['modcache'])),
               'PWD': str(root / 'ws'), 'PROBE_CANARY_NAME': self.canary}
        argv = grader.sandbox_argv(root, Path(self.prepared['modcache']), Path(self.prepared['goroot']), env,
                                   [str(root / 'probe'), *self.ops()])
        self.assert_isolated(self.run_stage('grader-probe', argv, root / 'ws'))
        logs = self.base / 'logs' / 'grade'
        logs.mkdir(parents=True)
        graded = grader.grade(root, ['go', 'test', '-count=1', './...'], ['TestRuns'], self.prepared, logs / 'out',
                              logs / 'err', 120)
        self.assertEqual((graded['exit_code'], graded['signal'], graded['oracle']['expected_passed']), (0, 'accepted', 1),
                         (logs / 'err').read_text())


if __name__ == '__main__':
    unittest.main()
