"""--resume compares the stored protocol with the new one after the JSON round trip."""
import json
import tempfile
import unittest
from pathlib import Path

import run_c


class ResumeTests(unittest.TestCase):
    def protocol(self):
        return {'frozen_hash': 'h', 'tasks': ['c01'], 'arms': {'native': {}}, 'order': [('c01', 'native', 1)],
                'model': 'opus', 'effort': 'high', 'timeout_seconds': 5400, 'harness_revision': 'r',
                'surfaces': {}, 'sandbox': True, 'permission_mode': 'p', 'trial_environment': {}}

    def test_the_same_protocol_resumes(self):
        # The order is built from tuples; protocol.json gives lists back.
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / 'protocol.json').write_text(json.dumps(self.protocol()))
            run_c.check_resume(Path(tmp), self.protocol())

    def test_a_different_order_is_refused(self):
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / 'protocol.json').write_text(json.dumps(self.protocol()))
            changed = self.protocol()
            changed['order'] = [('c01', 'full', 1)]
            with self.assertRaisesRegex(SystemExit, 'order'):
                run_c.check_resume(Path(tmp), changed)


if __name__ == '__main__':
    unittest.main()
