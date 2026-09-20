import tempfile
import tomllib
import unittest
from pathlib import Path

from permissions import profile_args


class PermissionTests(unittest.TestCase):
    def test_exact_roots_and_no_legacy_sandbox(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            roots = [base / name for name in ('workspace', 'cache', 'temp')]
            for root in roots:
                root.mkdir()
            args = profile_args(*roots)
            self.assertEqual(args[::2], ['-c'] * (len(args) // 2))
            values = args[1::2]
            self.assertIn('default_permissions="bench"', values)
            fs = tomllib.loads(next(value for value in values if value.startswith('permissions.bench.filesystem=')))['permissions']['bench']['filesystem']
            self.assertEqual(fs[':root'], 'deny')
            self.assertIn('permissions.bench.network.enabled=false', values)
            self.assertFalse(any('sandbox_mode' in value for value in values))
            for root in roots:
                self.assertEqual(fs[str(root.resolve())], 'write')
            self.assertNotIn(str(base.resolve()), fs)

    def test_rejects_overbroad_or_overlapping_roots(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaises(ValueError):
                profile_args(Path('/'), root / 'cache', root / 'temp')
            with self.assertRaises(ValueError):
                profile_args(root / 'workspace', root, root / 'temp')


if __name__ == '__main__':
    unittest.main()
