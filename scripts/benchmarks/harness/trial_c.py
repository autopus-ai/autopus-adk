"""One pilot C trial: prepare the checkout, run the agent under its deadline, grade with the oracle."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import time

import claude_agent
import oracle_c
import surface_c
from workspace import hashes, initialize

PROMPT_SUFFIX = (
    '\n\nWork only inside this checkout (the current directory). Production edits belong under: {paths}; '
    'you may add new files and tests there. Do not commit, push, or use the network beyond your own model. '
    'Hidden tests will exercise the pinned interface, and the existing tests must keep compiling and passing. '
    'Finish without asking for clarification.')


def prompt_for(task: dict) -> str:
    return task['prompt'] + PROMPT_SUFFIX.format(paths=', '.join(task['allowed_paths']))


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def execute_exact(argv: list, cwd: Path, env: dict, stdout: Path, stderr: Path, seconds: float,
                  stdin: str | None = None) -> dict:
    """Run with exactly env (nothing inherited) in a new process group; on the deadline SIGTERM the
    group, then SIGKILL it after 10 s. The group is emptied after a normal exit too."""
    started = time.monotonic()
    with open(stdout, 'wb') as out, open(stderr, 'wb') as err:
        process = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.PIPE if stdin is not None else
                                   subprocess.DEVNULL, stdout=out, stderr=err, start_new_session=True)
        timed_out = False
        try:
            process.communicate(stdin.encode() if stdin is not None else None, timeout=seconds)
        except subprocess.TimeoutExpired:
            timed_out = True
            _signal_group(process.pid, signal.SIGTERM)
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                pass
        _signal_group(process.pid, signal.SIGKILL)
        process.wait()
    return {'exit_code': process.returncode, 'timed_out': timed_out,
            'elapsed_seconds': round(time.monotonic() - started, 3)}


def _signal_group(group: int, sig: int) -> None:
    try:
        os.killpg(group, sig)
    except (ProcessLookupError, PermissionError):
        pass


def go_env(trial: Path, go_cache: Path) -> dict:
    """Go settings of a trial. GOPATH and GOMODCACHE are the host's, read once: with a scratch HOME
    their defaults would point at an empty module cache, and GOPROXY is off."""
    temp = trial / 'tmp'
    temp.mkdir(exist_ok=True)
    gopath, modcache = subprocess.check_output(['go', 'env', 'GOPATH', 'GOMODCACHE'], text=True).split('\n')[:2]
    return {'GOCACHE': str(go_cache), 'GOTMPDIR': str(temp), 'TMPDIR': str(temp), 'GOFLAGS': '-mod=mod',
            'GOPROXY': 'off', 'GOSUMDB': 'off', 'GOTELEMETRY': 'off', 'GOTOOLCHAIN': 'local',
            'GOPATH': gopath, 'GOMODCACHE': modcache}


def prepare(ctx: dict, task: dict, arm: str, trial: Path) -> dict:
    """Workspace, settings, isolated home and the sandboxed argv of one trial (no model call)."""
    source = ctx['sources'][task['heads']['base']]
    work = trial / 'workspace'
    shutil.copytree(source['tree'], work, symlinks=True)
    if arm != 'native':
        surface_c.install(work, source['surface'])
    initialize(work)
    settings = trial / 'settings.json'
    settings.write_text(json.dumps(claude_agent.arm_settings(arm), indent=2))
    home = claude_agent.isolated_home(trial)
    env = claude_agent.agent_env(ctx['base_env'], home, ctx['credential_kind'], ctx['credential'],
                                 [source['bin_dir'], source['tools_dir']])
    env.update(go_env(trial, ctx['go_cache']))
    argv = claude_agent.claude_argv(settings, ctx['model'], ctx['effort'], ctx['permission_mode'], ctx['claude'])
    warm = ['go', 'test', '-count=1', '-run', '^$', './' + task['oracle']['package']]
    if ctx['sandbox']:
        wrap = lambda command: claude_agent.sandboxed(command, trial, ctx['output'], ctx['go_cache'],
                                                      ctx['protected'], ctx['account_home'])
        argv, warm = wrap(argv), wrap(warm)
    return {'work': work, 'env': env, 'argv': argv, 'warm': warm, 'prompt': prompt_for(task),
            'settings': claude_agent.arm_settings(arm)}


def grade(ctx: dict, task: dict, trial: Path, work: Path) -> dict:
    """Grade a copy of the candidate with the base tests restored and the hidden oracle added."""
    source = ctx['sources'][task['heads']['base']]
    tree = trial / 'grade' / 'tree'
    shutil.copytree(work, tree, symlinks=True, ignore=shutil.ignore_patterns('.git'))
    reset = oracle_c.restore_tests(tree, source['tree'])
    oracle_c.install_oracle(tree, task)
    env = oracle_c.oracle_env(trial / 'grade', ctx['go_cache'])
    result = oracle_c.run_oracle(tree, task, trial / 'grade' / 'logs', env)
    shutil.rmtree(tree)
    kinds = {case['name']: case['kind'] for case in task['oracle']['cases']}
    passed = [name for name, verdict in result['cases'].items() if verdict == 'pass']
    return {**result, 'tests_reset': reset, 'passed': len(passed), 'total': len(kinds),
            'review_passed': sum(1 for name in passed if kinds[name] == 'review'),
            'review_total': sum(1 for kind in kinds.values() if kind == 'review')}


def run(ctx: dict, task: dict, arm: str, replicate: int) -> dict:
    trial = ctx['output'] / 'trials' / ('%s-%s-k%d' % (task['id'], arm, replicate))
    trial.mkdir(parents=True)
    plan = prepare(ctx, task, arm, trial)
    source = ctx['sources'][task['heads']['base']]
    warm = execute_exact(plan['warm'], plan['work'], plan['env'], trial / 'warm.log', trial / 'warm.err', 1800)
    before = hashes(plan['work'])
    agent = execute_exact(plan['argv'], plan['work'], plan['env'], trial / 'events.jsonl', trial / 'stderr.log',
                          ctx['timeout'], plan['prompt'])
    observation = claude_agent.parse_stream(trial / 'events.jsonl')
    scope = oracle_c.audit_globs(before, hashes(plan['work']), task['allowed_paths'])
    graded = grade(ctx, task, trial, plan['work'])
    operational = ('warmup_failed' if warm['exit_code'] else 'agent_timeout' if agent['timed_out'] else
                   'agent_exit' if agent['exit_code'] else 'agent_error' if observation['is_error'] else None)
    row = {'task_id': task['id'], 'arm': arm, 'replicate': replicate, 'operational_error': operational,
           'warm': warm, 'agent': agent, 'observation': observation, 'scope': scope, 'oracle': graded,
           'prompt_hash': digest(plan['prompt'].encode()), 'human_interventions': 0,
           'harness_revision': source['harness_revision'],
           'surface_hash': source['generated']['surface_hash'] if arm != 'native' else None}
    (trial / 'result.json').write_text(json.dumps(row, indent=2))
    return row
