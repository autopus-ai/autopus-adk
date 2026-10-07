"""Sandboxed oracle runs and the trusted test2json parser (SPEC-HARNEVAL-001 REQ-HE-08).

A grading run executes one corpus oracle from <root>/ws as
`env -i <allowlist> sandbox-exec -f grader.sb -D GRADE_ROOT=<root> -D MODCACHE=<module cache>
-D GOROOT=<toolchain> -D ACCOUNT_HOME=<home> -D CODEX_HOME_DIR=<codex home> go test -json ...`.
The profile denies all network and Mach lookups, every write outside the fresh grade root and
every file read below a home or a temporary root except the grade root, the module cache and the
toolchain; the allowlist is the whole environment, so no credential, GITHUB_*, ACTIONS_* or
RUNNER_* key reaches the oracle. Every process of the run is capped in CPU seconds, file size and
the account's process count. The trusted runner, not the oracle, writes the captured stdout (at
most 1 MiB) and the parser below judges only that file; stderr is kept to 1 MiB and stamped when cut.
"""
import json
import os
from pathlib import Path
import pwd
import resource
import shutil
import signal
import subprocess
import sys
import threading
import time

from observe import reject_json_constant, unique_object

HERE = Path(__file__).resolve().parent
PROFILE = HERE / 'grader.sb'
SANDBOX = '/usr/bin/sandbox-exec'
OUTPUT_LIMIT = 1024 * 1024
STDERR_STAMP = b'\n[grader.py: stderr cut at 1048576 bytes]\n'
# Per-process caps of a sandboxed run (soft = hard, so the run cannot raise them): CPU seconds,
# bytes per written file, and processes of the account beyond those running at launch.
FILE_SIZE_LIMIT = 1 << 30
PROCESS_HEADROOM = 512
FIELDS = {'Time': str, 'Action': str, 'Package': str, 'Test': str, 'Elapsed': (int, float), 'Output': str,
          'OutputType': str, 'FailedBuild': str, 'ImportPath': str}
ACTIONS = {'start', 'run', 'pause', 'cont', 'pass', 'bench', 'fail', 'output', 'skip', 'build-output', 'build-fail'}


def allowlist(root: Path, go: Path, modcache: Path, proxy: str = 'off') -> dict:
    """The whole grader environment; no inherited key survives `env -i`."""
    return {'PATH': str(go.parent), 'HOME': str(root / 'home'), 'TMPDIR': str(root / 'tmp'),
            'GOPATH': str(root / 'gopath'), 'GOCACHE': str(root / 'gocache'), 'GOMODCACHE': str(modcache),
            'GOFLAGS': '-mod=mod', 'GOPROXY': proxy, 'GOSUMDB': 'off', 'GOTOOLCHAIN': 'local'}


def credential_roots() -> tuple:
    """The canonical account home and Codex home whose credential stores grader.sb keeps unreadable.

    The account home comes from the password database, not $HOME, and the Codex home is $CODEX_HOME or
    its default <home>/.codex, the location the agent step authenticates from.
    """
    account = os.path.realpath(pwd.getpwuid(os.getuid()).pw_dir)
    return account, os.path.realpath(os.environ.get('CODEX_HOME') or os.path.join(account, '.codex'))


def toolchain() -> tuple:
    """The canonical go binary on PATH and its GOROOT, asked of that binary; None when go is absent."""
    found = shutil.which('go')
    if not found:
        return None
    go = Path(os.path.realpath(found))
    goroot = subprocess.run([str(go), 'env', 'GOROOT'], env={'PATH': str(go.parent), 'GOTOOLCHAIN': 'local'},
                            capture_output=True, text=True, timeout=60, check=True).stdout.strip()
    return go, Path(os.path.realpath(goroot))


def _canonical_dir(path, role: str) -> Path:
    path = Path(path)
    if not path.is_absolute() or not path.is_dir() or path.resolve() != path:
        raise ValueError(role + ' must be an existing canonical directory: ' + str(path))
    return path


def sandbox_argv(root: Path, modcache: Path, goroot: Path, env: dict, command: list,
                 profile: Path = PROFILE) -> list:
    """`env -i <env> sandbox-exec -f grader.sb ...` for one command that may write only below root.

    grader.sb opens file reads below the homes and temporary roots only for root, the module cache
    and the toolchain, so none of the three may be or contain a home or a protected path; the
    root additionally may not contain the module cache, the toolchain or the runner checkout.
    """
    root = _canonical_dir(root, 'sandbox root')
    modcache, goroot = _canonical_dir(modcache, 'module cache'), _canonical_dir(goroot, 'toolchain root')
    account, codex_home = credential_roots()
    homes = [Path(account), Path(codex_home), Path.home(), Path('/Users'), Path('/private/var/folders'),
             Path('/private/tmp'), Path('/private/var/tmp')]
    for path, role in ((root, 'sandbox root'), (modcache, 'module cache'), (goroot, 'toolchain root')):
        if any(home == path or path in home.parents for home in homes):
            raise ValueError(role + ' must not be or contain a home or temporary root: ' + str(path))
    if any(other == root or root in other.parents for other in (modcache, goroot, HERE.parents[2])):
        raise ValueError('sandbox root must not contain the caches, toolchain or checkout: ' + str(root))
    return ['/usr/bin/env', '-i', *(key + '=' + value for key, value in env.items()), SANDBOX, '-f', str(profile),
            '-D', 'GRADE_ROOT=' + str(root), '-D', 'MODCACHE=' + str(modcache), '-D', 'GOROOT=' + str(goroot),
            '-D', 'ACCOUNT_HOME=' + account, '-D', 'CODEX_HOME_DIR=' + codex_home, *command]


def grader_argv(root: Path, command: list, prepared: dict, profile: Path = PROFILE) -> list:
    """The `env -i` allowlist plus sandbox-exec argv for one oracle run in root/ws."""
    root, go = Path(root), Path(prepared['go'])
    if Path(prepared['warm_cache']) == root or root in Path(prepared['warm_cache']).parents:
        raise ValueError('grade root must not contain the warm build cache: ' + str(root))
    tool_flags = ('-json', '-exec', '-toolexec')
    if list(command[:2]) != ['go', 'test'] or any(arg.split('=')[0] in tool_flags for arg in command):
        raise ValueError('oracle must be a plain go test command')
    if not prepared.get('goroot'):
        raise ValueError('the prepared caches name no toolchain root')
    # PWD lets go find its working directory without listing the denied directories above the root.
    env = {**allowlist(root, go, Path(prepared['modcache'])), 'PWD': str(root / 'ws')}
    return sandbox_argv(root, Path(prepared['modcache']), Path(prepared['goroot']), env,
                        [str(go), 'test', '-json', *command[2:]], profile)


def _kill_group(group: int) -> bool:
    """SIGKILL whatever is left in the process group; False when no live member is left.

    XNU skips zombie members and answers EPERM, not ESRCH, while a group holds only zombies.
    """
    try:
        os.killpg(group, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        return False
    return True


def _group_alive(group: int, seconds: float = 2.0) -> bool:
    """True when the group still has members, zombies included, after `seconds`."""
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            os.killpg(group, 0)
        except ProcessLookupError:
            return False
        except PermissionError:
            pass
        time.sleep(0.05)
    return True


def _account_processes() -> int:
    """Processes of this real user id, zombies included: the count RLIMIT_NPROC is checked against."""
    listing = subprocess.run(['/bin/ps', '-A', '-o', 'ruid='], capture_output=True, text=True, timeout=30, check=True)
    return sum(1 for line in listing.stdout.split() if line == str(os.getuid()))


def run_limits(timeout: float) -> dict:
    """The rlimits every process of a sandboxed run gets: soft equals hard, so the run cannot raise them.

    CPU seconds are twice the wall timeout, a written file is at most FILE_SIZE_LIMIT bytes, and on
    macOS the account may run PROCESS_HEADROOM processes beyond those alive now. RLIMIT_NPROC counts
    every process of the user id, so a fixed cap would refuse the oracle any fork on a busy host or
    bound nothing.
    """
    limits = {resource.RLIMIT_CPU: max(60, int(2 * timeout)), resource.RLIMIT_FSIZE: FILE_SIZE_LIMIT}
    if sys.platform == 'darwin':
        # XNU counts processes per real user id; Linux counts threads, which `ps -A` does not list.
        limits[resource.RLIMIT_NPROC] = _account_processes() + PROCESS_HEADROOM
    capped = {}
    for kind, value in limits.items():
        hard = resource.getrlimit(kind)[1]
        capped[kind] = value if hard == resource.RLIM_INFINITY else min(value, hard)
    return capped


def run_sandboxed(argv: list, cwd: Path, stdout_path: Path, stderr_path: Path, timeout: float) -> dict:
    """Run in a new process group under run_limits(), keep at most OUTPUT_LIMIT bytes per stream (a cut
    stderr ends with STDERR_STAMP), then empty the group.

    `stragglers` reports processes still in the group after the oracle exited (they are killed);
    `leftover` reports any that survived the kill or kept an output pipe open.
    """
    started, limits = time.monotonic(), run_limits(timeout)

    def confine():
        for kind, value in limits.items():
            resource.setrlimit(kind, (value, value))
    process = subprocess.Popen(argv, cwd=cwd, env={}, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, start_new_session=True, preexec_fn=confine)
    overflow = threading.Event()

    def pump(stream, path, guard):
        kept, cut = 0, False
        with stream, open(path, 'wb') as sink:
            for chunk in iter(lambda: stream.read(65536), b''):
                sink.write(chunk[:OUTPUT_LIMIT - kept])
                cut = cut or kept + len(chunk) > OUTPUT_LIMIT
                if cut and guard and not overflow.is_set():
                    overflow.set()
                    _kill_group(process.pid)
                kept = min(OUTPUT_LIMIT, kept + len(chunk))
            if cut and not guard:
                sink.write(STDERR_STAMP)
    pumps = [threading.Thread(target=pump, args=(process.stdout, stdout_path, True), daemon=True),
             threading.Thread(target=pump, args=(process.stderr, stderr_path, False), daemon=True)]
    for thread in pumps:
        thread.start()
    try:
        process.wait(timeout=timeout)
        timed_out = False
    except subprocess.TimeoutExpired:
        timed_out = True
        _kill_group(process.pid)
        process.wait()
    stragglers = _kill_group(process.pid)
    for thread in pumps:
        thread.join(timeout=5)
    leftover = _group_alive(process.pid) or any(thread.is_alive() for thread in pumps)
    return {'exit_code': process.returncode, 'timed_out': timed_out, 'overflow': overflow.is_set(),
            'stragglers': stragglers, 'leftover': leftover, 'duration_s': round(time.monotonic() - started, 3)}


def parse_events(stdout: bytes) -> list | None:
    """Strict test2json decode: every line is one known event with typed known fields, else None."""
    if len(stdout) > OUTPUT_LIMIT or (stdout and not stdout.endswith(b'\n')):
        return None
    events = []
    for line in stdout.split(b'\n')[:-1] if stdout else []:
        try:
            event = json.loads(line, object_pairs_hook=unique_object, parse_constant=reject_json_constant)
        except (ValueError, RecursionError):
            return None
        if not isinstance(event, dict) or any(
                key not in FIELDS or isinstance(value, bool) or not isinstance(value, FIELDS[key])
                for key, value in event.items()) or event.get('Action') not in ACTIONS:
            return None
        events.append(event)
    return events


def judge(stdout: bytes, exit_code: int, timed_out: bool, overflow: bool, expected_tests: list) -> dict:
    """The record signal and oracle{ran, build_failed, expected_passed, expected_failed} of one run.

    `accepted` needs all four REQ-HE-08 conditions: exit 0, valid events within the limit, a top-level
    pass for every expected test and no fail for any of them. `ran` is true only when an expected test
    has a run, pass, fail or skip event, so a build failure alone never counts as a run.
    """
    expected = set(expected_tests)
    if not expected:
        raise ValueError('expected_tests must not be empty')
    events = None if overflow else parse_events(stdout)
    seen, passed, failed, build_failed = set(), set(), set(), False
    for event in events or []:
        action, test = event['Action'], event.get('Test')
        build_failed = build_failed or action == 'build-fail' or bool(event.get('FailedBuild'))
        if test in expected and action in ('run', 'pass', 'fail', 'skip'):
            seen.add(test)
            {'pass': passed, 'fail': failed}.get(action, set()).add(test)
    oracle = {'ran': bool(seen), 'build_failed': build_failed, 'expected_passed': len(passed),
              'expected_failed': len(failed)}
    if timed_out:
        verdict = 'oracle_timeout'
    elif events is None:
        verdict = 'oracle_output_invalid'
    elif exit_code != 0 or failed:
        verdict = 'oracle_failed'
    else:
        verdict = 'accepted' if passed == expected else 'oracle_output_invalid'
    return {'signal': verdict, 'oracle': oracle}


def grade(root: Path, command: list, expected_tests: list, prepared: dict, stdout_path: Path, stderr_path: Path,
          timeout: float = 300, profile: Path = PROFILE) -> dict:
    """One sandboxed oracle run from root/ws, judged by the trusted parser from the captured stdout."""
    run = run_sandboxed(grader_argv(root, command, prepared, profile), Path(root) / 'ws', stdout_path, stderr_path,
                        timeout)
    unsettled = run['overflow'] or run['leftover']
    return {**run, **judge(Path(stdout_path).read_bytes(), run['exit_code'], run['timed_out'], unsettled,
                           expected_tests)}
