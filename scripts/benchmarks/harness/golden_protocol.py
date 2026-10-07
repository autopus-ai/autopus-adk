"""Frozen live-session documents and digests of the golden mode (SPEC-HARNEVAL-001 REQ-HE-07, REQ-HE-08).

Everything here is trusted runner code: the balanced order, the protocol and record documents in the
T10 wire contract that `auto eval harness report` decodes strictly, the runner and surface digests,
and the golden set digests computed by the checkout's own `auto eval harness digest`.
"""
import hashlib
import json
import os
from pathlib import Path
import subprocess

HERE = Path(__file__).resolve().parent
PROTOCOL_SCHEMA = 'harness_golden_live_protocol.v1'
RECORD_SCHEMA = 'harness_golden_live_record.v1'
ARMS = ('baseline', 'candidate')
PLATFORMS = ('claude-code', 'codex', 'antigravity-cli', 'opencode', 'omp')

# The runner file set behind runner_sha256. REQ-HE-07 names golden.py; the digest covers every file
# the trusted runner process loads or hands to the sandbox (T11 handover): the orchestration, the
# trial stages, the sandboxed grader and its profile, the trusted preparation, and the pilot modules
# they import (report.py is loaded through run.py). test_golden.py keeps this list equal to them.
RUNNER_FILES = ('golden.py', 'golden_agent.py', 'golden_protocol.py', 'golden_trial.py', 'grader.py',
                'grader.sb', 'observe.py', 'permissions.py', 'prepare_grader.py', 'report.py', 'run.py',
                'workspace.py')

# REQ-HE-08 signal table: the signal fixes the outcome. Only a failure before the arm surface enters
# the trial workspace is an error, so a candidate surface cannot remove its own failures.
SIGNAL_OUTCOMES = {
    'workspace_setup_failed': 'error', 'mutation_failed': 'error', 'warmup_failed': 'error',
    'agent_launch_failed': 'fail', 'agent_exit_nonzero': 'fail', 'agent_timeout': 'fail',
    'forbidden_construct': 'fail', 'oracle_failed': 'fail', 'oracle_timeout': 'fail',
    'oracle_output_invalid': 'fail', 'scope_violation': 'fail', 'observation_failed': 'fail',
    'accepted': 'pass',
}
NO_ORACLE = {'ran': False, 'build_failed': False, 'expected_passed': 0, 'expected_failed': 0}

# Prompt Layer Manifest Contract: which protocol identifiers pin each layer. The ephemeral layer
# (trial workspace, agent output) is never stored, so it names nothing.
PROMPT_LAYERS = [
    {'layer': 'stable', 'identifiers': ['baseline_surface_digest', 'candidate_surface_digest', 'agent_set_digest']},
    {'layer': 'snapshot', 'identifiers': ['corpus_digests', 'workspace_revision', 'model', 'cli_version']},
    {'layer': 'ephemeral', 'identifiers': []},
]


def sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _rows_digest(rows: list) -> str:
    """SHA-256 hex of the sorted "relpath\\x00sha256(content)\\n" rows, the harneval tree digest format."""
    return sha256_hex(''.join(sorted(rows)).encode())


def runner_digest(directory: Path = HERE) -> str:
    """runner_sha256: the tree digest of RUNNER_FILES under their checkout path scripts/benchmarks/harness/."""
    return _rows_digest([f'scripts/benchmarks/harness/{name}\x00{sha256_hex((Path(directory) / name).read_bytes())}\n'
                         for name in RUNNER_FILES])


def _bookkeeping(rel: str) -> bool:
    """The closed timestamp-bearing ADK bookkeeping list that the Go SurfaceDigest leaves out."""
    return rel.startswith('.autopus/txns/') or rel in {f'.autopus/{name}-manifest.json' for name in PLATFORMS}


def _raise(error: OSError) -> None:
    raise error


def surface_digest(root: Path) -> str:
    """The Go harneval.SurfaceDigest of a generated tree; a symlink, special file or walk error is an error."""
    root, rows = Path(root), []
    if not root.is_dir() or root.is_symlink():
        raise ValueError('surface root ' + str(root) + ' is not a directory')
    for directory, dirs, files in os.walk(root, onerror=_raise):
        for name in sorted(dirs + files):
            path = Path(directory) / name
            rel = path.relative_to(root).as_posix()
            if path.is_symlink() or not (path.is_dir() or path.is_file()):
                raise ValueError('surface file ' + rel + ' is not a regular file')
            if path.is_file() and not _bookkeeping(rel):
                rows.append(rel + '\x00' + sha256_hex(path.read_bytes()) + '\n')
    return _rows_digest(rows)


def schedule(task_ids: list, k: int) -> list:
    """The balanced order: trial t, tasks by id, baseline first when t + task index is even."""
    order = []
    for trial in range(k):
        for index, task_id in enumerate(sorted(task_ids)):
            arms = ARMS if (trial + index) % 2 == 0 else ARMS[::-1]
            order.extend({'task_id': task_id, 'arm': arm, 'trial': trial} for arm in arms)
    return order


def corpus_digests(tasks: list) -> list:
    """One {file, file_sha256} row per corpus file the active agent tasks pin, by file."""
    rows = {task['corpus_ref']['file']: task['corpus_ref']['file_sha256'] for task in tasks}
    return [{'file': name, 'file_sha256': rows[name]} for name in sorted(rows)]


def protocol_document(session_id: str, started_at: str, manifest: dict, surfaces: dict, agent_set_digest: str,
                      corpus: list, runner: str, profile: str, calibration: dict, cli_version: str,
                      order: list) -> dict:
    """harness_golden_live_protocol.v1 exactly as the T10 decoder accepts it; policy is the manifest live block."""
    live = manifest['live']
    return {'schema_version': PROTOCOL_SCHEMA, 'session_id': session_id, 'started_at': started_at,
            'workspace_revision': live['workspace_revision'], 'baseline_ref': live['baseline_ref'],
            'baseline_surface_digest': surfaces['baseline'], 'candidate_surface_digest': surfaces['candidate'],
            'agent_set_digest': agent_set_digest, 'corpus_digests': corpus, 'runner_sha256': runner,
            'grader_profile_sha256': profile,
            'calibration': {'status': calibration['status'], 'tasks': calibration['tasks']},
            'policy': dict(live), 'pins': dict(manifest['pins']), 'cli_version': cli_version,
            'model': live['model'], 'order': order, 'prompt_layers': PROMPT_LAYERS}


def record_document(session_id: str, attempt: dict, signal: str, oracle: dict, duration_s: float) -> dict:
    """harness_golden_live_record.v1; the outcome always follows from the signal."""
    return {'schema_version': RECORD_SCHEMA, 'session_id': session_id, 'task_id': attempt['task_id'],
            'arm': attempt['arm'], 'trial': attempt['trial'], 'outcome': SIGNAL_OUTCOMES[signal], 'signal': signal,
            'oracle': {key: oracle[key] for key in NO_ORACLE}, 'duration_s': round(max(duration_s, 0.0), 3)}


def write_exclusive(path: Path, document: dict) -> None:
    """Create a frozen document with O_EXCL; an existing file raises FileExistsError and is left untouched."""
    with open(path, 'x') as sink:
        sink.write(json.dumps(document, indent=2) + '\n')


def append_record(path: Path, record: dict) -> None:
    """Append one record line and flush it to disk before the next trial starts."""
    with open(path, 'a') as sink:
        sink.write(json.dumps(record, separators=(',', ':')) + '\n')
        sink.flush()
        os.fsync(sink.fileno())


def set_digests(auto: str, set_root: Path) -> dict:
    """Strictly load the golden set with the checkout's own `auto eval harness digest` and return its digests.

    The Go loader is the authority for agent_set_digest; a set it rejects raises ValueError.
    """
    completed = subprocess.run([auto, 'eval', 'harness', 'digest', '--format', 'json', '--dir', str(set_root)],
                               capture_output=True, text=True, timeout=900)
    if completed.returncode:
        raise ValueError('auto eval harness digest failed: ' + ' '.join(completed.stderr.split())[-400:])
    return json.loads(completed.stdout)
