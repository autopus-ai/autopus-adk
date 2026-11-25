"""Opt-in Codex CLI for pilot C trials (`run_c.py --codex-auth-file`), for harness variants whose review
step also asks Codex (OpenAI) for a second review.

The `codex` npm entry point is a node script (`#!/usr/bin/env node`); linked by --tools next to `node`,
it resolves node from the trial PATH and its native binary from the package's own directory, which
claude_agent.tool_dirs reopens for reading. Every trial gets its own CODEX_HOME below the trial
(`<trial>/codex-home`, outside the workspace, so neither the scope audit, the candidate overlay nor
measure/ ever sees it) holding a copy of the maintainer's auth file (mode 0600) and CONFIG. Codex's own
seatbelt sandbox is not used: the harness runs it with `-s danger-full-access`, and claude_agent.sb
confines it exactly as it confines the agent (writes only below the trial, no keychain, no Unix sockets).

Auth. A ChatGPT login refreshes its tokens in CODEX_HOME/auth.json, and a refresh token is single use:
two homes refreshing from the same copy log each other out. After each trial a refreshed, well-formed
auth.json for the same account is written back to --codex-auth-file, so the next trial starts from the
newest tokens. The maintainer's own ~/.codex login is a separate copy and is never touched; refresh it
(any `codex` call) before copying it, so a run inside the token lifetime never refreshes at all.

Contamination. CONFIG turns web search and every browser, app, plugin and connector tool off; the
rollouts the trial's codex wrote (CODEX_HOME/sessions) are still scanned after the trial for any web
search event and for shell commands that reach the network (network_c), and stream_c flags web search
markers in the output of a Bash call that ran codex.
"""
import hashlib
import json
import os
from pathlib import Path
import subprocess

from network_c import network_reason
import surface_c

MODEL = 'gpt-6.2-sol'
EFFORT = 'high'
# Keys checked against codex-cli 0.170.0 with `codex exec --strict-config` (an unknown key or value is
# an error there); `web_search = "disabled"` is the current key, `[tools] web_search` the legacy one.
CONFIG = '''model = "{model}"
model_reasoning_effort = "{effort}"
web_search = "disabled"
cli_auth_credentials_store = "file"
check_for_update_on_startup = false

[tools]
web_search = false

[agents]
enabled = false

[analytics]
enabled = false

[feedback]
enabled = false

[features]
apps = false
browser_use = false
browser_use_external = false
computer_use = false
image_generation = false
in_app_browser = false
memories = false
multi_agent_v2 = false
plugins = false
remote_plugin = false
skill_mcp_dependency_install = false
standalone_web_search = false
tool_suggest = false
'''
# Inside claude_agent.sb codex's default trust path (the system roots through the platform verifier) fails
# every TLS connect (`is_connect=true`), while curl in the same sandbox succeeds; the macOS bundle loaded
# through CODEX_CA_CERTIFICATE, which only codex reads, works. Other tools keep their own trust path.
CA_BUNDLE = '/etc/ssl/cert.pem'
TOKEN_KEYS = ('input_tokens', 'cached_input_tokens', 'output_tokens', 'reasoning_output_tokens', 'total_tokens')
WEB_SEARCH_MARKERS = ('web_search', 'websearch', 'web search:')


def config_text(model: str = MODEL, effort: str = EFFORT) -> str:
    return CONFIG.format(model=model, effort=effort)


def add_arguments(ap) -> None:
    ap.add_argument('--codex-auth-file', type=Path,
                    help='mode-0600 copy of a codex auth.json: links codex onto the trial PATH with a per-trial '
                         'CODEX_HOME (codex_c); refreshed logins are written back to this file')
    ap.add_argument('--codex-model', default=MODEL)
    ap.add_argument('--codex-effort', default=EFFORT)


def tool_names(args) -> list:
    """--tools for every arm. With --codex-auth-file (checked here, before any work) codex is left out:
    prepare_run links it into its own directory, which only arms that use it get (arm_uses)."""
    names = [name for name in args.tools.split(',') if name]
    if args.codex_auth_file:
        read_auth(args.codex_auth_file)
        names = [name for name in names if name != 'codex']
    return names


def arm_uses(ctx: dict, arm: str) -> bool:
    """native has no project harness, so nothing in it calls codex: it gets no codex, home or package dir."""
    return bool(ctx.get('codex')) and arm != 'native'


def trial_settings(args) -> dict | None:
    if not args.codex_auth_file:
        return None
    return {'auth_file': Path(args.codex_auth_file).resolve(), 'model': args.codex_model, 'effort': args.codex_effort}


def settings(args) -> dict | None:
    """What the protocol records (never the auth file): None when codex is off."""
    if not args.codex_auth_file:
        return None
    text = config_text(args.codex_model, args.codex_effort)
    return {'model': args.codex_model, 'effort': args.codex_effort,
            'config_sha256': hashlib.sha256(text.encode()).hexdigest(), 'home': '<trial>/codex-home',
            'sandbox': 'codex -s danger-full-access inside claude_agent.sb'}


def prepare_run(protocol: dict, ctx: dict) -> None:
    """Link codex into <output>/bin/tools-codex (once; a resumed run reuses it) and record its target and
    `codex --version` as a trial resolves it: the linked entry point run by the linked node."""
    codex = ctx.get('codex')
    if not codex:
        return
    codex['tools_dir'] = ctx['output'] / 'bin' / 'tools-codex'
    if codex['tools_dir'].exists():
        codex['tools'] = {'codex': os.path.realpath(codex['tools_dir'] / 'codex')}
    else:
        codex['tools'] = surface_c.link_tools(codex['tools_dir'], ['codex'], ctx['base_env'].get('PATH', ''))
    path = '%s:%s:/usr/bin:/bin' % (codex['tools_dir'], ctx['tools_dir'])
    protocol['codex']['target'] = codex['tools']['codex']
    protocol['codex']['version'] = subprocess.run(
        [str(codex['tools_dir'] / 'codex'), '--version'], capture_output=True, text=True,
        env={'PATH': path, 'HOME': str(ctx['output'])}).stdout.strip()


def _auth_object(data: bytes) -> dict:
    auth = json.loads(data)
    if not isinstance(auth, dict) or not (isinstance(auth.get('tokens'), dict) or auth.get('OPENAI_API_KEY')):
        raise ValueError('codex auth file holds no tokens and no API key')
    return auth


def read_auth(path: Path) -> bytes:
    """The auth file's bytes; refuse a group- or world-readable file or one that is not a codex login."""
    path = Path(path)
    if path.stat().st_mode & 0o077:
        raise ValueError('codex auth file must be mode 0600: ' + str(path))
    data = path.read_bytes()
    try:
        _auth_object(data)
    except ValueError as error:
        raise ValueError('not a codex auth.json: %s (%s)' % (path, type(error).__name__)) from None
    return data


def _write_private(path: Path, data: bytes) -> None:
    temp = path.with_name('.' + path.name + '.tmp')
    descriptor = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, 'wb') as out:
        out.write(data)
    os.chmod(temp, 0o600)
    os.replace(temp, path)


def install_home(trial: Path, codex: dict) -> dict:
    """<trial>/codex-home with the auth copy and CONFIG; returns the agent env entries (CODEX_HOME, CA)."""
    home = Path(trial) / 'codex-home'
    home.mkdir(mode=0o700, parents=False, exist_ok=False)
    _write_private(home / 'auth.json', read_auth(codex['auth_file']))
    (home / 'config.toml').write_text(config_text(codex['model'], codex['effort']))
    return {'CODEX_HOME': str(home), 'CODEX_CA_CERTIFICATE': CA_BUNDLE}


def write_back(trial: Path, auth_file: Path) -> str:
    """Carry a refreshed login from the trial's home to auth_file; the trial controls that file, so only
    a well-formed login for the same account replaces it. Returns unchanged, refreshed or rejected."""
    copy = Path(trial) / 'codex-home' / 'auth.json'
    try:
        data = copy.read_bytes() if copy.is_file() and not copy.is_symlink() else None
        current = Path(auth_file).read_bytes()
        if data is None or data == current:
            return 'unchanged'
        new, old = _auth_object(data), _auth_object(current)
    except (OSError, ValueError):
        return 'rejected'
    account = lambda auth: (auth.get('tokens') or {}).get('account_id')
    if account(new) != account(old) or str(new.get('last_refresh') or '') < str(old.get('last_refresh') or ''):
        return 'rejected'
    _write_private(Path(auth_file), data)
    return 'refreshed'


def _commands(payload: dict) -> list:
    """Shell commands in one rollout function call (exec / shell / local_shell argument shapes)."""
    found = []
    args = payload.get('arguments')
    if isinstance(args, str):
        try:
            args = json.loads(args)
        except ValueError:
            args = {'cmd': args}
    for source in (args, payload.get('action')):
        if not isinstance(source, dict):
            continue
        for key in ('cmd', 'command', 'code', 'input'):
            value = source.get(key)
            if isinstance(value, list):
                value = ' '.join(map(str, value))
            if isinstance(value, str):
                found.append(value)
    return found


def observe(trial: Path) -> dict | None:
    """Sessions, web search events and network-reaching commands in the trial's codex rollouts."""
    sessions = Path(trial) / 'codex-home' / 'sessions'
    if not (Path(trial) / 'codex-home').is_dir():
        return None
    result = {'sessions': 0, 'commands': 0, 'web_search': [], 'network_commands': [], 'tokens': None,
              'sessions_without_usage': 0}
    # The trial controls this tree: only regular files that really lie below it are read.
    inside = sessions.is_dir() and Path(trial).resolve() in sessions.resolve().parents
    for rollout in sorted(sessions.rglob('*.jsonl')) if inside else []:
        if rollout.is_symlink():
            continue
        result['sessions'] += 1
        usage = None
        for line in rollout.read_text(errors='replace').splitlines():
            try:
                event = json.loads(line)
            except ValueError:
                continue
            payload = event.get('payload') if isinstance(event, dict) else None
            if not isinstance(payload, dict):
                continue
            kind = str(payload.get('type') or '')
            if kind == 'token_count':
                # Cumulative per session: the last event holds the session's total.
                usage = (payload.get('info') or {}).get('total_token_usage') or usage
            if 'web_search' in kind.lower() or 'websearch' in kind.lower():
                result['web_search'].append(json.dumps(payload)[:400])
            if kind not in ('function_call', 'custom_tool_call', 'local_shell_call'):
                continue
            for command in _commands(payload):
                result['commands'] += 1
                try:
                    reached = network_reason(command)
                except ValueError:
                    reached = 'unparsed'
                if reached:
                    result['network_commands'].append(command[:500])
        _add_usage(result, usage)
    return result


def _add_usage(result: dict, usage) -> None:
    """Sum one session's OpenAI usage into result['tokens']; a session without any stays None-free but
    is counted in sessions_without_usage, so a partial total is never mistaken for the whole."""
    if not isinstance(usage, dict):
        result['sessions_without_usage'] += 1
        return
    totals = result['tokens'] or dict.fromkeys(TOKEN_KEYS, 0)
    for key in TOKEN_KEYS:
        value = usage.get(key)
        if isinstance(value, int) and not isinstance(value, bool) and value >= 0:
            totals[key] += value
    result['tokens'] = totals


def contamination(observed: dict | None) -> list:
    """The observation's flags in stream_c's contamination format."""
    if not observed:
        return []
    return (['CODEX_WEB_SEARCH ' + entry for entry in observed['web_search']] +
            ['CODEX ' + command for command in observed['network_commands']])


def web_search_in_output(text: str) -> bool:
    lowered = text.lower()
    return any(marker in lowered for marker in WEB_SEARCH_MARKERS)
