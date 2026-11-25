"""What a pilot C trial's codex did, read fail-closed (codex_c holds the home and the login).

Two independent sources. The agent's Bash commands (stream_c) say how often it started a codex session
and with which flags; network_c's command reading finds codex behind wrappers (`timeout`, `perl -e
'alarm N; exec @ARGV'`, `env`, `nohup`, `npx`, `node .../codex.js`) and inside `sh -c` programs, and
never in here-document text or quoted arguments. Flags that leave the frozen configuration
(`--ephemeral`, `--ignore-user-config`, `--search`, `-c`, `--enable`, `--profile`, `--model`, a
CODEX_HOME override, ...) are recorded as bypasses. The rollouts codex wrote below the trial's
CODEX_HOME say what each session did: web search events, network-reaching shell commands and the
OpenAI token usage. The trial controls that tree, so it is walked without following links, only
regular files up to MAX_ROLLOUT_BYTES are opened (O_NOFOLLOW | O_NONBLOCK: a FIFO cannot block the
runner) and anything else is counted as unscanned. reconcile then fails closed: fewer observed
sessions than started ones, an unscanned rollout, a replaced home or a changed config.toml is a
contamination flag, and the usage of such a trial is unknown, never a partial sum.
"""
import hashlib
import json
import os
from pathlib import Path
import shlex
import stat

from network_c import network_reason, simple_commands, strip_wrappers

MAX_ROLLOUT_BYTES = 64 * 1024 * 1024
TOKEN_KEYS = ('input_tokens', 'cached_input_tokens', 'output_tokens', 'reasoning_output_tokens', 'total_tokens')
WEB_SEARCH_MARKERS = ('web_search', 'websearch', 'web search:')
# Global options of codex 0.170 that take a value (`codex --help`, `codex exec --help`).
VALUE_OPTIONS = {'-c', '--config', '-m', '--model', '-p', '--profile', '-s', '--sandbox', '-C', '--cd', '-i',
                 '--image', '--enable', '--disable', '-a', '--ask-for-approval', '--local-provider', '-o',
                 '--output-last-message', '--output-schema', '--color', '--add-dir', '--thread-source',
                 '--cyber-access-program'}
BYPASS_OPTIONS = {'--ephemeral', '--ignore-user-config', '--ignore-rules', '--search', '-c', '--config', '--enable',
                  '--disable', '-p', '--profile', '-m', '--model', '--oss', '--local-provider', '--strict-config'}
NOT_SESSIONS = {'login', 'logout', 'features', 'help', 'completion', 'debug', 'doctor', 'mcp', 'mcp-server',
                'app-server', 'apply', 'sandbox', 'cloud', 'migrate-nfs-db', '--version', '-V', '--help', '-h'}


def web_search_in_output(text: str) -> bool:
    lowered = text.lower()
    return any(marker in lowered for marker in WEB_SEARCH_MARKERS)


def _peel(words: list) -> list | None:
    """The codex argv inside one simple command (after wrappers), or None."""
    for _ in range(6):
        words = strip_wrappers(words)
        if not words:
            return None
        tool = words[0].lstrip('`$(').rsplit('/', 1)[-1]
        if tool in ('codex', 'codex.js'):
            return words
        if tool == 'perl' and len(words) > 3 and words[1] == '-e' and 'exec @ARGV' in words[2]:
            words = words[3:]
        elif tool in ('npx', 'node', 'bunx', 'pnpx'):
            words = [w for w in words[1:] if not w.startswith('-')]
            if words and words[0] == 'exec':
                words = words[1:]
        else:
            return None
    return None


def codex_runs(command: str) -> list:
    """[{'session': bool, 'bypass': [flags]}] for every codex the Bash command line runs."""
    runs = []
    for raw in simple_commands(command):
        argv = _peel(raw)
        if argv is None:
            continue
        bypass = sorted({w.split('=', 1)[0] for w in raw if w.startswith('CODEX_HOME=')} |
                        {a.split('=', 1)[0] for a in argv[1:] if a.split('=', 1)[0] in BYPASS_OPTIONS})
        positional, i = None, 1
        while i < len(argv):
            word = argv[i]
            if word in NOT_SESSIONS or not word.startswith('-'):
                positional = word
                break
            i += 2 if word in VALUE_OPTIONS else 1
        runs.append({'session': positional not in NOT_SESSIONS, 'bypass': bypass})
    if 'CODEX_HOME' in command and not any('CODEX_HOME=' in b for r in runs for b in r['bypass']):
        # `export CODEX_HOME=...` before the run, or any other way of naming another home.
        for run in runs:
            run['bypass'] = sorted(set(run['bypass']) | {'CODEX_HOME'})
    return runs


def _read_regular(path: Path, cap: int) -> bytes | None:
    """The file's bytes when it is a regular file (never a link, FIFO or device) of at most cap bytes."""
    try:
        info = os.lstat(path)
        if not stat.S_ISREG(info.st_mode) or info.st_size > cap:
            return None
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except OSError:
        return None
    with os.fdopen(descriptor, 'rb') as source:
        if not stat.S_ISREG(os.fstat(source.fileno()).st_mode):
            return None
        return source.read(cap + 1)[:cap + 1]


def real_dir(path: Path, trial: Path) -> bool:
    """path is a directory, not a link, whose resolved path stays inside the trial."""
    try:
        return stat.S_ISDIR(os.lstat(path).st_mode) and Path(trial).resolve() in Path(path).resolve().parents
    except OSError:
        return False


def _commands(payload: dict) -> list:
    """Shell commands in one rollout tool call: exec/shell argument shapes, list argv and write_stdin."""
    found, args = [], payload.get('arguments')
    if isinstance(args, str):
        try:
            args = json.loads(args)
        except ValueError:
            args = {'cmd': args}
    for source in (args, payload.get('action')):
        if not isinstance(source, dict):
            continue
        for key in ('cmd', 'command', 'code', 'input', 'chars'):
            value = source.get(key)
            if isinstance(value, list):
                value = shlex.join(map(str, value))
            if isinstance(value, str) and value.strip():
                found.append(value)
    return found


def observe(trial: Path, config_sha256: str) -> dict:
    """Sessions, web search, network commands, usage and integrity of the trial's codex home."""
    home = Path(trial) / 'codex-home'
    result = {'sessions': 0, 'unscanned': 0, 'commands': 0, 'web_search': [], 'network_commands': [],
              'tokens': None, 'sessions_without_usage': 0, 'home_intact': real_dir(home, trial),
              'config_intact': False}
    if not result['home_intact']:
        return result
    config = _read_regular(home / 'config.toml', 1 << 20)
    result['config_intact'] = config is not None and hashlib.sha256(config).hexdigest() == config_sha256
    sessions = home / 'sessions'
    if not real_dir(sessions, trial):
        return result
    for directory, dirs, files in os.walk(sessions, followlinks=False):
        for name in sorted(files):
            if '.jsonl' not in name:
                continue
            data = _read_regular(Path(directory) / name, MAX_ROLLOUT_BYTES) if name.endswith('.jsonl') else None
            if data is None or len(data) > MAX_ROLLOUT_BYTES:
                result['unscanned'] += 1
                continue
            result['sessions'] += 1
            _observe_rollout(data.decode(errors='replace'), result)
    return result


def _observe_rollout(text: str, result: dict) -> None:
    usage = None
    for line in text.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        payload = event.get('payload') if isinstance(event, dict) else None
        if not isinstance(payload, dict):
            continue
        kind = str(payload.get('type') or '')
        if kind == 'token_count':
            # Cumulative within a session: the last event holds the session's total.
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
    if not isinstance(usage, dict):
        result['sessions_without_usage'] += 1
        return
    totals = result['tokens'] or dict.fromkeys(TOKEN_KEYS, 0)
    for key in TOKEN_KEYS:
        value = usage.get(key)
        if isinstance(value, int) and not isinstance(value, bool) and value >= 0:
            totals[key] += value
    result['tokens'] = totals


def reconcile(observed: dict, stream: dict) -> list:
    """Contamination flags of the trial's codex use; marks the usage unknown when it cannot be whole."""
    flags = ['CODEX_WEB_SEARCH ' + entry for entry in observed['web_search']]
    flags += ['CODEX ' + command for command in observed['network_commands']]
    flags += ['CODEX_BYPASS %s' % flag for flag in stream.get('codex_bypass') or []]
    started = stream.get('codex_calls') or 0
    if not observed['home_intact']:
        flags.append('CODEX_HOME_REPLACED')
    elif not observed['config_intact']:
        flags.append('CODEX_CONFIG_CHANGED')
    if observed['unscanned']:
        flags.append('CODEX_UNSCANNED %d rollout(s)' % observed['unscanned'])
    if started > observed['sessions']:
        flags.append('CODEX_UNOBSERVED %d started, %d observed' % (started, observed['sessions']))
    observed['usage_unknown'] = bool(observed['unscanned'] or started > observed['sessions'] or
                                     not observed['home_intact'] or observed['sessions_without_usage'])
    if observed['usage_unknown']:
        observed['tokens'] = None
    return flags
