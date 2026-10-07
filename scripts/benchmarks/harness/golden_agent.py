"""The confined agent step of a golden-mode trial (SPEC-HARNEVAL-001 REQ-HE-08).

The agent process gets exactly the grader allowlist environment (`env -i` plus allowlist) with the
codex and system directories on PATH and the maintainer's credentials; codex passes only the
allowlisted names to the commands it runs, under the pilot Seatbelt permission profile. When the agent
exits, times out or the runner is interrupted, every process of the agent's session is killed.
"""
import json
import os
from pathlib import Path
import signal
import subprocess
import time

import grader
from permissions import profile_args

# The agent process environment keys; codex passes exactly these names to the commands it runs.
AGENT_KEYS = ('PATH', 'HOME', 'TMPDIR', 'GOPATH', 'GOCACHE', 'GOMODCACHE', 'GOFLAGS', 'GOPROXY', 'GOSUMDB',
              'GOTOOLCHAIN')
SYSTEM_PATH = ('/usr/bin', '/bin', '/usr/sbin', '/sbin')
DISABLED_FEATURES = ('multi_agent', 'multi_agent_v2', 'memories', 'hooks', 'apps', 'browser_use')
EFFORT = 'medium'


def prompt_text(corpus: dict) -> str:
    """The pilot task prompt: corpus prompt, allowed files, single-agent rules and the oracle command."""
    return (corpus['prompt'] + '\n\nAllowed production files: ' + ', '.join(corpus['allowed_paths']) + '.\n'
            'Use a single agent; do not delegate. Fix the behavior and verify it. Read only this workspace and '
            'installed toolchain files; do not inspect parent directories, other trials, or benchmark '
            'implementation. Existing tests and harness files are immutable. You may add regression tests. '
            'Do not commit or use network. Finish without requesting clarification.\n'
            'Focused acceptance command: ' + ' '.join(corpus['oracle']['command']))


def agent_env(root: Path, prepared: dict, codex: str, credentials: dict) -> dict:
    """The whole agent process environment (`env -i` plus allowlist): the grader allowlist of root with the
    codex and system directories on PATH, plus the credentials, which exist in this environment only."""
    env = grader.allowlist(Path(root), Path(prepared['go']), Path(prepared['modcache']))
    env['PATH'] = os.pathsep.join(dict.fromkeys([str(Path(codex).parent), env['PATH'], *SYSTEM_PATH]))
    clash = sorted(set(credentials) & set(env))
    if clash:
        raise ValueError('a credential may not replace an allowlisted key: ' + ', '.join(clash))
    return {**env, **credentials}


def agent_argv(codex: str, model: str, root: Path, modcache: Path) -> list:
    """codex exec under the pilot permission profile; commands inherit only AGENT_KEYS, never a credential.

    --strict-config makes codex refuse an override key it does not recognize instead of ignoring it.
    """
    root = Path(root)
    policy = ['-c', 'shell_environment_policy.inherit="all"',
              '-c', 'shell_environment_policy.include_only=' + json.dumps(list(AGENT_KEYS))]
    return [codex, 'exec', '--ephemeral', '--ignore-user-config', '--strict-config',
            *(arg for feature in DISABLED_FEATURES for arg in ('--disable', feature)),
            *profile_args(root / 'ws', root / 'gocache', root / 'tmp', extra_reads=[modcache]), *policy,
            '-m', model, '-c', f'model_reasoning_effort="{EFFORT}"', '-c', 'approval_policy="never"',
            '--skip-git-repo-check', '-C', str(root / 'ws'), '--json', '-']


def _exited(pid: int, seconds: float) -> bool:
    """Wait for the child to exit without reaping it, so its pid and session id stay reserved meanwhile."""
    deadline = time.monotonic() + seconds
    while os.waitid(os.P_PID, pid, os.WEXITED | os.WNOHANG | os.WNOWAIT) is None:
        if time.monotonic() >= deadline:
            return False
        time.sleep(0.05)
    return True


def _members(session: int) -> list:
    """Live (non-zombie) processes whose session is `session`, its leader excluded."""
    listing = subprocess.run(['/bin/ps', '-A', '-o', 'pid=,stat='], capture_output=True, text=True, check=True)
    members = []
    for line in listing.stdout.splitlines():
        fields = line.split()
        if len(fields) < 2 or fields[1].startswith('Z') or int(fields[0]) == session:
            continue
        try:
            if os.getsid(int(fields[0])) == session:
                members.append(int(fields[0]))
        except OSError:
            continue
    return members


def sweep(session: int, rounds: int = 40) -> tuple:
    """SIGKILL every live member of the session, which codex's per-command process groups stay in.

    Returns (stragglers, leftover): whether any member was found, and whether one survived the kills.
    """
    found = False
    for _ in range(rounds):
        members = _members(session)
        if not members:
            return found, False
        found = True
        for pid in members:
            try:
                os.kill(pid, signal.SIGKILL)
            except (ProcessLookupError, PermissionError):
                pass
        time.sleep(0.05)
    return found, bool(_members(session))


def _end(pid: int) -> None:
    """Stop a running agent: TERM, then KILL, its process group, waiting without reaping it."""
    for sig, grace in ((signal.SIGTERM, 5), (signal.SIGKILL, 30)):
        try:
            os.killpg(pid, sig)
        except (ProcessLookupError, PermissionError):
            pass
        if _exited(pid, grace):
            return


def run_agent(argv: list, cwd: Path, env: dict, prompt: str, events: Path, stderr: Path, timeout: float) -> dict:
    """Run the agent in a new session with exactly `env`, the prompt on stdin, then empty its session.

    An interrupt of the runner (Ctrl-C, or SIGTERM and SIGHUP through golden.main) ends the agent's
    session before it propagates, so no agent keeps spending model quota after the runner stopped.
    """
    started = time.monotonic()
    try:
        with open(events, 'wb') as out, open(stderr, 'wb') as err:
            process = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.PIPE, stdout=out, stderr=err,
                                       start_new_session=True)
    except OSError as error:
        return {'launched': False, 'exit_code': None, 'timed_out': False, 'stragglers': False, 'leftover': False,
                'duration_s': 0.0, 'error': type(error).__name__}
    try:
        try:
            process.stdin.write(prompt.encode())
            process.stdin.close()
        except BrokenPipeError:
            pass
        timed_out = not _exited(process.pid, timeout)
        if timed_out:
            _end(process.pid)
    except BaseException:
        _end(process.pid)
        sweep(process.pid)
        process.wait()
        raise
    stragglers, leftover = sweep(process.pid)
    process.wait()
    return {'launched': True, 'exit_code': process.returncode, 'timed_out': timed_out, 'stragglers': stragglers,
            'leftover': leftover, 'duration_s': round(time.monotonic() - started, 3),
            'transcript_bytes': Path(events).stat().st_size}
