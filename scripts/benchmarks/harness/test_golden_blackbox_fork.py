"""artifact.sb run mode and oracle.sb deny fork, so a re-exec+setsid chain leaves nothing (S2).

SPEC-HARNEVAL-003 S2: a stage judged by output alone never needs to start another process, yet a Go
artifact that forks, calls setsid and re-execs itself every generation (the parent exiting at once,
the tail sleeping) left a sandboxed survivor that a single leftover walk could miss. The run and
oracle profiles now deny fork; this builds that artifact and runs it under each profile, asserting
the fork is refused and no generation outlives the stage. macOS only; no credential file is read.
"""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

import golden_sandbox as gs
import grader

SANDBOXED = sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and bool(shutil.which('go'))

FORKER = '''package main

import (
\t"fmt"
\t"os"
\t"os/exec"
\t"strconv"
\t"syscall"
\t"time"
)

func main() {
\tdepth := 0
\tif len(os.Args) > 1 {
\t\tdepth, _ = strconv.Atoi(os.Args[1])
\t}
\tos.WriteFile(fmt.Sprintf("gen-%d.pid", depth), []byte(strconv.Itoa(os.Getpid())), 0o644)
\tif depth >= 2 {
\t\ttime.Sleep(120 * time.Second)
\t\treturn
\t}
\tself, _ := os.Executable()
\tcmd := exec.Command(self, strconv.Itoa(depth+1))
\tcmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
\tif err := cmd.Start(); err != nil {
\t\tfmt.Println("FORK_DENIED")
\t\treturn
\t}
\tfmt.Println("SPAWNED", cmd.Process.Pid)
}
'''


def alive(pid: int) -> bool:
    state = subprocess.run(['/bin/ps', '-o', 'stat=', '-p', str(pid)], capture_output=True, text=True).stdout.strip()
    return bool(state) and not state.startswith('Z')


@unittest.skipUnless(SANDBOXED, 'requires macOS sandbox-exec and the Go toolchain')
class ForkDenialTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        directory = tempfile.TemporaryDirectory()
        cls.addClassCleanup(directory.cleanup)
        cls.base = Path(directory.name).resolve()
        source = cls.base / 'forker-src'
        source.mkdir()
        (source / 'go.mod').write_text('module forker\n\ngo 1.21\n')
        (source / 'main.go').write_text(FORKER)
        cls.forker = cls.base / 'bin' / 'forker'
        subprocess.run(['go', 'build', '-o', str(cls.forker), '.'], cwd=source, check=True, capture_output=True,
                       env={**os.environ, 'CGO_ENABLED': '0'})
        (cls.base / 'set').mkdir()
        cls.params = gs.guard(cls.base / 'set')

    def stage_dirs(self, name: str) -> tuple:
        work = self.base / name
        for sub in ('input', 'run/out', 'run/home', 'run/tmp', 'logs', 'oracle'):
            (work / sub).mkdir(parents=True)
        return work, work / 'input', work / 'run'

    def assert_fork_denied(self, run: dict, stdout_path: Path, pid_dir: Path) -> None:
        stdout = Path(stdout_path).read_bytes().decode()
        self.assertIn('FORK_DENIED', stdout, stdout)
        self.assertNotIn('SPAWNED', stdout)
        self.assertFalse(run['leftover'])
        for pidfile in sorted(Path(pid_dir).glob('gen-*.pid')):
            pid = int(pidfile.read_text())
            self.addCleanup(lambda p=pid: subprocess.run(['/bin/kill', '-9', str(p)], capture_output=True))
            self.assertFalse(alive(pid), pidfile.name + ' outlived the stage: fork was not denied')

    def test_run_mode_denies_fork(self):
        for index in range(5):
            work, inputs, run_root = self.stage_dirs('run-%d' % index)
            argv = gs.run_argv(self.forker, inputs, run_root, [str(self.forker)], self.params)
            run = gs.run_stage(argv, run_root / 'out', work / 'logs/out', work / 'logs/err', 30, confined=run_root)
            self.assert_fork_denied(run, work / 'logs/out', run_root / 'out')

    def test_oracle_sb_denies_fork(self):
        for index in range(5):
            work, _, run_root = self.stage_dirs('oracle-%d' % index)
            result_dir = work / 'oracle'
            argv = gs.oracle_argv(self.forker, run_root / 'out', result_dir, 'GT-AGENT-X01', self.params)
            argv[argv.index(str(self.forker), argv.index('-f')):] = [str(self.forker)]
            run = gs.run_stage(argv, result_dir, work / 'logs/out', work / 'logs/err', 30, confined=result_dir)
            self.assert_fork_denied(run, work / 'logs/out', result_dir)


if __name__ == '__main__':
    unittest.main()
