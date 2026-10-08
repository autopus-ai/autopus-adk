"""Sibling-process stages of a black-box trial (SPEC-HARNEVAL-003 REQ-HR-08, T9 and T13).

The trusted runner starts each stage itself, one after the other, as a separate process under its
own profile; no stage nests a sandbox (sandbox_apply is refused inside a restricted profile):

    build:  env -i <env> sandbox-exec -f artifact.sb -D MODE=build ... go build -o <bin>/artifact <pkg>
    run:    env -i <env> sandbox-exec -f artifact.sb -D MODE=run ... <artifact> <args>
    oracle: env -i <env> sandbox-exec -f oracle.sb ... harneval-oracle --task <id> ... < bundle

Every stage runs in a new session with the grader's hard rlimits and an allowlist environment;
stdout is kept to 1 MiB (more ends the stage), stderr to 1 MiB with a stamp. When the stage ends
or times out, every process left in its session is killed, and `leftover` reports any that
survived or still held an output pipe: the runner then starts no later stage.
"""
import os
from pathlib import Path
import resource
import subprocess
import threading
import time

import grader
from golden_agent import _exited, sweep

HERE = Path(__file__).resolve().parent
CHECKOUT = HERE.parents[2]
ARTIFACT_PROFILE = HERE / 'artifact.sb'
ORACLE_PROFILE = HERE / 'oracle.sb'
SYSTEM_PATH = '/usr/bin:/bin:/usr/sbin:/sbin'
BUILD_TIMEOUT, ARTIFACT_TIMEOUT, ORACLE_TIMEOUT = 900, 120, 60


def guard(set_root: Path) -> dict:
    """The protected roots both profiles close entirely: checkout, golden set root, Codex home, keychains."""
    account, codex_home = grader.credential_roots()
    return {'CHECKOUT': str(CHECKOUT), 'SET_ROOT': os.path.realpath(set_root), 'ACCOUNT_HOME': account,
            'CODEX_HOME_DIR': codex_home}


def _protected(params: dict, extra: tuple = ()) -> list:
    return [Path(params['ACCOUNT_HOME']), Path(params['CODEX_HOME_DIR']), Path.home(), Path(params['CHECKOUT']),
            Path(params['SET_ROOT']), Path('/Users'), Path('/private/var/folders'), Path('/private/tmp'),
            Path('/private/var/tmp'), *map(Path, extra)]


def check_roots(roots: dict, params: dict, extra: tuple = ()) -> None:
    """Every root a profile opens must be an existing canonical directory that neither is nor contains a
    home, a temporary root, the checkout, the golden set root or another protected path."""
    protected = _protected(params, extra)
    for role, value in roots.items():
        path = Path(value)
        if not path.is_absolute() or not path.is_dir() or Path(os.path.realpath(path)) != path:
            raise ValueError(role + ' must be an existing canonical directory: ' + str(path))
        if path == Path('/') or any(other == path or path in other.parents for other in protected):
            raise ValueError(role + ' must not be or contain a protected path: ' + str(path))


def _canonical_file(path: Path, role: str) -> str:
    path = Path(path)
    if not path.is_absolute() or not path.is_file() or Path(os.path.realpath(path)) != path:
        raise ValueError(role + ' must be an existing canonical file: ' + str(path))
    return str(path)


def _argv(env: dict, profile: Path, params: dict, command: list) -> list:
    return ['/usr/bin/env', '-i', *(key + '=' + str(value) for key, value in env.items()), grader.SANDBOX, '-f',
            str(profile), *(item for key, value in params.items() for item in ('-D', key + '=' + str(value))),
            *map(str, command)]


def build_argv(prepared: dict, build_root: Path, package: str, params: dict, extra: tuple = ()) -> list:
    """artifact.sb build mode for <build_root>: ws/ is the read-only grade copy, gocache/ the trial build
    cache, bin/ the artifact directory and scratch/ HOME, TMPDIR and GOPATH; offline, no cgo."""
    root, go = Path(build_root), Path(prepared['go'])
    roots = {'SCRATCH': root / 'scratch', 'INPUT': root / 'ws', 'BUILD_CACHE': root / 'gocache',
             'OUTPUT': root / 'bin'}
    check_roots(roots, params, extra)
    tools = {'GOROOT': Path(prepared['goroot']), 'MODCACHE': Path(prepared['modcache'])}
    check_roots(tools, params)
    env = {'PATH': str(go.parent), 'HOME': root / 'scratch' / 'home', 'TMPDIR': root / 'scratch' / 'tmp',
           'GOPATH': root / 'scratch' / 'gopath', 'GOCACHE': root / 'gocache', 'GOMODCACHE': tools['MODCACHE'],
           'GOFLAGS': '-mod=readonly -buildvcs=false', 'GOPROXY': 'off', 'GOSUMDB': 'off', 'GOWORK': 'off',
           'GOTOOLCHAIN': 'local', 'CGO_ENABLED': '0', 'TZ': 'UTC', 'PWD': root / 'ws'}
    return _argv(env, ARTIFACT_PROFILE, {'MODE': 'build', **roots, **tools, **params},
                 [go, 'build', '-trimpath', '-o', root / 'bin' / 'artifact', package])


def run_argv(artifact: Path, input_dir: Path, run_root: Path, args: list, params: dict, extra: tuple = ()) -> list:
    """artifact.sb run mode: <run_root> holds out/ (the output root and working directory), home/ and tmp/;
    the trial input directory is read-only; the environment is PATH, HOME, TMPDIR, TZ and PWD only."""
    roots = {'SCRATCH': Path(run_root), 'INPUT': Path(input_dir)}
    check_roots(roots, params, extra)
    binary = _canonical_file(artifact, 'artifact')
    env = {'PATH': SYSTEM_PATH, 'HOME': Path(run_root) / 'home', 'TMPDIR': Path(run_root) / 'tmp', 'TZ': 'UTC',
           'PWD': Path(run_root) / 'out'}
    return _argv(env, ARTIFACT_PROFILE, {'MODE': 'run', **roots, 'ARTIFACT': binary, **params}, args)


def oracle_argv(oracle: Path, output_root: Path, result_dir: Path, task_id: str, params: dict) -> list:
    """oracle.sb for the harness: it reads the output root (whatever the artifact left there) and writes
    only the fresh result directory; the expected outputs come on stdin."""
    check_roots({'RESULT_DIR': Path(result_dir)}, params)
    output = Path(output_root)
    if not output.is_absolute() or os.path.normpath(output) != str(output) or \
            any(other == output or output in other.parents for other in _protected(params)):
        raise ValueError('output root must be a normalized path outside the protected roots: ' + str(output))
    binary = _canonical_file(oracle, 'oracle harness')
    env = {'PATH': SYSTEM_PATH, 'HOME': result_dir, 'TMPDIR': result_dir, 'TZ': 'UTC'}
    params = {'ORACLE': binary, 'OUTPUT_ROOT': output, 'RESULT_DIR': result_dir, **params}
    return _argv(env, ORACLE_PROFILE, params,
                 [binary, '--task', task_id, '--outputs', output, '--result', result_dir])


def _pump(stream, path: Path, guard_stdout: bool, overflow: threading.Event, group: int) -> None:
    kept, cut = 0, False
    with stream, open(path, 'wb') as sink:
        for chunk in iter(lambda: stream.read(65536), b''):
            sink.write(chunk[:grader.OUTPUT_LIMIT - kept])
            cut = cut or kept + len(chunk) > grader.OUTPUT_LIMIT
            if cut and guard_stdout and not overflow.is_set():
                overflow.set()
                grader._kill_group(group)
            kept = min(grader.OUTPUT_LIMIT, kept + len(chunk))
        if cut and not guard_stdout:
            sink.write(grader.STDERR_STAMP)


def _feed(pipe, data: bytes) -> None:
    try:
        pipe.write(data)
        pipe.close()
    except (BrokenPipeError, OSError):
        pass


def run_stage(argv: list, cwd: Path, stdout_path: Path, stderr_path: Path, timeout: float,
              stdin: Path | bytes | None = None) -> dict:
    """Run one stage and empty its session. `stdin` is an input file, bytes piped by the runner, or nothing."""
    limits, started = grader.run_limits(timeout), time.monotonic()

    def confine():
        for kind, value in limits.items():
            resource.setrlimit(kind, (value, value))
    source = open(stdin, 'rb') if isinstance(stdin, Path) else (subprocess.PIPE if stdin is not None else
                                                                subprocess.DEVNULL)
    try:
        process = subprocess.Popen(argv, cwd=cwd, env={}, stdin=source, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, start_new_session=True, preexec_fn=confine)
    except OSError as error:
        return {'launched': False, 'exit_code': None, 'timed_out': False, 'overflow': False, 'leftover': False,
                'duration_s': 0.0, 'error': type(error).__name__}
    finally:
        if isinstance(stdin, Path):
            source.close()
    overflow = threading.Event()
    threads = [threading.Thread(target=_pump, args=(process.stdout, stdout_path, True, overflow, process.pid),
                                daemon=True),
               threading.Thread(target=_pump, args=(process.stderr, stderr_path, False, overflow, process.pid),
                                daemon=True)]
    if isinstance(stdin, bytes):
        threads.append(threading.Thread(target=_feed, args=(process.stdin, stdin), daemon=True))
    for thread in threads:
        thread.start()
    try:
        timed_out = not _exited(process.pid, timeout)
        if timed_out:
            grader._kill_group(process.pid)
    finally:
        stragglers, leftover = sweep(process.pid)
        grader._kill_group(process.pid)
        for thread in threads[:2]:
            thread.join(timeout=5)
        process.wait()
    leftover = leftover or any(thread.is_alive() for thread in threads[:2])
    return {'launched': True, 'exit_code': process.returncode, 'timed_out': timed_out,
            'overflow': overflow.is_set(), 'stragglers': stragglers, 'leftover': leftover,
            'duration_s': round(time.monotonic() - started, 3)}


# The runner environment keys the trusted oracle build keeps: the toolchain and its caches, never a
# credential or an Actions token that the golden step's environment may hold.
BUILD_KEYS = ('PATH', 'HOME', 'TMPDIR', 'GOROOT', 'GOPATH', 'GOCACHE', 'GOMODCACHE', 'GOFLAGS', 'GOPROXY',
              'GOSUMDB', 'GOTOOLCHAIN')


def build_oracle(directory: Path) -> Path:
    """Build the trusted oracle harness from the runner checkout, outside any sandbox."""
    target = Path(directory).resolve() / 'harneval-oracle'
    env = {key: os.environ[key] for key in BUILD_KEYS if key in os.environ}
    subprocess.run(['go', 'build', '-trimpath', '-o', str(target), './cmd/harneval-oracle'], cwd=CHECKOUT, check=True,
                   capture_output=True, timeout=900, env={**env, 'CGO_ENABLED': '0'})
    return target


def warm_build(prepared: dict, source: Path, packages: list) -> None:
    """Trusted stage: compile each black-box package once into the warm build cache with the trial build's
    flags, so every trial build clone starts warm; outside any sandbox and before any agent."""
    warm = Path(prepared['warm_cache']).parent
    env = {**grader.allowlist(warm, Path(prepared['go']), Path(prepared['modcache'])),
           'GOFLAGS': '-mod=readonly -buildvcs=false', 'GOWORK': 'off', 'CGO_ENABLED': '0', 'TZ': 'UTC'}
    for package in packages:
        subprocess.run([prepared['go'], 'build', '-trimpath', '-o', os.devnull, package], cwd=source, env=env,
                       check=True, capture_output=True, timeout=1800)

