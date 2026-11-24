"""Pilot C re-grade: grade a finished run's kept candidates against a corpus and oracle set again,
without any agent or model call, into a new output directory.

A run keeps each graded trial's production edits in measure/<trial>/candidate/ (run_c.prune_trial)
and the overlay it applied in result.json (oracle.overlay: copied and deleted paths). The graded tree
is rebuilt from those, exactly as grade_c.overlay built it: the task's base snapshot, the kept files,
and the recorded deletions. Then the (new) oracle runs as in a trial (grade_c.run_graded, grader.sb,
a build cache warmed on base). The source run is only read; everything is written below --output.

A re-grade is only valid when the agent saw the same task, so a task whose prompt, heads or
allowed_paths differ from the source run's frozen corpus is refused. A trial whose kept files do not
match its recorded overlay (still being pruned, or kept with --keep-trials) is not re-graded and is
counted as ungraded, as are the trials the source run could not grade.
"""
import argparse
import json
from pathlib import Path
import shutil
import sys

import grade_c
from gate_c import load_corpus
import oracle_c
import report_c
from workspace import snapshot

HERE = Path(__file__).resolve().parent
SAME_TASK = ('prompt', 'heads', 'allowed_paths')


def check_compatible(frozen: list, tasks: list) -> None:
    """Refuse a task the source run's agents did not see in this form."""
    known = {task['id']: task for task in frozen}
    for task in tasks:
        source = known.get(task['id'])
        if source is None:
            raise SystemExit('task %s is not in the source run' % task['id'])
        changed = [key for key in SAME_TASK if source.get(key) != task.get(key)]
        if changed:
            raise SystemExit('task %s differs from the source run in %s: its trials cannot be re-graded'
                             % (task['id'], ', '.join(changed)))


def check_output(run: Path, output: Path) -> None:
    if output == run or run in output.parents or output in run.parents:
        raise SystemExit('--output must lie outside the source run')
    if output.exists() and any(output.iterdir()):
        raise SystemExit('--output must be new or empty')


def kept_files(measure: Path) -> list:
    candidate = measure / 'candidate'
    if not candidate.is_dir():
        return []
    return sorted(p.relative_to(candidate).as_posix() for p in candidate.rglob('*') if p.is_file())


def rebuild(base_tree: Path, measure: Path, overlay: dict, dest: Path) -> None:
    """dest = base + measure/candidate + the recorded deletions; refuses a candidate that does not
    match the overlay the source run recorded."""
    copied, kept = sorted(overlay.get('copied') or []), kept_files(measure)
    if copied != kept:
        raise ValueError('kept candidate files differ from the recorded overlay')
    dest.parent.mkdir(parents=True, exist_ok=True)
    grade_c.clone_tree(base_tree, dest)
    for rel in copied:
        target = dest / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        if target.is_symlink() or target.exists():
            target.unlink()
        shutil.copyfile(measure / 'candidate' / rel, target)
    for rel in overlay.get('deleted') or []:
        (dest / rel).unlink(missing_ok=True)


def source_rows(run: Path, ids: set) -> list:
    rows = []
    for result in sorted((run / 'measure').glob('*/result.json')):
        row = json.loads(result.read_text())
        if row.get('task_id') in ids:
            rows.append((result.parent, row))
    return rows


def regrade_row(row: dict, measure: Path, task: dict, ctx: dict) -> dict:
    """The source row with its oracle replaced by the new grade; source verdicts kept under regrade."""
    old = row.get('oracle')
    name = measure.name
    note = {'source_run': str(ctx['run']), 'source_passed': None if old is None else old.get('passed'),
            'source_total': None if old is None else old.get('total'), 'status': 'regraded'}
    out = dict(row)
    if old is None:
        note['status'] = 'ungraded_in_source'
    else:
        root = ctx['output'] / 'grading' / name
        try:
            rebuild(ctx['bases'][task['heads']['base']], measure, old.get('overlay') or {}, root / 'tree')
        except ValueError as error:
            note['status'], out['oracle'] = 'candidate_mismatch: %s' % error, None
        else:
            result = grade_c.run_graded(root / 'tree', task, root, ctx['output'] / 'measure' / name / 'grade',
                                        ctx['caches'][task['heads']['base']], grade_c.grader.PROFILE, ctx['oracles'])
            graded = {**grade_c.score(task, result), 'overlay': old.get('overlay')}
            graded.update(report_c.review_beyond_base(graded, ctx['base_passing'].get(task['id'])))
            out['oracle'] = graded
            if not ctx['keep']:
                shutil.rmtree(root, ignore_errors=True)
    out['regrade'] = note
    return out


def prepare_bases(repo: Path, output: Path, tasks: list) -> tuple:
    """One base snapshot and one warmed grading build cache per base head."""
    bases, caches = {}, {}
    for task in tasks:
        head = task['heads']['base']
        if head in bases:
            continue
        bases[head] = output / 'sources' / head[:12]
        snapshot(repo, head, bases[head])
        caches[head] = output / 'cache' / head[:12]
        packages = [p for t in tasks if t['heads']['base'] == head for p in oracle_c.packages(t)]
        grade_c.warm(bases[head], packages, caches[head], output / 'warm-tmp' / head[:12])
    return bases, caches


def write_reports(output: Path, records: list, excluded: list) -> None:
    summary = report_c.summarize(records)
    (output / 'records.json').write_text(json.dumps(records, indent=2))
    (output / 'report.json').write_text(json.dumps(summary, indent=2))
    (output / 'report.md').write_text(report_c.to_markdown(summary, records, excluded))


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument('--repo', type=Path, required=True, help='checkout holding every base commit')
    ap.add_argument('--run', type=Path, required=True, help='finished run_c.py --output (only read)')
    ap.add_argument('--output', type=Path, required=True, help='new empty directory for the re-grade')
    ap.add_argument('--corpus', type=Path, default=HERE / 'corpus_c.json')
    ap.add_argument('--oracles', type=Path, default=oracle_c.ORACLES)
    ap.add_argument('--gates', type=Path, help='gates.json of gate_c.py for this corpus (review excl. base)')
    ap.add_argument('--tasks', default='', help='comma-separated task ids (default: every task of the run)')
    ap.add_argument('--exclude-from-comparison', default='',
                    help='comma-separated saturated task ids left out of the arm comparison table')
    ap.add_argument('--report-only', action='store_true', help='no grading: report the source records')
    ap.add_argument('--keep-grading', action='store_true', help='keep each rebuilt grading tree')
    args = ap.parse_args(argv)
    run, output = args.run.resolve(), args.output.resolve()
    check_output(run, output)
    excluded = [i for i in args.exclude_from_comparison.split(',') if i]
    frozen = json.loads((run / 'frozen' / 'corpus_c.json').read_text())
    ids = [i for i in args.tasks.split(',') if i] or [t['id'] for t in frozen]
    output.mkdir(parents=True, exist_ok=True)
    if args.report_only:
        write_reports(output, [row for _, row in source_rows(run, set(ids))], excluded)
        return 0
    tasks = load_corpus(args.corpus, ids)
    check_compatible(frozen, tasks)
    gates = json.loads(args.gates.read_text()) if args.gates else None
    # gates.json hashes the tasks it checked, which may be a subset of the re-graded ones.
    if gates is not None and gates.get('frozen_hash') != oracle_c.frozen_hash(
            load_corpus(args.corpus, gates.get('tasks') or []), args.oracles):
        raise SystemExit('--gates was made for another corpus or oracle set')
    bases, caches = prepare_bases(args.repo.resolve(), output, tasks)
    ctx = {'run': run, 'output': output, 'oracles': args.oracles.resolve(), 'bases': bases, 'caches': caches,
           'base_passing': report_c.base_passing(gates) if gates else {}, 'keep': args.keep_grading}
    by_id = {task['id']: task for task in tasks}
    (output / 'regrade.json').write_text(json.dumps({
        'source_run': str(run), 'corpus': str(args.corpus.resolve()), 'oracles': str(ctx['oracles']),
        'frozen_hash': oracle_c.frozen_hash(tasks, ctx['oracles']), 'gates': str(args.gates) if args.gates else None,
        'tasks': ids, 'excluded_from_comparison': excluded}, indent=2))
    records = []
    for measure, row in source_rows(run, set(ids)):
        print(json.dumps({'stage': 'regrade', 'trial': measure.name}), flush=True)
        record = regrade_row(row, measure, by_id[row['task_id']], ctx)
        target = output / 'measure' / measure.name
        target.mkdir(parents=True, exist_ok=True)
        (target / 'result.json').write_text(json.dumps(record, indent=2))
        records.append(record)
        write_reports(output, records, excluded)
    write_reports(output, records, excluded)
    print('REGRADE_COMPLETE', flush=True)
    return 0


if __name__ == '__main__':
    sys.exit(main())
