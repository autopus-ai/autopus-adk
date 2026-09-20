"""Export benchmark observations to auto telemetry harness v1.

Receipts are session aggregates, NOT provider model-call counts. Requested model
aliases are unverified and shared provider caches are uncontrolled. Identity
matching denotes common declared conditions, not verified model/cache pinning.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path

from observe import unique_object, reject_json_constant


ARMS = {'native', 'current', 'reduced'}


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':'), allow_nan=False).encode()).hexdigest()


def number(value):
    return type(value) is int and 0 <= value <= 10**12


def identity_text(value):
    if not isinstance(value, str) or not value or len(value.encode()) > 256 or not value.isprintable():
        raise ValueError('invalid comparison identity')
    return value


def session_usage(protocol, row, identity):
    obs = row['observation']
    if obs.get('schema_version') != 'codex_exec_observation.v1':
        raise ValueError('unsupported observation schema')
    usage = {key: identity[key] for key in ['provider', 'provider_version', 'model', 'model_version', 'effort', 'cache_stratum']}
    usage.update(version=1, run_id=digest([protocol['revision'], row['task_id'], row['arm']]),
                 call_id='session-aggregate', task_id=row['task_id'], usage_source='provider',
                 source_schema='codex/exec/turn.completed/session-aggregate', usage_status='unavailable',
                 raw_total_tokens=None, input_tokens_total=None, output_tokens_total=None,
                 uncached_input_tokens=None, cached_input_tokens=None, actual_cost_usd=None,
                 unavailable_reason='session_aggregate_incomplete')
    complete = (protocol.get('single_agent') is True and
                obs.get('usage_scope') == 'single_agent_transcript' and
                obs.get('incomplete_reasons') == [] and obs.get('failed') is False and
                row.get('timed_out') is False and not row.get('operational_error') and
                number(obs.get('completed_turns')) and obs['completed_turns'] > 0 and
                all(number(obs.get(k)) for k in ['input_tokens', 'output_tokens', 'total_tokens']))
    if not complete:
        return usage
    incoming, outgoing = obs['input_tokens'], obs['output_tokens']
    if incoming + outgoing != obs['total_tokens'] or obs['total_tokens'] != obs.get('known_total_tokens'):
        raise ValueError('session usage total disagrees with components')
    usage.update(usage_status='actual', input_tokens_total=incoming, output_tokens_total=outgoing,
                 raw_total_tokens=obs['total_tokens'], unavailable_reason='')
    cached = obs.get('cached_input_tokens')
    if cached is not None:
        if not number(cached) or cached > incoming:
            raise ValueError('invalid cached input subset')
        usage.update(cached_input_tokens=cached, uncached_input_tokens=incoming-cached)
    reasoning = obs.get('reasoning_output_tokens')
    if reasoning is not None:
        if not number(reasoning) or reasoning > outgoing:
            raise ValueError('invalid reasoning output subset')
        usage.update(reasoning_tokens=reasoning, reasoning_relation='subset_of_output')
    return usage


def build_evidence(protocol, records):
    if protocol.get('schema') != 'harness_benchmark.v1' or not isinstance(records, list) or len(records) > 3000:
        raise ValueError('invalid benchmark protocol or record count')
    tasks = protocol.get('tasks')
    if not isinstance(tasks, list) or not 0 < len(tasks) <= 1000:
        raise ValueError('invalid expected task corpus')
    indexed = {identity_text(t['id']): t for t in tasks}
    if len(indexed) != len(tasks):
        raise ValueError('duplicate expected task')
    common = {'provider': 'codex', 'provider_version': identity_text(protocol['cli_version']),
              'model': identity_text(protocol['model']), 'model_version': 'unverified-alias',
              'effort': identity_text(protocol['effort']), 'cache_stratum': 'shared-uncontrolled',
              'revision': identity_text(protocol['revision']),
              'environment_hash': digest({k: protocol.get(k) for k in ['go_version', 'go_cache', 'global_customization', 'cache']}),
              'budget_hash': digest({'timeout_seconds': protocol['timeout_seconds'], 'single_agent': protocol['single_agent']})}
    result = {'version': 1, 'expected_task_ids': sorted(indexed), 'observations': []}
    seen = set()
    for row in records:
        task_id, arm = row['task_id'], row['arm']
        if task_id not in indexed or arm not in ARMS or (task_id, arm) in seen:
            raise ValueError('unexpected or duplicate task/arm')
        seen.add((task_id, arm))
        if type(row.get('accepted')) is not bool or type(row.get('timed_out')) is not bool:
            raise ValueError('missing explicit outcome')
        elapsed = row.get('elapsed_seconds')
        if elapsed is not None and (type(elapsed) not in (int, float) or not math.isfinite(elapsed) or not 0 <= elapsed <= 10**9):
            raise ValueError('invalid elapsed time')
        corrections = row.get('human_corrections')
        if corrections is not None and not number(corrections):
            raise ValueError('invalid human correction count')
        task = indexed[task_id]
        identity = dict(common, task_hash=digest(task), oracle_hash=digest(task['oracle']))
        usage = session_usage(protocol, row, identity)
        status = 'FAIL' if row['observation'].get('failed') or row['timed_out'] or row.get('operational_error') else 'PASS'
        commands = row['observation'].get('command_actions')
        if not number(commands):
            raise ValueError('invalid command action count')
        run = {'agent_name': 'single-agent-session', 'task_id': task_id, 'run_id': usage['run_id'],
               'attempt': 1, 'status': status, 'acceptance_status': 'PASS' if row['accepted'] else 'FAIL',
               'tool_calls': commands, 'usage': [usage]}
        result['observations'].append({'task_id': task_id, 'arm': arm,
            'harness_revision': common['revision'],
            'harness_config_hash': identity_text(protocol['arms'][arm]['surface_hash']),
            'identity': identity, 'accepted': row['accepted'],
            'elapsed_ms': None if elapsed is None else round(elapsed * 1000),
            'human_corrections': corrections, 'runs': [run]})
    return result


def load(path):
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 10 * 1024 * 1024:
        raise ValueError('input must be a bounded regular file')
    return json.loads(path.read_text(), object_pairs_hook=unique_object, parse_constant=reject_json_constant)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    evidence = build_evidence(load(args.input / 'protocol.json'), load(args.input / 'records.json'))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(evidence, indent=2, allow_nan=False) + '\n')


if __name__ == '__main__':
    main()
