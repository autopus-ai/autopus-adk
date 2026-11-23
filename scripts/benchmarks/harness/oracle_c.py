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


def file_specs(task: dict, oracles: Path = ORACLES) -> list:
    """(path, package) per oracle file. A file entry is a name (in oracle.package) or
    {"name": ..., "package": ...}, so one task can grade cases in several packages."""
    root, default = Path(oracles) / task['id'], task['oracle']['package']
    specs = []
    for entry in task['oracle']['files']:
        name, package = (entry, default) if isinstance(entry, str) else (entry['name'], entry['package'])
        path = root / name
        if not path.name.endswith('_test.go' + SUFFIX) or not path.is_file():
            raise ValueError('oracle file must be an existing *_test.go.oracle: ' + str(path))
        specs.append((path, package))
    return specs


def oracle_files(task: dict, oracles: Path = ORACLES) -> list:
    return [path for path, _ in file_specs(task, oracles)]


def stub_specs(task: dict, oracles: Path = ORACLES) -> list:
    """(path, package) of the base stubs gate 3 adds to base: oracle.base_stub is a name (in
    oracle.package), a {"name", "package"} entry, or a list of either."""
    stub = task['oracle'].get('base_stub')
    entries = [] if not stub else stub if isinstance(stub, list) else [stub]
    default = task['oracle']['package']
    return [(Path(oracles) / task['id'] / (e if isinstance(e, str) else e['name']),
             default if isinstance(e, str) else e['package']) for e in entries]


def case_package(task: dict, case: dict) -> str:
    return case.get('package', task['oracle']['package'])


def packages(task: dict) -> list:
    """Every package the oracle compiles: oracle.package first, then the others in file order."""
    found = [task['oracle']['package']]
    for entry in task['oracle']['files']:
        if not isinstance(entry, str) and entry['package'] not in found:
            found.append(entry['package'])
    return found


def present_packages(tree: Path, task: dict) -> list:
    """The oracle packages that exist in tree: a package the feature adds is absent from base, so the
    warm-ups and the self-test compile only the others there."""
    return [package for package in packages(task) if (Path(tree) / package).is_dir()]


def install_oracle(tree: Path, task: dict, oracles: Path = ORACLES) -> list:
    """Copy the oracle files into tree/<package> without their suffix; refuse to overwrite a file."""
    installed = []
    for source, package in file_specs(task, oracles):
        target = Path(tree) / package / source.name[:-len(SUFFIX)]
        # A package the feature adds is missing from base and from a candidate that never made it.
        target.parent.mkdir(parents=True, exist_ok=True)
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
    """Compile each oracle package's test binary once, then run each case alone. A package whose build
    fails fails its own cases (build_failed). wrap(argv, cwd) returns the argv that actually runs
    (grade_c wraps every run in grader.sb). build is the first failing package's build, else the
    default package's; builds holds every package's."""
    wrap = wrap or (lambda argv, cwd: argv)
    oracle = task['oracle']
    names = [case['name'] for case in oracle['cases']]
    if not names or any(not CASE_NAME.match(name) for name in names):
        raise ValueError('oracle cases must be plain test names')
    known = packages(task)
    if any(case_package(task, case) not in known for case in oracle['cases']):
        raise ValueError('an oracle case names a package no oracle file is in')
    logs = Path(logs)
    logs.mkdir(parents=True, exist_ok=True)
    builds, binaries = {}, {}
    for index, package in enumerate(known):
        binary = Path(env['TMPDIR']) / ('oracle-%d.test' % index)
        log = logs / ('build.log' if index == 0 else 'build-%d.log' % index)
        code, timed_out, seconds = _run(wrap(['go', 'test', '-c', '-o', str(binary), './' + package], tree),
                                        tree, env, log, timeout)
        builds[package] = {'exit_code': code, 'timed_out': timed_out, 'seconds': seconds}
        if not (code or timed_out or not binary.is_file()):
            binaries[package] = binary
    failing = [package for package in known if package not in binaries]
    result = {'build': builds[failing[0] if failing else known[0]], 'builds': builds, 'cases': {}}
    for case in oracle['cases']:
        name, package = case['name'], case_package(task, case)
        if package not in binaries:
            result['cases'][name] = 'build_failed'
            continue
        log = logs / (name + '.log')
        argv = [str(binaries[package]), '-test.run', '^' + name + '$', '-test.count=1', '-test.v',
                '-test.timeout=' + str(int(oracle.get('case_timeout', 900))) + 's']
        cwd = Path(tree) / package
        code, timed_out, _ = _run(wrap(argv, cwd), cwd, env, log, oracle.get('case_timeout', 900) + 60)
        result['cases'][name] = case_verdict(log.read_text(errors='replace'), name, code, timed_out)
    for binary in binaries.values():
        binary.unlink(missing_ok=True)
    return result


def frozen_hash(tasks: list, oracles: Path = ORACLES) -> str:
    """One hash over the task definitions and the bytes of every oracle file they name."""
    import hashlib
    import json
    digest = hashlib.sha256(json.dumps(tasks, sort_keys=True).encode())
    for task in sorted(tasks, key=lambda t: t['id']):
        for path in oracle_files(task, oracles) + [p for p, _ in stub_specs(task, oracles)]:
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
