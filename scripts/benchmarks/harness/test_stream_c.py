"""stream-json observation: usage, Stop outcomes and blocks, contamination, arm validity."""
import json
import tempfile
import unittest
from pathlib import Path

import stream_c

USAGE = {'input_tokens': 10, 'output_tokens': 20, 'cache_read_input_tokens': 300, 'cache_creation_input_tokens': 4}


def write_events(root: Path, events: list, tail: bytes = b'') -> Path:
    path = root / 'events.jsonl'
    path.write_bytes(b''.join(json.dumps(e).encode() + b'\n' for e in events) + tail)
    return path


def stop(**fields):
    return {'type': 'system', 'subtype': 'hook_response', 'hook_event': 'Stop', 'hook_name': 'Stop', **fields}


def bash(command):
    return {'type': 'assistant', 'message': {'content': [{'type': 'tool_use', 'name': 'Bash',
                                                          'input': {'command': command}}]}}


class StreamTests(unittest.TestCase):
    def test_usage_outcomes_blocks_and_contamination(self):
        with tempfile.TemporaryDirectory() as d:
            path = write_events(Path(d), [
                {'type': 'system', 'subtype': 'init', 'model': 'claude-x', 'permissionMode': 'bypassPermissions'},
                {'type': 'assistant', 'message': {'content': [{'type': 'tool_use', 'name': 'Task', 'input': {}}]}},
                bash('go test ./internal/cli'), bash('gh pr view 347'), bash('git fetch origin main'),
                {'type': 'system', 'subtype': 'hook_started', 'hook_event': 'Stop'},
                stop(stdout='{"decision":"block","reason":"go test failed"}\n', exit_code=0),
                stop(stdout='', stderr='vet failed', exit_code=2),
                stop(stdout='{"systemMessage":"verify stop: budget exceeded, not run"}', exit_code=0),
                {'type': 'system', 'subtype': 'hook_response', 'hook_event': 'PreToolUse', 'exit_code': 2},
                {'type': 'result', 'is_error': False, 'result': 'done', 'num_turns': 3, 'usage': USAGE,
                 'terminal_reason': 'completed', 'total_cost_usd': 0.5}])
            obs = stream_c.parse_stream(path)
        self.assertEqual(obs['total_tokens'], 334)
        self.assertEqual((obs['tool_uses'], obs['subagent_tool_uses']), (4, 1))
        self.assertEqual(obs['stop_hook_runs'], 3)
        self.assertEqual([b['reason'] for b in obs['stop_hook_blocks']], ['go test failed', 'vet failed'])
        self.assertEqual(obs['stop_outcomes'][2]['system_message'], 'verify stop: budget exceeded, not run')
        self.assertFalse(obs['stop_outcomes'][2]['blocked'])
        self.assertEqual(obs['contamination'], ['gh pr view 347', 'git fetch origin main'])
        self.assertEqual(obs['hook_events'], 5)
        self.assertEqual(obs['incomplete_reasons'], [])

    def test_unknown_usage_is_never_zero(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            self.assertEqual(stream_c.parse_stream(root / 'none.jsonl')['incomplete_reasons'], ['no_transcript'])
            killed = stream_c.parse_stream(write_events(root, [{'type': 'assistant', 'message': {}}], b'{"type": "r'))
            self.assertIsNone(killed['total_tokens'])
            self.assertIn('no_result_event', killed['incomplete_reasons'])
            self.assertIn('truncated_or_oversized_line', killed['incomplete_reasons'])
            invalid = stream_c.parse_stream(write_events(root, [{'type': 'result', 'usage': dict(USAGE, output_tokens=-1)}]))
            self.assertIsNone(invalid['total_tokens'])
            self.assertIn('invalid_usage', invalid['incomplete_reasons'])
            (root / 'dup.jsonl').write_text('{"type": "result", "type": "assistant"}\n')
            self.assertIn('malformed_json', stream_c.parse_stream(root / 'dup.jsonl')['incomplete_reasons'])

    def test_arm_validity(self):
        hooked, quiet = dict(stream_c.empty(), hook_events=3), stream_c.empty()
        self.assertTrue(stream_c.arm_valid('full', hooked))
        self.assertFalse(stream_c.arm_valid('full', quiet))
        self.assertTrue(stream_c.arm_valid('instructions', quiet))
        self.assertFalse(stream_c.arm_valid('instructions', hooked))
        self.assertFalse(stream_c.arm_valid('native', hooked))


if __name__ == '__main__':
    unittest.main()
