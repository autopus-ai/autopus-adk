"""Claude Code print-mode driver for pilot C: settings, isolation, argv and stream observation.

Credentials. On macOS Claude Code keeps its login in the login keychain under a service name derived
from the configuration directory ("Claude Code-credentials" for the default ~/.claude), and elsewhere
in <config dir>/.credentials.json. A fresh CLAUDE_CONFIG_DIR therefore starts logged out (checked
with `claude auth status`). The isolated home gets no copy of the login; the credential is passed
only as CLAUDE_CODE_OAUTH_TOKEN (a `claude setup-token` token) or ANTHROPIC_API_KEY, read from a file
the maintainer names. claude_agent.sb denies the keychain service and the account's ~/.claude, so a
trial cannot fall back to the account login either.

What the isolation does not cover: managed (policy) settings and anything the account or
organisation serves remotely (for example organisation skills or plugins) still apply, identically
in every arm; the network is open, since the model is remote; the trial can read the system and
the Go toolchain and module cache.
"""
import json
import os
from pathlib import Path

from observe import count, reject_json_constant, unique_object

ARMS = ('native', 'instructions', 'full')
PROFILE = Path(__file__).resolve().parent / 'claude_agent.sb'
SANDBOX = '/usr/bin/sandbox-exec'
MAX_LINE_BYTES = 8 * 1024 * 1024
SYSTEM_PATH = ('/usr/bin', '/bin', '/usr/sbin', '/sbin')
TOKEN_ENV = {'oauth': 'CLAUDE_CODE_OAUTH_TOKEN', 'api_key': 'ANTHROPIC_API_KEY'}
# Environment keys a trial never inherits: other credentials and the host's Claude Code state.
SCRUBBED_PREFIXES = ('ANTHROPIC_', 'CLAUDE_', 'CLAUDECODE', 'GITHUB_', 'GH_', 'OPENAI_', 'CODEX_', 'GIT_')


def arm_settings(arm: str) -> dict:
    """The --settings document of one arm. Hooks come only from the generated project surface."""
    if arm not in ARMS:
        raise ValueError('unknown arm: ' + arm)
    settings = {'cleanupPeriodDays': 1, 'includeCoAuthoredBy': False}
    if arm == 'instructions':
        # Reproduces the 2026-09-20 condition: instructions and skills, no enforcement hooks.
        settings['disableAllHooks'] = True
    return settings


def isolated_home(trial: Path) -> dict:
    """Create a HOME and CLAUDE_CONFIG_DIR below the trial holding no login, only onboarding state."""
    home = Path(trial) / 'home'
    config = home / '.claude'
    config.mkdir(parents=True, exist_ok=False)
    state = {'hasCompletedOnboarding': True, 'bypassPermissionsModeAccepted': True}
    (config / '.claude.json').write_text(json.dumps(state))
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


def agent_env(base: dict, home: dict, credential_kind: str, credential: str, path_prefix: list = ()) -> dict:
    """The full environment of the agent process: the host's minus scrubbed keys, plus isolation."""
    env = {k: v for k, v in base.items() if not k.startswith(SCRUBBED_PREFIXES)}
    env.update(home)
    env[TOKEN_ENV[credential_kind]] = credential
    env.update(DISABLE_AUTOUPDATER='1', CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC='1', DISABLE_TELEMETRY='1',
               DISABLE_ERROR_REPORTING='1')
    if path_prefix:
        # The host PATH is replaced, not extended: other agent CLIs on it (codex, gemini, ...) would let
        # a review step spend another account or be detected as platforms.
        env['PATH'] = os.pathsep.join([*map(str, path_prefix), *SYSTEM_PATH])
    return env


def claude_argv(settings_path: Path, model: str, effort: str, permission_mode: str = 'bypassPermissions',
                claude: str = 'claude') -> list:
    """`claude -p` reading the prompt from stdin; flags checked against `claude --help` 2.1.322."""
    return [claude, '-p', '--output-format', 'stream-json', '--verbose', '--include-hook-events',
            '--model', model, '--effort', effort, '--permission-mode', permission_mode,
            '--settings', str(settings_path), '--setting-sources', 'project',
            '--strict-mcp-config', '--no-session-persistence', '--no-chrome']


def sandboxed(argv: list, trial: Path, output: Path, go_cache: Path, protected: list,
              account_home: str, profile: Path = PROFILE) -> list:
    """Wrap argv in claude_agent.sb: writes stay below the trial and the Go cache, reads skip the
    output root (other trials, oracles), the protected checkouts and the account's Claude state."""
    trial, output = Path(trial).resolve(), Path(output).resolve()
    if output not in trial.parents:
        raise ValueError('trial must live below the output root')
    for path in protected:
        path = Path(path).resolve()
        if path == trial or path in trial.parents:
            raise ValueError('a protected checkout must not contain the trial: ' + str(path))
    params = ['-D', 'TRIAL=' + str(trial), '-D', 'OUTPUT=' + str(output), '-D', 'GOCACHE=' + str(Path(go_cache).resolve()),
              '-D', 'ACCOUNT_HOME=' + os.path.realpath(account_home)]
    for index in range(2):
        target = Path(protected[index]).resolve() if index < len(protected) else trial / '.no-such-protected'
        params += ['-D', 'PROTECTED%d=%s' % (index, target)]
    return [SANDBOX, '-f', str(profile), *params, *argv]


def _hook_block(event: dict):
    """A Stop hook response that blocks: exit code 2, or a JSON decision "block" on stdout."""
    if event.get('hook_event') != 'Stop' or event.get('subtype') != 'hook_response':
        return None
    stdout = event.get('stdout') if isinstance(event.get('stdout'), str) else ''
    reason = None
    if event.get('exit_code') == 2:
        reason = event.get('stderr') if isinstance(event.get('stderr'), str) else ''
    else:
        try:
            decision = json.loads(stdout.strip() or 'null')
        except ValueError:
            decision = None
        if isinstance(decision, dict) and decision.get('decision') == 'block':
            reason = str(decision.get('reason', ''))
    if reason is None:
        return None
    return {'hook': event.get('hook_name'), 'reason': reason[:2000]}


def parse_stream(path: Path) -> dict:
    """Observation of one stream-json transcript; unknown usage stays None, never zero."""
    result = {'schema_version': 'claude_stream_observation.v1', 'result_seen': False, 'is_error': None,
              'terminal_reason': None, 'num_turns': None, 'duration_ms': None, 'total_cost_usd': None,
              'input_tokens': None, 'output_tokens': None, 'cache_read_input_tokens': None,
              'cache_creation_input_tokens': None, 'total_tokens': None, 'model': None, 'permission_mode': None,
              'tool_uses': 0, 'subagent_tool_uses': 0, 'stop_hook_runs': 0, 'stop_hook_blocks': [],
              'hook_events': 0, 'final_text': None, 'incomplete_reasons': []}
    reasons = set()
    try:
        source = Path(path).open('rb')
    except FileNotFoundError:
        result['incomplete_reasons'] = ['no_transcript']
        return result
    with source:
        for line in iter(lambda: source.readline(MAX_LINE_BYTES + 1), b''):
            if len(line) > MAX_LINE_BYTES or not line.endswith(b'\n'):
                reasons.add('truncated_or_oversized_line')
                continue
            try:
                event = json.loads(line, object_pairs_hook=unique_object, parse_constant=reject_json_constant)
            except (ValueError, UnicodeDecodeError, RecursionError):
                reasons.add('malformed_json')
                continue
            if not isinstance(event, dict):
                reasons.add('invalid_event_schema')
                continue
            _observe(event, result)
    if not result['result_seen']:
        reasons.add('no_result_event')
    result['incomplete_reasons'] = sorted(reasons | set(result['incomplete_reasons']))
    return result


def _observe(event: dict, result: dict) -> None:
    kind, subtype = event.get('type'), event.get('subtype')
    if kind == 'system' and subtype == 'init':
        result['model'], result['permission_mode'] = event.get('model'), event.get('permissionMode')
    elif kind == 'system' and subtype in ('hook_started', 'hook_response'):
        result['hook_events'] += 1
        if subtype == 'hook_response' and event.get('hook_event') == 'Stop':
            result['stop_hook_runs'] += 1
        block = _hook_block(event)
        if block:
            result['stop_hook_blocks'].append(block)
    elif kind == 'assistant':
        content = (event.get('message') or {}).get('content') or []
        uses = [c for c in content if isinstance(c, dict) and c.get('type') == 'tool_use']
        result['tool_uses'] += len(uses)
        result['subagent_tool_uses'] += sum(1 for c in uses if c.get('name') in ('Task', 'Agent'))
    elif kind == 'result':
        _observe_result(event, result)


def _observe_result(event: dict, result: dict) -> None:
    result['result_seen'] = True
    result['is_error'] = event.get('is_error') if isinstance(event.get('is_error'), bool) else None
    for key in ('terminal_reason', 'num_turns', 'duration_ms', 'total_cost_usd'):
        result[key] = event.get(key)
    if isinstance(event.get('result'), str):
        result['final_text'] = event['result'][:4000]
    usage = event.get('usage')
    if not isinstance(usage, dict):
        result['incomplete_reasons'].append('missing_usage')
        return
    keys = ('input_tokens', 'output_tokens', 'cache_read_input_tokens', 'cache_creation_input_tokens')
    if not all(count(usage.get(key)) for key in keys):
        result['incomplete_reasons'].append('invalid_usage')
        return
    for key in keys:
        result[key] = usage[key]
    result['total_tokens'] = sum(usage[key] for key in keys)
