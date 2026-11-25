"""Pilot C runner: Claude Code `claude -p` across native / instructions / full arms, serial.

Real trials spend the account's model quota. --dry-run prepares everything a trial needs (pinned
harness binary, base snapshots, generated surfaces, warm caches, isolated home, sandboxed argv) and
runs one sandboxed warm-up per arm, then prints the plan without starting the agent. A run that
stopped can be continued with --resume: finished trials are kept and a partial one is set aside.
"""
import argparse
import itertools
import shutil
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import traceback

import claude_agent
import codex_c
import guard_c
import oracle_c
import report_c
import sources_c
import trial_c
import trial_env_c
from gate_c import load_corpus
from oracle_c import frozen_hash

HERE = Path(__file__).resolve().parent
STOP_SIGNALS = (signal.SIGTERM, signal.SIGINT, signal.SIGHUP)
TRIAL_ENVIRONMENT = {
    'module_cache': 'per-trial APFS clone (trial/gomodcache) of output/cache/modcache, filled from the host '
                    'download cache with GOPROXY=file:// and -mod=readonly; grading reads output/cache/modcache',
    'xcrun_db': '<stage temp>/xcrun_db', 'TMPPREFIX': '<stage temp>/zsh', 'GOPATH': '<trial>/gopath',
    'CGO_ENABLED': 'go default (1), identical for warm-up, agent and grading',
    'warm_up': 'go build ./... && go test -count=1 -run ^$ ./<oracle package>, sandboxed, before the agent clock'}


class Interrupted(BaseException):
    """A stop signal reached the runner. Not an Exception, so a trial's runner_exception handler cannot
    swallow it; execute_tree sweeps the running stage's tree while it propagates."""

    def __init__(self, signum: int):
        super().__init__(signum)
        self.signum = signum


def install_stop_handlers() -> list:
    """SIGTERM, SIGINT and SIGHUP raise Interrupted once; later ones are ignored so the sweep finishes.

    The agent runs in its own session, so `kill -TERM -- -<runner pgid>` alone never reaches it. A
    signal the runner inherited as ignored stays ignored: under `nohup` (README) SIGHUP is SIG_IGN, and
    closing the terminal must not stop an unattended run. Returns the signals that got the handler.
    """
    def handler(signum, _frame):
        for stop in STOP_SIGNALS:
            signal.signal(stop, signal.SIG_IGN)
        raise Interrupted(signum)
    installed = []
    for stop in STOP_SIGNALS:
        if signal.getsignal(stop) is signal.SIG_IGN:
            continue
        signal.signal(stop, handler)
        installed.append(stop)
    return installed


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
    ap.add_argument('--timeout', type=int, default=5400, help='agent deadline in seconds per trial')
    ap.add_argument('--credential-file', type=Path, help='mode-0600 file with the token (not needed for --dry-run)')
    ap.add_argument('--credential-kind', choices=tuple(claude_agent.TOKEN_ENV), default='oauth')
    ap.add_argument('--permission-mode', default='bypassPermissions')
    ap.add_argument('--tools', default='go,gofmt,claude,node',
                    help='host tools linked onto the trial PATH (with /usr/bin:/bin:/usr/sbin:/sbin and auto)')
    codex_c.add_arguments(ap)
    ap.add_argument('--no-sandbox', action='store_true', help='debug only; recorded in the protocol')
    ap.add_argument('--resume', action='store_true', help='continue the run already in --output')
    ap.add_argument('--keep-trials', action='store_true',
                    help='keep each graded trial workspace and grading tree (default: prune them, keeping measure/)')
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
    install_stop_handlers()
    output.mkdir(parents=True, exist_ok=True)
    frozen = guard_c.frozen_paths(output) if args.resume else guard_c.freeze(output, corpus)
    ctx = {'output': output, 'keep_trials': args.keep_trials, 'base_env': dict(os.environ), 'model': args.model,
           'effort': args.effort,
           'timeout': args.timeout, 'permission_mode': args.permission_mode, 'frozen': frozen,
           'tools_dir': output / 'bin' / 'tools', 'tool_names': codex_c.tool_names(args),
           'claude': str(output / 'bin' / 'tools' / 'claude'), 'sandbox': not args.no_sandbox,
           'shell': output / 'bin' / 'shell' / 'zsh',
           'protected': [repo, HERE.parents[2]], 'account_home': str(Path.home()), 'harness_revision': harness,
           'credential_kind': args.credential_kind,
           'codex': codex_c.trial_settings(args),
           'credential': claude_agent.read_credential(args.credential_file) if args.credential_file else 'DRY-RUN'}
    # Review cases that pass on base, so the report can show review without them (report_c).
    ctx['base_passing'] = report_c.base_passing(json.loads(args.gates.read_text())) if args.gates else {}
    ctx['sources'] = sources_c.prepare_sources(ctx, tasks, repo)
    if 'claude' not in ctx['tools']:
        raise SystemExit('--tools must include claude')
    plan = schedule(tasks, arms, args.replicates)
    protocol = {'schema': 'harness_pilot_c.v2', 'frozen_hash': frozen_hash(tasks, frozen['oracles']),
                'tasks': [t['id'] for t in tasks], 'arms': {a: claude_agent.arm_settings(a) for a in arms},
                'order': [(t['id'], a, k) for t, a, k in plan], 'model': args.model, 'effort': args.effort,
                'timeout_seconds': args.timeout, 'permission_mode': args.permission_mode, 'sandbox': ctx['sandbox'],
                'credential_kind': args.credential_kind, 'tools': ctx['tools'],
                'codex': codex_c.settings(args),
                'claude_version': subprocess.run([ctx['claude'], '--version'], capture_output=True,
                                                 text=True).stdout.strip(),
                'go_version_in_trials': sources_c.trial_go_version(ctx),
                'harness_revision': {'requested': args.harness_revision, 'sha': harness},
                'gates': str(args.gates) if args.gates else None,
                'surfaces': {b[:12]: {k: v for k, v in s['generated'].items() if k != 'manifest'}
                             for b, s in ctx['sources'].items()}, 'dry_run': args.dry_run,
                'trial_environment': TRIAL_ENVIRONMENT, 'modcache': modcache_size(ctx['modcache'])}
    codex_c.record_version(protocol, ctx)
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
                'surfaces', 'sandbox', 'permission_mode', 'trial_environment', 'codex'):
        # protocol.json stores tuples as lists, so compare both through the same JSON round trip.
        if frozen.get(key) != json.loads(json.dumps(protocol.get(key))):
            raise SystemExit('--resume with a different protocol: ' + key)


def prune_trial(row: dict, trial: Path, measure: Path, grading: Path) -> None:
    """Keep the graded candidate files under measure/, then drop the trial and grading trees.

    A full run clones a module cache, a build cache and a workspace per trial; kept, 30 trials need far
    more disk than a shared machine has. The candidate's production edits are copied from the grading
    tree (built by the runner from a base snapshot), never read through the trial's own .git, which the
    agent controls: a git command there could run its hooks or fsmonitor outside the sandbox.
    """
    tree = grading / 'tree'
    copied = ((row.get('oracle') or {}).get('overlay') or {}).get('copied') or []
    for rel in copied:
        source = tree / rel
        if source.is_file() and not source.is_symlink():
            target = measure / 'candidate' / rel
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(source, target)
    for path in (trial, grading):
        if path.exists():
            for sub in path.rglob('*'):
                if sub.is_dir() and not sub.is_symlink():
                    sub.chmod(0o700)
            shutil.rmtree(path, ignore_errors=True)


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
        except Interrupted as stop:
            # The trial's tree is already swept; without result.json --resume sets it aside and reruns it.
            name = signal.Signals(stop.signum).name
            measure.mkdir(parents=True, exist_ok=True)
            (measure / 'aborted.json').write_text(json.dumps({'signal': name, 'started': stamp}))
            print(json.dumps({'stage': 'aborted', 'index': index, 'task': task['id'], 'arm': arm, 'signal': name}),
                  flush=True)
            return 128 + stop.signum
        except Exception:
            # A runner failure is an observation of this trial, not the end of the run.
            row = {'task_id': task['id'], 'arm': arm, 'replicate': replicate, 'operational_error': 'runner_exception',
                   'traceback': traceback.format_exc()[-4000:], 'oracle': None}
            measure.mkdir(parents=True, exist_ok=True)
            trial_c.write_json(measure / 'result.json', row)
        records.append(row)
        # A runner exception leaves the trees for diagnosis and a re-grade; only graded trials are pruned.
        if not ctx.get('keep_trials') and row.get('operational_error') != 'runner_exception':
            prune_trial(row, trial, measure, grading)
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
    """Prepare one trial per task and arm, run its sandboxed warm-up and the environment self-test
    (trial_env_c.selftest), never the agent. Fails when a warm-up or a self-test step fails. Every task
    is covered because bases differ: each base has its own snapshot, surface and warm cache."""
    print(json.dumps({k: v for k, v in protocol.items() if k != 'order'}, indent=2))
    print('order:', ' '.join('%s/%s/k%d' % entry for entry in protocol['order']))
    seen, failed = set(), []
    for task, arm, _ in plan:
        if (task['id'], arm) in seen:
            continue
        seen.add((task['id'], arm))
        name = 'dry-%s-%s' % (task['id'], arm)
        prepared = trial_c.prepare(ctx, task, arm, name)
        warm = trial_c.execute_tree(prepared['warm'], prepared['work'], prepared['env'], prepared['measure'] /
                                    'warm.log', prepared['measure'] / 'warm.err', 1800, scratch=prepared['trial'])
        resolved = subprocess.run(prepared['path_probe'], cwd=prepared['work'], env=prepared['env'],
                                  capture_output=True, text=True, timeout=120).stdout.split()
        check = trial_env_c.selftest(prepared, oracle_c.present_packages(prepared['work'], task), trial_c.execute_tree)
        if warm['exit_code'] or not check['ok']:
            failed.append(name)
        print(json.dumps({'trial': name, 'argv': prepared['argv'], 'env_keys': sorted(prepared['env']),
                          'bash_tool_resolves': resolved,
                          'prompt_sha256': trial_c.digest(prepared['prompt'].encode()), 'warm': warm,
                          'modcache_clone': prepared['modcache_clone'], 'selftest': check,
                          'harness_files': sorted(p.name for p in prepared['work'].iterdir()
                                                  if p.name in ('CLAUDE.md', '.claude', 'autopus.yaml'))}, indent=2))
        print('SELFTEST %s %s go_test_compile=%.1fs warm=%.1fs' % (
            name, 'ok' if check['ok'] and not warm['exit_code'] else 'FAILED ' + ','.join(check['failed']),
            check['steps'].get('go_test_compile', {}).get('seconds', -1), warm['elapsed_seconds']), flush=True)
    if failed:
        print('DRY_RUN_FAILED (environment self-test): ' + ', '.join(failed))
        return 1
    print('DRY_RUN_COMPLETE (no agent started)')
    return 0


def modcache_size(path: Path) -> dict:
    """Files and apparent bytes of the run-level module cache (a trial's APFS clone shares the blocks)."""
    files = size = 0
    for directory, _dirs, names in os.walk(path):
        for entry in names:
            files += 1
            size += os.lstat(os.path.join(directory, entry)).st_size
    return {'files': files, 'apparent_bytes': size}

if __name__ == '__main__':
    sys.exit(main())
