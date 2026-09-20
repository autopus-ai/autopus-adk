import unittest

from export import build_evidence


def fixture():
    protocol = {'schema': 'harness_benchmark.v1', 'revision': 'abcdef',
                'tasks': [{'id': 'a01', 'prompt': 'fix', 'mutation': {'path': 'p.go', 'before': 'a', 'after': 'b'}, 'oracle': {'command': ['go', 'test']}, 'allowed_paths': ['p.go']}],
                'model': 'gpt-6-astra', 'effort': 'medium', 'timeout_seconds': 180,
                'cli_version': 'codex-cli 1.0.0', 'go_version': 'go1.26', 'single_agent': True,
                'cache': 'shared', 'go_cache': 'shared', 'global_customization': 'common',
                'arms': {arm: {'surface_hash': arm} for arm in ['native', 'current', 'reduced']}}
    observation = {'schema_version': 'codex_exec_observation.v1', 'usage_scope': 'single_agent_transcript',
                   'total_tokens': 15, 'known_total_tokens': 15, 'input_tokens': 10, 'output_tokens': 5,
                   'cached_input_tokens': 4, 'reasoning_output_tokens': None, 'completed_turns': 1,
                   'command_actions': 2, 'failed': False, 'incomplete_reasons': []}
    records = [{'task_id': 'a01', 'arm': 'native', 'accepted': False, 'timed_out': False,
                'elapsed_seconds': 2.5, 'human_corrections': 0, 'observation': observation}]
    return protocol, records


class ExportTests(unittest.TestCase):
    def test_complete_session_aggregate_is_labelled_and_rejected_work_counts(self):
        p, rows = fixture()
        result = build_evidence(p, rows)
        record = result['observations'][0]
        self.assertFalse(record['accepted'])
        self.assertEqual(record['elapsed_ms'], 2500)
        usage = record['runs'][0]['usage'][0]
        self.assertEqual(usage['raw_total_tokens'], 15)
        self.assertEqual(usage['uncached_input_tokens'], 6)
        self.assertEqual(usage['call_id'], 'session-aggregate')
        self.assertTrue(usage['source_schema'].endswith('/session-aggregate'))
        self.assertEqual(record['identity']['model_version'], 'unverified-alias')
        self.assertEqual(record['identity']['cache_stratum'], 'shared-uncontrolled')

    def test_incomplete_or_timeout_has_no_actual_total(self):
        for timeout in [False, True]:
            p, rows = fixture()
            rows[0]['timed_out'] = timeout
            if not timeout:
                rows[0]['observation']['incomplete_reasons'] = ['truncated_transcript']
            usage = build_evidence(p, rows)['observations'][0]['runs'][0]['usage'][0]
            self.assertEqual(usage['usage_status'], 'unavailable')
            self.assertIsNone(usage['raw_total_tokens'])

    def test_shared_identity_and_missing_human_corrections(self):
        p, rows = fixture()
        candidate = dict(rows[0], arm='current')
        candidate.pop('human_corrections')
        output = build_evidence(p, rows + [candidate])['observations']
        self.assertEqual(output[0]['identity'], output[1]['identity'])
        self.assertNotEqual(output[0]['harness_config_hash'], output[1]['harness_config_hash'])
        self.assertNotEqual(output[0]['runs'][0]['run_id'], output[1]['runs'][0]['run_id'])
        self.assertIsNone(output[1]['human_corrections'])
        self.assertIsNone(output[0]['runs'][0]['usage'][0]['actual_cost_usd'])

    def test_duplicate_and_bad_arithmetic_rejected(self):
        p, rows = fixture()
        with self.assertRaises(ValueError):
            build_evidence(p, rows + rows)
        rows[0]['observation']['output_tokens'] = 99
        with self.assertRaises(ValueError):
            build_evidence(p, rows)


if __name__ == '__main__':
    unittest.main()
