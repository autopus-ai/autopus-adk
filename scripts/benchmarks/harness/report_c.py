"""Pilot C per-arm summary and Markdown report. Failed, timed-out and ungraded trials stay counted."""

KINDS = ('review', 'feature', 'guard')


def summarize(records: list) -> dict:
    """Per-arm totals; unknown token usage stays out of the sums and is counted apart."""
    arms = {}
    for row in records:
        arm = arms.setdefault(row['arm'], {'trials': 0, 'ungraded': 0, 'timed_out': 0, 'operational_errors': 0,
                                           'scope_violations': 0, 'arm_invalid': 0, 'contaminated': 0,
                                           'elapsed_seconds': 0.0, 'known_tokens': 0, 'usage_unknown': 0,
                                           'stop_hook_blocks': 0, 'env_override_trials': 0, **{k + '_passed': 0 for k in KINDS},
                                           **{k + '_total': 0 for k in KINDS}})
        arm['trials'] += 1
        agent, observation, oracle = row.get('agent') or {}, row.get('observation') or {}, row.get('oracle')
        if oracle is None:
            arm['ungraded'] += 1
        else:
            for key in [k + suffix for k in KINDS for suffix in ('_passed', '_total')]:
                arm[key] += oracle.get(key, 0)
        arm['timed_out'] += int(bool(agent.get('timed_out')))
        arm['operational_errors'] += int(row.get('operational_error') is not None)
        arm['scope_violations'] += int(not (row.get('scope') or {}).get('accepted_scope', False))
        arm['arm_invalid'] += int(row.get('arm_valid') is False)
        arm['contaminated'] += int(bool(observation.get('contamination')))
        arm['elapsed_seconds'] = round(arm['elapsed_seconds'] + agent.get('elapsed_seconds', 0.0), 3)
        arm['stop_hook_blocks'] += len(observation.get('stop_hook_blocks') or [])
        arm['env_override_trials'] += int(bool(observation.get('env_overrides')))
        if observation.get('total_tokens') is None:
            arm['usage_unknown'] += 1
        else:
            arm['known_tokens'] += observation['total_tokens']
    return arms


def _cases(o: dict | None, kind: str) -> str:
    return '-' if o is None else '%d/%d' % (o.get(kind + '_passed', 0), o.get(kind + '_total', 0))


def to_markdown(summary: dict, records: list) -> str:
    lines = ['# Pilot C report', '',
             '| arm | trials | review | feature | guard | ungraded | timeouts | op errors | scope | invalid arm |'
             ' contaminated | elapsed s | known tokens | usage unknown | stop blocks | env overrides |', '|' + '---|' * 16]
    for arm, s in summary.items():
        lines.append('| %s | %d | %d/%d | %d/%d | %d/%d | %d | %d | %d | %d | %d | %d | %.0f | %d | %d | %d | %d |' % (
            arm, s['trials'], s['review_passed'], s['review_total'], s['feature_passed'], s['feature_total'],
            s['guard_passed'], s['guard_total'], s['ungraded'], s['timed_out'], s['operational_errors'],
            s['scope_violations'], s['arm_invalid'], s['contaminated'], s['elapsed_seconds'], s['known_tokens'],
            s['usage_unknown'], s['stop_hook_blocks'], s['env_override_trials']))
    lines += ['', '| trial | review | feature | guard | elapsed s | tokens | error | env overrides |',
              '|' + '---|' * 8]
    for row in records:
        o = row.get('oracle')
        overrides = sorted({v for o in (row.get('observation') or {}).get('env_overrides') or [] for v in o['vars']})
        lines.append('| %s-%s-k%d | %s | %s | %s | %.0f | %s | %s | %s |' % (
            row['task_id'], row['arm'], row['replicate'], _cases(o, 'review'), _cases(o, 'feature'),
            _cases(o, 'guard'), (row.get('agent') or {}).get('elapsed_seconds', 0.0),
            (row.get('observation') or {}).get('total_tokens'), row.get('operational_error'),
            ', '.join(overrides) or '-'))
    return '\n'.join(lines) + '\n'
