"""What a trial's codex did (codex_observe_c, stream_c, report_c): command detection, rollouts read
fail-closed, reconcile and the usage table; synthetic inputs only."""
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import codex_c
import codex_observe_c as observe_c
import report_c
import stream_c

CONFIG_SHA = codex_c.settings(type('A', (), {'codex_auth_file': 'x', 'codex_model': codex_c.MODEL,
                                             'codex_effort': codex_c.EFFORT})())['config_sha256']


def usage(inputs, cached, output):
    total = {'input_tokens': inputs, 'cached_input_tokens': cached, 'output_tokens': output,
             'reasoning_output_tokens': 0, 'total_tokens': inputs + output}
    return {'type': 'event_msg', 'payload': {'type': 'token_count', 'info': {'total_token_usage': total}}}


def call(name, **arguments):
    return {'type': 'response_item', 'payload': {'type': 'function_call', 'name': name,
                                                'arguments': json.dumps(arguments)}}


def home(trial: Path) -> Path:
    codex_home = trial / 'codex-home'
    (codex_home / 'sessions' / 'd').mkdir(parents=True)
    (codex_home / 'config.toml').write_text(codex_c.config_text())
    return codex_home / 'sessions' / 'd'


class DetectionTests(unittest.TestCase):
    def test_codex_is_found_behind_wrappers_and_shells_but_not_in_text(self):
        runs = ['codex exec "x" </dev/null', "perl -e 'alarm 480; exec @ARGV' codex exec -s danger-full-access 'b'",
                'git diff | codex exec -', 'timeout 15m codex exec x', 'gtimeout 900 codex exec x',
                'nohup codex exec x', 'bash -c "codex exec review-this"', 'if true; then codex exec x; fi',
                'npx codex exec x', 'node /opt/lib/codex.js exec x', 'cd w && /opt/bin/codex review', 'codex']
        for command in runs:
            self.assertEqual([r['session'] for r in observe_c.codex_runs(command)], [True], command)
        texts = ['grep -rn codex .', 'grep codex exec f', 'ls ~/.codex', 'echo codex exec', 'cat codex_c.py',
                 'cat > n.md <<EOF\ncodex exec is not here\nEOF']
        for command in texts:
            self.assertEqual(observe_c.codex_runs(command), [], command)
        self.assertEqual(observe_c.codex_runs('codex --version'), [{'session': False, 'bypass': []}])

    def test_flags_that_leave_the_frozen_config_are_bypasses(self):
        cases = {'codex exec --ephemeral x': '--ephemeral', 'codex exec --ignore-user-config x': '--ignore-user-config',
                 'codex --search exec x': '--search', 'codex -c web_search=live exec x': '-c',
                 'codex exec --enable web_search x': '--enable', 'codex -m gpt-5 exec x': '-m',
                 'CODEX_HOME=/t/h codex exec x': 'CODEX_HOME', 'env CODEX_HOME=/t/h codex exec x': 'CODEX_HOME',
                 'export CODEX_HOME=/x; codex exec y': 'CODEX_HOME'}
        for command, flag in cases.items():
            self.assertIn(flag, observe_c.codex_runs(command)[0]['bypass'], command)
        brief = "perl -e 'alarm 480; exec @ARGV' codex exec --skip-git-repo-check -s danger-full-access " \
                "-o \"${TMPDIR:-/tmp}/codex-review.md\" \"<brief>\" </dev/null 2>&1"
        self.assertEqual(observe_c.codex_runs(brief), [{'session': True, 'bypass': []}], 'the v3c brief')


class RolloutTests(unittest.TestCase):
    def test_usage_network_and_web_search_are_read_from_regular_rollouts(self):
        with tempfile.TemporaryDirectory() as d:
            trial = Path(d)
            day = home(trial)
            events = [call('exec', cmd='go test ./...'), call('shell', command=['bash', '-lc', 'curl -s https://x/y']),
                      call('write_stdin', session_id=1, chars='gh pr view 7\n'),
                      {'type': 'response_item', 'payload': {'type': 'web_search_call', 'action': {'query': 'q'}}},
                      {'type': 'event_msg', 'payload': {'type': 'agent_message', 'message': 'web_search is off'}},
                      usage(100, 40, 5), usage(300, 120, 9)]
            (day / 'rollout-1.jsonl').write_text('\n'.join(map(json.dumps, events)) + '\nnot json\n')
            (day / 'rollout-2.jsonl').write_text(json.dumps(usage(50, 0, 1)) + '\n')
            seen = observe_c.observe(trial, CONFIG_SHA)
            self.assertTrue(seen['home_intact'] and seen['config_intact'])
            self.assertEqual((seen['sessions'], seen['commands'], seen['unscanned']), (2, 3, 0))
            self.assertEqual(seen['network_commands'], ["bash -lc 'curl -s https://x/y'", 'gh pr view 7\n'])
            self.assertEqual(len(seen['web_search']), 1)
            # token_count is cumulative within a session: the last one counts, sessions add up.
            self.assertEqual(seen['tokens'], {'input_tokens': 350, 'cached_input_tokens': 120, 'output_tokens': 10,
                                              'reasoning_output_tokens': 0, 'total_tokens': 360})
            flags = observe_c.reconcile(seen, {'codex_calls': 2, 'codex_bypass': []})
            self.assertEqual(len(flags), 3)
            self.assertFalse(seen['usage_unknown'])

    def test_links_fifos_other_formats_and_oversized_files_are_unscanned_without_blocking(self):
        with tempfile.TemporaryDirectory() as d:
            trial = Path(d)
            day = home(trial)
            outside = Path(d) / 'outside.jsonl'
            outside.write_text(json.dumps(usage(9, 0, 9)) + '\n')
            (day / 'rollout-link.jsonl').symlink_to(outside)
            os.mkfifo(day / 'rollout-fifo.jsonl')
            (day / 'rollout-b.jsonl.zst').write_bytes(b'\x28\xb5\x2f\xfd web_search_call')
            code = ('import sys; sys.path.insert(0, %r); import codex_observe_c as o; o.MAX_ROLLOUT_BYTES = 8; '
                    'print(o.observe(%r, %r)["unscanned"])' % (str(Path(__file__).parent), d, CONFIG_SHA))
            (day / 'rollout-big.jsonl').write_text('x' * 64)
            done = subprocess.run([sys.executable, '-I', '-c', code], capture_output=True, text=True, timeout=20)
            self.assertEqual(done.stdout.strip(), '4', done.stderr)
            seen = observe_c.observe(trial, CONFIG_SHA)
            flags = observe_c.reconcile(seen, {'codex_calls': 0})
            self.assertTrue(any(flag.startswith('CODEX_UNSCANNED') for flag in flags))
            self.assertTrue(seen['usage_unknown'])
            self.assertIsNone(seen['tokens'])

    def test_unobserved_sessions_a_changed_config_and_a_replaced_home_fail_closed(self):
        with tempfile.TemporaryDirectory() as d:
            trial = Path(d) / 't'
            day = home(trial)
            (day / 'rollout-1.jsonl').write_text(json.dumps(usage(10, 0, 1)) + '\n')
            seen = observe_c.observe(trial, CONFIG_SHA)
            flags = observe_c.reconcile(seen, {'codex_calls': 2, 'codex_bypass': ['--ephemeral: codex exec']})
            self.assertIn('CODEX_UNOBSERVED 2 started, 1 observed', flags)
            self.assertIn('CODEX_BYPASS --ephemeral: codex exec', flags)
            self.assertIsNone(seen['tokens'])
            (trial / 'codex-home' / 'config.toml').write_text('web_search = "live"\n')
            self.assertIn('CODEX_CONFIG_CHANGED', observe_c.reconcile(observe_c.observe(trial, CONFIG_SHA), {}))
            (trial / 'codex-home').rename(Path(d) / 'elsewhere')
            (trial / 'codex-home').symlink_to(Path(d) / 'elsewhere')
            seen = observe_c.observe(trial, CONFIG_SHA)
            self.assertEqual(seen['sessions'], 0, 'a linked home is never read')
            self.assertIn('CODEX_HOME_REPLACED', observe_c.reconcile(seen, {}))


class StreamTests(unittest.TestCase):
    def test_web_search_only_in_codex_output_and_native_must_not_run_codex(self):
        def use(uid, command):
            return {'type': 'assistant', 'message': {'content': [
                {'type': 'tool_use', 'id': uid, 'name': 'Bash', 'input': {'command': command}}]}}

        def output(uid, text):
            return {'type': 'user', 'message': {'content': [
                {'type': 'tool_result', 'tool_use_id': uid, 'content': [{'type': 'text', 'text': text}]}]}}
        events = [use('a', 'grep -rn web_search .'), output('a', 'config.go: web_search = "disabled"'),
                  use('b', 'git diff | codex exec --ephemeral - </dev/null'),
                  output('b', 'web search: golang context cancel\nfinding 1'),
                  use('c', 'codex exec "review" </dev/null'), output('c', 'no findings'),
                  {'type': 'result', 'is_error': False, 'usage': {}}]
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'events.jsonl'
            path.write_text(''.join(json.dumps(e) + '\n' for e in events))
            seen = stream_c.parse_stream(path)
        self.assertEqual(seen['codex_calls'], 2)
        self.assertEqual(seen['contamination'], ['CODEX_WEB_SEARCH in output of tool use b'])
        self.assertEqual([flag.split(':')[0] for flag in seen['codex_bypass']], ['--ephemeral'])
        json.dumps(seen)
        self.assertFalse(stream_c.arm_valid('native', seen))
        self.assertTrue(stream_c.arm_valid('native', dict(seen, codex_calls=0)))
        self.assertTrue(stream_c.arm_valid('full', dict(seen, hook_events=1)))


class ReportTests(unittest.TestCase):
    def test_openai_usage_is_reported_per_arm_and_trial_and_unknown_stays_unknown(self):
        base = {'task_id': 'c03', 'replicate': 1, 'operational_error': None, 'arm_valid': True,
                'agent': {'elapsed_seconds': 1.0}, 'observation': {'total_tokens': 10}, 'scope': {}, 'oracle': None}
        tokens = {'input_tokens': 350, 'cached_input_tokens': 120, 'output_tokens': 10,
                  'reasoning_output_tokens': 4, 'total_tokens': 360}
        records = [dict(base, arm='native', codex=None),
                   dict(base, arm='full', codex={'sessions': 2, 'tokens': tokens, 'usage_unknown': False},
                        observation={'total_tokens': 10, 'contamination': ['CODEX gh pr view']}),
                   dict(base, arm='full', replicate=2, codex={'sessions': 1, 'tokens': None, 'usage_unknown': True})]
        summary = report_c.summarize(records)
        self.assertEqual((summary['full']['codex_tokens'], summary['full']['codex_usage_unknown']), (360, 1))
        self.assertEqual(summary['native']['codex_tokens'], 0)
        text = report_c.to_markdown(summary, records)
        self.assertIn('| full | 3 | 360 | 1 |', text)
        self.assertIn('| c03-full-k1 | 2 | 350 | 120 | 10 | 4 | 360 | 1 |', text)
        self.assertIn('| c03-full-k2 | 1 | - | - | - | - | - | 0 |', text)
        self.assertNotIn('Codex (OpenAI)', report_c.to_markdown(summary, records[:1]))


if __name__ == '__main__':
    unittest.main()
