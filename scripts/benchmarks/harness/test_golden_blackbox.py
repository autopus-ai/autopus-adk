"""Black-box oracle documents, the REQ-HR-08 table and agent_termination (SPEC-HARNEVAL-003, cross-platform)."""
import base64
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import golden_blackbox as gb

IDS = ['exit', 'stdout', 'result']
CLEAN = {'launched': True, 'exit_code': 0, 'os_signal': None, 'timed_out': False}
ORACLE = {'build': './cmd/alpha', 'command': ['{artifact}', '--in', '{input}/in.txt', '--out', '{output}/r.txt'],
          'inputs': [{'path': 'evals/harness/oracles/X01/in.txt', 'sha256': 'a' * 64}],
          'assertions': [{'id': 'exit', 'kind': 'exit_code', 'exit_code': 0},
                         {'id': 'stdout', 'kind': 'stdout',
                          'expected': {'path': 'evals/harness/oracles/X01/stdout.txt', 'sha256': 'b' * 64}},
                         {'id': 'result', 'kind': 'file', 'path': 'out/result.txt',
                          'expected': {'path': 'evals/harness/oracles/X01/result.txt', 'sha256': 'c' * 64}}]}


def result(check='ok', passed=(True, True, True), timed_out=False, task='GT-AGENT-X01', **extra) -> bytes:
    items = [{'id': name, 'passed': value} for name, value in zip(IDS, passed)] if check == 'ok' else []
    document = {'schema_version': gb.RESULT_SCHEMA, 'task_id': task, 'output_check': check, 'assertions': items,
                'artifact_exit': None if timed_out else 0, 'timed_out': timed_out, **extra}
    return json.dumps(document).encode()


def derive(stage, signal=None, ended=CLEAN, data=None):
    outcome, name, oracle = gb.derive(stage, signal, ended, data, 'GT-AGENT-X01', IDS)
    return outcome, name, oracle['ran'], oracle['build_failed']


class JudgementTableTests(unittest.TestCase):
    """S8: every trial maps to exactly one (outcome, signal, oracle.ran, oracle.build_failed)."""

    def test_artifact_side_rows(self):
        cases = {
            'accepted': (derive('oracle', data=result()), ('pass', 'accepted', True, False)),
            'mutation kept': (derive('oracle', data=result(passed=(True, False, False))),
                              ('fail', 'expectation_mismatch', True, False)),
            'build failure': (derive('build'), ('fail', 'artifact_build_failed', False, True)),
            'partial output then hang': (derive('oracle', data=result('not_checked', timed_out=True)),
                                         ('fail', 'artifact_timeout', False, False)),
            'no result document': (derive('oracle'), ('fail', 'oracle_harness_error', False, False)),
            'linked output': (derive('oracle', data=result('link_rejected')),
                              ('fail', 'output_link_rejected', False, False)),
            'oversized output': (derive('oracle', data=result('too_large')), ('fail', 'output_too_large', False, False)),
        }
        for name, (got, want) in cases.items():
            with self.subTest(name):
                self.assertEqual(got, want)

    def test_trial_side_rows_and_their_precedence(self):
        killed = {'launched': True, 'exit_code': None, 'os_signal': 'SIGKILL', 'timed_out': True}
        crashed = {'launched': True, 'exit_code': 3, 'os_signal': None, 'timed_out': False}
        cases = {
            'setup failure': (derive('setup', 'workspace_setup_failed', gb.NOT_LAUNCHED),
                              ('error', 'workspace_setup_failed', False, False)),
            'agent timeout, then a passing grade': (derive('oracle', None, killed, result()),
                                                    ('fail', 'agent_timeout', True, False)),
            'agent exit nonzero, then a build failure': (derive('build', None, crashed),
                                                         ('fail', 'agent_exit_nonzero', False, True)),
            'agent never launched, still judged': (derive('oracle', None, gb.NOT_LAUNCHED, result()),
                                                   ('fail', 'agent_launch_failed', True, False)),
            'leftover after the artifact': (derive('run', 'observation_failed'),
                                            ('fail', 'observation_failed', False, False)),
            'scope violation': (derive('agent', 'scope_violation'), ('fail', 'scope_violation', False, False)),
            'leftover after a timeout': (derive('agent', 'observation_failed', killed),
                                         ('fail', 'agent_timeout', False, False)),
        }
        for name, (got, want) in cases.items():
            with self.subTest(name):
                self.assertEqual(got, want)

    def test_counts_follow_the_assertions_only_when_the_oracle_ran(self):
        _, _, oracle = gb.derive('oracle', None, CLEAN, result(passed=(True, False, True)), 'GT-AGENT-X01', IDS)
        self.assertEqual(oracle, {'ran': True, 'build_failed': False, 'expected_passed': 2, 'expected_failed': 1})
        _, _, oracle = gb.derive('oracle', None, CLEAN, result('link_rejected'), 'GT-AGENT-X01', IDS)
        self.assertEqual(oracle, {'ran': False, 'build_failed': False, 'expected_passed': 0, 'expected_failed': 0})

    def test_a_result_naming_other_assertions_is_a_harness_error(self):
        other = json.loads(result())
        other['assertions'][2]['id'] = 'renamed'
        self.assertEqual(derive('oracle', data=json.dumps(other).encode()),
                         ('fail', 'oracle_harness_error', False, False))
        fewer = json.loads(result())
        del fewer['assertions'][2]
        self.assertEqual(derive('oracle', data=json.dumps(fewer).encode())[1], 'oracle_harness_error')

    def test_no_row_matches_a_white_box_or_inconsistent_record(self):
        for stage, signal in (('setup', 'forbidden_construct'), ('agent', None), ('run', None), ('agent', 'accepted'),
                              ('nowhere', None)):
            with self.subTest(stage=stage, signal=signal), self.assertRaises(ValueError):
                gb.derive(stage, signal, CLEAN, None, 'GT-AGENT-X01', IDS)

    def test_every_table_signal_has_its_outcome_and_none_is_white_box(self):
        self.assertEqual(len(gb.OUTCOMES), 15)
        self.assertEqual([name for name, outcome in gb.OUTCOMES.items() if outcome == 'pass'], ['accepted'])
        self.assertEqual({name for name, outcome in gb.OUTCOMES.items() if outcome == 'error'}, set(gb.SETUP_SIGNALS))
        for white_box in ('forbidden_construct', 'oracle_failed', 'oracle_timeout', 'oracle_output_invalid'):
            self.assertNotIn(white_box, gb.OUTCOMES)


class ResultDecodeTests(unittest.TestCase):
    def test_accepts_only_the_strict_result_schema(self):
        self.assertIsNotNone(gb.decode_result(result(), 'GT-AGENT-X01'))
        bad = {
            'missing': None, 'not json': b'{', 'not an object': b'[]', 'nan': result().replace(b'0,', b'NaN,', 1),
            'duplicate key': result()[:-1] + b', "timed_out": false}', 'extra key': result(verdict='pass'),
            'other task': result(task='GT-AGENT-X02'), 'wrong schema': result().replace(b'.v1', b'.v0'),
            'bool exit': result().replace(b'"artifact_exit": 0', b'"artifact_exit": true'),
            'checked timeout': result('ok', timed_out=True), 'unchecked without timeout': result('not_checked'),
            'assertions on a rejection': json.dumps({**json.loads(result()), 'output_check': 'too_large'}).encode(),
            'unknown check': result('maybe'),
            'repeated id': json.dumps({**json.loads(result()), 'assertions': [{'id': 'a', 'passed': True}] * 2}).encode(),
            'string passed': json.dumps({**json.loads(result()), 'assertions': [{'id': 'a', 'passed': 'yes'}]}).encode(),
        }
        for name, data in bad.items():
            with self.subTest(name):
                self.assertIsNone(gb.decode_result(data, 'GT-AGENT-X01'))


class DefinitionTests(unittest.TestCase):
    def test_white_box_tasks_have_no_definition_and_black_box_ones_validate(self):
        self.assertIsNone(gb.definition({'id': 'GT-AGENT-A01'}))
        self.assertIsNone(gb.definition({'oracle_mode': 'white_box'}))
        oracle = gb.definition({'oracle_mode': 'black_box', 'black_box_oracle': ORACLE})
        self.assertEqual((oracle['build'], oracle['stdin'], len(oracle['assertions'])), ('./cmd/alpha', None, 3))

    def test_malformed_definitions_are_refused(self):
        def edited(**changes):
            return {'oracle_mode': 'black_box', 'black_box_oracle': {**ORACLE, **changes}}
        file_item = ORACLE['assertions'][2]
        cases = {
            'mode without oracle': {'oracle_mode': 'black_box'},
            'oracle without mode': {'black_box_oracle': ORACLE},
            'unknown key': edited(env={'A': 'b'}),
            'absolute build': edited(build='/usr/bin/true'), 'escaping build': edited(build='./../x'),
            'command not the artifact': edited(command=['/bin/sh', '-c', 'cat']),
            'artifact named twice': edited(command=['{artifact}', '{artifact}']),
            'repeated input name': edited(inputs=ORACLE['inputs'] * 2),
            'stdin not an input': edited(stdin='missing.txt'),
            'short digest': edited(inputs=[{'path': 'x', 'sha256': 'abc'}]),
            'no assertion': edited(assertions=[]),
            'repeated assertion id': edited(assertions=ORACLE['assertions'][:1] * 2),
            'escaping output path': edited(assertions=[{**file_item, 'path': '../result.txt'}]),
            'dot output path': edited(assertions=[{**file_item, 'path': 'a/./b'}]),
            'stdout with a path': edited(assertions=[{**ORACLE['assertions'][1], 'path': 'x'}]),
            'boolean exit code': edited(assertions=[{'id': 'exit', 'kind': 'exit_code', 'exit_code': True}]),
            'unknown kind': edited(assertions=[{'id': 'x', 'kind': 'regex', 'expected': file_item['expected']}]),
        }
        for name, task in cases.items():
            with self.subTest(name), self.assertRaises(ValueError):
                gb.definition(task)


class FixtureAndBundleTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()
        self.files = {'in.txt': b'input\n', 'stdout.txt': b'3\n', 'result.txt': b'3\n'}
        for name, data in self.files.items():
            (self.base / 'set/evals/harness/oracles/X01').mkdir(parents=True, exist_ok=True)
            (self.base / 'set/evals/harness/oracles/X01' / name).write_bytes(data)
        self.oracle = gb.definition({'oracle_mode': 'black_box', 'black_box_oracle': self.pinned()})

    def pinned(self, **digests) -> dict:
        def pin(name):
            return {'path': 'evals/harness/oracles/X01/' + name,
                    'sha256': digests.get(name) or hashlib.sha256(self.files[name]).hexdigest()}
        return {**ORACLE, 'inputs': [pin('in.txt')], 'stdin': 'in.txt',
                'assertions': [ORACLE['assertions'][0], {**ORACLE['assertions'][1], 'expected': pin('stdout.txt')},
                               {**ORACLE['assertions'][2], 'expected': pin('result.txt')}]}

    def test_inputs_are_copied_read_only_and_expected_outputs_stay_in_memory(self):
        expected = gb.prepare_fixtures(self.oracle, self.base / 'set', self.base / 'input')
        self.assertEqual(expected, {'stdout': b'3\n', 'result': b'3\n'})
        self.assertEqual(sorted(path.name for path in (self.base / 'input').iterdir()), ['in.txt'])
        self.assertEqual((self.base / 'input/in.txt').stat().st_mode & 0o777, 0o444)

    def test_a_changed_or_linked_fixture_is_a_setup_failure(self):
        tampered = gb.definition({'oracle_mode': 'black_box', 'black_box_oracle': self.pinned(**{'result.txt': 'd' * 64})})
        with self.assertRaisesRegex(ValueError, 'result.txt does not match'):
            gb.prepare_fixtures(tampered, self.base / 'set', self.base / 'input1')
        link = self.base / 'set/evals/harness/oracles/X01/stdout.txt'
        link.unlink()
        link.symlink_to(self.base / 'set/evals/harness/oracles/X01/result.txt')
        with self.assertRaisesRegex(ValueError, 'symlink'):
            gb.prepare_fixtures(self.oracle, self.base / 'set', self.base / 'input2')

    def test_bundle_carries_assertions_expected_outputs_stdout_and_status(self):
        args = gb.artifact_args(self.oracle, Path('/a/artifact'), Path('/t/input'), Path('/t/run/out'))
        self.assertEqual(args, ['/a/artifact', '--in', '/t/input/in.txt', '--out', '/t/run/out/r.txt'])
        run = {'exit_code': 0, 'timed_out': False, 'overflow': False}
        document = json.loads(gb.bundle('GT-AGENT-X01', self.oracle, {'stdout': b'3\n', 'result': b'3\n'}, run, b'3\n'))
        self.assertEqual((document['schema_version'], document['artifact_exit'], document['stdout']),
                         (gb.INPUT_SCHEMA, 0, base64.b64encode(b'3\n').decode()))
        self.assertEqual(document['assertions'][2], {'id': 'result', 'kind': 'file', 'path': 'out/result.txt',
                                                     'expected': base64.b64encode(b'3\n').decode()})
        for run, status in (({'exit_code': -9, 'timed_out': False}, None), ({'exit_code': 0, 'timed_out': True}, None),
                            ({'exit_code': 2, 'timed_out': False}, 2), ({'exit_code': None, 'timed_out': False}, None)):
            self.assertEqual(gb.artifact_exit(run), status, run)

    def test_records_add_the_three_signed_lane_fields(self):
        attempt = {'task_id': 'GT-AGENT-X01', 'arm': 'baseline', 'trial': 0}
        derived = gb.derive('oracle', None, CLEAN, result(), 'GT-AGENT-X01', IDS)
        record = gb.record('f' * 32, attempt, derived, 1.23456, 'oracle', CLEAN, 'e' * 64)
        self.assertEqual(list(record), ['schema_version', 'session_id', 'task_id', 'arm', 'trial', 'outcome', 'signal',
                                        'oracle', 'duration_s', 'stage_reached', 'agent_termination',
                                        'oracle_result_sha256'])
        self.assertEqual((record['outcome'], record['duration_s'], record['oracle_result_sha256']), ('pass', 1.235, 'e' * 64))
        early = gb.record('f' * 32, attempt, gb.derive('build', None, CLEAN, None, 'X', IDS), 0, 'build', CLEAN, 'e' * 64)
        self.assertIsNone(early['oracle_result_sha256'])
        digest = gb.store_result(self.base, b'{}')
        self.assertEqual((self.base / 'oracle-results' / (digest + '.json')).read_bytes(), b'{}')
        self.assertEqual(gb.store_result(self.base, b'{}'), digest)


class TerminationTests(unittest.TestCase):
    def test_agent_ends_are_normalized(self):
        run = {'launched': True, 'exit_code': 0, 'timed_out': False}
        cases = [(None, None, gb.NOT_LAUNCHED), ({**run, 'launched': False}, 'agent_launch_failed', gb.NOT_LAUNCHED),
                 ({**run, 'exit_code': 1}, 'agent_launch_failed', gb.NOT_LAUNCHED),
                 ({**run, 'exit_code': -9, 'timed_out': True}, 'agent_timeout',
                  {'launched': True, 'exit_code': None, 'os_signal': 'SIGKILL', 'timed_out': True}),
                 ({**run, 'exit_code': -15}, 'agent_exit_nonzero',
                  {'launched': True, 'exit_code': None, 'os_signal': 'SIGTERM', 'timed_out': False}),
                 ({**run, 'exit_code': -200}, 'agent_exit_nonzero',
                  {'launched': True, 'exit_code': None, 'os_signal': 'SIG200', 'timed_out': False}),
                 ({**run, 'exit_code': 3}, 'agent_exit_nonzero', {**CLEAN, 'exit_code': 3}), (run, None, CLEAN)]
        for given, agent, want in cases:
            with self.subTest(given=given):
                ended = gb.termination(given, agent)
                self.assertEqual(ended, want)
                self.assertEqual(gb.agent_failure(ended), None if want == CLEAN else gb.agent_failure(want))


if __name__ == '__main__':
    unittest.main()
