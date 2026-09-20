"""Create isolated, seeded benchmark workspaces and audit their edits."""

import hashlib
import os
import shutil
import subprocess
import tarfile
import tempfile
from pathlib import Path


HARNESS_DIRS = {'.codex', '.claude', '.agents', '.opencode', '.omp', '.gemini', '.autopus'}
HARNESS_FILES = {'AGENTS.md', 'CLAUDE.md', 'GEMINI.md', 'opencode.json', 'autopus.yaml'}
CACHE_DIRS = {'.git', '__pycache__', '.pytest_cache', '.mypy_cache', '.ruff_cache'}


def _relative(root: Path, name: str) -> Path:
    relative = Path(name)
    if relative.is_absolute() or '..' in relative.parts or not relative.parts:
        raise ValueError('expected a project-relative path')
    path = root / relative
    for parent in [path, *path.parents]:
        if parent == root:
            break
        if parent.is_symlink():
            raise ValueError('symlink path is not an editable benchmark input')
    return path


def snapshot(repo: Path, revision: str, destination: Path) -> None:
    """Export an exact commit without copying Git history or ambient harnesses."""
    if destination.exists() and any(destination.iterdir()):
        raise ValueError('snapshot destination must be empty')
    commit = subprocess.check_output(
        ['git', 'rev-parse', '--verify', '--end-of-options', revision + '^{commit}'],
        cwd=repo, text=True,
    ).strip()
    destination.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryFile() as archive:
        subprocess.run(['git', 'archive', '--format=tar', commit], cwd=repo, stdout=archive, check=True)
        archive.seek(0)
        with tarfile.open(fileobj=archive, mode='r:') as contents:
            members = contents.getmembers()
            for member in members:
                parts = Path(member.name).parts
                if Path(member.name).is_absolute() or '..' in parts:
                    raise ValueError('archive path escapes workspace')
            contents.extractall(destination, members=members, filter='data')
    for name in HARNESS_DIRS | HARNESS_FILES:
        path = destination / name
        if path.is_symlink() or path.is_file():
            path.unlink()
        elif path.is_dir():
            shutil.rmtree(path)


def apply_mutation(root: Path, task: dict) -> None:
    mutation = task['mutation']
    path = _relative(root, mutation['path'])
    original = path.read_text()
    before = mutation['before']
    if not before or original.count(before) != 1:
        raise ValueError('mutation source must match exactly once')
    path.write_text(original.replace(before, mutation['after'], 1))


def install_surface(root: Path, surface: Path) -> None:
    """Copy instructions, skills and common Codex config, never runtime hooks."""
    for name in ['AGENTS.md', '.codex/config.toml', '.codex/skills']:
        source = _relative(surface, name)
        if not source.exists():
            continue
        destination = _relative(root, name)
        destination.parent.mkdir(parents=True, exist_ok=True)
        if source.is_dir():
            for entry in source.rglob('*'):
                if entry.is_symlink():
                    raise ValueError('surface symlinks are not supported')
            shutil.copytree(source, destination, dirs_exist_ok=True)
        else:
            shutil.copy2(source, destination)


def hashes(root: Path) -> dict:
    """Hash bytes, executable permissions and symlink targets; never follow links."""
    result = {}
    for directory, dirs, files in os.walk(root, followlinks=False):
        base = Path(directory)
        dirs[:] = sorted(name for name in dirs if name not in CACHE_DIRS)
        links = [name for name in dirs if (base / name).is_symlink()]
        dirs[:] = [name for name in dirs if name not in links]
        for name in sorted(files + links):
            path = base / name
            key = path.relative_to(root).as_posix()
            if path.is_symlink():
                payload = b'link\0' + os.fsencode(os.readlink(path))
            elif path.is_file():
                payload = str(path.stat().st_mode & 0o111).encode() + b'\0' + path.read_bytes()
            else:
                payload = b'special\0' + str(path.lstat().st_mode).encode()
            result[key] = hashlib.sha256(payload).hexdigest()
    return result


def audit(before: dict, root: Path, allowed_paths: list) -> dict:
    """Protect all existing non-owned files, including immutable test oracles."""
    after = hashes(root)
    allowed = set(allowed_paths)
    changed = sorted(path for path in before.keys() | after.keys() if before.get(path) != after.get(path))
    protected = [path for path in changed if path in before and path not in allowed]
    unexpected = [path for path in changed if path not in before and
                  (not path.endswith('_test.go') or (root / path).is_symlink())]
    return {'changed_paths': changed, 'protected_changes': protected,
            'unexpected_paths': unexpected, 'accepted_scope': not protected and not unexpected}


def initialize(root: Path) -> None:
    """Commit only seeded files in a fresh object database with hooks disabled."""
    if (root / '.git').exists() or (root / '.git').is_symlink():
        raise ValueError('refusing to reuse a Git history')
    environment = {key: value for key, value in os.environ.items() if not key.startswith('GIT_')}
    environment.update(GIT_AUTHOR_NAME='Harness Benchmark', GIT_AUTHOR_EMAIL='benchmark@localhost',
                       GIT_COMMITTER_NAME='Harness Benchmark', GIT_COMMITTER_EMAIL='benchmark@localhost',
                       GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull)
    git = ['git', '-c', 'core.hooksPath=' + os.devnull, '-c', 'commit.gpgsign=false']
    for args in [['init', '--quiet', '--template=', '--initial-branch=benchmark'],
                 ['add', '--all', '--force'], ['commit', '--quiet', '--allow-empty', '-m', 'Seed benchmark workspace']]:
        subprocess.run(git + args, cwd=root, env=environment, check=True, capture_output=True)
