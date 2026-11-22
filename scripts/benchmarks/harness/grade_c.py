"""Pilot C grading: a base snapshot overlaid with the candidate's production edits, the hidden oracle,
and every build and case run sandboxed under grader.sb (the golden lane's profile).

The graded tree is the task's base snapshot plus, from the candidate workspace, only regular non-test
files inside allowed_paths (added or changed) and the deletion of base files there. Tests come from
base, the harness surface and everything outside allowed_paths never enter, and a symlink is
rejected. The grade root lies in the run output, outside every trial, and gets its own clone of the
grading build cache, which no agent can read.
"""
import fnmatch
import os
from pathlib import Path
import shutil
import subprocess

import grader
import oracle_c


def clone_tree(source: Path, dest: Path) -> None:
    """Copy a directory tree, as an APFS clone when the file system can (cp -c), else a plain copy."""
    if subprocess.run(['/bin/cp', '-c', '-R', str(source), str(dest)], capture_output=True).returncode:
        shutil.rmtree(dest, ignore_errors=True)
        shutil.copytree(source, dest, symlinks=True)


def _allowed(rel: str, allowed: list) -> bool:
    return any(fnmatch.fnmatchcase(rel, glob) for glob in allowed)


def overlay(base: Path, work: Path, allowed: list, dest: Path) -> dict:
    """dest = base + the candidate's regular non-test files under allowed_paths; see module doc."""
    shutil.copytree(base, dest, symlinks=True)
    record = {'copied': [], 'deleted': [], 'rejected_symlinks': []}
    for directory, dirs, files in os.walk(work):
        dirs[:] = [d for d in dirs if d != '.git']
        for name in files + [d for d in dirs if (Path(directory) / d).is_symlink()]:
            path = Path(directory) / name
            rel = path.relative_to(work).as_posix()
            if rel.endswith('_test.go') or oracle_c.owned(rel) or not _allowed(rel, allowed):
                continue
            if path.is_symlink():
                record['rejected_symlinks'].append(rel)
                continue
            target = dest / rel
            if target.is_file() and target.read_bytes() == path.read_bytes():
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            if target.is_symlink() or target.exists():
                target.unlink()
            shutil.copyfile(path, target)
            record['copied'].append(rel)
    for path in sorted(Path(base).rglob('*')):
        rel = path.relative_to(base).as_posix()
        if (path.is_file() and not rel.endswith('_test.go') and _allowed(rel, allowed)
                and not (Path(work) / rel).exists() and not (Path(work) / rel).is_symlink()):
            (dest / rel).unlink()
            record['deleted'].append(rel)
    return record


def sandbox_env(root: Path, prepared: dict) -> dict:
    """grader.allowlist with /usr/bin and /bin added (the oracles run git and sh)."""
    env = grader.allowlist(root, Path(prepared['go']), Path(prepared['modcache']))
    env['PATH'] = os.pathsep.join([env['PATH'], '/usr/bin', '/bin'])
    env.update(GOTELEMETRY='off', LANG='C.UTF-8', GIT_CONFIG_NOSYSTEM='1')
    return env


def toolchain() -> dict:
    found = grader.toolchain()
    if not found:
        raise RuntimeError('go is not on PATH')
    go, goroot = found
    modcache = subprocess.check_output([str(go), 'env', 'GOMODCACHE'], text=True).strip()
    return {'go': str(go), 'goroot': str(goroot), 'modcache': os.path.realpath(modcache)}


def warm(base_tree: Path, packages: list, cache: Path, scratch: Path) -> None:
    """Fill a build cache by compiling the tests of the task packages on a trusted base snapshot."""
    prepared = toolchain()
    scratch.mkdir(parents=True, exist_ok=True)
    env = {'PATH': os.pathsep.join([str(Path(prepared['go']).parent), '/usr/bin', '/bin']), 'HOME': str(scratch),
           'TMPDIR': str(scratch), 'GOTMPDIR': str(scratch), 'GOCACHE': str(cache), 'GOMODCACHE': prepared['modcache'],
           'GOFLAGS': '-mod=mod', 'GOPROXY': 'off', 'GOSUMDB': 'off', 'GOTOOLCHAIN': 'local', 'GOTELEMETRY': 'off'}
    cache.mkdir(parents=True, exist_ok=True)
    for package in sorted(set(packages)):
        subprocess.run([prepared['go'], 'test', '-count=1', '-run', '^$', './' + package], cwd=base_tree, env=env,
                       check=True, capture_output=True, timeout=3600)


def run_graded(tree: Path, task: dict, root: Path, logs: Path, cache: Path, profile: Path,
               oracles: Path) -> dict:
    """Install the oracle into tree (below root) and run it under grader.sb with a clone of cache."""
    root = Path(os.path.realpath(root))
    for name in ('home', 'tmp', 'gopath'):
        (root / name).mkdir(parents=True, exist_ok=True)
    grader.telemetry_off(root / 'home')
    clone_tree(cache, root / 'gocache')
    oracle_c.install_oracle(tree, task, oracles)
    prepared = toolchain()
    env = sandbox_env(root, prepared)

    def wrap(argv: list, cwd: Path) -> list:
        command = [prepared['go'], *argv[1:]] if argv[0] == 'go' else argv
        return grader.sandbox_argv(root, Path(prepared['modcache']), Path(prepared['goroot']),
                                   {**env, 'PWD': str(cwd)}, command, profile)
    return oracle_c.run_oracle(tree, task, logs, env, wrap=wrap)


def grade_candidate(task: dict, base_tree: Path, work: Path, root: Path, logs: Path, cache: Path,
                    profile: Path, oracles: Path) -> dict:
    """Overlay, sandboxed oracle, and the pass counts by case kind."""
    tree = Path(root) / 'tree'
    record = overlay(base_tree, work, task['allowed_paths'], tree)
    result = run_graded(tree, task, root, logs, cache, profile, oracles)
    kinds = {case['name']: case['kind'] for case in task['oracle']['cases']}
    passed = [name for name, verdict in result['cases'].items() if verdict == 'pass']
    counts = {kind + '_' + key: value for kind in ('review', 'feature', 'guard') for key, value in
              (('passed', sum(1 for n in passed if kinds[n] == kind)),
               ('total', sum(1 for k in kinds.values() if k == kind)))}
    return {**result, 'overlay': record, 'passed': len(passed), 'total': len(kinds), **counts}
