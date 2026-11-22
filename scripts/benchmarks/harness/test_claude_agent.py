"""Claude Code driver contracts: arm settings, isolation, argv and stream-json observation."""
import json
import os
import tempfile
import unittest
from pathlib import Path

import claude_agent


def write_events(root: Path, events: list, tail: bytes = b'') -> Path:
    path = root / 'events.jsonl'
    path.write_bytes(b''.join(json.dumps(e).encode() + b'\n' for e in events) + tail)
    return path


USAGE = {'input_tokens': 10, 'output_tokens': 20, 'cache_read_input_tokens': 300, 'cache_creation_input_tokens': 4}


class SettingsTests(unittest.TestCase):
    def test_only_instructions_disables_hooks(self):
        self.assertTrue(claude_agent.arm_settings('instructions')['disableAllHooks'])
        self.assertNotIn('disableAllHooks', claude_agent.arm_settings('native'))
        self.assertNotIn('disableAllHooks', claude_agent.arm_settings('full'))
        with self.assertRaises(ValueError):
            claude_agent.arm_settings('reduced')

    def test_argv_uses_verified_flags(self):
        argv = claude_agent.claude_argv(Path('/s.json'), 'opus', 'high')
        for flag in ('-p', '--verbose', '--include-hook-events', '--no-session-persistence', '--strict-mcp-config'):
            self.assertIn(flag, argv)
        self.assertEqual(argv[argv.index('--output-format') + 1], 'stream-json')
        self.assertEqual(argv[argv.index('--setting-sources') + 1], 'project')
        self.assertEqual(argv[argv.index('--permission-mode') + 1], 'bypassPermissions')
        self.assertNotIn('--dangerously-skip-permissions', argv)


class IsolationTests(unittest.TestCase):
    def test_home_has_no_login_and_env_is_scrubbed(self):
        with tempfile.TemporaryDirectory() as d:
            home = claude_agent.isolated_home(Path(d))
            config = Path(home['CLAUDE_CONFIG_DIR'])
            self.assertEqual(sorted(p.name for p in config.iterdir()), ['.claude.json'])
            env = claude_agent.agent_env({'PATH': '/usr/bin', 'ANTHROPIC_API_KEY': 'leak', 'GITHUB_TOKEN': 'x',
                                          'CLAUDE_CONFIG_DIR': '/account', 'GIT_DIR': '/x', 'LANG': 'C'},
                                         home, 'oauth', 'tok', [Path('/pinned/bin')])
            self.assertEqual(env['CLAUDE_CODE_OAUTH_TOKEN'], 'tok')
            self.assertNotIn('ANTHROPIC_API_KEY', env)
            self.assertNotIn('GITHUB_TOKEN', env)
            self.assertNotIn('GIT_DIR', env)
            self.assertEqual(env['CLAUDE_CONFIG_DIR'], str(config))
            self.assertEqual(env['PATH'], os.pathsep.join(['/pinned/bin', *claude_agent.SYSTEM_PATH]))
            with self.assertRaises(FileExistsError):
                claude_agent.isolated_home(Path(d))

    def test_credential_file_must_be_private_and_single(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'token'
            path.write_text('abc\n')
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                claude_agent.read_credential(path)
            path.chmod(0o600)
            self.assertEqual(claude_agent.read_credential(path), 'abc')
            path.write_text('a b')
            with self.assertRaises(ValueError):
                claude_agent.read_credential(path)

    def test_sandbox_refuses_trial_inside_a_protected_checkout(self):
        with tempfile.TemporaryDirectory() as d:
            out = Path(d) / 'out'
            trial = out / 'trials' / 't'
            trial.mkdir(parents=True)
            argv = claude_agent.sandboxed(['claude'], trial, out, out / 'cache', [Path(d) / 'repo'], d)
            self.assertEqual(argv[:3], [claude_agent.SANDBOX, '-f', str(claude_agent.PROFILE)])
            self.assertIn('PROTECTED1=' + str(trial.resolve() / '.no-such-protected'), argv)
            with self.assertRaises(ValueError):
                claude_agent.sandboxed(['claude'], trial, out, out / 'cache', [out], d)
            with self.assertRaises(ValueError):
                claude_agent.sandboxed(['claude'], Path(d) / 'elsewhere', out, out / 'cache', [], d)


class StreamTests(unittest.TestCase):
    def stop(self, **fields):
        return {'type': 'system', 'subtype': 'hook_response', 'hook_event': 'Stop', 'hook_name': 'Stop', **fields}

    def test_usage_result_and_stop_blocks(self):
        with tempfile.TemporaryDirectory() as d:
            path = write_events(Path(d), [
                {'type': 'system', 'subtype': 'init', 'model': 'claude-x', 'permissionMode': 'bypassPermissions'},
                {'type': 'assistant', 'message': {'content': [{'type': 'tool_use', 'name': 'Bash'},
                                                              {'type': 'tool_use', 'name': 'Task'}]}},
                {'type': 'system', 'subtype': 'hook_started', 'hook_event': 'Stop'},
                self.stop(stdout='{"decision":"block","reason":"go test failed"}\n', exit_code=0),
                self.stop(stdout='', stderr='vet failed', exit_code=2),
                self.stop(stdout='ok', exit_code=0),
                {'type': 'system', 'subtype': 'hook_response', 'hook_event': 'PreToolUse', 'exit_code': 2},
                {'type': 'result', 'is_error': False, 'result': 'done', 'num_turns': 3, 'usage': USAGE,
                 'terminal_reason': 'completed', 'total_cost_usd': 0.5}])
            obs = claude_agent.parse_stream(path)
        self.assertEqual(obs['total_tokens'], 334)
        self.assertEqual((obs['tool_uses'], obs['subagent_tool_uses']), (2, 1))
        self.assertEqual(obs['stop_hook_runs'], 3)
        self.assertEqual([b['reason'] for b in obs['stop_hook_blocks']], ['go test failed', 'vet failed'])
        self.assertEqual(obs['hook_events'], 5)
        self.assertEqual(obs['permission_mode'], 'bypassPermissions')
        self.assertEqual(obs['incomplete_reasons'], [])

    def test_unknown_usage_is_never_zero(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            missing = claude_agent.parse_stream(root / 'none.jsonl')
            self.assertEqual(missing['incomplete_reasons'], ['no_transcript'])
            killed = claude_agent.parse_stream(write_events(root, [{'type': 'assistant', 'message': {}}],
                                                            b'{"type": "res'))
            self.assertIsNone(killed['total_tokens'])
            self.assertIn('no_result_event', killed['incomplete_reasons'])
            self.assertIn('truncated_or_oversized_line', killed['incomplete_reasons'])
            bad = dict(USAGE, output_tokens=-1)
            invalid = claude_agent.parse_stream(write_events(root, [{'type': 'result', 'usage': bad}]))
            self.assertIsNone(invalid['total_tokens'])
            self.assertIn('invalid_usage', invalid['incomplete_reasons'])

    def test_duplicate_keys_are_malformed(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'e.jsonl'
            path.write_text('{"type": "result", "type": "assistant"}\n')
            self.assertIn('malformed_json', claude_agent.parse_stream(path)['incomplete_reasons'])


if __name__ == '__main__':
    unittest.main()
