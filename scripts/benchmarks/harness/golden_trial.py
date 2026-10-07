"""One golden-mode trial: trusted stages, classification and the record (SPEC-HARNEVAL-001 REQ-HE-08).

Stage order: snapshot, mutation, warmup, arm surface, agent, scope audit, grading. Only the three
stages before the arm surface enters the workspace can make an `error` trial, so a candidate surface
never removes its own failures from the denominator. Grading runs even when the agent step failed,
so `oracle.ran` stays observable, and is skipped only for a scope violation or a forbidden construct.
"""
import json
from pathlib import Path
import subprocess

import grader
from golden_agent import agent_argv, agent_env, prompt_text, run_agent
from golden_protocol import NO_ORACLE, record_document, sha256_hex
from observe import parse_events
from prepare_grader import new_grade, remove_tree
from run import copy_candidate
from workspace import apply_mutation, audit, hashes, initialize, install_surface

# Literal constructs an agent diff may not add to an allowed file (advisory-level forgery mitigation).
FORBIDDEN = ('func init(', 'os.Exit', 'syscall.', 'unsafe.', '//go:linkname')
ORACLE_TIMEOUT = 300
OBSERVED = ('completed_turns', 'command_actions', 'failed', 'failure_codes', 'incomplete_reasons',
            'known_total_tokens')


def warm_command(command: list) -> list:
    """The oracle command compiling its package without running a test."""
    warm = list(command)
    if '-run' in warm:
        warm[warm.index('-run') + 1] = '^$'
    else:
        warm += ['-run', '^$']
    return warm


def warmup(root: Path, command: list, prepared: dict, logs: Path) -> bool:
    """Compile the mutated oracle package into the agent root's build cache inside grader.sb."""
    run = grader.run_sandboxed(grader.grader_argv(root, warm_command(command), prepared), Path(root) / 'ws',
                               Path(logs) / 'warmup.jsonl', Path(logs) / 'warmup.stderr', ORACLE_TIMEOUT)
    return run['exit_code'] == 0 and not (run['timed_out'] or run['overflow'] or run['leftover'])


def new_forbidden(originals: dict, work: Path, allowed: list) -> list:
    """The FORBIDDEN literals that an allowed file now holds more often than before the agent ran."""
    found = set()
    for path in allowed:
        target = Path(work) / path
        if target.is_symlink() or not target.is_file():
            continue
        after = target.read_bytes().decode('utf-8', 'replace')
        before = originals.get(path, b'').decode('utf-8', 'replace')
        found.update(literal for literal in FORBIDDEN if after.count(literal) > before.count(literal))
    return sorted(found)


def agent_signal(run: dict, observation: dict | None) -> str | None:
    """The agent stage failure, if any: launch, timeout, exit code, then an unusable observation.

    An agent that exits nonzero before printing a single event never started a session (codex refused
    its configuration, for one), so that is a launch failure too.
    """
    if not run['launched']:
        return 'agent_launch_failed'
    if run['timed_out']:
        return 'agent_timeout'
    if run['exit_code'] != 0:
        return 'agent_launch_failed' if run['transcript_bytes'] == 0 else 'agent_exit_nonzero'
    if observation is None or observation['failed'] or run['leftover']:
        return 'observation_failed'
    return None


def decide(agent: str | None, scope_ok: bool, forbidden: list, oracle_signal: str | None) -> str:
    """The record signal of a trial whose surface entered the workspace, by stage order."""
    if agent:
        return agent
    if not scope_ok:
        return 'scope_violation'
    if forbidden:
        return 'forbidden_construct'
    return oracle_signal


def _observe(path: Path) -> dict | None:
    try:
        observation = parse_events(path)
    except (OSError, ValueError):
        return None
    return {key: observation[key] for key in OBSERVED}


def _grade(session, task: dict, work: Path, roots: Path, logs: Path, diagnostics: dict) -> tuple:
    """Grade a fresh snapshot copy holding the mutation and only the agent's allowed files."""
    corpus = task['corpus']
    try:
        root = new_grade(roots / 'grade', session.source, Path(session.prepared['warm_cache']))
        apply_mutation(root / 'ws', corpus)
        if not copy_candidate(work, root / 'ws', corpus['allowed_paths']):
            raise ValueError('candidate file vanished after the scope audit')
        result = session.steps.grade(root, corpus['oracle']['command'], task['expected_tests'], session.prepared,
                                     logs / 'grader.jsonl', logs / 'grader.stderr', ORACLE_TIMEOUT)
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        diagnostics['grader_error'] = type(error).__name__
        return 'oracle_output_invalid', NO_ORACLE
    diagnostics['grader'] = {key: result.get(key) for key in ('exit_code', 'timed_out', 'overflow', 'leftover')}
    return result['signal'], result['oracle']


def run_trial(session, index: int, attempt: dict) -> dict:
    """Run one scheduled attempt and return its record; body-free diagnostics go to trials/<name>/trial.json."""
    task = session.tasks[attempt['task_id']]
    name = f"{index:03d}-{attempt['task_id']}-{attempt['arm']}-{attempt['trial']}"
    logs, roots = session.out / 'trials' / name, session.scratch / 'trials' / name
    logs.mkdir(parents=True)
    roots.mkdir(parents=True)
    diagnostics = {'attempt': attempt}
    try:
        signal_name, oracle, seconds = _stages(session, task, attempt, logs, roots, diagnostics)
    finally:
        if not session.keep_scratch:
            remove_tree(roots)
    diagnostics['signal'] = signal_name
    (logs / 'trial.json').write_text(json.dumps(diagnostics, indent=2, sort_keys=True) + '\n')
    return record_document(session.session_id, attempt, signal_name, oracle, seconds)


def _stages(session, task: dict, attempt: dict, logs: Path, roots: Path, diagnostics: dict) -> tuple:
    corpus, allowed = task['corpus'], task['corpus']['allowed_paths']
    try:
        root = new_grade(roots / 'agent', session.source, Path(session.prepared['warm_cache']))
        diagnostics['snapshot_tree_sha256'] = sha256_hex(json.dumps(hashes(root / 'ws'), sort_keys=True).encode())
    except (OSError, ValueError, subprocess.SubprocessError):
        return 'workspace_setup_failed', NO_ORACLE, 0.0
    try:
        apply_mutation(root / 'ws', corpus)
    except (OSError, ValueError, KeyError):
        return 'mutation_failed', NO_ORACLE, 0.0
    if not session.steps.warmup(root, corpus['oracle']['command'], session.prepared, logs):
        return 'warmup_failed', NO_ORACLE, 0.0
    agent, seconds, scope_ok, forbidden = None, 0.0, True, []
    try:
        install_surface(root / 'ws', session.surfaces[attempt['arm']])
        initialize(root / 'ws')
        before = hashes(root / 'ws')
        originals = {path: (root / 'ws' / path).read_bytes() for path in allowed}
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        agent, diagnostics['surface_error'] = 'agent_launch_failed', type(error).__name__
    if agent is None:
        run = run_agent(agent_argv(session.codex, session.model, root, Path(session.prepared['modcache'])),
                        root / 'ws', agent_env(root, session.prepared, session.codex, session.credentials),
                        prompt_text(corpus), roots / 'events.jsonl', roots / 'agent.stderr', session.timeout)
        observation = _observe(roots / 'events.jsonl') if run['launched'] else None
        agent, seconds = agent_signal(run, observation), run['duration_s']
        diagnostics.update(agent=run, observation=observation)
        try:
            scope = audit(before, root / 'ws', allowed)
            regular = all((root / 'ws' / path).is_file() and not (root / 'ws' / path).is_symlink() for path in allowed)
            scope_ok, forbidden = scope['accepted_scope'] and regular, new_forbidden(originals, root / 'ws', allowed)
        except (OSError, ValueError) as error:
            scope, scope_ok = {'error': type(error).__name__}, False
        diagnostics.update(scope=scope, forbidden=forbidden)
    oracle_signal, oracle = None, NO_ORACLE
    if scope_ok and not forbidden:
        oracle_signal, oracle = _grade(session, task, root / 'ws', roots, logs, diagnostics)
    return decide(agent, scope_ok, forbidden, oracle_signal), oracle, seconds
