"""Opt-in Codex CLI for pilot C trials (`run_c.py --codex-auth-file`), for harness variants whose review
step also asks Codex (OpenAI) for a second review. What codex did is read by codex_observe_c.

PATH. `codex` is linked into <output>/bin/tools-codex, apart from the shared tools, and only arms whose
harness uses it (every arm but native, arm_uses) get that directory on PATH, the package directory in
the sandbox's tool slots and a CODEX_HOME. The npm entry point is a node script; it runs on the linked
`node`. The node install directory every arm reopens for reading already contains the codex package, so
a native agent could still run `node <that dir>/.../codex.js`; it has no CODEX_HOME and no login, and
stream_c makes a native trial that runs codex invalid.

Home. A codex arm's trial gets <trial>/codex-home (mode 0700, outside the workspace, so the scope audit
and the candidate overlay never see it) holding CONFIG and a copy (0600) of the login. Codex's own
seatbelt cannot nest inside claude_agent.sb, so the harness runs it with `-s danger-full-access` and the
trial profile confines it as it confines the agent: writes only below the trial, no keychain.

Login. Use a dedicated login, made only for the run in its own CODEX_HOME
(`CODEX_HOME=$HOME/pilot-c/codex-login codex login`, then pass $HOME/pilot-c/codex-login/auth.json),
never ~/.codex (refused), and log it out after the run. The runner never writes the file it was given.
A ChatGPT refresh token is single use: when a trial's copy refreshes, the given file holds a spent token,
so the trial records codex_auth_rotated and the run stops (log in again, then --resume). Every exit
path of a trial (normal, runner exception, a stop signal, --keep-trials, a partial trial set aside by
--resume) deletes the copy and any byte-identical copy below the trial (scrub); a --dry-run trial gets
PLACEHOLDER instead of the login. Residual risk, stated plainly: the agent can read its copy (and so
can codex), the network is open, and the transcript in measure/ keeps whatever the agent printed, so a
token it printed stays there; that is why the login is dedicated and logged out after the run.
"""
import hashlib
import json
import os
from pathlib import Path
import pwd
import stat
import subprocess

from codex_observe_c import real_dir
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


# What a --dry-run trial gets instead of the login: the dry run never starts codex.
PLACEHOLDER = b'{"auth_mode": "dry-run-placeholder"}\n'
MAX_AUTH_BYTES = 1 << 20


def config_text(model: str = MODEL, effort: str = EFFORT) -> str:
    return CONFIG.format(model=model, effort=effort)


def add_arguments(ap) -> None:
    ap.add_argument('--codex-auth-file', type=Path,
                    help='mode-0600 copy of a codex auth.json: links codex onto the trial PATH with a per-trial '
                         'CODEX_HOME (codex_c) for the arms that use it; a dedicated login, never ~/.codex, '
                         'is only read, never written')
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
    return {'auth_file': Path(args.codex_auth_file).resolve(), 'model': args.codex_model, 'effort': args.codex_effort,
            'dry_run': bool(args.dry_run), 'config_sha256': settings(args)['config_sha256']}


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


def _account_homes() -> list:
    """The account's own codex homes: ~/.codex under $HOME and under the passwd home."""
    homes = {Path.home(), Path(pwd.getpwuid(os.getuid()).pw_dir)}
    return [(home / '.codex').resolve() for home in homes]


def read_auth(path: Path) -> bytes:
    """The login's bytes; refuse a file below ~/.codex, a group- or world-readable one or a non-login."""
    resolved = Path(path).resolve()
    for home in _account_homes():
        if resolved == home or home in resolved.parents:
            raise ValueError('use a dedicated codex login, not the account login below %s' % home)
    if resolved.stat().st_mode & 0o077:
        raise ValueError('codex auth file must be mode 0600: ' + str(path))
    data = resolved.read_bytes()
    try:
        _auth_object(data)
    except ValueError as error:
        raise ValueError('not a codex auth.json: %s (%s)' % (path, type(error).__name__)) from None
    return data


def _refresh_token(data: bytes | None):
    try:
        return (_auth_object(data).get('tokens') or {}).get('refresh_token')
    except (TypeError, ValueError):
        return None


def install_home(trial: Path, codex: dict) -> dict:
    """<trial>/codex-home with CONFIG and the login copy (PLACEHOLDER in a dry run); returns the agent
    env entries (CODEX_HOME, CA) and keeps the installed bytes in codex['installed'] for check and scrub."""
    home = Path(trial) / 'codex-home'
    home.mkdir(mode=0o700, parents=False, exist_ok=False)
    home.chmod(0o700)
    data = PLACEHOLDER if codex.get('dry_run') else read_auth(codex['auth_file'])
    descriptor = os.open(home / 'auth.json', os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, 'wb') as out:
        out.write(data)
    (home / 'config.toml').write_text(config_text(codex['model'], codex['effort']))
    codex['installed'] = data
    return {'CODEX_HOME': str(home), 'CODEX_CA_CERTIFICATE': CA_BUNDLE}


def _read_copy(trial: Path) -> bytes | None:
    """The trial's login copy, read without following any link (home or file) out of the trial."""
    home = Path(trial) / 'codex-home'
    if not real_dir(home, trial):
        return None
    try:
        descriptor = os.open(home / 'auth.json', os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except OSError:
        return None
    with os.fdopen(descriptor, 'rb') as source:
        info = os.fstat(source.fileno())
        return source.read(MAX_AUTH_BYTES) if stat.S_ISREG(info.st_mode) else None


def auth_rotated(trial: Path, codex: dict):
    """True when the copy's refresh token is no longer the installed one, 'unknown' when the copy cannot
    be read (deleted, replaced, a link): either way the given login may be spent. False otherwise."""
    installed = _refresh_token(codex.get('installed'))
    current = _read_copy(trial)
    if current is None:
        return 'unknown'
    if current == codex.get('installed'):
        return False
    return _refresh_token(current) != installed


def scrub(trial: Path, codex: dict) -> int:
    """Delete <trial>/codex-home/auth.json (the entry itself, never a link's target) and every regular
    file below the trial that holds exactly the installed bytes. os.fwalk works on directory file
    descriptors without following links, so a link the agent swaps in mid-walk cannot lead the sweep
    out of the trial. Never raises: it runs in finally blocks, where an error would mask a stop signal.
    Returns how many entries were deleted."""
    trial, data, removed = Path(trial), codex.get('installed'), 0
    home = trial / 'codex-home'
    if real_dir(home, trial):
        try:
            os.unlink(home / 'auth.json')
            removed += 1
        except OSError:
            pass
    if not data or not real_dir(trial, trial.parent):
        return removed
    try:
        for _path, dirs, files, root in os.fwalk(trial, follow_symlinks=False):
            for name in dirs:
                try:
                    if stat.S_ISDIR(os.lstat(name, dir_fd=root).st_mode):
                        os.chmod(name, 0o700, dir_fd=root, follow_symlinks=False)
                except (OSError, NotImplementedError):
                    continue
            for name in files:
                removed += _remove_if_copy(name, root, data)
    except OSError:
        pass
    return removed


def _remove_if_copy(name: str, root: int, data: bytes) -> int:
    try:
        info = os.lstat(name, dir_fd=root)
        if not stat.S_ISREG(info.st_mode) or info.st_size != len(data):
            return 0
        descriptor = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=root)
        with os.fdopen(descriptor, 'rb') as source:
            same = source.read(len(data) + 1) == data
        if same:
            os.unlink(name, dir_fd=root)
            return 1
    except OSError:
        pass
    return 0


def last_rotation(ctx: dict, arm: str):
    """The rotation verdict of the arm's latest trial (trial_c.run records it before grading, or in its
    finally when the trial failed earlier), so a runner exception still stops the run."""
    return ctx['codex'].get('last_rotated') if arm_uses(ctx, arm) else None


def scrub_partial(ctx: dict, trial: Path, arm: str) -> None:
    """run_c's own sweep, after every trial whatever ended it and before --resume sets a partial trial
    aside: scrub the trial of a codex arm against the login as it is now (a no-op once it is pruned)."""
    codex = ctx.get('codex')
    if not (arm_uses(ctx, arm) and Path(trial).exists()):
        return
    try:
        data = PLACEHOLDER if codex.get('dry_run') else read_auth(codex['auth_file'])
    except (OSError, ValueError):
        data = codex.get('installed')
    scrub(trial, {**codex, 'installed': data})


def stop_for_rotation(row: dict) -> bool:
    """Print why the run stops when this trial's login copy refreshed or could not be checked."""
    if row.get('codex_auth_rotated') in (None, False):
        return False
    print('CODEX_AUTH_ROTATED: the codex login copy of %s-%s-k%d %s; the dedicated login given by '
          '--codex-auth-file may now hold a spent refresh token. Log it in again (CODEX_HOME=<dir> codex '
          'login), then continue with --resume.' % (row['task_id'], row['arm'], row['replicate'],
                                                   'refreshed' if row['codex_auth_rotated'] is True else
                                                   'could not be read back'), flush=True)
    return True
