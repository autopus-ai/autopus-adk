"""Agent step confinement, refusals before the first agent call, and the run.py golden dispatch."""
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

import golden
import golden_agent as ga
import golden_trial as gt
from test_golden_fixture import argv, build_world, calls

HERE = Path(__file__).resolve().parent
AGENT = '''#!{python}
import json, os, subprocess, sys, time
from pathlib import Path
out = Path({out!r})
(out / 'stdin.txt').write_text(sys.stdin.read())
(out / 'env.json').write_text(json.dumps(dict(os.environ)))
if sys.argv[1:] == ['sleep']:
    time.sleep(600)
children = [subprocess.Popen(['/bin/sleep', '300'], process_group=0), subprocess.Popen(['/bin/sleep', '301'])]
(out / 'children').write_text(' '.join(str(child.pid) for child in children))
print('{{"type":"turn.completed","usage":{{"input_tokens":1,"output_tokens":1}}}}')
'''


def gone(pid: int, seconds: float = 5.0) -> bool:
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        state = subprocess.run(['/bin/ps', '-o', 'stat=', '-p', str(pid)], capture_output=True, text=True).stdout
        if not state.strip() or state.strip().startswith('Z'):
            return True
        time.sleep(0.05)
    return False


class AgentRunnerTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()
        self.agent = self.base / 'agent'
        self.agent.write_text(AGENT.format(python=sys.executable, out=str(self.base)))
        self.agent.chmod(0o755)

    def run_agent(self, args, timeout=30):
        env = {'PATH': '/usr/bin:/bin', 'HOME': str(self.base), 'HARNEVAL_AGENT_CREDENTIAL': 'only-here'}
        with mock.patch.dict(os.environ, {'HARNEVAL_CANARY': 'leak'}):
            return ga.run_agent([str(self.agent), *args], self.base, env, 'the prompt', self.base / 'events.jsonl',
                                self.base / 'stderr.log', timeout), env

    def test_agent_gets_exactly_its_environment_and_its_whole_session_is_emptied(self):
        result, env = self.run_agent([])
        self.assertEqual((result['launched'], result['exit_code'], result['timed_out']), (True, 0, False))
        self.assertEqual((result['stragglers'], result['leftover']), (True, False))
        self.assertEqual((self.base / 'stdin.txt').read_text(), 'the prompt')
        recorded = json.loads((self.base / 'env.json').read_text())
        self.assertEqual({key: recorded[key] for key in env}, env)
        self.assertNotIn('HARNEVAL_CANARY', recorded)
        self.assertLessEqual(set(recorded) - set(env), {'__CF_USER_TEXT_ENCODING', 'LC_CTYPE'})
        for pid in map(int, (self.base / 'children').read_text().split()):
            self.assertTrue(gone(pid), 'background process %d survived the trial' % pid)
        self.assertGreater(result['transcript_bytes'], 0)

    def test_timeout_ends_the_agent_and_an_unstartable_agent_never_launches(self):
        result, _ = self.run_agent(['sleep'], timeout=1)
        self.assertTrue(result['timed_out'])
        self.assertFalse(result['leftover'])
        self.assertEqual(gt.agent_signal(result, None), 'agent_timeout')
        missing = ga.run_agent([str(self.base / 'missing')], self.base, {}, '', self.base / 'e', self.base / 's', 5)
        self.assertEqual((missing['launched'], gt.agent_signal(missing, None)), (False, 'agent_launch_failed'))

    def test_an_interrupted_runner_ends_the_agent_before_the_interrupt_propagates(self):
        real, started = ga._exited, []

        def interrupted(pid, seconds):
            if not started:
                started.append(pid)
                raise KeyboardInterrupt
            return real(pid, seconds)
        with mock.patch.object(ga, '_exited', side_effect=interrupted), self.assertRaises(KeyboardInterrupt):
            self.run_agent(['sleep'])
        self.assertTrue(gone(started[0]), 'the agent outlived the interrupted runner')

    def test_termination_signals_reach_the_runner_as_an_interrupt(self):
        for signum in (signal.SIGTERM, signal.SIGHUP):
            self.addCleanup(signal.signal, signum, signal.getsignal(signum))
        golden.interrupt_on_termination()
        for signum in (signal.SIGTERM, signal.SIGHUP):
            with self.subTest(signum=signum), self.assertRaises(KeyboardInterrupt):
                os.kill(os.getpid(), signum)
                time.sleep(5)


def fake_digests(auto, set_root):
    """The Go digest document shape for a fixture set, without building auto."""
    tasks = [json.loads(path.read_text()) for path in sorted(Path(set_root, 'evals/harness/tasks/agent').glob('*.json'))]
    return {'agent_set_digest': 'a' * 64,
            'tasks': [{'id': task['id'], 'kind': task['kind'], 'state': task['status']['state']} for task in tasks]}


class RefusalTests(unittest.TestCase):
    """Every refusal happens before the first agent call and writes no protocol."""

    def refuse(self, world, steps=None, *extra):
        steps = steps or golden.Steps(platform='darwin', set_digests=fake_digests)
        with self.assertRaises(golden.Refusal) as caught:
            golden.run_session(golden.parse(argv(world, '--auto', 'unused', *extra)), steps)
        self.assertEqual(calls(world), [])
        return caught.exception

    def world(self, **options):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        return build_world(Path(directory.name), ['alpha', 'beta'], **options)

    def test_preflight_refusals(self):
        world = self.world()
        self.assertEqual(self.refuse(world, golden.Steps(platform='linux', set_digests=fake_digests)).reason,
                         'unsupported_os')
        world.session.mkdir()
        (world.session / 'protocol.json').write_text('{"frozen": true}')
        self.assertEqual(self.refuse(world).reason, 'protocol_exists')
        self.assertEqual((world.session / 'protocol.json').read_text(), '{"frozen": true}')
        refused = self.refuse(self.world(max_runs=6))
        self.assertEqual((refused.reason, refused.detail), ('run_cap_exceeded',
                                                            '2 tasks x k=2 x 2 arms = 8 > max_agent_runs 6'))
        self.assertEqual(self.refuse(self.world(version='codex-cli 0.159.0')).reason, 'codex_cli_version_mismatch')
        broken = self.world()
        (broken.surfaces / 'candidate' / 'AGENTS.md').unlink()
        (broken.surfaces / 'candidate' / 'AGENTS.md').symlink_to('/etc/hosts')
        self.assertEqual(self.refuse(broken).reason, 'invalid')

    def test_workspace_mutation_mismatch_leaves_an_empty_session(self):
        world = self.world()
        (world.repo / 'beta' / 'beta.go').write_text('package beta\n\nfunc Value(x int) int { return x + 1 + 0 }\n'
                                                     '// return x + 1\n')
        subprocess.run(['git', '-c', 'user.name=f', '-c', 'user.email=f@l', '-c', 'commit.gpgsign=false', 'commit',
                        '-qam', 'drift'], cwd=world.repo, check=True)
        manifest = json.loads((world.root / 'evals/harness/manifest.json').read_text())
        manifest['live']['workspace_revision'] = subprocess.run(['git', 'rev-parse', 'HEAD'], cwd=world.repo,
                                                                capture_output=True, text=True).stdout.strip()
        (world.root / 'evals/harness/manifest.json').write_text(json.dumps(manifest))
        refused = self.refuse(world)
        self.assertEqual((refused.reason, refused.detail), ('workspace_mutation_mismatch', 'GT-AGENT-X02'))
        self.assertEqual(os.listdir(world.session), [])


class RunModeTests(unittest.TestCase):
    def run_py(self, *args):
        return subprocess.run([sys.executable, str(HERE / 'run.py'), *args], capture_output=True, text=True,
                              env={**os.environ, 'PYTHONDONTWRITEBYTECODE': '1'}, timeout=120)

    def test_golden_mode_dispatches_and_the_pilot_parser_is_unchanged(self):
        golden_help, pilot_help = self.run_py('--mode', 'golden', '--help'), self.run_py('--help')
        self.assertEqual((golden_help.returncode, pilot_help.returncode), (0, 0))
        self.assertIn('run.py --mode golden', golden_help.stdout)
        self.assertIn('--credential-env', golden_help.stdout)
        self.assertIn('--revision', pilot_help.stdout)
        self.assertNotIn('--credential-env', pilot_help.stdout)

    def test_golden_mode_reports_a_refusal_as_one_json_document(self):
        with tempfile.TemporaryDirectory() as directory:
            world = build_world(Path(directory), ['alpha'])
            auto = world.base / 'auto'
            auto.write_text('#!%s\nimport json\nprint(json.dumps(%r))\n' % (sys.executable, fake_digests(None, world.root)))
            auto.chmod(0o755)
            world.session.mkdir()
            (world.session / 'protocol.json').write_text('{}')
            completed = self.run_py('--mode', 'golden', *argv(world, '--auto', str(auto)))
        reason = 'protocol_exists' if sys.platform == 'darwin' else 'unsupported_os'
        self.assertEqual(completed.returncode, 1, completed.stderr)
        self.assertEqual(json.loads(completed.stdout)['reason'], reason)
        self.assertIn('golden: refused ' + reason, completed.stderr)


if __name__ == '__main__':
    unittest.main()
