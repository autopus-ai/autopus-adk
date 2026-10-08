"""The positive control of a black-box definition (SPEC-HARNEVAL-003 REQ-HR-08, review C2; cross-platform).

A task whose own assertions expect a refusal also pins a positive control: the same artifact and command
over a valid input must exit 0 and print the pinned stdout. The oracle harness reports it as two more
assertions, so a fix that refuses every input is expectation_mismatch instead of accepted.
"""
import base64
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import golden_blackbox as gb

CLEAN = {'launched': True, 'exit_code': 0, 'os_signal': None, 'timed_out': False}
ROOT = 'evals/harness/oracles/X01/'
FILES = {'bad.json': b'{"a": 1, "a": 2}\n', 'control/bad.json': b'{"a": 1}\n', 'empty.txt': b'',
         'control/stdout.txt': b'a=1\n'}


def pin(name: str) -> dict:
    return {'path': ROOT + name, 'sha256': hashlib.sha256(FILES[name]).hexdigest()}


ORACLE = {'build': './cmd/alpha', 'command': ['{artifact}', 'read', '{input}/bad.json'], 'inputs': [pin('bad.json')],
          'assertions': [{'id': 'exit', 'kind': 'exit_code', 'exit_code': 1},
                         {'id': 'stdout', 'kind': 'stdout', 'expected': pin('empty.txt')}],
          'positive_control': {'inputs': [pin('control/bad.json')], 'stdout': pin('control/stdout.txt')}}
IDS = ['exit', 'stdout', 'positive_control.exit', 'positive_control.stdout']


def task(**changes) -> dict:
    return {'oracle_mode': 'black_box', 'black_box_oracle': {**ORACLE, **changes}}


def result(*passed) -> bytes:
    return json.dumps({'schema_version': gb.RESULT_SCHEMA, 'task_id': 'GT-AGENT-X01', 'output_check': 'ok',
                       'assertions': [{'id': name, 'passed': value} for name, value in zip(IDS, passed)],
                       'artifact_exit': 1, 'timed_out': False}).encode()


class PositiveControlDefinitionTests(unittest.TestCase):
    def test_the_control_adds_its_two_reserved_assertion_ids(self):
        oracle = gb.definition(task())
        self.assertEqual(oracle['positive_control'], {'inputs': [pin('control/bad.json')], 'stdin': None,
                                                      'stdout': pin('control/stdout.txt')})
        self.assertEqual(gb.assertion_ids(oracle), IDS)
        plain = gb.definition(task(positive_control=None))
        self.assertEqual((plain['positive_control'], gb.assertion_ids(plain)), (None, ['exit', 'stdout']))

    def test_malformed_controls_and_reserved_ids_are_refused(self):
        control = ORACLE['positive_control']
        cases = {
            'unknown control key': {**control, 'exit_code': 0},
            'no stdout': {'inputs': control['inputs']},
            'no input': {**control, 'inputs': []},
            'inputs absent': {'stdout': control['stdout']},
            'repeated input name': {**control, 'inputs': [pin('control/bad.json'), pin('bad.json')]},
            'stdin names no control input': {**control, 'stdin': 'other.json'},
            'stdout outside the oracle root': {**control, 'stdout': {'path': 'README.md', 'sha256': 'a' * 64}},
        }
        for name, value in cases.items():
            with self.subTest(name), self.assertRaises(ValueError):
                gb.definition(task(positive_control=value))
        for reserved in ('positive_control.exit', 'positive_control.x'):
            with self.subTest(reserved), self.assertRaises(ValueError):
                gb.definition(task(assertions=[{'id': reserved, 'kind': 'exit_code', 'exit_code': 1}]))


class PositiveControlTrialTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()
        for name, data in FILES.items():
            (self.base / 'set' / ROOT / name).parent.mkdir(parents=True, exist_ok=True)
            (self.base / 'set' / ROOT / name).write_bytes(data)
        self.oracle = gb.definition(task())

    def test_control_inputs_get_their_own_read_only_directory(self):
        expected = gb.prepare_fixtures(self.oracle, self.base / 'set', self.base / 'input')
        self.assertEqual(expected, {'stdout': b'', 'positive_control.stdout': b'a=1\n'})
        control = gb.control_input(self.base / 'input')
        self.assertEqual(control, self.base / 'input-control')
        self.assertEqual((control / 'bad.json').read_bytes(), b'{"a": 1}\n')
        self.assertEqual((control / 'bad.json').stat().st_mode & 0o777, 0o444)
        self.assertEqual((self.base / 'input/bad.json').read_bytes(), b'{"a": 1, "a": 2}\n')

    def test_a_drifted_control_fixture_is_a_setup_failure(self):
        (self.base / 'set' / ROOT / 'control/stdout.txt').write_bytes(b'a=2\n')
        with self.assertRaisesRegex(ValueError, 'control/stdout.txt does not match'):
            gb.prepare_fixtures(self.oracle, self.base / 'set', self.base / 'input')

    def test_bundle_sends_the_control_run_beside_the_tasks(self):
        refused = {'exit_code': 1, 'timed_out': False, 'overflow': False}
        accepted = {'exit_code': 0, 'timed_out': False, 'overflow': False}
        expected = {'stdout': b'', 'positive_control.stdout': b'a=1\n'}
        document = json.loads(gb.bundle('GT-AGENT-X01', self.oracle, expected, refused, b'', (accepted, b'a=1\n')))
        self.assertEqual(document['positive_control'], {
            'artifact_exit': 0, 'timed_out': False, 'stdout': base64.b64encode(b'a=1\n').decode(),
            'stdout_overflow': False, 'expected_stdout': base64.b64encode(b'a=1\n').decode()})
        self.assertEqual([item['id'] for item in document['assertions']], ['exit', 'stdout'])
        killed = json.loads(gb.bundle('GT-AGENT-X01', self.oracle, expected, refused, b'',
                                      ({'exit_code': -9, 'timed_out': True, 'overflow': False}, b'')))
        self.assertEqual((killed['positive_control']['artifact_exit'], killed['positive_control']['timed_out']),
                         (None, True))
        plain = gb.definition(task(positive_control=None))
        self.assertNotIn('positive_control', json.loads(gb.bundle('GT-AGENT-X01', plain, expected, refused, b'')))

    def test_a_fix_that_refuses_every_input_is_an_expectation_mismatch(self):
        def judged(data):
            outcome, signal, oracle = gb.derive('oracle', None, CLEAN, data, 'GT-AGENT-X01', IDS)
            return outcome, signal, oracle['expected_passed'], oracle['expected_failed']
        self.assertEqual(judged(result(True, True, True, True)), ('pass', 'accepted', 4, 0))
        self.assertEqual(judged(result(True, True, False, False)), ('fail', 'expectation_mismatch', 2, 2))
        # A result without the control's assertions names other assertions than main's definition.
        self.assertEqual(judged(result(True, True))[:2], ('fail', 'oracle_harness_error'))


if __name__ == '__main__':
    unittest.main()
