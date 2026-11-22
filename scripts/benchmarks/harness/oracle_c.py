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


def oracle_files(task: dict) -> list:
    root = ORACLES / task['id']
    files = [root / name for name in task['oracle']['files']]
    for path in files:
        if not path.name.endswith('_test.go' + SUFFIX) or not path.is_file():
            raise ValueError('oracle file must be an existing *_test.go.oracle: ' + str(path))
    return files


def install_oracle(tree: Path, task: dict) -> list:
    """Copy the oracle files into tree/<package> without their suffix; refuse to overwrite a file."""
    package = Path(tree) / task['oracle']['package']
    installed = []
    for source in oracle_files(task):
        target = package / source.name[:-len(SUFFIX)]
        if target.exists() or target.is_symlink():
            raise ValueError('oracle file would overwrite a tree file: ' + str(target))
        shutil.copyfile(source, target)
        installed.append(target)
    return installed


def restore_tests(candidate: Path, source: Path) -> dict:
    """Make every *_test.go in candidate equal to source: delete added ones, restore changed ones.

    Grading uses the base tests plus the oracle, never the agent's own tests, so an agent test that
    does not compile cannot hide or fake an oracle result. Returns what was reset.
    """
    candidate, source = Path(candidate), Path(source)
    reset = {'removed': [], 'restored': []}
    for path in sorted(candidate.rglob('*_test.go')):
        rel = path.relative_to(candidate)
        if rel.parts[0] == '.git':
            continue
        original = source / rel
        if not original.is_file():
            path.unlink()
            reset['removed'].append(rel.as_posix())
        elif path.is_symlink() or path.read_bytes() != original.read_bytes():
            path.unlink()
            shutil.copyfile(original, path)
            reset['restored'].append(rel.as_posix())
    for original in sorted(source.rglob('*_test.go')):
        rel = original.relative_to(source)
        if not (candidate / rel).exists():
            (candidate / rel).parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(original, candidate / rel)
            reset['restored'].append(rel.as_posix())
    return reset


def oracle_env(root: Path, go_cache: Path, extra_path: list = ()) -> dict:
    """A scrubbed environment: no inherited credentials, a scratch HOME and temp, the shared Go cache."""
    go = shutil.which('go')
    if not go:
        raise RuntimeError('go is not on PATH')
    home, temp = Path(root) / 'home', Path(root) / 'tmp'
    home.mkdir(parents=True, exist_ok=True)
    temp.mkdir(parents=True, exist_ok=True)
    modcache = subprocess.check_output([go, 'env', 'GOMODCACHE'], text=True).strip()
    paths = [str(Path(os.path.realpath(go)).parent), *map(str, extra_path), '/usr/bin', '/bin']
    return {'PATH': os.pathsep.join(paths), 'HOME': str(home), 'TMPDIR': str(temp), 'GOTMPDIR': str(temp),
            'GOCACHE': str(go_cache), 'GOMODCACHE': modcache, 'GOFLAGS': '-mod=mod', 'GOPROXY': 'off',
            'GOSUMDB': 'off', 'GOTOOLCHAIN': 'local', 'GOTELEMETRY': 'off', 'GOMAXPROCS': '4',
            'GIT_CONFIG_NOSYSTEM': '1', 'LANG': 'C.UTF-8'}


def _run(argv: list, cwd: Path, env: dict, log: Path, timeout: float) -> tuple:
    started = time.monotonic()
    with open(log, 'wb') as sink:
        process = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL, stdout=sink,
                                   stderr=subprocess.STDOUT, start_new_session=True)
        try:
            process.wait(timeout=timeout)
            timed_out = False
        except subprocess.TimeoutExpired:
            timed_out = True
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


def run_oracle(tree: Path, task: dict, logs: Path, env: dict, timeout: float = 2400) -> dict:
    """Compile the package's test binary once, then run each case alone. A build failure fails all."""
    oracle = task['oracle']
    names = [case['name'] for case in oracle['cases']]
    if not names or any(not CASE_NAME.match(name) for name in names):
        raise ValueError('oracle cases must be plain test names')
    logs = Path(logs)
    logs.mkdir(parents=True, exist_ok=True)
    package = Path(tree) / oracle['package']
    binary = Path(env['TMPDIR']) / 'oracle.test'
    code, timed_out, seconds = _run(['go', 'test', '-c', '-o', str(binary), './' + oracle['package']], tree, env,
                                    logs / 'build.log', timeout)
    result = {'build': {'exit_code': code, 'timed_out': timed_out, 'seconds': seconds}, 'cases': {}}
    if code or timed_out or not binary.is_file():
        result['cases'] = {name: 'build_failed' for name in names}
        return result
    for name in names:
        log = logs / (name + '.log')
        code, timed_out, _ = _run([str(binary), '-test.run', '^' + name + '$', '-test.count=1', '-test.v',
                                   '-test.timeout=' + str(int(oracle.get('case_timeout', 900))) + 's'],
                                  package, env, log, oracle.get('case_timeout', 900) + 60)
        result['cases'][name] = case_verdict(log.read_text(errors='replace'), name, code, timed_out)
    binary.unlink(missing_ok=True)
    return result


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
