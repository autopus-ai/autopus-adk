"""Opt-in codex plumbing for pilot C (codex_c, run_c options, stream_c flags); no network, no model call."""
import json
import os
import tempfile
import unittest
from pathlib import Path

import codex_c
import report_c
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
            # codex gets its own directory (prepare_run), never the shared tools every arm sees.
            self.assertEqual(codex_c.tool_names(args), ['go', 'gofmt', 'claude', 'node'])
            args = run_c.parse(ARGS + ['--codex-auth-file', str(path), '--tools', 'go,codex,node'])
            self.assertEqual(codex_c.tool_names(args), ['go', 'node'])
            ctx = {'codex': codex_c.trial_settings(args)}
            self.assertEqual([codex_c.arm_uses(ctx, a) for a in ('native', 'instructions', 'full')],
                             [False, True, True])
            self.assertFalse(codex_c.arm_uses({'codex': None}, 'full'))
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


class PrepareRunTests(unittest.TestCase):
    def test_codex_is_linked_apart_and_its_version_recorded(self):
        with tempfile.TemporaryDirectory() as d:
            host = Path(d) / 'host'
            host.mkdir()
            fake = host / 'codex'
            fake.write_text('#!/bin/sh\necho codex-cli 9.9.9\n')
            fake.chmod(0o755)
            ctx = {'codex': {'model': 'm'}, 'output': Path(d) / 'out', 'tools_dir': Path(d) / 'out' / 'bin' / 'tools',
                   'base_env': {'PATH': str(host)}}
            protocol = {'codex': {}}
            codex_c.prepare_run(protocol, ctx)
            self.assertEqual(ctx['codex']['tools_dir'], Path(d) / 'out' / 'bin' / 'tools-codex')
            self.assertEqual(ctx['codex']['tools'], {'codex': os.path.realpath(fake)})
            self.assertEqual(protocol['codex']['version'], 'codex-cli 9.9.9')
            self.assertFalse(ctx['tools_dir'].exists(), 'the shared tools stay without codex')
            ctx['codex'] = {'model': 'm'}
            codex_c.prepare_run(protocol, ctx)
            self.assertEqual(ctx['codex']['tools'], {'codex': os.path.realpath(fake)}, 'a resume reuses the link')
            codex_c.prepare_run(protocol, {'codex': None})


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


def usage(inputs, cached, output):
    total = {'input_tokens': inputs, 'cached_input_tokens': cached, 'output_tokens': output,
             'reasoning_output_tokens': 0, 'total_tokens': inputs + output}
    return {'type': 'event_msg', 'payload': {'type': 'token_count', 'info': {'total_token_usage': total}}}


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
                      {'type': 'event_msg', 'payload': {'type': 'agent_message', 'message': 'web_search is off'}},
                      usage(100, 40, 5), usage(300, 120, 9)]
            (day / 'rollout-1.jsonl').write_text('\n'.join(map(json.dumps, events)) + '\nnot json\n')
            (day / 'rollout-2.jsonl').write_text(json.dumps(usage(50, 0, 1)) + '\n')
            (day / 'rollout-3.jsonl').write_text('{}\n')
            seen = codex_c.observe(trial)
            self.assertEqual((seen['sessions'], seen['commands']), (3, 2))
            # token_count is cumulative within a session: the last one counts, sessions add up.
            self.assertEqual(seen['tokens'], {'input_tokens': 350, 'cached_input_tokens': 120, 'output_tokens': 10,
                                              'reasoning_output_tokens': 0, 'total_tokens': 360})
            self.assertEqual(seen['sessions_without_usage'], 1)
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
        # native has no codex and no harness that asks for it: running it makes the trial invalid.
        self.assertFalse(stream_c.arm_valid('native', seen))
        self.assertTrue(stream_c.arm_valid('native', dict(seen, codex_calls=0)))
        self.assertTrue(stream_c.arm_valid('full', dict(seen, hook_events=1)))

    def test_codex_command_position_only(self):
        runs = ['codex exec "x" </dev/null', "perl -e 'alarm 480; exec @ARGV' codex exec -s danger-full-access 'b'",
                'git diff | codex exec -', 'timeout 900 codex exec x', 'cd w && /opt/bin/codex review']
        mentions = ['grep -rn codex .', 'grep codex exec f', 'ls ~/.codex', 'echo codex exec', 'cat codex_c.py']
        self.assertEqual([bool(stream_c.CODEX_COMMAND.search(c)) for c in runs], [True] * 5)
        self.assertEqual([bool(stream_c.CODEX_COMMAND.search(c)) for c in mentions], [False] * 5)


class ReportTests(unittest.TestCase):
    def test_openai_usage_is_reported_per_arm_and_trial(self):
        base = {'task_id': 'c03', 'replicate': 1, 'operational_error': None, 'arm_valid': True,
                'agent': {'elapsed_seconds': 1.0}, 'observation': {'total_tokens': 10}, 'scope': {}, 'oracle': None}
        tokens = {'input_tokens': 350, 'cached_input_tokens': 120, 'output_tokens': 10,
                  'reasoning_output_tokens': 4, 'total_tokens': 360}
        records = [dict(base, arm='native', codex=None),
                   dict(base, arm='full', codex={'sessions': 2, 'tokens': tokens, 'sessions_without_usage': 1,
                                                 'web_search': [], 'network_commands': ['gh pr view']})]
        summary = report_c.summarize(records)
        self.assertEqual((summary['full']['codex_tokens'], summary['full']['codex_usage_unknown']), (360, 1))
        self.assertEqual(summary['native']['codex_tokens'], 0)
        text = report_c.to_markdown(summary, records)
        self.assertIn('| full | 2 | 360 | 1 |', text)
        self.assertIn('| c03-full-k1 | 2 | 350 | 120 | 10 | 4 | 360 | 1 |', text)
        self.assertNotIn('c03-native-k1 | 0', text)
        self.assertNotIn('Codex (OpenAI)', report_c.to_markdown(summary, records[:1]))


if __name__ == '__main__':
    unittest.main()
