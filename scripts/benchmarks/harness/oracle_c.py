"""Pilot C hidden oracles: install, compile once, run every case in its own process, audit scope.

An oracle is one or more `*_test.go.oracle` files under oracles/<task>/. They are stored with a
suffix the go tool ignores, so `go test ./...` on this repository never compiles them, and are
copied into the graded tree only at grading. Each case runs as its own test-binary process, so a
case cannot depend on process state another case left (doctor diagnostics are printed once per
process, for example).
"""
import fnmatch
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import time

HERE = Path(__file__).resolve().parent
ORACLES = HERE / 'oracles'
SUFFIX = '.oracle'
CASE_NAME = re.compile(r'^Test[A-Za-z0-9_]+$')
# Trial-owned paths that are never candidate edits: the harness surface and agent runtime state.
OWNED_PREFIXES = ('.claude/', '.autopus/', '.agents/', '.codex/', '.gemini/', '.opencode/', '.omp/')
OWNED_FILES = {'CLAUDE.md', 'AGENTS.md', 'GEMINI.md', 'autopus.yaml', 'opencode.json', '.mcp.json'}


def oracle_files(task: dict, oracles: Path = ORACLES) -> list:
    root = Path(oracles) / task['id']
    files = [root / name for name in task['oracle']['files']]
    for path in files:
        if not path.name.endswith('_test.go' + SUFFIX) or not path.is_file():
            raise ValueError('oracle file must be an existing *_test.go.oracle: ' + str(path))
    return files


def install_oracle(tree: Path, task: dict, oracles: Path = ORACLES) -> list:
    """Copy the oracle files into tree/<package> without their suffix; refuse to overwrite a file."""
    package = Path(tree) / task['oracle']['package']
    installed = []
    for source in oracle_files(task, oracles):
        target = package / source.name[:-len(SUFFIX)]
        if target.exists() or target.is_symlink():
            raise ValueError('oracle file would overwrite a tree file: ' + str(target))
        shutil.copyfile(source, target)
        installed.append(target)
    return installed


def _run(argv: list, cwd: Path, env: dict, log: Path, timeout: float) -> tuple:
    started = time.monotonic()
    with open(log, 'wb') as sink:
        process = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL, stdout=sink,
                                   stderr=subprocess.STDOUT, start_new_session=True)
        timed_out = False
        try:
            process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
        finally:
            # Also when a stop signal to the runner interrupts the wait (run_c).
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except (ProcessLookupError, PermissionError):
                pass
            process.wait()
    return process.returncode, timed_out, round(time.monotonic() - started, 3)


def case_verdict(output: str, name: str, code: int, timed_out: bool) -> str:
    """pass, fail, skip, timeout or no_result for one `-test.run ^name$ -test.v` process."""
    if timed_out:
        return 'timeout'
    if re.search(r'^--- FAIL: ' + re.escape(name) + r' ', output, re.M):
        return 'fail'
    if re.search(r'^--- SKIP: ' + re.escape(name) + r' ', output, re.M):
        return 'skip'
    if code == 0 and re.search(r'^--- PASS: ' + re.escape(name) + r' ', output, re.M):
        return 'pass'
    return 'fail' if code else 'no_result'


def run_oracle(tree: Path, task: dict, logs: Path, env: dict, wrap=None, timeout: float = 2400) -> dict:
    """Compile the package's test binary once, then run each case alone. A build failure fails all.
    wrap(argv, cwd) returns the argv that actually runs (grade_c wraps every run in grader.sb)."""
    wrap = wrap or (lambda argv, cwd: argv)
    oracle = task['oracle']
    names = [case['name'] for case in oracle['cases']]
    if not names or any(not CASE_NAME.match(name) for name in names):
        raise ValueError('oracle cases must be plain test names')
    logs = Path(logs)
    logs.mkdir(parents=True, exist_ok=True)
    package = Path(tree) / oracle['package']
    binary = Path(env['TMPDIR']) / 'oracle.test'
    code, timed_out, seconds = _run(wrap(['go', 'test', '-c', '-o', str(binary), './' + oracle['package']], tree),
                                    tree, env, logs / 'build.log', timeout)
    result = {'build': {'exit_code': code, 'timed_out': timed_out, 'seconds': seconds}, 'cases': {}}
    if code or timed_out or not binary.is_file():
        result['cases'] = {name: 'build_failed' for name in names}
        return result
    for name in names:
        log = logs / (name + '.log')
        argv = [str(binary), '-test.run', '^' + name + '$', '-test.count=1', '-test.v',
                '-test.timeout=' + str(int(oracle.get('case_timeout', 900))) + 's']
        code, timed_out, _ = _run(wrap(argv, package), package, env, log, oracle.get('case_timeout', 900) + 60)
        result['cases'][name] = case_verdict(log.read_text(errors='replace'), name, code, timed_out)
    binary.unlink(missing_ok=True)
    return result


def frozen_hash(tasks: list, oracles: Path = ORACLES) -> str:
    """One hash over the task definitions and the bytes of every oracle file they name."""
    import hashlib
    import json
    digest = hashlib.sha256(json.dumps(tasks, sort_keys=True).encode())
    for task in sorted(tasks, key=lambda t: t['id']):
        for path in oracle_files(task, oracles):
            digest.update(task['id'].encode() + b'\0' + path.name.encode() + b'\0' + path.read_bytes())
    return digest.hexdigest()


def owned(path: str) -> bool:
    return path in OWNED_FILES or path.startswith(OWNED_PREFIXES)


def audit_globs(before: dict, after: dict, allowed: list) -> dict:
    """Scope of one trial from two workspace.hashes() maps; allowed entries are fnmatch globs
    in which `*` also crosses `/`. Harness and agent runtime files are reported apart."""
    changed = sorted(path for path in before.keys() | after.keys() if before.get(path) != after.get(path))
    harness = [path for path in changed if owned(path)]
    outside = [path for path in changed if not owned(path) and not any(fnmatch.fnmatchcase(path, g) for g in allowed)]
    deleted = [path for path in changed if path in before and path not in after and not owned(path)]
    tests = [path for path in changed if path.endswith('_test.go') and path in before]
    return {'changed_paths': changed, 'outside_allowed': outside, 'harness_changes': harness,
            'deleted_paths': deleted, 'changed_existing_tests': tests, 'accepted_scope': not outside}
