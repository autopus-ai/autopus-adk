"""A stop signal to the pilot C runner sweeps the running trial's whole tree and leaves it resumable; a
signal the runner inherited as ignored (SIGHUP under nohup) does not stop it."""
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest

import run_c
import trial_c

HERE = Path(__file__).resolve().parent

# The runner side: run_plan with trial_c.run replaced by one execute_tree stage whose agent, like
# `claude -p`, starts in its own session and leaves a setsid grandchild behind.
RUNNER = r'''
import json, sys
from pathlib import Path
sys.path.insert(0, sys.argv[1])
import run_c, trial_c
out = Path(sys.argv[2])
agent = [sys.executable, '-c', (
    'import os, subprocess, sys, time\n'
    'child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(120)"], start_new_session=True)\n'
    'open(sys.argv[1], "w").write("%d %d" % (os.getpid(), child.pid))\n'
    'time.sleep(120)\n'), str(out / 'pids')]

def fake_run(ctx, task, arm, replicate):
    trial, measure, _ = trial_c.names(ctx, '%s-%s-k%d' % (task['id'], arm, replicate))
    trial.mkdir(parents=True)
    measure.mkdir(parents=True)
    trial_c.execute_tree(agent, trial, {'PATH': '/usr/bin:/bin'}, measure / 'events.jsonl', measure / 'err', 600)
    raise AssertionError('the stage outlived the signal')

trial_c.run = fake_run
run_c.install_stop_handlers()
sys.exit(run_c.run_plan({'output': out}, [({'id': 'c01'}, 'native', 1)]))
'''


def alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    return True


class StopSignalTests(unittest.TestCase):
    def stop_with(self, signum: int, ignored: tuple = ()) -> None:
        """Start the runner (under /usr/bin/nohup when ignored names SIGHUP), send each ignored signal and
        check the run goes on, then send signum and check the tree is gone and the trial resumable."""
        with tempfile.TemporaryDirectory() as d:
            out = Path(d)
            launcher = ['/usr/bin/nohup'] if signal.SIGHUP in ignored else []
            runner = subprocess.Popen([*launcher, sys.executable, '-c', RUNNER, str(HERE), str(out)],
                                      start_new_session=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                                      env={**os.environ, 'PYTHONDONTWRITEBYTECODE': '1'})
            deadline = time.monotonic() + 30
            while not (out / 'pids').is_file() or len((out / 'pids').read_text().split()) < 2:
                self.assertLess(time.monotonic(), deadline, 'the agent never started')
                time.sleep(0.05)
            agent, grandchild = map(int, (out / 'pids').read_text().split())
            time.sleep(0.3)  # let the tree walk see the grandchild's own session
            for quiet in ignored:
                # Closing the terminal of a nohup run: the run and its trial go on.
                os.killpg(runner.pid, quiet)
                time.sleep(1.0)
                self.assertIsNone(runner.poll(), '%s stopped a runner that inherited it as ignored' % quiet)
                self.assertTrue(alive(agent) and alive(grandchild))
            # What the maintainer ran: the runner's process group only; the agent is in another session.
            os.killpg(runner.pid, signum)
            _, stderr = runner.communicate(timeout=60)
            self.assertEqual(runner.returncode, 128 + signum, stderr)
            for pid in (agent, grandchild):
                for _ in range(40):
                    if not alive(pid):
                        break
                    time.sleep(0.05)
                self.assertFalse(alive(pid), 'pid %d survived the runner' % pid)
            measure = out / 'measure' / 'c01-native-k1'
            self.assertFalse((measure / 'result.json').exists())
            self.assertEqual(json.loads((measure / 'aborted.json').read_text())['signal'],
                             signal.Signals(signum).name)

    def test_sigterm_to_the_runner_group_leaves_no_descendant(self):
        self.stop_with(signal.SIGTERM)

    def test_sigint_and_sighup_outside_nohup_do_the_same(self):
        self.stop_with(signal.SIGINT)
        self.stop_with(signal.SIGHUP)

    def test_under_nohup_sighup_is_ignored_and_sigterm_still_stops(self):
        self.stop_with(signal.SIGTERM, ignored=(signal.SIGHUP,))

    def test_an_inherited_ignore_gets_no_handler(self):
        saved = {stop: signal.getsignal(stop) for stop in run_c.STOP_SIGNALS}
        signal.signal(signal.SIGHUP, signal.SIG_IGN)
        try:
            installed = run_c.install_stop_handlers()
            self.assertEqual(installed, [s for s in run_c.STOP_SIGNALS if s != signal.SIGHUP])
            self.assertIs(signal.getsignal(signal.SIGHUP), signal.SIG_IGN)
        finally:
            for stop, disposition in saved.items():
                signal.signal(stop, disposition)


class ResultWriteTests(unittest.TestCase):
    def test_result_is_replaced_whole_and_no_temporary_stays(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'result.json'
            path.write_text('{"old": tru')  # a torn earlier write
            trial_c.write_json(path, {'new': True})
            self.assertEqual(json.loads(path.read_text()), {'new': True})
            self.assertEqual(sorted(p.name for p in Path(d).iterdir()), ['result.json'])


if __name__ == '__main__':
    unittest.main()
