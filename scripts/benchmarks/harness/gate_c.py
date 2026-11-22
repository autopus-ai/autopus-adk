"""Pilot C validity gates, checked without any model call (docs/benchmarks/harness-pilot-c-design.md).

Per task: (1) every oracle case passes on the reviewed-and-fixed head; (2) every `review` case fails
on the pre-review head, the defect the review found; (3) on base, with the pinned interface stubbed
when base lacks it, the oracle fails, and every `feature` case fails there. A `guard` case passes on
fixed and on base: behaviour the feature must keep, scored apart from defects. A case that fails its
gate is reported for dropping; this script never edits the corpus.

Every head is graded exactly as a trial is (grade_c.run_graded: grader.sb, a clone of a build cache
warmed on base), and gates.json records the corpus and oracle hash that run_c.py --gates checks.
"""
import argparse
import json
from pathlib import Path
import shutil
import sys

import grade_c
from oracle_c import ORACLES, frozen_hash
from workspace import snapshot

HERE = Path(__file__).resolve().parent


def load_corpus(path: Path = HERE / 'corpus_c.json', ids: list = ()) -> list:
    tasks = json.loads(Path(path).read_text())
    if ids:
        known = {task['id'] for task in tasks}
        missing = [i for i in ids if i not in known]
        if missing:
            raise SystemExit('unknown task ids: ' + ', '.join(missing))
        tasks = [task for task in tasks if task['id'] in ids]
    return tasks


def judge(task: dict, runs: dict) -> dict:
    """Gate verdicts from {head: run_oracle result}; pure, so it is unit-tested without Go."""
    cases = task['oracle']['cases']
    fixed, pre, base = (runs[h]['cases'] for h in ('fixed', 'pre_review', 'base'))
    rows, drop = [], []
    for case in cases:
        name, kind = case['name'], case['kind']
        satisfiable = fixed.get(name) == 'pass'
        if kind == 'review':
            discriminates = pre.get(name) == 'fail'
        elif kind == 'feature':
            discriminates = base.get(name) in ('fail', 'build_failed')
        elif kind == 'guard':
            # Behaviour base already had that the feature must keep: it guards against overreach.
            discriminates = base.get(name) == 'pass'
        else:
            raise ValueError('case kind must be review, feature or guard: ' + name)
        if not (satisfiable and discriminates):
            drop.append(name)
        rows.append({'case': name, 'kind': kind, 'fixed': fixed.get(name), 'pre_review': pre.get(name),
                     'base': base.get(name), 'gate1_satisfiable': satisfiable, 'discriminates': discriminates})
    base_values = [base.get(c['name']) for c in cases]
    gate3 = any(v in ('fail', 'build_failed') for v in base_values)
    stubbed = bool(task['oracle'].get('base_stub'))
    # Without a stub, a base that cannot compile the oracle shows only a missing name, not a defect.
    gate3_note = None
    if gate3 and not stubbed and all(v == 'build_failed' for v in base_values):
        gate3, gate3_note = False, 'base does not compile the oracle; add oracle.base_stub'
    return {'task': task['id'], 'cases': rows, 'gate1': all(r['gate1_satisfiable'] for r in rows),
            'gate2': all(r['discriminates'] for r in rows if r['kind'] == 'review'),
            'gate3': gate3, 'gate3_note': gate3_note, 'drop': drop,
            'build': {h: runs[h]['build'] for h in runs}}


def check_task(repo: Path, task: dict, work: Path, cache: Path) -> dict:
    runs = {}
    for head in ('fixed', 'pre_review', 'base'):
        root = work / task['id'] / head
        tree = root / 'tree'
        snapshot(repo, task['heads'][head], tree)
        if head == 'base' and task['oracle'].get('base_stub'):
            stub = ORACLES / task['id'] / task['oracle']['base_stub']
            shutil.copyfile(stub, tree / task['oracle']['package'] / stub.name.removesuffix('.oracle'))
        print(json.dumps({'stage': 'oracle', 'task': task['id'], 'head': head}), flush=True)
        runs[head] = grade_c.run_graded(tree, task, root, work / 'logs' / task['id'] / head, cache,
                                        grade_c.grader.PROFILE, ORACLES)
        shutil.rmtree(root)
    return judge(task, runs)


def table(verdict: dict) -> str:
    lines = ['%s  gate1=%s gate2=%s gate3=%s%s' % (verdict['task'], verdict['gate1'], verdict['gate2'],
                                                  verdict['gate3'], ' (' + verdict['gate3_note'] + ')'
                                                  if verdict['gate3_note'] else '')]
    for row in verdict['cases']:
        mark = 'ok  ' if row['gate1_satisfiable'] and row['discriminates'] else 'DROP'
        lines.append('  %s %-7s fixed=%-12s pre=%-12s base=%-12s %s' % (
            mark, row['kind'], row['fixed'], row['pre_review'], row['base'], row['case']))
    return '\n'.join(lines)


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--repo', type=Path, required=True, help='checkout holding every head commit')
    parser.add_argument('--work', type=Path, required=True, help='new empty directory for snapshots and logs')
    parser.add_argument('--tasks', default='', help='comma-separated task ids (default: all)')
    parser.add_argument('--corpus', type=Path, default=HERE / 'corpus_c.json')
    args = parser.parse_args(argv)
    work = args.work.resolve()
    if work.exists() and any(work.iterdir()):
        raise SystemExit('--work must be new or empty')
    work.mkdir(parents=True, exist_ok=True)
    tasks = load_corpus(args.corpus, [i for i in args.tasks.split(',') if i])
    verdicts = []
    for task in tasks:
        cache = work / 'warm-cache' / task['heads']['base'][:12]
        if not cache.exists():
            base = work / 'warm-src'
            snapshot(args.repo.resolve(), task['heads']['base'], base)
            grade_c.warm(base, [t['oracle']['package'] for t in tasks], cache, work / 'warm-tmp')
            shutil.rmtree(base)
        verdict = check_task(args.repo.resolve(), task, work, cache)
        verdicts.append(verdict)
        print(table(verdict), flush=True)
    ok = all(v['gate1'] and v['gate2'] and v['gate3'] and not v['drop'] for v in verdicts)
    (work / 'gates.json').write_text(json.dumps({'passed': ok, 'frozen_hash': frozen_hash(tasks, ORACLES),
                                                 'tasks': [t['id'] for t in tasks], 'verdicts': verdicts}, indent=2))
    print('GATES_PASSED' if ok else 'GATES_FAILED', flush=True)
    return 0 if ok else 1


if __name__ == '__main__':
    sys.exit(main())
