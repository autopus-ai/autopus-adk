"""One pilot C trial: prepare the checkout, run the agent under its deadline, grade with the oracle.

Layout below the run output: trials/<name>/ is the only tree the agent may touch (workspace, isolated
home, temp, its own clones of the warm build cache and of the run's module cache); measure/<name>/ holds what the runner records
(transcript, stderr, warm-up logs, result) and grading/<name>/ the sandboxed grade root. Neither of
the latter is readable inside the agent's sandbox.
"""
import hashlib
import json
from pathlib import Path
import signal
import os
import subprocess
import time

import claude_agent
import grade_c
import guard_c
import oracle_c
import process_tree
import report_c
import stream_c
import surface_c
import trial_env_c
from workspace import hashes, initialize

PROMPT_SUFFIX = (
    '\n\nWork only inside this checkout (the current directory). Production edits belong under: {paths}; '
    'you may add new files and tests there. Do not commit, push, or use the network beyond your own model. '
    'Hidden tests will exercise the pinned interface. At grading the existing tests are restored to their '
    'original versions and only production files under the paths above are kept, so the existing tests must '
    'keep compiling and passing against your code. Finish without asking for clarification.')


def prompt_for(task: dict) -> str:
    return task['prompt'] + PROMPT_SUFFIX.format(paths=', '.join(task['allowed_paths']))


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def write_json(path: Path, data) -> None:
    """Write JSON through a temporary file and os.replace: an interrupted write leaves either no file
    or the whole one, never a truncated result.json that --resume's json.loads would trip over."""
    path = Path(path)
    temp = path.with_name('.' + path.name + '.tmp')
    temp.write_text(json.dumps(data, indent=2))
    os.replace(temp, path)


def execute_tree(argv: list, cwd: Path, env: dict, stdout: Path, stderr: Path, seconds: float,
                 stdin: str | None = None, scratch: Path | None = None) -> dict:
    """Run with exactly env in a new session. On the deadline SIGTERM the group and wait 10 s; then
    always sweep the whole descendant tree (process_tree.Tree: setsid shells, orphans and every
    process the sandbox instance writing `scratch` confines) until it is empty. An exception raised
    meanwhile (run_c's stop signals raise one) sweeps the same way before it propagates."""
    started = time.monotonic()
    process, tree, timed_out = None, None, False
    try:
        with open(stdout, 'wb') as out, open(stderr, 'wb') as err:
            process = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.PIPE if stdin is not None else
                                       subprocess.DEVNULL, stdout=out, stderr=err, start_new_session=True)
        tree = process_tree.Tree(process.pid, scratch).watch()
        if stdin is not None:
            try:
                process.stdin.write(stdin.encode())
                process.stdin.close()
            except BrokenPipeError:
                pass
        try:
            process.wait(timeout=seconds)
        except subprocess.TimeoutExpired:
            timed_out = True
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except (ProcessLookupError, PermissionError):
                pass
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                pass
    finally:
        if process is not None:
            stragglers, leftover = (tree or process_tree.Tree(process.pid, scratch)).sweep()
            process.wait()
    if scratch is None:
        # Without a sandbox instance to count, a process that left the tree before the first walk is
        # invisible: an empty sweep proves nothing, so leftover is unknown, not False.
        leftover = leftover or None
    return {'exit_code': process.returncode, 'timed_out': timed_out, 'stragglers': stragglers,
            'leftover': leftover, 'elapsed_seconds': round(time.monotonic() - started, 3)}


def go_env(trial: Path, go_cache: Path, modcache: Path) -> dict:
    """Go settings of a stage rooted at trial: its build cache, module cache (trial_env_c), GOPATH and
    temp, and where xcrun and zsh write. GOPROXY is off; cgo keeps the go default."""
    temp = trial / 'tmp'
    temp.mkdir(exist_ok=True)
    return {'GOCACHE': str(go_cache), 'GOTMPDIR': str(temp), 'TMPDIR': str(temp), 'GOFLAGS': '-mod=mod',
            'GOPROXY': 'off', 'GOSUMDB': 'off', 'GOTELEMETRY': 'off', 'GOTOOLCHAIN': 'local',
            'GOPATH': str(trial / 'gopath'), 'GOMODCACHE': str(modcache), **trial_env_c.temp_env(temp)}


def names(ctx: dict, name: str) -> tuple:
    out = ctx['output']
    return out / 'trials' / name, out / 'measure' / name, out / 'grading' / name


def prepare(ctx: dict, task: dict, arm: str, name: str) -> dict:
    """Workspace, isolated home, build cache clone, environment and sandboxed argv (no model call)."""
    trial, measure, _ = names(ctx, name)
    trial.mkdir(parents=True)
    measure.mkdir(parents=True)
    source = ctx['sources'][task['heads']['base']]
    work = trial / 'workspace'
    grade_c.clone_tree(source['tree'], work)
    if arm != 'native':
        surface_c.install(work, source['surface'])
    initialize(work)
    home = claude_agent.isolated_home(trial, arm, work)
    grade_c.clone_tree(source['warm_cache'], trial / 'gocache')
    modcache = trial_env_c.clone_modcache(ctx['modcache'], trial / 'gomodcache')
    env = claude_agent.agent_env(ctx['base_env'], home, trial / 'tmp', ctx['credential_kind'], ctx['credential'],
                                 [source['bin_dir'], ctx['tools_dir']], ctx['shell'])
    env.update(go_env(trial, trial / 'gocache', trial / 'gomodcache'))
    argv = claude_agent.claude_argv(arm, ctx['model'], ctx['effort'], ctx['permission_mode'], ctx['claude'])
    # The build cache keys packages by directory, and this trial's workspace and module cache paths are
    # its own: the warm-up compiles them here, before the agent's clock, for `go build ./...` and the
    # oracle package's tests.
    warm = ['/bin/sh', '-c', 'go build ./... && go test -count=1 -run "^$" "$@"', 'warm',
            *('./' + package for package in oracle_c.present_packages(source['tree'], task))]

    def wrap(command):
        if not ctx['sandbox']:
            return command
        return claude_agent.sandboxed(command, trial, ctx['output'], ctx['output'] / 'bin', env['GOMODCACHE'],
                                      ctx['tools'], ctx['account_home'], ctx['protected'],
                                      ctx['frozen']['agent_profile'])
    # What the Bash tool resolves: Claude Code snapshots its environment with `$SHELL -c -l`.
    probe = [str(ctx['shell']), '-c', '-l', 'for t in auto go claude codex gemini gh omp opencode; do '
             'printf "%s=%s\\n" "$t" "$(command -v $t || echo none)"; done; echo "PATH=$PATH"']
    return {'trial': trial, 'measure': measure, 'work': work, 'env': env, 'argv': wrap(argv), 'warm': wrap(warm),
            'path_probe': wrap(probe), 'prompt': prompt_for(task), 'source': source, 'wrap': wrap,
            'sandboxed': ctx['sandbox'], 'modcache_clone': modcache}


def run(ctx: dict, task: dict, arm: str, replicate: int) -> dict:
    name = '%s-%s-k%d' % (task['id'], arm, replicate)
    plan = prepare(ctx, task, arm, name)
    trial, measure, grading = names(ctx, name)
    load_start = guard_c.loadavg()
    scratch = trial if ctx['sandbox'] else None
    warm = execute_tree(plan['warm'], plan['work'], plan['env'], measure / 'warm.log', measure / 'warm.err', 1800,
                        scratch=scratch)
    before = hashes(plan['work'])
    agent = execute_tree(plan['argv'], plan['work'], plan['env'], measure / 'events.jsonl', measure / 'stderr.log',
                         ctx['timeout'], plan['prompt'], scratch=scratch)
    load_end = guard_c.loadavg()
    observation = stream_c.parse_stream(measure / 'events.jsonl')
    scope = oracle_c.audit_globs(before, hashes(plan['work']), task['allowed_paths'])
    valid = stream_c.arm_valid(arm, observation)
    agent['tree_coverage'] = 'sandbox_instance_and_descendants' if ctx['sandbox'] else 'descendants_only'
    graded = None
    if not agent['leftover']:
        graded = grade_c.grade_candidate(task, plan['source']['tree'], plan['work'], grading, measure / 'grade',
                                         plan['source']['warm_cache'], ctx['frozen']['grader_profile'],
                                         ctx['frozen']['oracles'], modcache=ctx['modcache'])
        graded.update(report_c.review_beyond_base(graded, ctx.get('base_passing', {}).get(task['id'])))
    operational = ('warmup_failed' if warm['exit_code'] else 'leftover_processes' if agent['leftover'] else
                   'agent_timeout' if agent['timed_out'] else 'agent_exit' if agent['exit_code'] else
                   'agent_error' if observation['is_error'] else 'arm_invalid' if not valid else None)
    row = {'task_id': task['id'], 'arm': arm, 'replicate': replicate, 'operational_error': operational,
           'warm': warm, 'agent': agent, 'observation': observation, 'arm_valid': valid, 'scope': scope,
           'oracle': graded, 'modcache_clone': plan['modcache_clone'], 'loadavg': {'start': load_start, 'end': load_end},
           'prompt_hash': digest(plan['prompt'].encode()), 'human_interventions': 0,
           'harness_revision': plan['source']['harness_revision'],
           'surface_hash': plan['source']['generated']['surface_hash'] if arm != 'native' else None}
    write_json(measure / 'result.json', row)
    return row
