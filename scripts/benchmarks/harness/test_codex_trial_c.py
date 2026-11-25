"""Pilot C codex on the trial and run level: trial_c.run and run_plan exit paths, rotation and which
arms get codex in trial_c.prepare; stubbed stages, synthetic logins, no network."""
import contextlib
import io
import json
import signal
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import codex_c
import run_c
import trial_c
from test_codex_c import auth, codex_ctx, private


def stub_trial(ctx, agent_action, arm='full', fail=None, plan=False, at_grade=None):
    """trial_c.run with every stage stubbed; the agent acts only below its own trial."""
    out = ctx['output']
    trial, measure, _ = trial_c.names(ctx, 'c01-%s-k1' % arm)

    def fake_prepare(ctx_, task, arm_, name):
        trial.mkdir(parents=True)
        measure.mkdir(parents=True)
        (trial / 'workspace').mkdir()
        if codex_c.arm_uses(ctx_, arm_):
            codex_c.install_home(trial, ctx_['codex'])
        return {'trial': trial, 'measure': measure, 'work': trial / 'workspace', 'env': {}, 'argv': ['x'],
                'warm': ['x'], 'prompt': 'p', 'source': {'tree': trial, 'warm_cache': trial, 'harness_revision': 'h',
                                                         'generated': {'surface_hash': 's'}}, 'modcache_clone': {}}
    calls = []

    def fake_execute(argv, cwd, env, stdout, stderr, seconds, stdin=None, scratch=None):
        calls.append(1)
        if len(calls) == 2:
            agent_action(trial, Path(stdout))
        return {'exit_code': 0, 'timed_out': False, 'stragglers': 0, 'leftover': False, 'elapsed_seconds': 1.0}

    def fake_grade(*_args, **_kwargs):
        if at_grade:
            at_grade(trial)
        if fail:
            raise fail
        return {'review_passed': 0, 'review_total': 0}
    ctx.update(sandbox=True, frozen={'grader_profile': None, 'oracles': None}, modcache=None, timeout=1)
    with mock.patch.object(trial_c, 'prepare', fake_prepare), mock.patch.object(trial_c, 'execute_tree', fake_execute), \
            mock.patch.object(trial_c, 'hashes', lambda p: {}), mock.patch.object(trial_c.oracle_c, 'audit_globs',
                                                                                   lambda *a: {}), \
            mock.patch.object(trial_c.guard_c, 'loadavg', lambda: 0), \
            mock.patch.object(trial_c.grade_c, 'grade_candidate', fake_grade), \
            mock.patch.object(trial_c.report_c, 'review_beyond_base', lambda *a: {}):
        task = {'id': 'c01', 'allowed_paths': ['pkg/']}
        if plan:
            ctx.setdefault('keep_trials', True)
            with contextlib.redirect_stdout(io.StringIO()) as printed:
                return run_c.run_plan(ctx, [(task, arm, 1)]), printed.getvalue()
        return trial_c.run(ctx, task, arm, 1), trial


def rollout(trial: Path, events: list) -> None:
    day = trial / 'codex-home' / 'sessions' / '2026' / '11' / '25'
    day.mkdir(parents=True, exist_ok=True)
    (day / 'rollout-1.jsonl').write_text(''.join(json.dumps(e) + '\n' for e in events))


class TrialRunTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.root = Path(self.dir.name)
        self.source = private(self.root / 'auth.json', auth())

    def tearDown(self):
        self.dir.cleanup()

    def ctx(self):
        return {'output': self.root / 'out', 'codex': codex_ctx(self.source)}

    def test_a_default_run_ignores_a_codex_home_the_agent_made(self):
        row, _ = stub_trial({'output': self.root / 'out', 'codex': None},
                            lambda trial, _: (trial / 'codex-home' / 'sessions').mkdir(parents=True))
        self.assertIsNone(row['codex'])
        self.assertIsNone(row['codex_auth_rotated'])

    def test_rollout_flags_reach_the_observation_and_the_copy_is_gone(self):
        def agent(trial, events):
            rollout(trial, [{'type': 'response_item', 'payload': {'type': 'function_call', 'name': 'exec',
                                                                  'arguments': json.dumps({'cmd': 'gh pr view 7'})}}])
            use = {'type': 'tool_use', 'id': 'u1', 'name': 'Bash', 'input': {'command': 'codex exec "r" </dev/null'}}
            events.write_text(json.dumps({'type': 'assistant', 'message': {'content': [use]}}) + '\n')
        row, trial = stub_trial(self.ctx(), agent)
        self.assertIn('CODEX gh pr view 7', row['observation']['contamination'])
        self.assertEqual((row['codex']['sessions'], row['observation']['codex_calls']), (1, 1))
        self.assertIs(row['codex_auth_rotated'], False)
        self.assertFalse((trial / 'codex-home' / 'auth.json').exists())

    def test_a_refreshed_copy_is_recorded_and_stops_the_run(self):
        row, trial = stub_trial(self.ctx(), lambda trial, _: private(trial / 'codex-home' / 'auth.json',
                                                                     auth(refresh='r1')))
        self.assertIs(row['codex_auth_rotated'], True)
        self.assertFalse((trial / 'codex-home' / 'auth.json').exists())
        self.assertEqual(self.source.read_bytes(), auth(), 'the given login is never written')
        with contextlib.redirect_stdout(io.StringIO()) as out:
            self.assertTrue(codex_c.stop_for_rotation(row))
        self.assertIn('CODEX_AUTH_ROTATED', out.getvalue())
        self.assertFalse(codex_c.stop_for_rotation(dict(row, codex_auth_rotated=False)))

    def test_the_copy_is_deleted_on_an_exception_and_a_stop_signal(self):
        def raising(failure):
            def agent(trial, _events):
                # A copy outside the home too, so only a sweep on the way out can find it.
                private(trial / 'tmp' / 'stash.json', (trial / 'codex-home' / 'auth.json').read_bytes())
                raise failure
            return agent
        for stage in ('agent', 'grading'):
            for failure in (RuntimeError('blew up'), run_c.Interrupted(signal.SIGTERM)):
                with self.subTest(stage=stage, failure=type(failure).__name__):
                    ctx = {'output': self.root / stage / type(failure).__name__, 'codex': codex_ctx(self.source)}
                    with self.assertRaises(type(failure)):
                        if stage == 'agent':
                            stub_trial(ctx, raising(failure))
                        else:
                            stub_trial(ctx, lambda trial, _: None, fail=failure)
                    self.assertEqual(list((ctx['output']).rglob('auth.json')), [])
                    self.assertEqual(list((ctx['output']).rglob('stash.json')), [])

    def test_a_copy_under_an_allowed_path_is_gone_before_grading(self):
        seen = []

        def agent(trial, _events):
            private(trial / 'workspace' / 'pkg' / 'leak.json', (trial / 'codex-home' / 'auth.json').read_bytes())
        row, trial = stub_trial(self.ctx(), agent, at_grade=lambda t: seen.append(
            (t / 'workspace' / 'pkg' / 'leak.json').exists()))
        self.assertEqual(seen, [False], 'the overlay must never see the copy')
        self.assertEqual(list((self.root / 'out').rglob('leak.json')), [])

    def test_a_runner_exception_keeps_the_rotation_and_still_stops(self):
        code, printed = stub_trial(self.ctx(), lambda trial, _: private(trial / 'codex-home' / 'auth.json',
                                                                        auth(refresh='r1')),
                                   fail=RuntimeError('grading blew up'), plan=True)
        self.assertEqual(code, 3)
        self.assertIn('CODEX_AUTH_ROTATED', printed)
        result = json.loads((self.root / 'out' / 'measure' / 'c01-full-k1' / 'result.json').read_text())
        self.assertEqual((result['operational_error'], result['codex_auth_rotated']), ('runner_exception', True))

    def test_an_exception_before_the_check_counts_an_unreadable_copy_as_rotated(self):
        def agent(trial, _events):
            (trial / 'codex-home' / 'auth.json').unlink()
            raise RuntimeError('agent stage blew up')
        ctx = self.ctx()
        with self.assertRaises(RuntimeError):
            stub_trial(ctx, agent)
        self.assertEqual(ctx['codex']['last_rotated'], 'unknown')

    def test_native_never_gets_a_home(self):
        row, trial = stub_trial(self.ctx(), lambda trial, _: None, arm='native')
        self.assertIsNone(row['codex'])
        self.assertFalse((trial / 'codex-home').exists())


class RunPlanTests(unittest.TestCase):
    def test_rotation_stops_the_plan_and_resume_scrubs_a_partial_trial(self):
        with tempfile.TemporaryDirectory() as d:
            out = Path(d) / 'out'
            source = private(Path(d) / 'auth.json', auth())
            ctx = {'output': out, 'keep_trials': True, 'codex': codex_ctx(source)}
            partial = out / 'trials' / 'c01-full-k1'
            partial.mkdir(parents=True)
            codex_c.install_home(partial, dict(ctx['codex']))
            ran = []

            def fake_run(ctx_, task, arm, replicate):
                ran.append(task['id'])
                return {'task_id': task['id'], 'arm': arm, 'replicate': replicate, 'operational_error': None,
                        'oracle': None, 'agent': {}, 'observation': {}, 'codex_auth_rotated': True}
            plan = [({'id': 'c01'}, 'full', 1), ({'id': 'c02'}, 'full', 1)]
            with mock.patch.object(trial_c, 'run', fake_run), contextlib.redirect_stdout(io.StringIO()) as printed:
                self.assertEqual(run_c.run_plan(ctx, plan), 3)
            self.assertEqual(ran, ['c01'], 'the run stops after the trial whose login refreshed')
            self.assertIn('CODEX_AUTH_ROTATED', printed.getvalue())
            self.assertEqual(list(out.rglob('auth.json')), [], 'the set-aside trial holds no login copy')

    def test_every_way_out_of_run_plan_leaves_no_copy(self):
        for failure in (None, 'exception', 'sigterm'):
            for keep in (False, True):
                with self.subTest(failure=failure, keep=keep), tempfile.TemporaryDirectory() as d:
                    out = Path(d) / 'out'
                    source = private(Path(d) / 'auth.json', auth())
                    ctx = {'output': out, 'keep_trials': keep, 'codex': codex_ctx(source)}

                    def fake_run(ctx_, task, arm, replicate):
                        trial, measure, _ = trial_c.names(ctx_, '%s-%s-k%d' % (task['id'], arm, replicate))
                        trial.mkdir(parents=True)
                        measure.mkdir(parents=True)
                        codex_c.install_home(trial, ctx_['codex'])
                        if failure == 'sigterm':
                            raise run_c.Interrupted(signal.SIGTERM)
                        if failure == 'exception':
                            raise RuntimeError('grading blew up')
                        return {'task_id': task['id'], 'arm': arm, 'replicate': replicate, 'oracle': None,
                                'operational_error': None, 'agent': {}, 'observation': {}}
                    with mock.patch.object(trial_c, 'run', fake_run), contextlib.redirect_stdout(io.StringIO()):
                        run_c.run_plan(ctx, [({'id': 'c01'}, 'full', 1)])
                    self.assertEqual(list(out.rglob('auth.json')), [])


class PrepareTests(unittest.TestCase):
    """trial_c.prepare with a ctx fixture: only codex arms get the PATH entry, the home and the slot."""

    def test_native_has_no_codex_path_home_or_package_dir(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            out, checkout = root / 'out', root / 'checkout'
            for path in (out / 'bin' / 'tools', out / 'bin' / 'tools-codex', checkout, root / 'pkg' / 'codex' / 'bin',
                         root / 'go' / 'bin'):
                path.mkdir(parents=True)
            source = private(root / 'auth.json', auth())
            codex = dict(codex_ctx(source), tools_dir=out / 'bin' / 'tools-codex',
                         tools={'codex': str(root / 'pkg' / 'codex' / 'bin' / 'codex.js')})
            ctx = {'output': out, 'sources': {'b': {'tree': root, 'surface': root, 'bin_dir': out / 'bin' / 'h',
                                                    'warm_cache': root}},
                   'modcache': root / 'mod', 'base_env': {}, 'credential_kind': 'oauth', 'credential': 'X',
                   'tools_dir': out / 'bin' / 'tools', 'shell': out / 'bin' / 'shell' / 'zsh',
                   'tools': {'go': str(root / 'go' / 'bin' / 'go')}, 'codex': codex, 'model': 'opus',
                   'effort': 'high', 'permission_mode': 'bypassPermissions', 'claude': 'claude', 'sandbox': True,
                   'account_home': str(root / 'home'), 'protected': [checkout],
                   'frozen': {'agent_profile': root / 'p.sb'}}
            task = {'id': 'c01', 'heads': {'base': 'b'}, 'prompt': 'p', 'allowed_paths': ['pkg/']}
            with mock.patch.object(trial_c.grade_c, 'clone_tree', lambda s, dest: Path(dest).mkdir(parents=True)), \
                    mock.patch.object(trial_c.surface_c, 'install', lambda *a: None), \
                    mock.patch.object(trial_c, 'initialize', lambda *a: None), \
                    mock.patch.object(trial_c.trial_env_c, 'clone_modcache', lambda *a: {}), \
                    mock.patch.object(trial_c.oracle_c, 'present_packages', lambda *a: []):
                seen = {arm: trial_c.prepare(ctx, task, arm, 'c01-%s-k1' % arm) for arm in ('native', 'full')}
            for arm, plan in seen.items():
                uses = arm == 'full'
                self.assertEqual(str(out / 'bin' / 'tools-codex') in plan['env']['PATH'], uses, arm)
                self.assertEqual('CODEX_HOME' in plan['env'], uses, arm)
                self.assertEqual((plan['trial'] / 'codex-home').exists(), uses, arm)
                self.assertEqual(any(a.endswith('=' + str(root / 'pkg' / 'codex')) for a in plan['argv']), uses, arm)


if __name__ == '__main__':
    unittest.main()
