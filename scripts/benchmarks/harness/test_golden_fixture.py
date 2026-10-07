"""Fixture world for the golden-mode tests; it holds no test case and calls no model.

It builds a tiny Go module committed in its own git repository, a corpus over it, a golden set root
pinning that corpus, two arm surfaces and a scripted fake `codex` that records every call it gets.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
from types import SimpleNamespace

CHECKOUT = Path(__file__).resolve().parents[3]
PIN = 'codex-cli 0.160.0'
CORPUS_FILE = 'scripts/benchmarks/harness/corpus_fixture.json'
SOURCE = 'package {name}\n\n// Value is the fixture behavior the oracle protects.\nfunc Value(x int) int {{\n\treturn x + 1\n}}\n'
TESTS = ('package {name}\n\nimport "testing"\n\nfunc TestValue(t *testing.T) {{\n\tif Value(2) != 3 {{\n'
         '\t\tt.Fatalf("Value(2) = %d", Value(2))\n\t}}\n}}\n\nfunc TestZero(t *testing.T) {{\n'
         '\tif Value(-1) != 0 {{\n\t\tt.Fatal("Value(-1) != 0")\n\t}}\n}}\n')
EVENTS = ('{"type":"thread.started","thread_id":"fixture"}\n{"type":"turn.started"}\n'
          '{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":0,"output_tokens":5}}\n')

STUB = '''#!{python}
"""Fake codex: answers --version, records each exec call and acts on the workspace by a fixed script."""
import json, os, subprocess, sys, time
from pathlib import Path

CALLS, VERSION = Path({calls!r}), {version!r}
BEHAVIORS, MUTATIONS, REPAIRS, EVENTS = {behaviors!r}, {mutations!r}, {repairs!r}, {events!r}
args = sys.argv[1:]
if args == ['--version']:
    print(VERSION)
    sys.exit(0)
work = Path(args[args.index('-C') + 1])
prompt = sys.stdin.read()
allowed = prompt.split('Allowed production files: ', 1)[1].split('.\\n', 1)[0]
agents, config = work / 'AGENTS.md', work / '.codex' / 'config.toml'
policy = agents.read_text().strip() if agents.is_file() else ''
call = CALLS / ('call-%d.json' % time.monotonic_ns())
behavior = BEHAVIORS.get(allowed, 'policy')
call.write_text(json.dumps({{'argv': args, 'env': dict(os.environ), 'allowed': allowed, 'policy': policy,
                            'behavior': behavior, 'cwd': os.getcwd()}}))
if config.is_file() and 'FAKE-START: refuse' in config.read_text():
    sys.stderr.write('Error: the project configuration was rejected\\n')
    sys.exit(1)
sys.stdout.write(EVENTS)
sys.stdout.flush()


def repair():
    path, mutation = work / allowed, MUTATIONS[allowed]
    if allowed in REPAIRS:
        path.write_text(REPAIRS[allowed])
    else:
        path.write_text(path.read_text().replace(mutation['after'], mutation['before'], 1))


if behavior == 'policy' and 'FAKE-POLICY: repair' in policy:
    repair()
elif behavior == 'forbidden':
    repair()
    with open(work / allowed, 'a') as source:
        source.write('// calls os.Exit on purpose\\n')
elif behavior == 'scope':
    repair()
    with open(work / allowed.replace('.go', '_test.go'), 'a') as test:
        test.write('// edited by the agent\\n')
elif behavior == 'exit':
    repair()
    sys.exit(3)
elif behavior == 'background':
    repair()
    child = subprocess.Popen(['/bin/sleep', '300'], process_group=0, stdin=subprocess.DEVNULL,
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    (CALLS / ('child-%d.pid' % child.pid)).write_text(str(child.pid))
elif behavior == 'sleep':
    time.sleep(600)
'''


def git(repo: Path, *args: str) -> str:
    env = {'PATH': os.environ['PATH'], 'HOME': str(repo), 'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': os.devnull,
           'GIT_AUTHOR_NAME': 'Fixture', 'GIT_AUTHOR_EMAIL': 'fixture@localhost', 'GIT_COMMITTER_NAME': 'Fixture',
           'GIT_COMMITTER_EMAIL': 'fixture@localhost'}
    return subprocess.run(['git', '-c', 'commit.gpgsign=false', '-c', 'core.hooksPath=' + os.devnull, *args], cwd=repo,
                          env=env, check=True, capture_output=True, text=True).stdout.strip()


def corpus_entry(index: int, name: str, expected: list) -> dict:
    return {'id': 'x%02d' % index, 'category': 'fixture', 'prompt': f'Repair {name}.Value so its tests pass.',
            'mutation': {'path': f'{name}/{name}.go', 'before': 'return x + 1', 'after': 'return x + 2'},
            'oracle': {'command': ['go', 'test', '-p', '1', f'./{name}', '-run', '^(' + '|'.join(expected) + ')$',
                                   '-count=1']},
            'allowed_paths': [f'{name}/{name}.go'], 'difficulty': 'fixture', 'rationale': 'fixture'}


def task_document(entry: dict, digest: str, expected: list) -> dict:
    return {'schema_version': 'harness_golden_task.v1', 'id': 'GT-AGENT-' + entry['id'].upper(), 'kind': 'agent',
            'category': 'fixture', 'intent': entry['prompt'], 'outcome': 'The fixture oracle passes again.',
            'variants': [], 'assertions': [{'kind': 'file_exists', 'platform': 'codex', 'path': '.codex/config.toml'}],
            'corpus_ref': {'file': CORPUS_FILE, 'task_id': entry['id'], 'file_sha256': digest},
            'expected_tests': expected, 'provenance': {'kind': 'benchmark', 'ref': CORPUS_FILE + '#' + entry['id']},
            'status': {'state': 'active', 'reason': ''}}


def build_world(base: Path, names: list, behaviors: dict | None = None, *, k: int = 2, floor: float = 0.8,
                max_runs: int | None = None, timeout: int = 120, version: str = PIN,
                expected: dict | None = None, refuse_candidate: bool = False) -> SimpleNamespace:
    """Create the fixture world under base; `expected` overrides a package's expected test names and
    `refuse_candidate` gives the candidate arm a configuration the fake codex refuses to start with."""
    base = Path(base).resolve()
    repo, root, calls = base / 'repo', base / 'set', base / 'calls'
    for path in (repo, root / 'evals/harness/tasks/agent', root / 'evals/harness/fixtures', calls):
        path.mkdir(parents=True)
    (repo / 'go.mod').write_text('module example.com/golden\n\ngo 1.21\n')
    for name in names:
        (repo / name).mkdir()
        (repo / name / f'{name}.go').write_text(SOURCE.format(name=name))
        (repo / name / f'{name}_test.go').write_text(TESTS.format(name=name))
    git(repo, 'init', '--quiet', '--initial-branch=main')
    git(repo, 'add', '--all')
    git(repo, 'commit', '--quiet', '-m', 'fixture workspace')
    tests = {name: (expected or {}).get(name, ['TestValue', 'TestZero']) for name in names}
    corpus = [corpus_entry(index, name, tests[name]) for index, name in enumerate(names, 1)]
    data = json.dumps(corpus, indent=1).encode()
    (root / CORPUS_FILE).parent.mkdir(parents=True)
    (root / CORPUS_FILE).write_bytes(data)
    for entry, name in zip(corpus, names):
        document = task_document(entry, hashlib.sha256(data).hexdigest(), tests[name])
        (root / 'evals/harness/tasks/agent' / (document['id'] + '.json')).write_text(json.dumps(document, indent=2))
    shutil.copyfile(CHECKOUT / 'evals/harness/fixtures/codex-models.json', root / 'evals/harness/fixtures/codex-models.json')
    live = {'k': k, 'threshold_bp': -1000, 'completeness_floor': floor, 'max_agent_runs': max_runs or len(names) * k * 2,
            'trial_timeout_seconds': timeout, 'workspace_revision': git(repo, 'rev-parse', 'HEAD'),
            'baseline_ref': 'v0.50.123', 'model': 'gpt-6-astra'}
    pins = {'generator_version': 'v0.50.123', 'project_name': 'harness-golden',
            'codex_model_catalog': 'evals/harness/fixtures/codex-models.json', 'codex_cli_version': PIN,
            'opencode_cli_version': '1.18.7'}
    manifest = {'schema_version': 'harness_golden_set.v1', 'set_version': '1',
                'active_paths': ['evals/harness/tasks/agent'], 'floors': {'surface_tasks': 1, 'agent_tasks': 1},
                'live': live, 'pins': pins}
    (root / 'evals/harness/manifest.json').write_text(json.dumps(manifest, indent=2))
    surfaces = write_surfaces(base / 'surfaces', refuse_candidate)
    stub = write_stub(base / 'bin' / 'codex', calls, corpus, behaviors, version)
    return SimpleNamespace(base=base, repo=repo, root=root, calls=calls, surfaces=surfaces, stub=stub,
                           manifest=manifest, corpus=corpus, session=base / 'session')


def write_surfaces(surfaces: Path, refuse_candidate: bool = False) -> Path:
    """Two arm surfaces: the baseline AGENTS.md tells the fake codex to repair, the candidate one not to."""
    for arm, policy in (('baseline', 'repair'), ('candidate', 'idle')):
        (surfaces / arm / '.codex').mkdir(parents=True)
        (surfaces / arm / 'AGENTS.md').write_text(f'FAKE-POLICY: {policy}\n')
        refuse = '# FAKE-START: refuse\n' if refuse_candidate and arm == 'candidate' else ''
        (surfaces / arm / '.codex' / 'config.toml').write_text('project_doc_max_bytes = 262144\n' + refuse)
    return surfaces


def write_stub(path: Path, calls_dir: Path, corpus: list, behaviors: dict | None = None, version: str = PIN,
               repairs: dict | None = None) -> Path:
    """The fake codex executable for `corpus`; it repairs a task when its behavior says so, writing the clean
    file from `repairs` when given and otherwise reversing the mutation text."""
    path.parent.mkdir(parents=True, exist_ok=True)
    calls_dir.mkdir(parents=True, exist_ok=True)
    mutations = {entry['allowed_paths'][0]: entry['mutation'] for entry in corpus}
    path.write_text(STUB.format(python=sys.executable, calls=str(calls_dir), version=version,
                                behaviors=behaviors or {}, mutations=mutations, repairs=repairs or {}, events=EVENTS))
    path.chmod(0o755)
    return path


def calls(world: SimpleNamespace) -> list:
    """The recorded fake codex exec calls, in call order."""
    paths = sorted(world.calls.glob('call-*.json'), key=lambda path: int(path.stem.split('-')[1]))
    return [json.loads(path.read_text()) for path in paths]


def argv(world: SimpleNamespace, *extra: str) -> list:
    """golden.py arguments for the world: its set, workspace repository, surfaces and fake codex."""
    return ['--output', str(world.session), '--surfaces', str(world.surfaces), '--repo', str(world.repo),
            '--dir', str(world.root), '--codex', str(world.stub), *extra]
