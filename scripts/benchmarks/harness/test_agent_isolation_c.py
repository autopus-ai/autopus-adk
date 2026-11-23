"""claude_agent.sb keeps other processes closed (SPEC-HARNEVAL-003 S1, as grader.sb does).

A same-user process can read another one's environment through the kern.procargs2 sysctl and its
state through proc_pidinfo/proc_pidpath. The runner's parent is the maintainer's shell, whose
environment holds tokens, and a sibling outside the sandbox may hold a credential. A probe built
here reads a canary from a sibling's and from its parent's environment: outside any profile, and
under the profile without its process rules, it succeeds, so the test is not vacuous; under
claude_agent.sb both are refused while the agent still inspects and signals its own children.
macOS only; needs go with cgo.
"""
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import tempfile
import unittest

import claude_agent

CHECKOUT = Path(__file__).resolve().parents[3]
PROBE = r'''package main

/*
#include <libproc.h>
#include <sys/proc_info.h>
static int bsdinfo(int pid) { struct proc_bsdinfo b; return proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &b, sizeof b); }
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func main() {
	for _, arg := range os.Args[1:] {
		op, target, _ := strings.Cut(arg, "=")
		pid, _ := strconv.Atoi(target)
		if target == "parent" {
			pid = os.Getppid()
		}
		var err error
		switch op {
		case "hold":
			time.Sleep(time.Duration(pid) * time.Second)
			continue
		case "environ":
			var data []byte
			data, err = unix.SysctlRaw("kern.procargs2", pid)
			if err == nil && bytes.Contains(data, []byte(os.Getenv("PROBE_CANARY_NAME")+"=1")) {
				err = errors.New("canary read")
			}
		case "bsdinfo":
			if n, e := C.bsdinfo(C.int(pid)); n <= 0 {
				err = fmt.Errorf("denied (%v)", e)
			}
		case "child":
			cmd := exec.Command(os.Args[0], "hold=30")
			if err = cmd.Start(); err != nil {
				break
			}
			if n, e := C.bsdinfo(C.int(cmd.Process.Pid)); n <= 0 {
				err = fmt.Errorf("own child info: %v", e)
			}
			if e := cmd.Process.Signal(syscall.SIGTERM); e != nil && err == nil {
				err = e
			}
			_ = cmd.Wait()
		}
		fmt.Printf("PROBE %s %v\n", arg, err)
	}
}
'''
RULES = ('(deny sysctl-read (sysctl-name-prefix "kern.procargs"))', '(deny process-info*)',
         '(allow process-info* (target same-sandbox))')
SANDBOXED = sys.platform == 'darwin' and os.path.exists(claude_agent.SANDBOX) and shutil.which('go')


def results(stdout: str) -> dict:
    return dict(line.split(' ', 2)[1:] for line in stdout.splitlines() if line.startswith('PROBE '))


@unittest.skipUnless(SANDBOXED, 'requires macOS sandbox-exec and the Go toolchain')
class AgentProcessIsolationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root = Path(tempfile.mkdtemp()).resolve()
        cls.addClassCleanup(shutil.rmtree, cls.root, True)
        source = cls.root / 'probe-src'
        source.mkdir()
        (source / 'go.mod').write_text('module probe\n\ngo 1.25.0\n\nrequire golang.org/x/sys v0.45.0\n')
        sums = [line for line in (CHECKOUT / 'go.sum').read_text().splitlines()
                if line.startswith('golang.org/x/sys v0.45.0')]
        (source / 'go.sum').write_text('\n'.join(sums) + '\n')
        (source / 'main.go').write_text(PROBE)
        cls.out = cls.root / 'out'
        cls.trial = cls.out / 'trials' / 't1'
        (cls.trial / 'tmp').mkdir(parents=True)
        cls.probe = cls.trial / 'probe'
        subprocess.run(['go', 'build', '-o', str(cls.probe), '.'], cwd=source, check=True, capture_output=True,
                       env={**os.environ, 'CGO_ENABLED': '1', 'GOFLAGS': '-mod=mod', 'GOPROXY': 'off'})
        cls.canary = 'PILOTC_PROBE_CANARY_' + secrets.token_hex(8)
        cls.sibling = subprocess.Popen([str(cls.probe), 'hold=300'], env={'PATH': '/usr/bin:/bin', cls.canary: '1'},
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        cls.addClassCleanup(cls.sibling.wait)
        cls.addClassCleanup(cls.sibling.kill)

    def ops(self) -> list:
        pid = str(self.sibling.pid)
        return ['environ=' + pid, 'environ=parent', 'bsdinfo=' + pid, 'bsdinfo=parent', 'child=']

    def run_probe(self, profile: Path | None) -> dict:
        """The probe's results with a runner parent whose environment holds the canary; the
        probe's own environment does not."""
        argv = [str(self.probe), *self.ops()]
        if profile is not None:
            argv = claude_agent.sandboxed(argv, self.trial, self.out, self.out / 'bin', str(self.trial / 'gomodcache'),
                                          {}, str(Path.home()), [], profile)
        # The runner stands in for run_c's parent chain: it was started with the canary in its own
        # environment and drops it for the child.
        drop = ('import os, subprocess, sys; env = dict(os.environ); env.pop(%r); '
                'sys.exit(subprocess.run(sys.argv[1:], env=env).returncode)' % self.canary)
        runner = [sys.executable, '-I', '-c', drop, *argv]
        env = {'PATH': '/usr/bin:/bin', 'HOME': str(self.trial), 'TMPDIR': str(self.trial / 'tmp'),
               'PROBE_CANARY_NAME': self.canary, self.canary: '1'}
        done = subprocess.run(runner, cwd=self.trial, env=env, capture_output=True, text=True, timeout=120)
        return results(done.stdout)

    def without_process_rules(self) -> Path:
        text = claude_agent.PROFILE.read_text()
        for rule in RULES:
            self.assertIn(rule, text)
            text = text.replace(rule, '')
        variant = self.root / 'variant.sb'
        variant.write_text(text)
        return variant

    def test_the_probe_reads_the_canaries_without_the_process_rules(self):
        pid = str(self.sibling.pid)
        for profile in (None, self.without_process_rules()):
            got = self.run_probe(profile)
            self.assertEqual(got['environ=' + pid], 'canary read', profile)
            self.assertEqual(got['environ=parent'], 'canary read', profile)
            self.assertEqual(got['bsdinfo=' + pid], '<nil>', profile)

    def test_the_agent_cannot_read_other_processes_but_reaches_its_children(self):
        got = self.run_probe(claude_agent.PROFILE)
        pid = str(self.sibling.pid)
        for op in ('environ=' + pid, 'environ=parent'):
            self.assertIn('operation not permitted', got[op], op)
        for op in ('bsdinfo=' + pid, 'bsdinfo=parent'):
            self.assertTrue(got[op].startswith('denied'), (op, got[op]))
        self.assertEqual(got['child='], '<nil>', 'the agent inspects and signals its own children')


if __name__ == '__main__':
    unittest.main()
