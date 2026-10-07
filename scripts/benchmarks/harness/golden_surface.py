"""Per-revision arm surfaces of the golden mode (SPEC-HARNEVAL-001 REQ-HE-07, T13).

Each arm surface comes from the trusted surface driver built from that arm's own revision. The
revision is extracted with `git archive` (workspace.snapshot), this checkout's
surface_driver/main.go becomes the only file of the driver package in that tree, and
`go build -trimpath` links pins.generator_version into github.com/insajin/autopus-adk/pkg/version.
The driver then writes the default five-platform surface with every host probe pinned, under an
empty PATH and HOME. It uses only API that v0.50.122 already has. A revision it cannot be
extracted from, built at or run at raises SurfaceError; the runner refuses the session with
baseline_ref_unsupported when that is the baseline arm.
"""
import os
from pathlib import Path
import re
import shutil
import subprocess

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


def _run(command: list, cwd: Path, env: dict, timeout: int, stage: str) -> None:
    try:
        completed = subprocess.run(command, cwd=cwd, env=env, capture_output=True, text=True, timeout=timeout)
    except (OSError, subprocess.SubprocessError) as error:
        raise SurfaceError(stage, f'{type(error).__name__}: {error}') from error
    if completed.returncode:
        raise SurfaceError(stage, _tail(completed.stderr) or f'exit code {completed.returncode}')


def build_command(generator_version: str, binary: Path) -> list:
    """The driver build: trimmed paths and the pinned generator version linked into pkg/version."""
    if not LINKABLE.match(generator_version):
        raise SurfaceError('build', f'generator version {generator_version!r} cannot be linked with -X')
    return ['go', 'build', '-trimpath', f'-ldflags=-X {VERSION_SYMBOL}={generator_version}', '-o', str(binary),
            './' + DRIVER_PACKAGE]


def build_env(environ: dict) -> dict:
    """The trusted toolchain environment held to the arm's own go.mod and go.sum: no go.work, no module
    graph edit, no VCS stamp from an enclosing repository, and no toolchain switch."""
    return {**environ, 'GOWORK': 'off', 'GOFLAGS': '-mod=readonly -buildvcs=false', 'GOTOOLCHAIN': 'local'}


def build_driver(tree: Path, generator_version: str, binary: Path) -> None:
    """Make this checkout's driver the only file of the driver package in the arm tree, then build it."""
    command = build_command(generator_version, binary)
    try:
        package = workspace._relative(Path(tree), DRIVER_PACKAGE)
        if package.is_dir():
            shutil.rmtree(package)
        elif package.exists():
            package.unlink()
        package.mkdir(parents=True)
        shutil.copyfile(DRIVER, package / 'main.go')
    except (OSError, ValueError) as error:
        raise SurfaceError('build', f'driver package: {error}') from error
    _run(command, Path(tree), build_env(dict(os.environ)), BUILD_TIMEOUT, 'build')


def run_driver(binary: Path, destination: Path, pins: dict, catalog: Path | None, scratch: Path) -> None:
    """Write the default surface into the new destination. PATH and HOME are empty directories, so no
    host CLI can answer a probe the pins do not cover."""
    home, path, tmp = (Path(scratch) / name for name in ('home', 'path', 'tmp'))
    for directory in (home, path, tmp, Path(destination).parent):
        directory.mkdir(parents=True, exist_ok=True)
    command = [str(binary), '--output', str(destination), '--project-name', pins['project_name'],
               '--generator-version', pins['generator_version'], '--codex-model-catalog', str(catalog or ''),
               '--codex-cli-version', pins['codex_cli_version'], '--opencode-cli-version', pins['opencode_cli_version']]
    _run(command, tmp, {'PATH': str(path), 'HOME': str(home), 'TMPDIR': str(tmp)}, RUN_TIMEOUT, 'generate')


def arm_surface(repo: Path, revision: str, pins: dict, set_root: Path, destination: Path, scratch: Path) -> Path:
    """Generate one arm surface into destination with the driver built from `revision` of `repo`.

    `pins` are the candidate manifest's pins for both arms; a codex model catalog path is relative to
    `set_root`, the root holding that manifest. The extracted revision is removed after the build.
    """
    scratch, tree = Path(scratch), Path(scratch) / 'src'
    try:
        workspace.snapshot(Path(repo), revision, tree)
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        raise SurfaceError('revision', f'{revision} cannot be extracted from {repo} ({type(error).__name__})') from error
    try:
        catalog = None
        if pins.get('codex_model_catalog'):
            catalog = workspace._relative(Path(set_root).resolve(), pins['codex_model_catalog'])
        build_driver(tree, pins['generator_version'], scratch / 'surface_driver')
    except ValueError as error:
        raise SurfaceError('build', f'codex model catalog: {error}') from error
    finally:
        shutil.rmtree(tree, ignore_errors=True)
    run_driver(scratch / 'surface_driver', Path(destination), pins, catalog, scratch / 'run')
    return Path(destination)
