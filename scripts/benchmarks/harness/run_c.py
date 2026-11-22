"""Pilot C runner: Claude Code `claude -p` across native / instructions / full arms, serial.

Real trials spend the account's model quota. --dry-run prepares everything a trial needs (source
snapshot, pinned `auto`, generated surface, per-arm settings, isolated home, sandboxed argv) and
the warm-up compile, then prints the plan without starting the agent.
"""
import argparse
import hashlib
import itertools
import json
import os
from pathlib import Path
import subprocess
import sys

import claude_agent
import surface_c
import trial_c
from gate_c import load_corpus
from oracle_c import oracle_files
from workspace import snapshot

HERE = Path(__file__).resolve().parent


def schedule(tasks: list, arms: tuple, replicates: int) -> list:
    """Every task runs every arm, replicate by replicate; arm order rotates through all permutations."""
    orders = list(itertools.permutations(arms))
    plan, slot = [], 0
    for replicate in range(1, replicates + 1):
        for task in tasks:
            plan += [(task, arm, replicate) for arm in orders[slot % len(orders)]]
            slot += 1
    return plan


def summarize(records: list) -> dict:
    """Per-arm totals; unknown token usage stays out of the sums and is counted apart."""
    arms = {}
    for row in records:
        arm = arms.setdefault(row['arm'], {'trials': 0, 'cases_passed': 0, 'cases_total': 0, 'review_passed': 0,
                                           'review_total': 0, 'timed_out': 0, 'operational_errors': 0,
                                           'scope_violations': 0, 'elapsed_seconds': 0.0, 'known_tokens': 0,
                                           'usage_unknown': 0, 'stop_hook_blocks': 0})
        oracle, observation = row['oracle'], row['observation']
        arm['trials'] += 1
        arm['cases_passed'] += oracle['passed']
        arm['cases_total'] += oracle['total']
        arm['review_passed'] += oracle['review_passed']
        arm['review_total'] += oracle['review_total']
        arm['timed_out'] += int(row['agent']['timed_out'])
        arm['operational_errors'] += int(row['operational_error'] is not None)
        arm['scope_violations'] += int(not row['scope']['accepted_scope'])
        arm['elapsed_seconds'] = round(arm['elapsed_seconds'] + row['agent']['elapsed_seconds'], 3)
        arm['stop_hook_blocks'] += len(observation['stop_hook_blocks'])
        if observation['total_tokens'] is None:
            arm['usage_unknown'] += 1
        else:
            arm['known_tokens'] += observation['total_tokens']
    return arms


def to_markdown(summary: dict, records: list) -> str:
    lines = ['# Pilot C report', '', '| arm | trials | oracle cases | review cases | timeouts | op errors |'
             ' scope | elapsed s | known tokens | usage unknown | stop blocks |', '|' + '---|' * 11]
    for arm, s in summary.items():
        lines.append('| %s | %d | %d/%d | %d/%d | %d | %d | %d | %.0f | %d | %d | %d |' % (
            arm, s['trials'], s['cases_passed'], s['cases_total'], s['review_passed'], s['review_total'],
            s['timed_out'], s['operational_errors'], s['scope_violations'], s['elapsed_seconds'], s['known_tokens'],
            s['usage_unknown'], s['stop_hook_blocks']))
    lines += ['', '| trial | oracle | review | elapsed s | tokens | error |', '|---|---|---|---|---|---|']
    for row in records:
        o = row['oracle']
        lines.append('| %s-%s-k%d | %d/%d | %d/%d | %.0f | %s | %s |' % (
            row['task_id'], row['arm'], row['replicate'], o['passed'], o['total'], o['review_passed'],
            o['review_total'], row['agent']['elapsed_seconds'], row['observation']['total_tokens'],
            row['operational_error']))
    return '\n'.join(lines) + '\n'


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def prepare_sources(ctx: dict, tasks: list, repo: Path) -> dict:
    """Per distinct base: snapshot, `auto` built from it, and the surface it generates."""
    sources = {}
    ctx['tools'] = surface_c.link_tools(ctx['tools_dir'], ctx['tool_names'], ctx['base_env'].get('PATH', ''))
    for base in sorted({task['heads']['base'] for task in tasks}):
        root = ctx['output'] / 'sources' / base[:12]
        tree = root / 'tree'
        snapshot(repo, base, tree)
        bin_dir = ctx['output'] / 'bin' / base[:12]
        env = surface_c.generation_env(ctx['base_env'], root / 'home', bin_dir, ctx['tools_dir'])
        env.update(trial_c.go_env(root, ctx['go_cache']))
        surface_c.build_auto(tree, bin_dir / 'auto', env)
        generated = surface_c.generate_surface(bin_dir / 'auto', tree, root, env)
        sources[base] = {'tree': tree, 'surface': root / 'surface', 'bin_dir': bin_dir, 'tools_dir': ctx['tools_dir'],
                         'generated': generated}
    return sources


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument('--repo', type=Path, required=True, help='checkout holding the task base commits')
    ap.add_argument('--output', type=Path, required=True, help='new directory outside --repo for the run')
    ap.add_argument('--tasks', default='', help='comma-separated task ids (default: all of corpus_c.json)')
    ap.add_argument('--arms', default=','.join(claude_agent.ARMS))
    ap.add_argument('--replicates', type=int, default=1)
    ap.add_argument('--model', required=True)
    ap.add_argument('--effort', default='high', choices=('low', 'medium', 'high', 'xhigh', 'max'))
    ap.add_argument('--timeout', type=int, default=1800, help='agent deadline in seconds per trial')
    ap.add_argument('--credential-file', type=Path, help='mode-0600 file with the token (not needed for --dry-run)')
    ap.add_argument('--credential-kind', choices=tuple(claude_agent.TOKEN_ENV), default='oauth')
    ap.add_argument('--permission-mode', default='bypassPermissions')
    ap.add_argument('--tools', default='go,gofmt,claude,node',
                    help='host tools linked onto the trial PATH (with /usr/bin:/bin:/usr/sbin:/sbin and auto)')
    ap.add_argument('--no-sandbox', action='store_true', help='debug only; recorded in the protocol')
    ap.add_argument('--dry-run', action='store_true')
    args = ap.parse_args(argv)
    arms = tuple(a for a in args.arms.split(',') if a)
    if any(arm not in claude_agent.ARMS for arm in arms):
        raise SystemExit('unknown arm in --arms')
    if not args.dry_run and not args.credential_file:
        raise SystemExit('--credential-file is required for a real run')
    repo, output = args.repo.resolve(), args.output.resolve()
    if repo == output or repo in output.parents or HERE.parents[2] in output.parents:
        raise SystemExit('--output must be outside the checkouts')
    if (output / 'protocol.json').exists():
        raise SystemExit('refusing to overwrite a frozen run')
    output.mkdir(parents=True, exist_ok=True)
    tasks = load_corpus(HERE / 'corpus_c.json', [i for i in args.tasks.split(',') if i])
    ctx = {'output': output, 'go_cache': output / 'go-cache', 'base_env': dict(os.environ), 'model': args.model,
           'effort': args.effort, 'timeout': args.timeout, 'permission_mode': args.permission_mode,
           'tools_dir': output / 'bin' / 'tools', 'tool_names': [t for t in args.tools.split(',') if t],
           'claude': str(output / 'bin' / 'tools' / 'claude'), 'sandbox': not args.no_sandbox, 'protected': [repo, HERE.parents[2]],
           'account_home': str(Path.home()), 'credential_kind': args.credential_kind,
           'credential': claude_agent.read_credential(args.credential_file) if args.credential_file else 'DRY-RUN'}
    ctx['go_cache'].mkdir(exist_ok=True)
    ctx['sources'] = prepare_sources(ctx, tasks, repo)
    plan = schedule(tasks, arms, args.replicates)
    if 'claude' not in ctx['tools']:
        raise SystemExit('--tools must include claude')
    version = subprocess.run([ctx['claude'], '--version'], capture_output=True, text=True).stdout.strip()
    protocol = {'schema': 'harness_pilot_c.v1', 'corpus_hash': sha(json.dumps(tasks, sort_keys=True).encode()),
                'oracle_hashes': {t['id']: {p.name: sha(p.read_bytes()) for p in oracle_files(t)} for t in tasks},
                'tasks': [t['id'] for t in tasks], 'arms': {a: claude_agent.arm_settings(a) for a in arms},
                'order': [(t['id'], a, k) for t, a, k in plan], 'model': args.model, 'effort': args.effort,
                'timeout_seconds': args.timeout, 'permission_mode': args.permission_mode, 'sandbox': ctx['sandbox'],
                'credential_kind': args.credential_kind, 'claude_version': version, 'tools': ctx['tools'],
                'go_version': subprocess.check_output(['go', 'version'], text=True).strip(),
                'surfaces': {b[:12]: {k: v for k, v in s['generated'].items() if k != 'manifest'}
                             for b, s in ctx['sources'].items()}, 'dry_run': args.dry_run}
    if args.dry_run:
        return dry_run(ctx, plan, protocol)
    (output / 'protocol.json').write_text(json.dumps(protocol, indent=2))
    records = []
    for index, (task, arm, replicate) in enumerate(plan, 1):
        print(json.dumps({'stage': 'start', 'index': index, 'total': len(plan), 'task': task['id'], 'arm': arm}),
              flush=True)
        row = trial_c.run(ctx, task, arm, replicate)
        records.append(row)
        (output / 'records.json').write_text(json.dumps(records, indent=2))
        summary = summarize(records)
        (output / 'report.json').write_text(json.dumps(summary, indent=2))
        (output / 'report.md').write_text(to_markdown(summary, records))
        print(json.dumps({'stage': 'done', 'index': index, 'task': task['id'], 'arm': arm,
                          'oracle': '%d/%d' % (row['oracle']['passed'], row['oracle']['total']),
                          'seconds': row['agent']['elapsed_seconds'], 'error': row['operational_error']}), flush=True)
    print('PILOT_C_COMPLETE', flush=True)
    return 0


def dry_run(ctx: dict, plan: list, protocol: dict) -> int:
    """Prepare the first trial of each arm and run its warm-up, never the agent."""
    print(json.dumps({k: v for k, v in protocol.items() if k != 'order'}, indent=2))
    print('order:', ' '.join('%s/%s/k%d' % entry for entry in protocol['order']))
    seen = set()
    for task, arm, replicate in plan:
        if arm in seen:
            continue
        seen.add(arm)
        trial = ctx['output'] / 'dry' / ('%s-%s' % (task['id'], arm))
        trial.mkdir(parents=True)
        prepared = trial_c.prepare(ctx, task, arm, trial)
        warm = trial_c.execute_exact(prepared['warm'], prepared['work'], prepared['env'], trial / 'warm.log',
                                     trial / 'warm.err', 1800)
        print(json.dumps({'trial': trial.name, 'settings': prepared['settings'], 'argv': prepared['argv'],
                          'prompt_sha256': sha(prepared['prompt'].encode()), 'warm': warm,
                          'harness_files': sorted(p.name for p in prepared['work'].iterdir()
                                                  if p.name in ('CLAUDE.md', '.claude', 'autopus.yaml'))}, indent=2))
    print('DRY_RUN_COMPLETE (no agent started)')
    return 0


if __name__ == '__main__':
    sys.exit(main())
