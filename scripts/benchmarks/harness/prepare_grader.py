"""Trusted grader preparation and two-direction oracle calibration (SPEC-HARNEVAL-001 REQ-HE-09).

The trusted stage runs outside any sandbox and before any agent. It fills a
session module cache from the workspace go.mod and go.sum, requires every
downloaded hash to be pinned in go.sum, freezes the cache read-only and
compiles the oracle packages into a warm build cache. Each grading run then
gets a fresh grade root holding clonefile (APFS) copies of the snapshot and
of the warm cache, so no run sees what another run wrote. Calibration grades
every agent task twice under grader.sb: clean must be accepted, mutated must
not. A failed calibration refuses the session before any agent call.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys

from grader import HERE, PROFILE, allowlist, grade
from workspace import apply_mutation, snapshot


class PrepareError(Exception):
    """The trusted stage could not build the read-only module cache or the warm build cache."""


def _go(go: Path, args: list, cwd: Path, env: dict) -> subprocess.CompletedProcess:
    try:
        return subprocess.run([str(go), *args], cwd=cwd, env=env, capture_output=True, text=True, timeout=1800)
    except subprocess.TimeoutExpired as error:
        raise PrepareError('go ' + args[0] + ' timed out') from error


def _tail(*texts: str) -> str:
    return ' '.join(' '.join(texts).split())[-600:]


def _modules(stdout: str) -> list:
    """Decode the concatenated JSON objects that `go mod download -json` prints."""
    decoder, position, modules = json.JSONDecoder(), 0, []
    while True:
        while position < len(stdout) and stdout[position].isspace():
            position += 1
        if position == len(stdout):
            return modules
        module, position = decoder.raw_decode(stdout, position)
        modules.append(module)


def unpinned(modules: list, go_sum: str) -> list:
    """path@version of every downloaded go.mod or zip whose hash go.sum does not pin."""
    pinned = {tuple(line.split()) for line in go_sum.splitlines()}
    missing = []
    for module in modules:
        name, version = module.get('Path', ''), module.get('Version', '')
        wanted = [(name, version + '/go.mod', module.get('GoModSum'))]
        if module.get('Zip'):
            wanted.append((name, version, module.get('Sum')))
        if 'Error' in module or any(entry not in pinned for entry in wanted):
            missing.append(name + '@' + version)
    return sorted(missing)


def set_writable(root: Path, writable: bool) -> None:
    """Add (owner rwx on directories) or drop every write bit below root; symlinks are never followed."""
    for directory, dirs, files in os.walk(root):
        for path in [directory] + [os.path.join(directory, name) for name in dirs + files]:
            if os.path.islink(path):
                continue
            mode = os.lstat(path).st_mode & 0o7777
            grant = 0o700 if os.path.isdir(path) else 0o200
            os.chmod(path, mode | grant if writable else mode & ~0o222)


def remove_tree(path: Path) -> None:
    """Delete a tree even where the frozen cache or the grader dropped write permission.

    A plain rmtree comes first: a grade root holds thousands of clones and is normally writable.
    """
    if not os.path.lexists(path):
        return
    try:
        shutil.rmtree(path)
    except OSError:
        set_writable(path, True)
        shutil.rmtree(path)


def prepare(source: Path, directory: Path, packages: list, proxy: str | None = None) -> dict:
    """Trusted stage: go.sum-verified read-only module cache, then the warm build cache.

    `directory` must be new. `proxy` defaults to a file proxy over the local module cache, so the
    download needs no network; any GOPROXY works because go.sum, not the source, is trusted.
    """
    found = shutil.which('go')
    if not found:
        raise PrepareError('go toolchain not found')
    go, source = Path(os.path.realpath(found)), Path(source).resolve()
    Path(directory).mkdir(parents=True)
    directory = Path(directory).resolve()
    modcache, warm = directory / 'modcache', directory / 'warm'
    for path in (modcache, warm / 'home', warm / 'tmp', warm / 'gopath', warm / 'gocache'):
        path.mkdir(parents=True)
    if proxy is None:
        local = subprocess.check_output([str(go), 'env', 'GOMODCACHE'], text=True).strip()
        proxy = Path(local, 'cache', 'download').as_uri()
    sums = source / 'go.sum'
    go_sum = sums.read_text() if sums.is_file() else ''
    download = _go(go, ['mod', 'download', '-json'], source, allowlist(warm, go, modcache, proxy))
    try:
        modules = _modules(download.stdout)
    except ValueError as error:
        raise PrepareError('go mod download -json printed invalid JSON') from error
    if download.returncode:
        raise PrepareError('go mod download failed: ' + _tail(download.stderr, *(m.get('Error', '') for m in modules)))
    missing = unpinned(modules, go_sum)
    if missing:
        raise PrepareError('module hash not pinned in go.sum: ' + ', '.join(missing))
    if (sums.read_text() if sums.is_file() else '') != go_sum:
        raise PrepareError('go.sum changed during the module download')
    env = allowlist(warm, go, modcache)
    verified = _go(go, ['mod', 'verify'], source, env)
    if verified.returncode:
        raise PrepareError('go mod verify failed: ' + _tail(verified.stdout, verified.stderr))
    set_writable(modcache, False)
    warmed = _go(go, ['test', '-count=1', '-run', '^$', *packages], source, env)
    if warmed.returncode:
        raise PrepareError('warm build failed: ' + _tail(warmed.stdout, warmed.stderr))
    return {'go': str(go), 'go_version': _go(go, ['version'], source, env).stdout.strip(),
            'modcache': str(modcache), 'warm_cache': str(warm / 'gocache'), 'modules': len(modules)}


def new_grade(root: Path, source: Path, warm_cache: Path) -> Path:
    """Create a fresh grade root: clonefile copies (cp -c) of the snapshot and of the warm build cache."""
    Path(root).mkdir(parents=True)
    root = Path(root).resolve()
    for origin, name in ((source, 'ws'), (warm_cache, 'gocache')):
        subprocess.run(['cp', '-c', '-R', str(origin), str(root / name)], check=True)
    for name in ('home', 'tmp', 'gopath'):
        (root / name).mkdir()
    return root


def calibrate(tasks: list, source: Path, prepared: dict, scratch: Path, logs: Path, timeout: float = 300,
              profile: Path = PROFILE) -> dict:
    """Grade every task clean and mutated; `passed` only when all clean runs accept and no mutated run does.

    Each run gets its own grade root under `scratch`, removed afterwards; the captured stdout and
    stderr stay in `logs`. `runs` holds the per-run signals for diagnosis; calibration.json does not.
    """
    Path(logs).mkdir(parents=True, exist_ok=True)
    rows, runs = [], []
    for task in sorted(tasks, key=lambda item: item['id']):
        row = {'task_id': task['id']}
        for direction in ('clean', 'mutated'):
            name = task['id'] + '-' + direction
            root = new_grade(Path(scratch) / name, source, Path(prepared['warm_cache']))
            try:
                if direction == 'mutated':
                    apply_mutation(root / 'ws', task['corpus'])
                result = grade(root, task['corpus']['oracle']['command'], task['expected_tests'], prepared,
                               Path(logs) / (name + '.jsonl'), Path(logs) / (name + '.stderr'), timeout, profile)
            finally:
                remove_tree(root)
            row[direction + '_accepted'] = result['signal'] == 'accepted'
            runs.append({'task_id': task['id'], 'direction': direction, **result})
        rows.append(row)
    passed = bool(rows) and all(row['clean_accepted'] and not row['mutated_accepted'] for row in rows)
    return {'status': 'passed' if passed else 'failed', 'tasks': rows, 'runs': runs}


def _section(result: dict) -> dict:
    return {'status': result['status'], 'tasks': result['tasks']}


def write_calibration(path: Path, session_id: str, before: dict) -> None:
    """Create harness_golden_calibration.v1 (calibration.json) exclusively with the `before` results."""
    with open(path, 'x') as sink:
        sink.write(json.dumps({'session_id': session_id, 'before': _section(before)}, indent=2) + '\n')


def record_after(path: Path, after: dict) -> None:
    """Add the session-end recalibration once, replacing calibration.json atomically."""
    document = json.loads(Path(path).read_text())
    if 'after' in document:
        raise ValueError('calibration after is already recorded')
    document['after'] = _section(after)
    temporary = Path(str(path) + '.tmp')
    temporary.write_text(json.dumps(document, indent=2) + '\n')
    os.replace(temporary, path)


def agent_tasks(checkout: Path) -> tuple:
    """The manifest and its active agent tasks resolved to their digest-pinned corpus entries."""
    manifest = json.loads((Path(checkout) / 'evals/harness/manifest.json').read_text())
    found, corpora = {}, {}
    for active in manifest['active_paths']:
        for path in sorted((Path(checkout) / active).rglob('*.json')):
            task = json.loads(path.read_text())
            if path.is_symlink() or task['id'] in found:
                raise ValueError('symlinked or duplicate task: ' + str(path))
            if task['kind'] != 'agent' or task['status']['state'] != 'active':
                continue
            ref = task['corpus_ref']
            if ref['file'] not in corpora:
                data = (Path(checkout) / ref['file']).read_bytes()
                corpora[ref['file']] = hashlib.sha256(data).hexdigest(), {row['id']: row for row in json.loads(data)}
            digest, entries = corpora[ref['file']]
            if digest != ref['file_sha256']:
                raise ValueError('corpus_digest_mismatch: ' + task['id'])
            if not task.get('expected_tests'):
                raise ValueError('expected_tests_missing: ' + task['id'])
            found[task['id']] = {'id': task['id'], 'expected_tests': list(task['expected_tests']),
                                 'corpus': entries[ref['task_id']], 'corpus_ref': dict(ref)}
    return manifest, [found[key] for key in sorted(found)]


def main(argv: list | None = None) -> int:
    parser = argparse.ArgumentParser(description='Prepare the grader caches and calibrate every agent oracle.')
    parser.add_argument('--repo', type=Path, required=True, help='Git repository holding the workspace revision')
    parser.add_argument('--output', type=Path, required=True, help='new directory for snapshot, caches, results')
    parser.add_argument('--revision', help='workspace revision (default: manifest live.workspace_revision)')
    parser.add_argument('--proxy', help='GOPROXY for the trusted download (default: local module cache file proxy)')
    parser.add_argument('--timeout', type=float, default=300, help='seconds per oracle run')
    args = parser.parse_args(argv)
    if sys.platform != 'darwin':
        print('unsupported_os: grader.sb is a Seatbelt profile', file=sys.stderr)
        return 1
    manifest, tasks = agent_tasks(HERE.parents[2])
    out = args.output.resolve()
    if out.exists() and any(out.iterdir()):
        print('refusing to reuse a non-empty output directory', file=sys.stderr)
        return 1
    snapshot(args.repo, args.revision or manifest['live']['workspace_revision'], out / 'source')
    packages = sorted({arg for task in tasks for arg in task['corpus']['oracle']['command'] if arg.startswith('./')})
    try:
        prepared = prepare(out / 'source', out / 'grader', packages, args.proxy)
        before = calibrate(tasks, out / 'source', prepared, out / 'grade', out / 'logs', args.timeout)
    except PrepareError as error:
        prepared = {'error': str(error)}
        before = {'status': 'failed', 'tasks': [{'task_id': task['id'], 'clean_accepted': False,
                                                  'mutated_accepted': False} for task in tasks]}
    write_calibration(out / 'calibration.json', secrets.token_hex(16), before)
    print(json.dumps({'prepared': prepared, **before}, indent=2))
    if before['status'] != 'passed':
        failed = [row['task_id'] for row in before['tasks'] if not row['clean_accepted'] or row['mutated_accepted']]
        print('oracle_calibration_failed: ' + (', '.join(failed) or 'no active agent task'), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
