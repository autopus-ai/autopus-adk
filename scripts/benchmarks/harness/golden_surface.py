"""Per-revision arm surfaces of the golden mode (SPEC-HARNEVAL-001 REQ-HE-07, T13).

Each arm surface comes from the trusted surface driver built from that arm's own revision. The
revision is extracted with `git archive` (workspace.snapshot) into a fresh build root, and this
checkout's surface_driver/main.go becomes the only file of the driver package in that tree. A trusted
stage outside any sandbox fills a session module cache with the arm's modules, verified by its own
go.sum, from a file proxy over the local module cache (or --proxy). The build then runs under
grader.sb, offline (GOPROXY=off) and without cgo, able to write only its build root:
`go build -trimpath` links pins.generator_version into github.com/insajin/autopus-adk/pkg/version.
The driver runs under grader.sb too, writing the default five-platform surface into its own run root
with every host probe pinned and an empty PATH and HOME, and the runner moves that surface to its
destination. The driver uses only API that v0.50.122 already has. A revision it cannot be extracted
from, built at or run at raises SurfaceError; the runner refuses the session with
baseline_ref_unsupported when that is the baseline arm.
"""
import os
from pathlib import Path
import re
import shutil
import subprocess

import grader
import prepare_grader
import workspace

HERE = Path(__file__).resolve().parent
DRIVER = HERE / 'surface_driver' / 'main.go'
DRIVER_PACKAGE = 'scripts/benchmarks/harness/surface_driver'
VERSION_SYMBOL = 'github.com/insajin/autopus-adk/pkg/version.version'
# A version the go tool passes to the linker as one -X value: no space or quote can split it.
LINKABLE = re.compile(r'^[0-9A-Za-z][0-9A-Za-z.+_-]*$')
BUILD_TIMEOUT, RUN_TIMEOUT = 1800, 300


class SurfaceError(Exception):
    """An arm revision the surface driver cannot be extracted from, built at, or run at."""

    def __init__(self, stage: str, detail: str):
        super().__init__(stage + ': ' + detail)
        self.stage, self.detail = stage, detail


def _tail(text: str, limit: int = 400) -> str:
    return ' '.join((text or '').split())[-limit:]


def _run(command: list, cwd: Path, env: dict, timeout: int, stage: str) -> str:
    """A trusted command outside the sandbox; its stdout, or SurfaceError with the stderr tail."""
    try:
        completed = subprocess.run(command, cwd=cwd, env=env, capture_output=True, text=True, timeout=timeout)
    except (OSError, subprocess.SubprocessError) as error:
        raise SurfaceError(stage, f'{type(error).__name__}: {error}') from error
    if completed.returncode:
        raise SurfaceError(stage, _tail(completed.stderr) or f'exit code {completed.returncode}')
    return completed.stdout


def _sandboxed(root: Path, modcache: Path, goroot: Path, env: dict, command: list, cwd: Path, timeout: int,
               stage: str) -> None:
    """One command under grader.sb that may write only below root, with the grader's rlimits."""
    logs = Path(root).parent
    try:
        argv = grader.sandbox_argv(root, modcache, goroot, env, command)
        run = grader.run_sandboxed(argv, cwd, logs / (stage + '.stdout'), logs / (stage + '.stderr'), timeout)
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        raise SurfaceError(stage, f'{type(error).__name__}: {error}') from error
    if run['exit_code'] != 0 or run['timed_out']:
        stderr = (logs / (stage + '.stderr')).read_text(errors='replace')
        raise SurfaceError(stage, _tail(stderr) or f"exit code {run['exit_code']}, timed out {run['timed_out']}")


def build_command(generator_version: str, binary: Path) -> list:
    """The driver build: trimmed paths and the pinned generator version linked into pkg/version."""
    if not LINKABLE.match(generator_version):
        raise SurfaceError('build', f'generator version {generator_version!r} cannot be linked with -X')
    return ['go', 'build', '-trimpath', f'-ldflags=-X {VERSION_SYMBOL}={generator_version}', '-o', str(binary),
            './' + DRIVER_PACKAGE]


def build_env(root: Path, go: Path, modcache: Path) -> dict:
    """The whole sandboxed build environment: the toolchain alone on PATH, HOME, TMPDIR, GOPATH and GOCACHE
    in the build root, the session module cache, no network and no cgo, held to the arm's own go.mod and
    go.sum: no go.work, no module graph edit, no VCS stamp and no toolchain switch."""
    root = Path(root)
    return {'PATH': str(Path(go).parent), 'HOME': str(root / 'home'), 'TMPDIR': str(root / 'tmp'),
            'GOPATH': str(root / 'gopath'), 'GOCACHE': str(root / 'gocache'), 'GOMODCACHE': str(modcache),
            'GOFLAGS': '-mod=readonly -buildvcs=false', 'GOPROXY': 'off', 'GOSUMDB': 'off', 'GOWORK': 'off',
            'GOTOOLCHAIN': 'local', 'CGO_ENABLED': '0', 'PWD': str(root / 'src')}


def download_modules(tree: Path, modcache: Path, go: Path, scratch: Path, proxy: str | None = None) -> None:
    """Trusted stage: the arm's modules into a new session module cache, from a file proxy over the local
    module cache unless `proxy` names another; the arm's go.sum verifies them and may not change."""
    if proxy is None:
        local = _run([str(go), 'env', 'GOMODCACHE'], tree, dict(os.environ), 60, 'build').strip()
        proxy = Path(local, 'cache', 'download').as_uri()
    sums = Path(tree) / 'go.sum'
    pinned = sums.read_bytes() if sums.is_file() else b''
    for path in (modcache, scratch / 'home', scratch / 'gopath', scratch / 'gocache'):
        path.mkdir(parents=True, exist_ok=True)
    env = {'PATH': str(go.parent), 'HOME': str(scratch / 'home'), 'GOPATH': str(scratch / 'gopath'),
           'GOCACHE': str(scratch / 'gocache'), 'GOMODCACHE': str(modcache), 'GOPROXY': proxy, 'GOSUMDB': 'off',
           'GOFLAGS': '-mod=mod', 'GOWORK': 'off', 'GOTOOLCHAIN': 'local', 'CGO_ENABLED': '0'}
    try:
        _run([str(go), 'mod', 'download'], tree, env, BUILD_TIMEOUT, 'build')
    except SurfaceError as error:
        raise SurfaceError('build', 'module download (pass --proxy when the local module cache lacks a module): '
                           + error.detail) from error
    if (sums.read_bytes() if sums.is_file() else b'') != pinned:
        raise SurfaceError('build', 'go.sum changed during the module download')


def build_driver(root: Path, generator_version: str, modcache: Path, scratch: Path, proxy: str | None = None) -> Path:
    """Make this checkout's driver the only file of the driver package in root/src, then build it there
    under grader.sb into root/surface_driver; the session module cache is filled first, outside it."""
    root = Path(root).resolve()
    tree, binary = root / 'src', root / 'surface_driver'
    command = build_command(generator_version, binary)
    try:
        go, goroot = grader.toolchain() or (None, None)
    except (OSError, subprocess.SubprocessError) as error:
        raise SurfaceError('build', f'go env GOROOT: {type(error).__name__}') from error
    if go is None:
        raise SurfaceError('build', 'go toolchain not found')
    try:
        package = workspace._relative(tree, DRIVER_PACKAGE)
        if package.is_dir():
            shutil.rmtree(package)
        elif package.exists():
            package.unlink()
        package.mkdir(parents=True)
        shutil.copyfile(DRIVER, package / 'main.go')
        for name in ('home', 'tmp', 'gopath', 'gocache'):
            (root / name).mkdir(exist_ok=True)
    except (OSError, ValueError) as error:
        raise SurfaceError('build', f'driver package: {error}') from error
    download_modules(tree, Path(modcache), go, Path(scratch), proxy)
    _sandboxed(root, Path(modcache).resolve(), goroot, build_env(root, go, Path(modcache).resolve()),
               [str(go), *command[1:]], tree, BUILD_TIMEOUT, 'build')
    return binary


def run_driver(binary: Path, destination: Path, pins: dict, catalog: Path | None, root: Path, modcache: Path) -> None:
    """Write the default surface under grader.sb into root/surface, then move it to the new destination.
    PATH and HOME are empty directories, so no host CLI can answer a probe the pins do not cover; the
    driver binary and the codex model catalog are copied into root, the only tree the run may read."""
    root = Path(root)
    home, path, tmp = (root / name for name in ('home', 'path', 'tmp'))
    try:
        for directory in (home, path, tmp, Path(destination).parent):
            directory.mkdir(parents=True, exist_ok=True)
        shutil.copy2(binary, root / 'surface_driver')
        if catalog:
            shutil.copyfile(catalog, root / 'codex-models.json')
        go, goroot = grader.toolchain() or (None, None)
    except (OSError, subprocess.SubprocessError) as error:
        raise SurfaceError('generate', f'run root: {type(error).__name__}: {error}') from error
    root, output = root.resolve(), root.resolve() / 'surface'
    command = [str(root / 'surface_driver'), '--output', str(output), '--project-name', pins['project_name'],
               '--generator-version', pins['generator_version'],
               '--codex-model-catalog', str(root / 'codex-models.json') if catalog else '',
               '--codex-cli-version', pins['codex_cli_version'], '--opencode-cli-version', pins['opencode_cli_version']]
    env = {'PATH': str(root / 'path'), 'HOME': str(root / 'home'), 'TMPDIR': str(root / 'tmp'), 'PWD': str(root / 'tmp')}
    _sandboxed(root, Path(modcache).resolve(), goroot, env, command, root / 'tmp', RUN_TIMEOUT, 'generate')
    try:
        shutil.move(str(output), str(destination))
    except OSError as error:
        raise SurfaceError('generate', f'surface: {error}') from error


def arm_surface(repo: Path, revision: str, pins: dict, set_root: Path, destination: Path, scratch: Path,
                proxy: str | None = None) -> Path:
    """Generate one arm surface into destination with the driver built from `revision` of `repo`.

    `pins` are the candidate manifest's pins for both arms; a codex model catalog path is relative to
    `set_root`, the root holding that manifest. `proxy` is the GOPROXY of the trusted module download.
    The extracted revision and the session module cache are removed once the surface is written.
    """
    scratch = Path(scratch)
    build, modcache = scratch / 'build', scratch / 'modcache'
    try:
        workspace.snapshot(Path(repo), revision, build / 'src')
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        raise SurfaceError('revision', f'{revision} cannot be extracted from {repo} ({type(error).__name__})') from error
    try:
        catalog = None
        if pins.get('codex_model_catalog'):
            catalog = workspace._relative(Path(set_root).resolve(), pins['codex_model_catalog'])
        binary = build_driver(build, pins['generator_version'], modcache, scratch / 'download', proxy)
        run_driver(binary, Path(destination), pins, catalog, scratch / 'run', modcache)
    except ValueError as error:
        raise SurfaceError('build', f'codex model catalog: {error}') from error
    finally:
        for path in (build / 'src', modcache):
            prepare_grader.remove_tree(path)
    return Path(destination)
