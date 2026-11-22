"""Pilot C runner: Claude Code `claude -p` across native / instructions / full arms, serial.

Real trials spend the account's model quota. --dry-run prepares everything a trial needs (pinned
harness binary, base snapshots, generated surfaces, warm caches, isolated home, sandboxed argv) and
runs one sandboxed warm-up per arm, then prints the plan without starting the agent. A run that
stopped can be continued with --resume: finished trials are kept and a partial one is set aside.
"""
import argparse
import itertools
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import traceback

import claude_agent
import guard_c
import report_c
import sources_c
import trial_c
from gate_c import load_corpus
from oracle_c import frozen_hash

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


def parse(argv) -> argparse.Namespace:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument('--repo', type=Path, required=True, help='checkout holding the task and harness commits')
    ap.add_argument('--output', type=Path, required=True,
                    help='new directory outside the checkouts and below no CLAUDE.md or .claude/')
    ap.add_argument('--tasks', default='', help='comma-separated task ids (default: all of corpus_c.json)')
    ap.add_argument('--arms', default=','.join(claude_agent.ARMS))
    ap.add_argument('--replicates', type=int, default=1)
    ap.add_argument('--harness-revision', default='origin/main',
                    help='revision whose auto generates the surface and runs the hooks (resolved to a SHA); it '
                         'must not contain any task pre-review or fixed head')
    ap.add_argument('--gates', type=Path, help='gates.json from gate_c.py for this corpus (required unless --dry-run)')
    ap.add_argument('--model', required=True)
    ap.add_argument('--effort', default='high', choices=('low', 'medium', 'high', 'xhigh', 'max'))
    ap.add_argument('--timeout', type=int, default=1800, help='agent deadline in seconds per trial')
    ap.add_argument('--credential-file', type=Path, help='mode-0600 file with the token (not needed for --dry-run)')
    ap.add_argument('--credential-kind', choices=tuple(claude_agent.TOKEN_ENV), default='oauth')
    ap.add_argument('--permission-mode', default='bypassPermissions')
    ap.add_argument('--tools', default='go,gofmt,claude,node',
                    help='host tools linked onto the trial PATH (with /usr/bin:/bin:/usr/sbin:/sbin and auto)')
    ap.add_argument('--no-sandbox', action='store_true', help='debug only; recorded in the protocol')
    ap.add_argument('--resume', action='store_true', help='continue the run already in --output')
    ap.add_argument('--dry-run', action='store_true')
    return ap.parse_args(argv)


def main(argv=None) -> int:
    args = parse(argv)
    arms = tuple(a for a in args.arms.split(',') if a)
    if any(arm not in claude_agent.ARMS for arm in arms):
        raise SystemExit('unknown arm in --arms')
    if not args.dry_run and not args.credential_file:
        raise SystemExit('--credential-file is required for a real run')
    repo, output = args.repo.resolve(), args.output.resolve()
    if repo == output or repo in output.parents or HERE.parents[2] in output.parents:
        raise SystemExit('--output must be outside the checkouts')
    if (output / 'protocol.json').exists() != args.resume:
        raise SystemExit('--resume needs an existing run' if args.resume else 'refusing to overwrite a frozen run')
    if not args.resume and output.exists() and any(output.iterdir()):
        raise SystemExit('--output must be new or empty')
    harness = sources_c.resolve_revision(repo, args.harness_revision)
    corpus = guard_c.frozen_paths(output)['corpus'] if args.resume else HERE / 'corpus_c.json'
    oracles = guard_c.frozen_paths(output)['oracles'] if args.resume else HERE / 'oracles'
    tasks = load_corpus(corpus, [i for i in args.tasks.split(',') if i])
    guard_c.preflight(repo, output, harness, tasks, args.gates, oracles, args.dry_run)
    output.mkdir(parents=True, exist_ok=True)
    frozen = guard_c.frozen_paths(output) if args.resume else guard_c.freeze(output, corpus)
    ctx = {'output': output, 'base_env': dict(os.environ), 'model': args.model, 'effort': args.effort,
           'timeout': args.timeout, 'permission_mode': args.permission_mode, 'frozen': frozen,
           'tools_dir': output / 'bin' / 'tools', 'tool_names': [t for t in args.tools.split(',') if t],
           'claude': str(output / 'bin' / 'tools' / 'claude'), 'sandbox': not args.no_sandbox,
           'shell': output / 'bin' / 'shell' / 'zsh',
           'protected': [repo, HERE.parents[2]], 'account_home': str(Path.home()), 'harness_revision': harness,
           'credential_kind': args.credential_kind,
           'credential': claude_agent.read_credential(args.credential_file) if args.credential_file else 'DRY-RUN'}
    ctx['sources'] = sources_c.prepare_sources(ctx, tasks, repo)
    if 'claude' not in ctx['tools']:
        raise SystemExit('--tools must include claude')
    plan = schedule(tasks, arms, args.replicates)
    protocol = {'schema': 'harness_pilot_c.v2', 'frozen_hash': frozen_hash(tasks, frozen['oracles']),
                'tasks': [t['id'] for t in tasks], 'arms': {a: claude_agent.arm_settings(a) for a in arms},
                'order': [(t['id'], a, k) for t, a, k in plan], 'model': args.model, 'effort': args.effort,
                'timeout_seconds': args.timeout, 'permission_mode': args.permission_mode, 'sandbox': ctx['sandbox'],
                'credential_kind': args.credential_kind, 'tools': ctx['tools'],
                'claude_version': subprocess.run([ctx['claude'], '--version'], capture_output=True,
                                                 text=True).stdout.strip(),
                'go_version_in_trials': sources_c.trial_go_version(ctx),
                'harness_revision': {'requested': args.harness_revision, 'sha': harness},
                'gates': str(args.gates) if args.gates else None,
                'surfaces': {b[:12]: {k: v for k, v in s['generated'].items() if k != 'manifest'}
                             for b, s in ctx['sources'].items()}, 'dry_run': args.dry_run}
    if args.dry_run:
        return dry_run(ctx, plan, protocol)
    if args.resume:
        check_resume(output, protocol)
    else:
        (output / 'protocol.json').write_text(json.dumps(protocol, indent=2))
    return run_plan(ctx, plan)


def check_resume(output: Path, protocol: dict) -> None:
    frozen = json.loads((output / 'protocol.json').read_text())
    for key in ('frozen_hash', 'tasks', 'arms', 'order', 'model', 'effort', 'timeout_seconds', 'harness_revision',
                'surfaces', 'sandbox', 'permission_mode'):
        if frozen.get(key) != protocol.get(key):
            raise SystemExit('--resume with a different protocol: ' + key)


def run_plan(ctx: dict, plan: list) -> int:
    output = ctx['output']
    records = []
    for index, (task, arm, replicate) in enumerate(plan, 1):
        name = '%s-%s-k%d' % (task['id'], arm, replicate)
        trial, measure, grading = trial_c.names(ctx, name)
        if (measure / 'result.json').is_file():
            records.append(json.loads((measure / 'result.json').read_text()))
            continue
        stamp = time.strftime('%Y%m%d-%H%M%S')
        for partial in (trial, measure, grading):
            if partial.exists():
                (output / 'aborted').mkdir(exist_ok=True)
                partial.rename(output / 'aborted' / ('%s-%s-%s' % (partial.parent.name, name, stamp)))
        print(json.dumps({'stage': 'start', 'index': index, 'total': len(plan), 'task': task['id'], 'arm': arm}),
              flush=True)
        try:
            row = trial_c.run(ctx, task, arm, replicate)
        except Exception:
            # A runner failure is an observation of this trial, not the end of the run.
            row = {'task_id': task['id'], 'arm': arm, 'replicate': replicate, 'operational_error': 'runner_exception',
                   'traceback': traceback.format_exc()[-4000:], 'oracle': None}
            measure.mkdir(parents=True, exist_ok=True)
            (measure / 'result.json').write_text(json.dumps(row, indent=2))
        records.append(row)
        (output / 'records.json').write_text(json.dumps(records, indent=2))
        summary = report_c.summarize(records)
        (output / 'report.json').write_text(json.dumps(summary, indent=2))
        (output / 'report.md').write_text(report_c.to_markdown(summary, records))
        oracle = row.get('oracle')
        print(json.dumps({'stage': 'done', 'index': index, 'task': task['id'], 'arm': arm,
                          'review': None if oracle is None else '%d/%d' % (oracle['review_passed'], oracle['review_total']),
                          'seconds': (row.get('agent') or {}).get('elapsed_seconds'),
                          'error': row['operational_error']}), flush=True)
    print('PILOT_C_COMPLETE', flush=True)
    return 0


def dry_run(ctx: dict, plan: list, protocol: dict) -> int:
    """Prepare the first trial of each arm and run its sandboxed warm-up, never the agent."""
    print(json.dumps({k: v for k, v in protocol.items() if k != 'order'}, indent=2))
    print('order:', ' '.join('%s/%s/k%d' % entry for entry in protocol['order']))
    seen = set()
    for task, arm, _ in plan:
        if arm in seen:
            continue
        seen.add(arm)
        name = 'dry-%s-%s' % (task['id'], arm)
        prepared = trial_c.prepare(ctx, task, arm, name)
        warm = trial_c.execute_tree(prepared['warm'], prepared['work'], prepared['env'], prepared['measure'] /
                                    'warm.log', prepared['measure'] / 'warm.err', 1800, scratch=prepared['trial'])
        resolved = subprocess.run(prepared['path_probe'], cwd=prepared['work'], env=prepared['env'],
                                  capture_output=True, text=True, timeout=120).stdout.split()
        print(json.dumps({'trial': name, 'argv': prepared['argv'], 'env_keys': sorted(prepared['env']),
                          'bash_tool_resolves': resolved,
                          'prompt_sha256': trial_c.digest(prepared['prompt'].encode()), 'warm': warm,
                          'harness_files': sorted(p.name for p in prepared['work'].iterdir()
                                                  if p.name in ('CLAUDE.md', '.claude', 'autopus.yaml'))}, indent=2))
    print('DRY_RUN_COMPLETE (no agent started)')
    return 0


if __name__ == '__main__':
    sys.exit(main())
