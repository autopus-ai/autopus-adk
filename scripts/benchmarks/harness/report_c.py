"""Pilot C per-arm summary and Markdown report. Failed, timed-out and ungraded trials stay counted.

Some review cases already pass on base (the defect came with the feature): an unchanged candidate
passes them too. So review is reported raw and excluding the cases gates.json records as passing on
base; unknown (no gates) stays unknown.
"""

KINDS = ('review', 'feature', 'guard')


def base_passing(gates: dict) -> dict:
    """{task id: review case names that passed on base} from a gate_c gates.json."""
    return {v['task']: [r['case'] for r in v['cases'] if r['kind'] == 'review' and r['base'] == 'pass']
            for v in gates.get('verdicts', [])}


def review_beyond_base(oracle: dict, names: list | None) -> dict:
    """review_base_passing (how many review cases pass on base) and the review count without them."""
    if names is None:
        return {'review_base_passing': None, 'review_new_passed': None, 'review_new_total': None}
    on_base = sum(1 for name in names if oracle['cases'].get(name) == 'pass')
    return {'review_base_passing': len(names), 'review_new_passed': oracle['review_passed'] - on_base,
            'review_new_total': oracle['review_total'] - len(names)}


def summarize(records: list) -> dict:
    """Per-arm totals; unknown token usage stays out of the sums and is counted apart."""
    arms = {}
    for row in records:
        arm = arms.setdefault(row['arm'], {'trials': 0, 'ungraded': 0, 'timed_out': 0, 'operational_errors': 0,
                                           'scope_violations': 0, 'arm_invalid': 0, 'contaminated': 0,
                                           'elapsed_seconds': 0.0, 'known_tokens': 0, 'usage_unknown': 0,
                                           'stop_hook_blocks': 0, 'env_override_trials': 0, 'review_new_passed': 0,
                                           'review_new_total': 0, 'review_new_unknown': 0, 'codex_sessions': 0,
                                           'codex_tokens': 0, 'codex_usage_unknown': 0,
                                           **{k + '_passed': 0 for k in KINDS},
                                           **{k + '_total': 0 for k in KINDS}})
        arm['trials'] += 1
        agent, observation, oracle = row.get('agent') or {}, row.get('observation') or {}, row.get('oracle')
        if oracle is None:
            arm['ungraded'] += 1
        else:
            for key in [k + suffix for k in KINDS for suffix in ('_passed', '_total')]:
                arm[key] += oracle.get(key, 0)
            if oracle.get('review_new_total') is None:
                arm['review_new_unknown'] += 1
            else:
                arm['review_new_passed'] += oracle['review_new_passed']
                arm['review_new_total'] += oracle['review_new_total']
        arm['timed_out'] += int(bool(agent.get('timed_out')))
        arm['operational_errors'] += int(row.get('operational_error') is not None)
        arm['scope_violations'] += int(not (row.get('scope') or {}).get('accepted_scope', False))
        arm['arm_invalid'] += int(row.get('arm_valid') is False)
        arm['contaminated'] += int(bool(observation.get('contamination')))
        arm['elapsed_seconds'] = round(arm['elapsed_seconds'] + agent.get('elapsed_seconds', 0.0), 3)
        arm['stop_hook_blocks'] += len(observation.get('stop_hook_blocks') or [])
        arm['env_override_trials'] += int(bool(observation.get('env_overrides')))
        _add_codex(arm, row.get('codex'))
        if observation.get('total_tokens') is None:
            arm['usage_unknown'] += 1
        else:
            arm['known_tokens'] += observation['total_tokens']
    return arms


def _add_codex(arm: dict, codex: dict | None) -> None:
    """OpenAI usage of the trial's codex sessions (codex_c.observe); a session without usage is unknown."""
    if not codex:
        return
    arm['codex_sessions'] += codex.get('sessions', 0)
    arm['codex_tokens'] += (codex.get('tokens') or {}).get('total_tokens', 0)
    # A trial whose usage cannot be whole (codex_observe_c.reconcile) counts as unknown, never as a sum.
    arm['codex_usage_unknown'] += int(bool(codex.get('usage_unknown')))


def _codex_table(summary: dict, records: list) -> list:
    """OpenAI (codex) usage per arm and per trial; empty when no trial ran with codex."""
    rows = [row for row in records if row.get('codex')]
    if not rows:
        return []
    lines = ['', '## Codex (OpenAI) usage', '', '| arm | sessions | total tokens (known) | trials usage unknown |',
             '|' + '---|' * 4]
    lines += ['| %s | %d | %d | %d |' % (arm, s['codex_sessions'], s['codex_tokens'], s['codex_usage_unknown'])
              for arm, s in summary.items() if s['codex_sessions'] or s['codex_usage_unknown']]
    lines += ['', '| trial | sessions | input | cached input | output | reasoning | total | flags |', '|' + '---|' * 8]
    for row in rows:
        codex, tokens = row['codex'], row['codex'].get('tokens') or {}
        flags = sum(1 for flag in (row.get('observation') or {}).get('contamination') or [] if 'CODEX' in flag)
        lines.append('| %s-%s-k%d | %d | %s | %s | %s | %s | %s | %d |' % (
            row['task_id'], row['arm'], row['replicate'], codex.get('sessions', 0),
            *(tokens.get(key, '-') for key in ('input_tokens', 'cached_input_tokens', 'output_tokens',
                                                'reasoning_output_tokens', 'total_tokens')), flags))
    return lines


def _cases(o: dict | None, kind: str) -> str:
    return '-' if o is None else '%d/%d' % (o.get(kind + '_passed', 0), o.get(kind + '_total', 0))


def _new(o: dict | None) -> str:
    if o is None or o.get('review_new_total') is None:
        return '-'
    return '%d/%d' % (o['review_new_passed'], o['review_new_total'])


def without(records: list, excluded) -> list:
    """The records of every task not in excluded. A saturated task (every arm passes all of it, or none
    does) cannot tell arms apart, so the arm comparison may leave it out; the per-trial table keeps it."""
    excluded = set(excluded)
    return [row for row in records if row['task_id'] not in excluded]


def to_markdown(summary: dict, records: list, excluded=()) -> str:
    lines = ['# Pilot C report', ''] + _arm_table(summary)
    if excluded:
        lines += ['', '## Arm comparison without saturated tasks (%s)' % ', '.join(sorted(excluded)), '']
        lines += _arm_table(summarize(without(records, excluded)))
    lines += ['', '| trial | review | review excl. base | feature | guard | elapsed s | tokens | error | env overrides |',
              '|' + '---|' * 9]
    for row in records:
        o = row.get('oracle')
        overrides = sorted({v for o in (row.get('observation') or {}).get('env_overrides') or [] for v in o['vars']})
        lines.append('| %s-%s-k%d | %s | %s | %s | %s | %.0f | %s | %s | %s |' % (
            row['task_id'], row['arm'], row['replicate'], _cases(o, 'review'), _new(o), _cases(o, 'feature'),
            _cases(o, 'guard'), (row.get('agent') or {}).get('elapsed_seconds', 0.0),
            (row.get('observation') or {}).get('total_tokens'), row.get('operational_error'),
            ', '.join(overrides) or '-'))
    lines += _codex_table(summary, records)
    return '\n'.join(lines) + '\n'


def _arm_table(summary: dict) -> list:
    lines = ['| arm | trials | review | feature | guard | ungraded | timeouts | op errors | scope | invalid arm |'
             ' contaminated | elapsed s | known tokens | usage unknown | stop blocks | env overrides | review excl. base |',
             '|' + '---|' * 17]
    for arm, s in summary.items():
        lines.append('| %s | %d | %d/%d | %d/%d | %d/%d | %d | %d | %d | %d | %d | %d | %.0f | %d | %d | %d | %d | %s |' % (
            arm, s['trials'], s['review_passed'], s['review_total'], s['feature_passed'], s['feature_total'],
            s['guard_passed'], s['guard_total'], s['ungraded'], s['timed_out'], s['operational_errors'],
            s['scope_violations'], s['arm_invalid'], s['contaminated'], s['elapsed_seconds'], s['known_tokens'],
            s['usage_unknown'], s['stop_hook_blocks'], s['env_override_trials'],
            '%d/%d%s' % (s['review_new_passed'], s['review_new_total'],
                         ' (+%d unknown)' % s['review_new_unknown'] if s['review_new_unknown'] else '')))
    return lines
