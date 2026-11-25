"""Opt-in codex plumbing for pilot C (codex_c, run_c options, stream_c flags); no network, no model call."""
import json
import os
import tempfile
import unittest
from pathlib import Path

import codex_c
import run_c
import stream_c

ARGS = ['--repo', '/r', '--output', '/o', '--model', 'opus']


def auth(account='acct-1', refreshed='2026-11-01T00:00:00Z', token='t0'):
    return json.dumps({'OPENAI_API_KEY': None, 'auth_mode': 'chatgpt', 'last_refresh': refreshed,
                       'tokens': {'account_id': account, 'access_token': token, 'refresh_token': 'r-' + token,
                                  'id_token': 'i'}}).encode()


def private(path: Path, data: bytes) -> Path:
    path.write_bytes(data)
    path.chmod(0o600)
    return path


class OptionTests(unittest.TestCase):
    def test_off_by_default_changes_nothing(self):
        args = run_c.parse(ARGS)
        self.assertEqual(codex_c.tool_names(args), ['go', 'gofmt', 'claude', 'node'])
        self.assertIsNone(codex_c.trial_settings(args))
        self.assertIsNone(codex_c.settings(args))

    def test_auth_file_links_codex_once_and_records_no_secret(self):
        with tempfile.TemporaryDirectory() as d:
            path = private(Path(d) / 'auth.json', auth(token='SECRET-TOKEN'))
            args = run_c.parse(ARGS + ['--codex-auth-file', str(path)])
            self.assertEqual(codex_c.tool_names(args), ['go', 'gofmt', 'claude', 'node', 'codex'])
            args = run_c.parse(ARGS + ['--codex-auth-file', str(path), '--tools', 'go,codex,node'])
            self.assertEqual(codex_c.tool_names(args), ['go', 'codex', 'node'])
            recorded = codex_c.settings(args)
            self.assertEqual((recorded['model'], recorded['effort']), (codex_c.MODEL, codex_c.EFFORT))
            self.assertNotIn('SECRET', json.dumps(recorded))
            self.assertEqual(codex_c.trial_settings(args)['auth_file'], path.resolve())

    def test_readable_or_foreign_auth_file_is_refused(self):
        with tempfile.TemporaryDirectory() as d:
            loose = Path(d) / 'loose.json'
            loose.write_bytes(auth())
            loose.chmod(0o644)
            with self.assertRaises(ValueError):
                codex_c.read_auth(loose)
            with self.assertRaises(ValueError) as caught:
                codex_c.read_auth(private(Path(d) / 'other.json', b'{"x": "SECRET"}'))
            self.assertNotIn('SECRET', str(caught.exception))


class HomeTests(unittest.TestCase):
    def test_trial_home_holds_a_private_copy_and_a_closed_config(self):
        with tempfile.TemporaryDirectory() as d:
            trial = Path(d) / 'trial'
            trial.mkdir()
            source = private(Path(d) / 'auth.json', auth())
            env = codex_c.install_home(trial, {'auth_file': source, 'model': 'm1', 'effort': 'high'})
            home = Path(env['CODEX_HOME'])
            self.assertEqual(home, trial / 'codex-home')
            self.assertEqual(env['CODEX_CA_CERTIFICATE'], codex_c.CA_BUNDLE)
            self.assertNotIn(trial / 'workspace', home.parents)
            self.assertEqual(sorted(p.name for p in home.iterdir()), ['auth.json', 'config.toml'])
            self.assertEqual((home / 'auth.json').stat().st_mode & 0o777, 0o600)
            self.assertEqual((home / 'auth.json').read_bytes(), source.read_bytes())
            config = (home / 'config.toml').read_text()
            for line in ('model = "m1"', 'web_search = "disabled"', 'cli_auth_credentials_store = "file"',
                         'web_search = false', 'browser_use = false', 'apps = false', 'plugins = false'):
                self.assertIn(line, config)
            with self.assertRaises(FileExistsError):
                codex_c.install_home(trial, {'auth_file': source, 'model': 'm1', 'effort': 'high'})


class WriteBackTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        root = Path(self.dir.name)
        self.trial = root / 'trial'
        (self.trial / 'codex-home').mkdir(parents=True)
        self.source = private(root / 'auth.json', auth())
        self.copy = self.trial / 'codex-home' / 'auth.json'

    def tearDown(self):
        self.dir.cleanup()

    def test_refresh_reaches_the_next_trial(self):
        private(self.copy, auth(refreshed='2026-11-20T00:00:00Z', token='t1'))
        self.assertEqual(codex_c.write_back(self.trial, self.source), 'refreshed')
        self.assertEqual(self.source.read_bytes(), self.copy.read_bytes())
        self.assertEqual(self.source.stat().st_mode & 0o777, 0o600)

    def test_unchanged_other_account_older_or_garbage_is_kept_out(self):
        original = self.source.read_bytes()
        private(self.copy, original)
        self.assertEqual(codex_c.write_back(self.trial, self.source), 'unchanged')
        for data in (auth(account='acct-2', refreshed='2026-11-20T00:00:00Z'), auth(refreshed='2026-10-01T00:00:00Z',
                                                                                    token='t9'), b'not json'):
            private(self.copy, data)
            self.assertEqual(codex_c.write_back(self.trial, self.source), 'rejected')
        self.copy.unlink()
        os.symlink(self.source, self.copy)
        self.assertEqual(codex_c.write_back(self.trial, self.source), 'unchanged')
        self.assertEqual(self.source.read_bytes(), original)


class ObservationTests(unittest.TestCase):
    def test_rollout_web_search_and_network_commands_are_contamination(self):
        with tempfile.TemporaryDirectory() as d:
            trial = Path(d)
            self.assertIsNone(codex_c.observe(trial))
            day = trial / 'codex-home' / 'sessions' / '2026' / '11' / '25'
            day.mkdir(parents=True)
            events = [{'type': 'response_item', 'payload': {'type': 'function_call', 'name': 'exec',
                                                            'arguments': json.dumps({'cmd': 'go test ./...'})}},
                      {'type': 'response_item', 'payload': {'type': 'function_call', 'name': 'exec',
                                                            'arguments': json.dumps({'cmd': 'gh pr view 12'})}},
                      {'type': 'response_item', 'payload': {'type': 'web_search_call', 'action': {'query': 'q'}}},
                      {'type': 'event_msg', 'payload': {'type': 'agent_message', 'message': 'web_search is off'}}]
            (day / 'rollout-1.jsonl').write_text('\n'.join(map(json.dumps, events)) + '\nnot json\n')
            seen = codex_c.observe(trial)
            self.assertEqual((seen['sessions'], seen['commands']), (1, 2))
            self.assertEqual(seen['network_commands'], ['gh pr view 12'])
            self.assertEqual(len(seen['web_search']), 1)
            flags = codex_c.contamination(seen)
            self.assertEqual(len(flags), 2)
            self.assertTrue(flags[0].startswith('CODEX_WEB_SEARCH'))

    def test_stream_flags_web_search_only_in_codex_tool_output(self):
        def use(uid, command):
            return {'type': 'assistant', 'message': {'content': [
                {'type': 'tool_use', 'id': uid, 'name': 'Bash', 'input': {'command': command}}]}}

        def output(uid, text):
            return {'type': 'user', 'message': {'content': [
                {'type': 'tool_result', 'tool_use_id': uid, 'content': [{'type': 'text', 'text': text}]}]}}
        events = [use('a', 'grep -rn web_search .'), output('a', 'config.go: web_search = "disabled"'),
                  use('b', 'git diff | codex exec --skip-git-repo-check -s danger-full-access - </dev/null'),
                  output('b', 'web search: golang context cancel\nfinding 1'),
                  use('c', 'codex exec "review" </dev/null'), output('c', 'no findings'),
                  {'type': 'result', 'is_error': False, 'usage': {}}]
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'events.jsonl'
            path.write_text(''.join(json.dumps(e) + '\n' for e in events))
            seen = stream_c.parse_stream(path)
        self.assertEqual(seen['codex_calls'], 2)
        self.assertEqual(seen['contamination'], ['CODEX_WEB_SEARCH in output of tool use b'])
        json.dumps(seen)


if __name__ == '__main__':
    unittest.main()
