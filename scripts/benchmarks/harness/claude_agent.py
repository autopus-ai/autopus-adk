"""Claude Code print-mode driver for pilot C: arm settings, isolation, environment and argv.

Credentials. On macOS Claude Code keeps its login in the login keychain under a service name derived
from the configuration directory ("Claude Code-credentials" for the default ~/.claude), and elsewhere
in <config dir>/.credentials.json. A fresh CLAUDE_CONFIG_DIR therefore starts logged out (checked
with `claude auth status`). The isolated home gets no copy of the login; the credential is passed
only as CLAUDE_CODE_OAUTH_TOKEN (a `claude setup-token` token) or ANTHROPIC_API_KEY, read from a file
the maintainer names. claude_agent.sb closes the keychain and every file below the homes and
temporary roots except the trial's own, so a trial cannot fall back to the account login.

What the isolation does not cover: managed (policy) settings and anything the account or
organisation serves remotely (for example organisation skills or plugins) still apply, identically
in every arm; TCP is open, since the model is remote; the system, toolchain and module cache stay
readable. Stream observation lives in stream_c.py.
"""
import json
import os
from pathlib import Path

ARMS = ('native', 'instructions', 'full')
PROFILE = Path(__file__).resolve().parent / 'claude_agent.sb'
SANDBOX = '/usr/bin/sandbox-exec'
SYSTEM_PATH = ('/usr/bin', '/bin', '/usr/sbin', '/sbin')
TOKEN_ENV = {'oauth': 'CLAUDE_CODE_OAUTH_TOKEN', 'api_key': 'ANTHROPIC_API_KEY'}
# The only host keys a trial inherits; everything else (SSH_AUTH_SOCK, other credentials, IDE and
# terminal integrations) is dropped, as with `env -i`.
INHERITED = ('LANG', 'LC_ALL', 'LC_CTYPE', 'USER', 'LOGNAME', 'TERM')
TOOL_SLOTS = 4


def arm_settings(arm: str) -> dict:
    """The settings of one arm. Hooks come only from the generated project surface."""
    if arm not in ARMS:
        raise ValueError('unknown arm: ' + arm)
    settings = {'cleanupPeriodDays': 1, 'includeCoAuthoredBy': False}
    if arm == 'instructions':
        # Reproduces the 2026-09-20 condition: instructions and skills, no enforcement hooks.
        settings['disableAllHooks'] = True
    return settings


def isolated_home(trial: Path, arm: str, workspace: Path) -> dict:
    """A HOME and CLAUDE_CONFIG_DIR below the trial holding no login.

    The arm's settings are also the user settings there: a nested `claude -p` the harness starts
    (pkg/qa/agentexec, for example) reads user settings but not the top-level --settings, so without
    this an instructions trial would run hooks in its nested sessions. The workspace is marked
    trusted, as a user's own checkout is; otherwise the generated permissions are ignored.
    """
    home = Path(trial) / 'home'
    config = home / '.claude'
    config.mkdir(parents=True, exist_ok=False)
    state = {'hasCompletedOnboarding': True, 'bypassPermissionsModeAccepted': True,
             'projects': {str(Path(workspace).resolve()): {'hasTrustDialogAccepted': True}}}
    (config / '.claude.json').write_text(json.dumps(state))
    (config / 'settings.json').write_text(json.dumps(arm_settings(arm)))
    return {'HOME': str(home), 'CLAUDE_CONFIG_DIR': str(config)}


def read_credential(path: Path) -> str:
    """One token from a file only the maintainer can read; refuse a group- or world-readable file."""
    path = Path(path)
    if path.stat().st_mode & 0o077:
        raise ValueError('credential file must be mode 0600: ' + str(path))
    value = path.read_text().strip()
    if not value or any(ch.isspace() for ch in value):
        raise ValueError('credential file must hold exactly one token')
    return value


def agent_env(base: dict, home: dict, temp: Path, credential_kind: str, credential: str, path: list) -> dict:
    """The whole environment of the agent process: an allowlist, never the host's environment."""
    env = {key: base[key] for key in INHERITED if key in base}
    env.update(home)
    env.update(TMPDIR=str(temp), CLAUDE_CODE_TMPDIR=str(temp), SHELL='/bin/zsh',
               PATH=os.pathsep.join([*map(str, path), *SYSTEM_PATH]))
    env[TOKEN_ENV[credential_kind]] = credential
    env.update(DISABLE_AUTOUPDATER='1', CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC='1', DISABLE_TELEMETRY='1',
               DISABLE_ERROR_REPORTING='1')
    return env


def claude_argv(arm: str, model: str, effort: str, permission_mode: str = 'bypassPermissions',
                claude: str = 'claude') -> list:
    """`claude -p` reading the prompt from stdin; flags checked against `claude --help` 2.1.322.

    --settings takes the JSON itself, so no settings file lies where the agent could edit it.
    """
    return [claude, '-p', '--output-format', 'stream-json', '--verbose', '--include-hook-events',
            '--model', model, '--effort', effort, '--permission-mode', permission_mode,
            '--settings', json.dumps(arm_settings(arm), sort_keys=True), '--setting-sources', 'project',
            '--strict-mcp-config', '--no-session-persistence', '--no-chrome',
            '--disallowedTools', 'WebFetch', 'WebSearch']


def tool_dirs(tools: dict) -> list:
    """Install directories to reopen for reading: the directory above each linked tool's binary."""
    dirs = sorted({str(Path(target).parents[1]) for target in tools.values()})
    if len(dirs) > TOOL_SLOTS:
        raise ValueError('at most %d tool install directories' % TOOL_SLOTS)
    return dirs


def sandboxed(argv: list, trial: Path, output: Path, bin_dir: Path, modcache: str, tools: dict,
              account_home: str, protected: list, profile: Path = PROFILE) -> list:
    """Wrap argv in claude_agent.sb (see the profile header)."""
    trial, output = Path(trial).resolve(), Path(output).resolve()
    if output not in trial.parents:
        raise ValueError('trial must live below the output root')
    for path in protected:
        path = Path(path).resolve()
        if path == trial or path in trial.parents:
            raise ValueError('a protected checkout must not contain the trial: ' + str(path))
    dirs = tool_dirs(tools)
    for path in [*map(Path, dirs), Path(modcache)]:
        if any(path == Path(p).resolve() or Path(p).resolve() in path.parents for p in [output, *protected]):
            raise ValueError('a reopened directory lies inside a protected tree: ' + str(path))
    dirs += [str(trial / '.unused-slot')] * (TOOL_SLOTS - len(dirs))
    checkouts = [str(Path(p).resolve()) for p in protected][:2]
    checkouts += [str(trial / '.unused-checkout')] * (2 - len(checkouts))
    params = ['-D', 'TRIAL=' + str(trial), '-D', 'OUTPUT=' + str(output),
              '-D', 'PROTECTED0=' + checkouts[0], '-D', 'PROTECTED1=' + checkouts[1],
              '-D', 'ACCOUNT_HOME=' + os.path.realpath(account_home),
              '-D', 'MODCACHE=' + str(Path(modcache).resolve()), '-D', 'BIN=' + str(Path(bin_dir).resolve())]
    for index, path in enumerate(dirs):
        params += ['-D', 'TOOL%d=%s' % (index, path)]
    return [SANDBOX, '-f', str(profile), *params, *argv]
