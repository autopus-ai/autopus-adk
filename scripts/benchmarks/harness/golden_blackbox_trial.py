"""One black-box trial and the black-box calibration (SPEC-HARNEVAL-003 REQ-HR-08, T13).

Stage order: setup (snapshot copy, mutation, input fixtures and expected-output digests, warmup),
agent (surface, agent, scope audit), build, run, oracle. As in SPEC-HARNEVAL-001 a failed agent
still gets built and judged, so `oracle.ran` stays observable; observation_failed (processes the
runner could not account for) and scope_violation skip every later stage. No literal check runs
and no white-box grade: the record follows from the REQ-HR-08 table over the record fields and
the oracle result bytes, which the session keeps in oracle-results/<sha256>.json.
"""
import json
from pathlib import Path
import subprocess
import sys

import golden_blackbox as gb
import golden_sandbox as gs
from golden_agent import agent_argv, agent_env, prompt_text, run_agent
from golden_protocol import sha256_hex
from golden_trial import _observe, agent_signal
from prepare_grader import new_grade, remove_tree
from run import copy_candidate
from workspace import apply_mutation, audit, hashes, initialize, install_surface

# Calibration has no agent: its runs are judged as if the agent step had ended cleanly.
CLEAN_END = {'launched': True, 'exit_code': 0, 'os_signal': None, 'timed_out': False}
STATS = ('exit_code', 'timed_out', 'overflow', 'leftover', 'duration_s')


def new_build_root(root: Path, source: Path, warm_cache: Path) -> Path:
    """A fresh build root: clonefile copies of the snapshot (ws/) and the warm cache (gocache/), scratch/
    for HOME, TMPDIR and GOPATH, and bin/ for the artifact."""
    Path(root).mkdir(parents=True)
    root = Path(root).resolve()
    for origin, name in ((source, 'ws'), (warm_cache, 'gocache')):
        subprocess.run(['cp', '-c', '-R', str(origin), str(root / name)], check=True)
    for name in ('scratch/home', 'scratch/tmp', 'scratch/gopath', 'bin'):
        (root / name).mkdir(parents=True)
    return root


def _stats(run: dict) -> dict:
    return {key: run.get(key) for key in STATS}


def _run_artifact(ctx, oracle: dict, artifact: Path, input_dir: Path, run_root: Path, stdin_name: str | None,
                  logs: Path, name: str) -> tuple:
    """One run of the artifact under artifact.sb run mode in a fresh <run_root>: (run, captured stdout)."""
    for sub in ('out', 'home', 'tmp'):
        (Path(run_root) / sub).mkdir(parents=True)
    run_root, input_dir = Path(run_root).resolve(), Path(input_dir).resolve()
    args = gb.artifact_args(oracle, artifact, input_dir, run_root / 'out')
    ran = gs.run_stage(gs.run_argv(artifact, input_dir, run_root, args, ctx.params, (ctx.out,)), run_root / 'out',
                       logs / (name + '.stdout'), logs / (name + '.stderr'), gs.ARTIFACT_TIMEOUT,
                       input_dir / stdin_name if stdin_name else None, confined=run_root)
    return ran, (logs / (name + '.stdout')).read_bytes() if ran['launched'] else b''


def artifact_stages(ctx, task: dict, prepared: dict, build_root: Path, input_dir: Path, expected: dict, work: Path,
                    logs: Path, diagnostics: dict) -> tuple:
    """Build, run and judge one artifact as sibling processes: (stage_reached, stage signal, oracle bytes).
    A task with a positive control runs the same artifact a second time over the control's inputs."""
    oracle, control = task['black_box'], task['black_box'].get('positive_control')
    built = gs.run_stage(gs.build_argv(prepared, build_root, oracle['build'], ctx.params, (ctx.out,)),
                         build_root / 'ws', logs / 'build.stdout', logs / 'build.stderr', gs.BUILD_TIMEOUT,
                         confined=build_root / 'scratch')
    diagnostics['build'] = _stats(built)
    artifact = build_root / 'bin' / 'artifact'
    if built['leftover']:
        return 'build', 'observation_failed', None
    if built['exit_code'] != 0 or built['timed_out'] or built['overflow'] or not artifact.is_file():
        return 'build', None, None
    ran, stdout = _run_artifact(ctx, oracle, artifact, input_dir, Path(work) / 'run', oracle['stdin'], logs,
                                'artifact')
    diagnostics['artifact'] = _stats(ran)
    if ran['leftover']:
        return 'run', 'observation_failed', None
    observed = None
    if control:
        observed = _run_artifact(ctx, oracle, artifact, gb.control_input(input_dir), Path(work) / 'control',
                                 control['stdin'], logs, 'control')
        diagnostics['control'] = _stats(observed[0])
        if observed[0]['leftover']:
            return 'run', 'observation_failed', None
    result_dir = Path(work) / 'oracle'
    result_dir.mkdir()
    result_dir = result_dir.resolve()
    judged = gs.run_stage(gs.oracle_argv(ctx.oracle, Path(work).resolve() / 'run' / 'out', result_dir, task['id'],
                                         ctx.params),
                          result_dir, logs / 'oracle.stdout', logs / 'oracle.stderr', gs.ORACLE_TIMEOUT,
                          gb.bundle(task['id'], oracle, expected, ran, stdout, observed), confined=result_dir)
    diagnostics['oracle'] = _stats(judged)
    path = result_dir / gb.RESULT_FILE
    if judged['exit_code'] != 0 or judged['timed_out'] or path.is_symlink() or not path.is_file():
        return 'oracle', None, None
    with open(path, 'rb') as source:
        data = source.read(gb.OUTPUT_LIMIT + 1)
    return 'oracle', None, data if len(data) <= gb.OUTPUT_LIMIT else None


def run_trial(ctx, session, index: int, attempt: dict) -> dict:
    """Run one scheduled black-box attempt and return its record; diagnostics go to trials/<name>/trial.json."""
    task = session.tasks[attempt['task_id']]
    name = f"{index:03d}-{attempt['task_id']}-{attempt['arm']}-{attempt['trial']}"
    logs, roots = session.out / 'trials' / name, session.scratch / 'trials' / name
    logs.mkdir(parents=True)
    roots.mkdir(parents=True)
    diagnostics = {'attempt': attempt}
    try:
        stage, signal, ended, data, seconds = _stages(ctx, session, task, attempt, logs, roots, diagnostics)
    finally:
        if not session.keep_scratch:
            remove_tree(roots)
    digest = gb.store_result(session.out, data) if data is not None else None
    ids = gb.assertion_ids(task['black_box'])
    derived = gb.derive(stage, signal, ended, data, task['id'], ids)
    diagnostics.update(stage_reached=stage, signal=derived[1], oracle_result_sha256=digest)
    (logs / 'trial.json').write_text(json.dumps(diagnostics, indent=2, sort_keys=True) + '\n')
    return gb.record(session.session_id, attempt, derived, seconds, stage, ended, digest)


def _setup(ctx, session, task: dict, logs: Path, roots: Path, diagnostics: dict) -> tuple:
    """(agent root, expected outputs, None) or (None, None, the setup signal)."""
    try:
        root = new_grade(roots / 'agent', session.source, Path(session.prepared['warm_cache']))
        diagnostics['snapshot_tree_sha256'] = sha256_hex(json.dumps(hashes(root / 'ws'), sort_keys=True).encode())
    except (OSError, ValueError, subprocess.SubprocessError):
        return None, None, 'workspace_setup_failed'
    try:
        apply_mutation(root / 'ws', task['corpus'])
    except (OSError, ValueError, KeyError):
        return None, None, 'mutation_failed'
    try:
        expected = gb.prepare_fixtures(task['black_box'], ctx.set_root, roots / 'input')
    except (OSError, ValueError) as error:
        diagnostics['fixture_error'] = str(error)
        return None, None, 'workspace_setup_failed'
    if not session.steps.warmup(root, task['corpus']['oracle']['command'], session.prepared, logs):
        return None, None, 'warmup_failed'
    return root, expected, None


def _stages(ctx, session, task: dict, attempt: dict, logs: Path, roots: Path, diagnostics: dict) -> tuple:
    corpus, allowed = task['corpus'], task['corpus']['allowed_paths']
    root, expected, failed = _setup(ctx, session, task, logs, roots, diagnostics)
    if failed:
        return 'setup', failed, dict(gb.NOT_LAUNCHED), None, 0.0
    run, agent, seconds, scope_ok = None, 'agent_launch_failed', 0.0, True
    try:
        install_surface(root / 'ws', session.surfaces[attempt['arm']])
        initialize(root / 'ws')
        before = hashes(root / 'ws')
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        diagnostics['surface_error'] = type(error).__name__
    else:
        run = run_agent(agent_argv(session.codex, session.model, root, Path(session.prepared['modcache'])),
                        root / 'ws', agent_env(root, session.prepared, session.codex, session.credentials),
                        prompt_text(corpus), roots / 'events.jsonl', roots / 'agent.stderr', session.timeout)
        observation = _observe(roots / 'events.jsonl') if run['launched'] else None
        agent, seconds = agent_signal(run, observation), run['duration_s']
        diagnostics.update(agent=run, observation=observation)
        try:
            scope = audit(before, root / 'ws', allowed)
            regular = all((root / 'ws' / path).is_file() and not (root / 'ws' / path).is_symlink() for path in allowed)
            scope_ok = scope['accepted_scope'] and regular
        except (OSError, ValueError) as error:
            scope, scope_ok = {'error': type(error).__name__}, False
        diagnostics['scope'] = scope
    ended = gb.termination(run, agent)
    if agent == 'observation_failed' or (run is not None and run['leftover']):
        return 'agent', 'observation_failed', ended, None, seconds
    if not scope_ok:
        return 'agent', 'scope_violation', ended, None, seconds
    try:
        build_root = new_build_root(roots / 'build', session.source, Path(session.prepared['warm_cache']))
        apply_mutation(build_root / 'ws', corpus)
        if not copy_candidate(root / 'ws', build_root / 'ws', allowed):
            raise ValueError('candidate file vanished after the scope audit')
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        diagnostics['build_error'] = type(error).__name__
        return 'build', None, ended, None, seconds
    stage, signal, data = artifact_stages(ctx, task, session.prepared, build_root, roots / 'input', expected, roots,
                                          logs, diagnostics)
    return stage, signal, ended, data, seconds


def calibrate(ctx, tasks: list, source: Path, prepared: dict, scratch: Path, logs: Path, _timeout: float = 0) -> dict:
    """Two-direction black-box calibration through the same sibling stages: each clean reference
    artifact must be accepted, and each mutated one must be rejected by the output comparison
    itself (expectation_mismatch), so its row's mutated_accepted is true for any other outcome."""
    Path(logs).mkdir(parents=True, exist_ok=True)
    rows, runs = [], []
    try:
        gs.warm_build(prepared, source, sorted({task['black_box']['build'] for task in tasks}))
    except (OSError, subprocess.SubprocessError) as error:
        print('golden: black-box warm build failed: ' + type(error).__name__, file=sys.stderr)
        rows = [{'task_id': task['id'], 'clean_accepted': False, 'mutated_accepted': False} for task in tasks]
        return {'status': 'failed', 'tasks': sorted(rows, key=lambda row: row['task_id']), 'runs': []}
    for task in sorted(tasks, key=lambda item: item['id']):
        row, ids = {'task_id': task['id']}, gb.assertion_ids(task['black_box'])
        for direction in ('clean', 'mutated'):
            base, log, diagnostics = Path(scratch) / (task['id'] + '-' + direction), Path(logs) / (task['id'] + '-'
                                                                                                    + direction), {}
            log.mkdir(parents=True)
            try:
                expected = gb.prepare_fixtures(task['black_box'], ctx.set_root, base / 'input')
                build_root = new_build_root(base / 'build', source, Path(prepared['warm_cache']))
                if direction == 'mutated':
                    apply_mutation(build_root / 'ws', task['corpus'])
                stage, signal, data = artifact_stages(ctx, task, prepared, build_root, base / 'input', expected, base,
                                                      log, diagnostics)
                verdict = gb.derive(stage, signal, CLEAN_END, data, task['id'], ids)[1]
            except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
                verdict, diagnostics['error'] = 'workspace_setup_failed', str(error)
            finally:
                remove_tree(base)
            if direction == 'clean':
                row['clean_accepted'] = verdict == 'accepted'
            else:
                row['mutated_accepted'] = verdict != 'expectation_mismatch'
            runs.append({'task_id': task['id'], 'direction': direction, 'signal': verdict, **diagnostics})
        rows.append(row)
    passed = bool(rows) and all(row['clean_accepted'] and not row['mutated_accepted'] for row in rows)
    return {'status': 'passed' if passed else 'failed', 'tasks': rows, 'runs': runs}
