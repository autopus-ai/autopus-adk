"""Restricted local-command permissions and no-model benchmark preflight."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile


def profile_args(workspace: Path, cache: Path, temp: Path) -> list:
    roots = [path.resolve() for path in (workspace, cache, temp)]
    forbidden = {Path('/'), Path('/tmp').resolve(), Path.home().resolve(), Path('/Users')}
    if any(root in forbidden for root in roots):
        raise ValueError('benchmark write roots must be narrowly scoped')
    for index, root in enumerate(roots):
        if any(root == other or root in other.parents or other in root.parents
               for other in roots[index + 1:]):
            raise ValueError('benchmark write roots must be disjoint')
    gopath = subprocess.check_output(['go', 'env', 'GOPATH'], text=True, timeout=10).strip()
    values = ['default_permissions="bench"',
              'permissions.bench.network.enabled=false']
    filesystem = {':root': 'deny', ':minimal': 'read'}
    reads = [Path(p) for p in ('/opt/homebrew', '/usr', '/bin', '/sbin', '/System', '/Library')]
    reads += [Path(p) / 'pkg' / 'mod' for p in gopath.split(os.pathsep) if p]
    # These are instruction libraries, never the original repository or its parent.
    reads += [Path.home() / '.agents' / 'skills']
    for path in sorted(set(path.resolve() for path in reads)):
        filesystem[str(path)] = 'read'
    for path in roots:
        filesystem[str(path)] = 'write'
    for name in ('.git', '.codex'):
        filesystem[str(roots[0] / name)] = 'read'
    # CLI dotted keys do not parse quoted path components; use a TOML table value.
    table = '{' + ', '.join(json.dumps(key) + '=' + json.dumps(value)
                            for key, value in filesystem.items()) + '}'
    values.append('permissions.bench.filesystem=' + table)
    return [item for value in values for item in ('-c', value)]


def preflight(original_repo: Path, directory: Path | None = None) -> dict:
    """Exercise installed Codex Seatbelt without model or authentication calls.

    A persistent supplied directory receives only task-owned fixture files.
    This verifies command confinement, not every model-visible tool surface.
    """
    parent = Path(tempfile.mkdtemp(prefix='harness-permission-preflight-', dir=directory)).resolve()
    work, cache, temporary = [parent / name for name in ('workspace', 'cache', 'temp')]
    for path in (work, cache, temporary):
        path.mkdir()
    secret = parent / 'source-answer.txt'
    secret.write_text('PRIVATE_BENCHMARK_ORACLE')
    protocol = parent / 'protocol.json'
    protocol.write_text('{"private":"oracle"}')
    (work / 'outside-link').symlink_to(secret)
    (work / 'go.mod').write_text('module preflight\n\ngo 1.26\n')
    (work / 'probe.go').write_text('package preflight\nfunc Value() int { return 7 }\n')
    (work / 'probe_test.go').write_text('package preflight\nimport "testing"\nfunc TestValue(t *testing.T) { if Value()!=7 { t.Fatal("bad") } }\n')
    overrides = profile_args(work, cache, temporary)
    env = os.environ.copy()
    env.update(TMPDIR=str(temporary), GOTMPDIR=str(temporary), GOCACHE=str(cache),
               GOMAXPROCS='2', GOPROXY='off', GOSUMDB='off', GOTELEMETRY='off')
    commands = [
        ('workspace_write', ['/bin/sh', '-c', 'printf ok > permitted.txt'], True),
        ('workspace_read', ['/bin/cat', str(work / 'probe.go')], True),
        ('go_version', ['go', 'version'], True),
        ('go_test', ['go', 'test', '-p', '1', './...', '-count=1'], True),
        ('source_sibling_denied', ['/bin/cat', str(secret)], False),
        ('protocol_sibling_denied', ['/bin/cat', str(protocol)], False),
        ('source_symlink_denied', ['/bin/cat', str(work / 'outside-link')], False),
        ('original_repo_denied', ['/bin/cat', str(original_repo.resolve() / 'go.mod')], False),
    ]
    results = []
    for name, command, allowed in commands:
        args = ['codex', 'sandbox', '-P', 'bench', '-C', str(work), *overrides, '--', *command]
        try:
            result = subprocess.run(args, cwd=work, env=env, capture_output=True, text=True, timeout=60)
            diagnostic = result.stderr
            permission_denied = 'Operation not permitted' in diagnostic or 'Permission denied' in diagnostic
            passed = result.returncode == 0 if allowed else result.returncode != 0 and permission_denied
            results.append({'check': name, 'exit_code': result.returncode, 'passed': passed,
                            'diagnostic': diagnostic[:2000]})
        except subprocess.TimeoutExpired:
            results.append({'check': name, 'passed': False, 'diagnostic': 'timeout'})
    report = {'root': str(parent), 'scope': 'sandboxed_local_commands_only',
              'profile_overrides': overrides, 'results': results,
              'passed': all(row['passed'] for row in results)}
    (parent / 'report.json').write_text(json.dumps(report, indent=2))
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--repo', type=Path, required=True)
    parser.add_argument('--directory', type=Path)
    arguments = parser.parse_args()
    result = preflight(arguments.repo, arguments.directory)
    print(json.dumps(result, indent=2))
    raise SystemExit(0 if result['passed'] else 1)
