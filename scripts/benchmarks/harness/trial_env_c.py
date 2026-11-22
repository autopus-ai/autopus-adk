"""Pilot C trial environment: module caches, the temp locations of xcrun and zsh, and the dry-run self-test.

Module cache. The host GOMODCACHE stays outside every stage. Once per run, output/cache/modcache is
filled from the host cache's download directory (GOPROXY=file://, so nothing is fetched) with what the
harness snapshot and every task base need to build and test. Each trial gets its own APFS clone of it
below the trial (trial/gomodcache), which only that trial's sandbox may read or write, so a stat-cache
or lock file the go command writes (go build stamps the main module's pseudo-version from VCS and
records it there) reaches no other trial. Grading and the warm build caches read the run-level cache,
which no agent can open. A clone costs metadata only on APFS.

xcrun and zsh. /usr/bin/git, /usr/bin/cc and the other developer shims run xcrun, which keeps its
lookup database in confstr(_CS_DARWIN_USER_TEMP_DIR) (/var/folders/.../T), not $TMPDIR, unless
the environment variable xcrun_db names an absolute path. claude_agent.sb denies writes there, so every
trial sets xcrun_db inside its own temp. zsh writes here-document files to $TMPPREFIX (default
/tmp/zsh), which the trial sets inside its temp too; bash uses $TMPDIR.
"""
import os
from pathlib import Path
import re
import shutil
import subprocess
import time

ENV_OVERRIDE = re.compile(r'(?<![\w$])(GOCACHE|TMPDIR|CGO_ENABLED|GOMODCACHE|GOFLAGS)=')
DENIAL = re.compile(r'operation not permitted|couldn.t create cache file|can.t create temp file', re.I)


def temp_env(temp: Path) -> dict:
    """Where xcrun keeps its database and zsh its here-documents: inside the stage's own temp."""
    return {'xcrun_db': str(Path(temp) / 'xcrun_db'), 'TMPPREFIX': str(Path(temp) / 'zsh')}


def host_modcache(base_env: dict) -> str:
    """The canonical host GOMODCACHE, asked of the go command under the runner's own environment."""
    out = subprocess.check_output(['go', 'env', 'GOMODCACHE'], env=base_env, text=True).strip()
    return os.path.realpath(out)


def populate_modcache(tree: Path, dest: Path, host: str, base_env: dict) -> None:
    """Add to dest what tree needs to build and test, read from host's download cache; never the network.

    -mod=readonly: a tree whose go.mod would have to change is an error here, not a silent edit.
    """
    dest = Path(dest)
    scratch = dest.parent / 'modcache-tmp'
    scratch.mkdir(parents=True, exist_ok=True)
    go = shutil.which('go', path=base_env.get('PATH', ''))
    if not go:
        raise RuntimeError('go is not on PATH')
    env = {'PATH': os.pathsep.join([str(Path(os.path.realpath(go)).parent), '/usr/bin', '/bin']),
           'HOME': str(scratch), 'TMPDIR': str(scratch), 'GOTMPDIR': str(scratch), 'GOMODCACHE': str(dest),
           'GOPROXY': 'file://' + str(Path(host) / 'cache' / 'download'), 'GOSUMDB': 'off',
           'GOFLAGS': '-mod=readonly', 'GOTOOLCHAIN': 'local', 'GOTELEMETRY': 'off',
           'GOCACHE': str(scratch / 'gocache'), **temp_env(scratch)}
    for argv in (['go', 'mod', 'download'], ['go', 'list', '-deps', '-test', './...']):
        result = subprocess.run(argv, cwd=tree, env=env, capture_output=True, text=True, timeout=1800)
        if result.returncode:
            raise RuntimeError('%s failed in %s:\n%s' % (' '.join(argv), tree, result.stderr[-4000:]))


def clone_modcache(source: Path, dest: Path) -> dict:
    """An APFS clone (cp -c) of the run-level module cache; a plain copy where cloning is unsupported."""
    started = time.monotonic()
    cloned = subprocess.run(['/bin/cp', '-c', '-R', str(source), str(dest)], capture_output=True).returncode == 0
    if not cloned:
        shutil.rmtree(dest, ignore_errors=True)
        shutil.copytree(source, dest, symlinks=True)
    return {'apfs_clone': cloned, 'seconds': round(time.monotonic() - started, 3)}


def env_overrides(command: str) -> list:
    """The Go and temp variables a Bash tool command assigns (export X=, X=... cmd, go env -w X=)."""
    return sorted(set(ENV_OVERRIDE.findall(command)))


def selftest_steps(shell: str, package: str) -> list:
    """(name, argv, expected stdout fragment) of the no-model environment self-test."""
    return [
        ('go_build', ['go', 'build', './...'], None),
        ('go_test_compile', ['go', 'test', '-count=1', '-run', '^$', './' + package], None),
        ('zsh_heredoc', [shell, '-c', 'cat <<EOF\nzsh-heredoc-ok\nEOF'], 'zsh-heredoc-ok'),
        ('bash_heredoc', ['/bin/bash', '-c', 'cat <<EOF\nbash-heredoc-ok\nEOF'], 'bash-heredoc-ok'),
        ('git_status', ['git', 'status', '--short'], None),
        ('cc_compile', ['/bin/sh', '-c', 'printf "int selftest;\\n" | cc -x c -c -o "$TMPDIR/selftest.o" -'], None),
        ('go_env', ['go', 'env', 'GOCACHE', 'GOMODCACHE', 'GOTMPDIR', 'CGO_ENABLED'], None),
    ]


def judge(name: str, result: dict, out: str, err: str, expected: str | None, env: dict) -> list:
    """Why one self-test step failed; empty when it passed."""
    problems = []
    if result['exit_code'] or result['timed_out']:
        problems.append('exit %s%s' % (result['exit_code'], ' (timed out)' if result['timed_out'] else ''))
    if DENIAL.search(out + err):
        problems.append('sandbox denial: ' + DENIAL.search(out + err).group(0))
    if expected and expected not in out:
        problems.append('missing output ' + expected)
    if name == 'go_env':
        want = [env['GOCACHE'], env['GOMODCACHE'], env['GOTMPDIR'], '1']
        if out.split('\n')[:4] != want:
            problems.append('go env %r, want %r' % (out.split('\n')[:4], want))
    return problems


def selftest(plan: dict, package: str, execute) -> dict:
    """Run every step inside the trial's sandbox (plan['wrap']) with the trial's environment.

    execute is trial_c.execute_tree. Logs go to measure/selftest/<step>.{out,err}.
    """
    logs = plan['measure'] / 'selftest'
    logs.mkdir(parents=True, exist_ok=True)
    steps, failed = {}, []
    for name, argv, expected in selftest_steps(plan['env']['SHELL'], package):
        out_path, err_path = logs / (name + '.out'), logs / (name + '.err')
        result = execute(plan['wrap'](argv), plan['work'], plan['env'], out_path, err_path, 1800,
                         scratch=plan['trial'] if plan['sandboxed'] else None)
        out, err = out_path.read_text(errors='replace'), err_path.read_text(errors='replace')
        problems = judge(name, result, out, err, expected, plan['env'])
        steps[name] = {'seconds': result['elapsed_seconds'], 'ok': not problems, 'problems': problems,
                       'stderr_tail': err[-600:] if problems else ''}
        if problems:
            failed.append(name)
    return {'ok': not failed, 'failed': failed, 'steps': steps}
