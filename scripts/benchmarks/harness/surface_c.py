"""Pilot C harness surface: the claude-code surface a pinned harness revision's `auto` generates.

`auto` is built from a snapshot of the harness revision (run_c.py --harness-revision), not from the
task's base, and generates into a copy of the task's base checkout: the surface (instructions,
skills, rules, settings and hook registrations) is what a user of that `auto` gets from `auto init`
in that code. The config
is the init default for claude-code with one pinned change: hooks.language_stop is false, because
the prompts are English and a language retry would measure the policy, not verification.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess

from workspace import hashes

PROJECT = 'pilot-c'


def build_auto(source: Path, binary: Path, env: dict) -> Path:
    """go build ./cmd/auto from the snapshot; the binary the generated hooks will call."""
    binary.parent.mkdir(parents=True, exist_ok=True)
    result = subprocess.run(['go', 'build', '-o', str(binary), './cmd/auto'], cwd=source, env=env,
                            capture_output=True, text=True, timeout=3600)
    if result.returncode:
        raise RuntimeError('go build ./cmd/auto failed:\n' + result.stderr[-4000:])
    return binary


def disable_language_stop(config_text: str) -> str:
    """Set hooks.language_stop: false in an autopus.yaml written by `auto init`."""
    match = re.search(r'^hooks:[ \t]*\n((?:[ \t]+.*\n|\n)*)', config_text, re.M)
    if not match:
        raise ValueError('autopus.yaml has no hooks block')
    body = match.group(1)
    indent = re.match(r'([ \t]+)', body)
    pad = indent.group(1) if indent else '  '
    if re.search(r'^[ \t]+language_stop:', body, re.M):
        body = re.sub(r'^([ \t]+)language_stop:.*$', r'\1language_stop: false', body, count=1, flags=re.M)
    else:
        body = pad + 'language_stop: false\n' + body
    return config_text[:match.start(1)] + body + config_text[match.end(1):]


def _auto(auto: Path, args: list, cwd: Path, env: dict, log: Path) -> None:
    with open(log, 'ab') as sink:
        sink.write(('$ auto ' + ' '.join(args) + '\n').encode())
        sink.flush()
        result = subprocess.run([str(auto), *args], cwd=cwd, env=env, stdin=subprocess.DEVNULL, stdout=sink,
                                stderr=subprocess.STDOUT, timeout=900)
    if result.returncode:
        raise RuntimeError('auto %s failed (%d); see %s' % (' '.join(args), result.returncode, log))


def generate_surface(auto: Path, source: Path, dest: Path, env: dict) -> dict:
    """Run init + update in a copy of source and keep only what they added or changed in dest/surface."""
    tree, surface, log = dest / 'generate-tree', dest / 'surface', dest / 'generate.log'
    shutil.copytree(source, tree, symlinks=True)
    before = hashes(tree)
    _auto(auto, ['init', '--dir', str(tree), '--platforms', 'claude-code', '--project', PROJECT, '--yes'],
          tree, env, log)
    config = tree / 'autopus.yaml'
    config.write_text(disable_language_stop(config.read_text()))
    _auto(auto, ['update', '--dir', str(tree), '--yes', '--local'], tree, env, log)
    after = hashes(tree)
    removed = sorted(set(before) - set(after))
    if removed:
        raise RuntimeError('surface generation removed source files: ' + ', '.join(removed[:5]))
    changed = sorted(path for path in after if before.get(path) != after[path])
    for rel in changed:
        source_path, target = tree / rel, surface / rel
        if source_path.is_symlink():
            raise RuntimeError('surface symlinks are not supported: ' + rel)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source_path, target)
    shutil.rmtree(tree)
    foreign = sorted(name for name in ('.codex', '.gemini', '.opencode', '.omp', '.agents', 'opencode.json')
                     if (surface / name).exists())
    if foreign:
        raise RuntimeError('update detected other platforms on PATH: ' + ', '.join(foreign))
    hooks = stop_hooks(surface)
    if any('verify language' in command for command in hooks):
        raise RuntimeError('language stop hook is still registered: ' + ', '.join(hooks))
    manifest = {rel: after[rel] for rel in changed}
    digest = hashlib.sha256(json.dumps(manifest, sort_keys=True).encode()).hexdigest()
    return {'files': len(changed), 'surface_hash': digest, 'stop_hooks': hooks, 'manifest': manifest}


AUTO_COMMAND = re.compile(r'(?<![\w./-])auto(?=\s)')


def pin_hooks(surface: Path, auto: Path) -> dict:
    """Point every hook command at the pinned binary by absolute path, so no `auto` the agent builds
    or puts first on PATH answers a hook. Returns the hash of the pinned surface settings."""
    settings = surface / '.claude' / 'settings.json'
    data = json.loads(settings.read_text())
    for groups in (data.get('hooks') or {}).values():
        for group in groups:
            for hook in group.get('hooks') or []:
                if isinstance(hook.get('command'), str):
                    hook['command'] = AUTO_COMMAND.sub(lambda _: shlex.quote(str(auto)), hook['command'])
    settings.write_text(json.dumps(data, indent=2) + '\n')
    # Project MCP servers load in no arm (--strict-mcp-config); the file would still let a nested
    # session start them through `bash -l`, so it does not enter the trial.
    mcp = surface / '.mcp.json'
    removed = mcp.is_file()
    mcp.unlink(missing_ok=True)
    return {'pinned_settings_hash': hashlib.sha256(settings.read_bytes()).hexdigest(), 'mcp_json_removed': removed}


def stop_hooks(surface: Path) -> list:
    """Commands registered on the Stop event in the generated .claude/settings.json."""
    settings = surface / '.claude' / 'settings.json'
    if not settings.is_file():
        return []
    data = json.loads(settings.read_text())
    commands = []
    for group in (data.get('hooks') or {}).get('Stop') or []:
        for hook in group.get('hooks') or []:
            commands.append(str(hook.get('command', '')))
    return commands


def install(work: Path, surface: Path) -> None:
    """Copy the generated surface over a trial workspace (it may replace e.g. .gitignore)."""
    for entry in surface.rglob('*'):
        if entry.is_symlink():
            raise ValueError('surface symlinks are not supported')
    shutil.copytree(surface, work, dirs_exist_ok=True)


def link_tools(tools_dir: Path, names: list, base_path: str) -> dict:
    """A directory of symlinks to exactly the named host tools; returns name -> resolved target."""
    tools_dir.mkdir(parents=True, exist_ok=True)
    linked = {}
    for name in names:
        found = shutil.which(name, path=base_path)
        if not found:
            raise RuntimeError('tool not found on PATH: ' + name)
        target = os.path.realpath(found)
        (tools_dir / name).symlink_to(target)
        linked[name] = target
    return linked


def generation_env(base: dict, home: Path, bin_dir: Path, tools_dir: Path) -> dict:
    """The environment `auto` generates under: a scratch HOME and no inherited credentials."""
    home.mkdir(parents=True, exist_ok=True)
    keep = ('PATH', 'LANG', 'GOCACHE', 'GOMODCACHE', 'GOFLAGS', 'GOPROXY', 'GOSUMDB', 'GOTOOLCHAIN', 'TMPDIR',
            'GOTMPDIR')
    env = {key: base[key] for key in keep if key in base}
    env['HOME'] = str(home)
    env['PATH'] = os.pathsep.join([str(bin_dir), str(tools_dir), '/usr/bin', '/bin', '/usr/sbin', '/sbin'])
    env['GIT_CONFIG_GLOBAL'] = os.devnull
    return env
