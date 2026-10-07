"""Sandboxed oracle runs and the trusted test2json parser (SPEC-HARNEVAL-001 REQ-HE-08).

A grading run executes one corpus oracle from <root>/ws as
`env -i <allowlist> sandbox-exec -f grader.sb -D GRADE_ROOT=<root> -D ACCOUNT_HOME=<home>
-D CODEX_HOME_DIR=<codex home> go test -json ...`.
The profile denies all network, every write outside the fresh grade root and
every read of the account's credential stores; the allowlist is the whole
environment, so no credential, GITHUB_*, ACTIONS_* or RUNNER_* key reaches the
oracle. The trusted runner, not the oracle, writes the captured stdout (at most
1 MiB) and the parser below judges only that file.
"""
import json
import os
from pathlib import Path
import pwd
import signal
import subprocess
import threading
import time

from observe import reject_json_constant, unique_object

HERE = Path(__file__).resolve().parent
PROFILE = HERE / 'grader.sb'
SANDBOX = '/usr/bin/sandbox-exec'
OUTPUT_LIMIT = 1024 * 1024
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


def grader_argv(root: Path, command: list, prepared: dict, profile: Path = PROFILE) -> list:
    """The `env -i` allowlist plus sandbox-exec argv for one oracle run in root/ws."""
    root, go = Path(root), Path(prepared['go'])
    if not root.is_absolute() or not root.is_dir() or root.resolve() != root:
        raise ValueError('grade root must be an existing canonical directory: ' + str(root))
    protected = [Path(prepared['modcache']), Path(prepared['warm_cache']), go.parent.parent, HERE.parents[2],
                 Path.home()]
    if any(path == root or root in path.parents for path in protected):
        raise ValueError('grade root must not contain the caches, toolchain, checkout or home: ' + str(root))
    tool_flags = ('-json', '-exec', '-toolexec')
    if list(command[:2]) != ['go', 'test'] or any(arg.split('=')[0] in tool_flags for arg in command):
        raise ValueError('oracle must be a plain go test command')
    env = allowlist(root, go, Path(prepared['modcache']))
    account, codex_home = credential_roots()
    return ['/usr/bin/env', '-i', *(key + '=' + value for key, value in env.items()), SANDBOX, '-f', str(profile),
            '-D', 'GRADE_ROOT=' + str(root), '-D', 'ACCOUNT_HOME=' + account, '-D', 'CODEX_HOME_DIR=' + codex_home,
            str(go), 'test', '-json', *command[2:]]


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


def run_sandboxed(argv: list, cwd: Path, stdout_path: Path, stderr_path: Path, timeout: float) -> dict:
    """Run in a new process group, keep at most OUTPUT_LIMIT bytes per stream, then empty the group.

    `stragglers` reports processes still in the group after the oracle exited (they are killed);
    `leftover` reports any that survived the kill or kept an output pipe open.
    """
    started = time.monotonic()
    process = subprocess.Popen(argv, cwd=cwd, env={}, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, start_new_session=True)
    overflow = threading.Event()

    def pump(stream, path, guard):
        kept = 0
        with stream, open(path, 'wb') as sink:
            for chunk in iter(lambda: stream.read(65536), b''):
                sink.write(chunk[:OUTPUT_LIMIT - kept])
                if kept + len(chunk) > OUTPUT_LIMIT and guard and not overflow.is_set():
                    overflow.set()
                    _kill_group(process.pid)
                kept = min(OUTPUT_LIMIT, kept + len(chunk))
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
