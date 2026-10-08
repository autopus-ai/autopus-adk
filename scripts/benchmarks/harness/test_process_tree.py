"""Leftover processes of a stage: setsid descendants and double forks are found and killed (SPEC-HARNEVAL-003 S2).

A stage's leftover check used to look only at the stage's session, so a descendant that called setsid
(directly, or after a double fork reparented it to launchd) outlived the stage unseen. Each case below
starts a real stage, lets it leave such a process, and asserts the runner killed it before returning.
"""
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

import golden_agent as ga
import golden_sandbox as gs
import grader
import process_tree as pt

# The stage root spawns a child that starts a setsid grandchild, records its pid, and lingers long
# enough for the runner's walk to see the tree before every ancestor exits.
SETSID_GRANDCHILD = '''
import os, subprocess, sys, time
if len(sys.argv) > 2:
    child = subprocess.Popen(['/bin/sleep', '300'], start_new_session=True, stdin=subprocess.DEVNULL,
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    open(sys.argv[1], 'w').write(str(child.pid))
    time.sleep(0.6)
else:
    subprocess.run([sys.executable, sys.argv[0], sys.argv[1], 'child'])
'''
# A classic daemon: fork, setsid, fork again, the middle process exits at once, the daemon closes
# every inherited descriptor; the stage root exits as soon as the middle one has.
DOUBLE_FORK = '''
import os, sys, time
path = sys.argv[1]
middle = os.fork()
if middle == 0:
    os.setsid()
    if os.fork() == 0:
        open(path + '.tmp', 'w').write(str(os.getpid()))
        os.rename(path + '.tmp', path)
        for fd in range(3):
            os.close(fd)
        time.sleep(300)
    os._exit(0)
os.waitpid(middle, 0)
'''


def alive(pid: int) -> bool:
    state = subprocess.run(['/bin/ps', '-o', 'stat=', '-p', str(pid)], capture_output=True, text=True).stdout.strip()
    return bool(state) and not state.startswith('Z')


class StageTreeTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()
        (self.base / 'scratch').mkdir()

    def script(self, name: str, text: str) -> Path:
        path = self.base / name
        path.write_text(text)
        return path

    def wait_pid(self, path: Path) -> int:
        deadline = time.monotonic() + 10
        while not path.exists() and time.monotonic() < deadline:
            time.sleep(0.02)
        pid = int(path.read_text())
        self.addCleanup(lambda: subprocess.run(['/bin/kill', '-9', str(pid)], capture_output=True))
        return pid

    def stage(self, argv, confined=None):
        return gs.run_stage(argv, self.base, self.base / 'out', self.base / 'err', 30, confined=confined)

    def test_a_setsid_grandchild_of_a_stage_is_killed(self):
        script = self.script('tree.py', SETSID_GRANDCHILD)
        result = self.stage([sys.executable, str(script), str(self.base / 'pid')])
        pid = self.wait_pid(self.base / 'pid')
        self.assertFalse(alive(pid), 'the setsid grandchild outlived its stage')
        self.assertEqual((result['stragglers'], result['leftover'], result['exit_code']), (True, False, 0))

    @unittest.skipUnless(sys.platform == 'darwin' and os.path.exists(grader.SANDBOX), 'requires macOS sandbox-exec')
    def test_a_double_forked_daemon_under_the_stage_profile_is_killed(self):
        scratch = self.base / 'scratch'
        profile = '(version 1)(allow default)(deny file-write*)(allow file-write* (subpath "%s"))' % scratch
        script = self.script('daemon.py', DOUBLE_FORK)
        argv = [grader.SANDBOX, '-p', profile, sys.executable, str(script), str(scratch / 'pid')]
        result = self.stage(argv, confined=scratch)
        pid = self.wait_pid(scratch / 'pid')
        self.assertFalse(alive(pid), 'the double-forked daemon outlived its stage')
        self.assertEqual((result['stragglers'], result['leftover']), (True, False))

    def test_a_descendant_that_survives_every_kill_is_a_leftover(self):
        script = self.script('tree.py', SETSID_GRANDCHILD)
        with mock.patch.object(pt.os, 'kill'), mock.patch.object(pt.os, 'killpg'):
            result = self.stage([sys.executable, str(script), str(self.base / 'pid')])
        pid = self.wait_pid(self.base / 'pid')
        self.assertTrue(alive(pid))
        self.assertEqual((result['stragglers'], result['leftover']), (True, True))

    def test_the_agent_step_kills_a_setsid_descendant_of_the_agent(self):
        script = self.script('tree.py', SETSID_GRANDCHILD)
        run = ga.run_agent([sys.executable, str(script), str(self.base / 'pid')], self.base, dict(os.environ), '',
                           self.base / 'events', self.base / 'stderr', 30)
        pid = self.wait_pid(self.base / 'pid')
        self.assertFalse(alive(pid), 'the setsid descendant outlived the agent step')
        self.assertEqual((run['stragglers'], run['leftover']), (True, False))


class ReachTests(unittest.TestCase):
    """The fixed point over one process table, as hand-made rows {pid: (ppid, pgid, sid, uid)}."""

    def test_children_groups_and_sessions_of_reached_processes_are_reached(self):
        rows = {10: (1, 10, 10, 0), 11: (10, 10, 10, 0), 12: (11, 12, 12, 0), 13: (1, 12, 12, 0),
                14: (1, 30, 30, 0), 15: (1, 40, 40, 0), 16: (15, 40, 40, 0)}
        tree = pt.Tree(10)
        self.assertEqual(tree.reach(rows), {11, 12, 13})
        # 13 was orphaned and left the group of 12, which is gone: it stays tracked by pid.
        self.assertEqual(tree.reach({10: rows[10], 13: (1, 13, 13, 0), 14: rows[14]}), {13})
        # a dead reached leader still names its session: an orphan in it is reached.
        self.assertEqual(tree.reach({10: rows[10], 20: (1, 12, 12, 0)}), {20})

    def test_the_runner_and_unrelated_processes_are_never_reached(self):
        rows = {10: (1, 10, 10, 0), os.getpid(): (10, 10, 10, 0), 99: (1, 99, 99, 0)}
        self.assertEqual(pt.Tree(10).reach(rows), set())


if __name__ == '__main__':
    unittest.main()
