"""The shared REQ-HR-08 judgement table (SPEC-HARNEVAL-003): runner and signer agree on hand-made inputs.

pkg/harneval/testdata/derive-table.json lists records and oracle result bytes the runner never writes
itself (a setup-stage record with an agent signal, a result without schema_version or with a repeated
key, an exit status out of range, ...). Go's TestDeriveTrial_SharedTable judges the same file with the
signer's deriveTrial, so a row the two implementations read differently fails one side.
"""
import json
from pathlib import Path
import unittest

import golden_blackbox as gb

TABLE = Path(__file__).resolve().parents[3] / 'pkg/harneval/testdata/derive-table.json'


class SharedDeriveTableTests(unittest.TestCase):
    def test_every_case_gets_its_want(self):
        table = json.loads(TABLE.read_text())
        self.assertGreaterEqual(len(table['cases']), 40)
        for case in table['cases']:
            with self.subTest(case['name']):
                name = case['oracle_result']
                data = table['results'][name].encode() if name is not None else None
                ids = case.get('assertion_ids', table['assertion_ids'])
                args = (case['stage_reached'], case['signal'], table['terminations'][case['agent_termination']], data,
                        table['task_id'], ids)
                if case['want'] is None:
                    with self.assertRaises(ValueError):
                        gb.derive(*args)
                    continue
                outcome, signal, oracle = gb.derive(*args)
                self.assertEqual([outcome, signal, oracle['ran'], oracle['build_failed'], oracle['expected_passed'],
                                  oracle['expected_failed']], case['want'])


if __name__ == '__main__':
    unittest.main()
