import subprocess
import tempfile
import unittest
from pathlib import Path

import workspace


class WorkspaceTests(unittest.TestCase):
    def test_exact_mutation_and_scope(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'source.go').write_text('old value')
            (root / 'oracle_test.go').write_text('protected')
            task = {'mutation': {'path': 'source.go', 'before': 'old', 'after': 'new'}}
            workspace.apply_mutation(root, task)
            self.assertEqual((root / 'source.go').read_text(), 'new value')
            with self.assertRaises(ValueError):
                workspace.apply_mutation(root, task)
            before = workspace.hashes(root)
            (root / 'source.go').write_text('fixed')
            (root / 'extra_test.go').write_text('new test')
            self.assertTrue(workspace.audit(before, root, ['source.go'])['accepted_scope'])
            (root / 'oracle_test.go').write_text('tampered')
            report = workspace.audit(before, root, ['source.go'])
            self.assertFalse(report['accepted_scope'])
            self.assertEqual(report['protected_changes'], ['oracle_test.go'])

    def test_duplicate_mutation_and_deleted_oracle_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'source.go').write_text('old old')
            task = {'mutation': {'path': 'source.go', 'before': 'old', 'after': 'new'}}
            with self.assertRaises(ValueError):
                workspace.apply_mutation(root, task)
            (root / 'oracle_test.go').write_text('oracle')
            before = workspace.hashes(root)
            (root / 'oracle_test.go').unlink()
            self.assertEqual(workspace.audit(before, root, ['source.go'])['protected_changes'], ['oracle_test.go'])
            workspace.initialize(root)
            with self.assertRaises(ValueError):
                workspace.initialize(root)

    def test_cache_ignored_and_new_source_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            before = workspace.hashes(root)
            for name in ['.git/config', '__pycache__/module.pyc', '.pytest_cache/state']:
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text('cache')
            self.assertEqual(before, workspace.hashes(root))
            (root / 'backdoor.go').write_text('unexpected')
            self.assertEqual(workspace.audit(before, root, [])['unexpected_paths'], ['backdoor.go'])

    def test_install_copies_only_instruction_surface(self):
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            source, root = parent / 'surface', parent / 'work'
            root.mkdir()
            for name in ['AGENTS.md', '.codex/config.toml', '.codex/skills/example/SKILL.md', '.codex/hooks/run.sh', '.codex/agents/worker.toml', '.git/hooks/pre-commit']:
                path = source / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(name)
            workspace.install_surface(root, source)
            self.assertEqual(sorted(workspace.hashes(root)), ['.codex/config.toml', '.codex/skills/example/SKILL.md', 'AGENTS.md'])

    def test_snapshot_strips_harness_and_initializes_without_history(self):
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            repo = parent / 'repo'
            repo.mkdir()
            for name in ['source.go', 'AGENTS.md', '.autopus/spec.md', '.codex/config.toml', 'content/skills/example.md']:
                path = repo / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text('original')
            workspace.initialize(repo)
            revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip()
            destination = parent / 'snapshot'
            workspace.snapshot(repo, revision, destination)
            self.assertEqual(sorted(workspace.hashes(destination)), ['content/skills/example.md', 'source.go'])
            (destination / 'source.go').write_text('seeded')
            workspace.initialize(destination)
            count = subprocess.check_output(['git', 'rev-list', '--count', 'HEAD'], cwd=destination, text=True).strip()
            self.assertEqual(count, '1')
            previous = subprocess.run(['git', 'show', revision + ':source.go'], cwd=destination, capture_output=True)
            self.assertNotEqual(previous.returncode, 0)
            self.assertEqual(subprocess.check_output(['git', 'show', 'HEAD:source.go'], cwd=destination, text=True), 'seeded')


if __name__ == '__main__':
    unittest.main()
