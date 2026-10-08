"""fill_modules.sh, the trusted module step of the signed live lane (SPEC-HARNEVAL-003 REQ-HR-08).

The live-eval and sign jobs build offline from a module cache; a fresh hosted runner has no warm local
cache to serve as the file proxy. fill_modules.sh fills one new cache, outside every sandbox, with
`go mod download` in the git archive of each named commit, and refuses a go.sum the download changed.
Here a stand-in go records each call, so no module is fetched.
"""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from test_golden_fixture import git

SCRIPT = Path(__file__).resolve().parent / 'fill_modules.sh'
FAKE_GO = """#!/bin/sh
printf '%s|%s|%s|%s|%s\\n' "$*" "$(pwd -P)" "$GOMODCACHE" "$GOFLAGS" "$(cat go.mod)" >> "$FAKE_LOG"
if [ -n "$FAKE_TOUCH_SUM" ]; then echo 'example.com/x v1.0.0 h1:x=' >> go.sum; fi
"""


class FillModulesTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()
        self.repo = self.base / 'repo'
        self.repo.mkdir()
        git(self.repo, 'init', '--quiet')
        self.commits = []
        for version in ('1.26', '1.27'):
            (self.repo / 'go.mod').write_text('module example.com/m' + version + '\n')
            (self.repo / 'go.sum').write_text('')
            git(self.repo, 'add', '--all')
            git(self.repo, 'commit', '--quiet', '-m', version)
            self.commits.append(git(self.repo, 'rev-parse', 'HEAD'))
        self.bin, self.log = self.base / 'bin', self.base / 'go.log'
        self.bin.mkdir()
        (self.bin / 'go').write_text(FAKE_GO)
        (self.bin / 'go').chmod(0o755)

    def fill(self, cache: Path, *revisions: str, **env) -> subprocess.CompletedProcess:
        environ = {'PATH': f'{self.bin}:/usr/bin:/bin', 'HOME': str(self.base), 'FAKE_LOG': str(self.log), **env}
        return subprocess.run(['/bin/bash', str(SCRIPT), str(cache), *revisions], cwd=self.repo, env=environ,
                              capture_output=True, text=True, timeout=60)

    def test_each_commit_tree_downloads_into_the_new_cache(self):
        cache = self.base / 'modules'
        done = self.fill(cache, *self.commits)
        self.assertEqual(done.returncode, 0, done.stderr)
        calls = [line.split('|') for line in self.log.read_text().splitlines()]
        self.assertEqual([call[0] for call in calls], ['mod download'] * 2)
        self.assertEqual([call[2] for call in calls], [str(cache)] * 2)
        self.assertEqual([call[3] for call in calls], ['-mod=mod'] * 2)
        self.assertEqual([call[4] for call in calls], ['module example.com/m1.26', 'module example.com/m1.27'])
        for call in calls:
            self.assertNotEqual(Path(call[1]), self.repo, 'each download runs in its extracted tree')
            self.assertFalse(Path(call[1]).exists(), 'the extracted trees are removed')
        self.assertTrue(cache.is_dir())

    def test_a_changed_go_sum_an_existing_cache_or_a_non_commit_is_refused(self):
        changed = self.fill(self.base / 'm1', self.commits[0], FAKE_TOUCH_SUM='1')
        self.assertNotEqual(changed.returncode, 0)
        self.assertIn('go.sum', changed.stderr)
        existing = self.base / 'm2'
        existing.mkdir()
        self.assertNotEqual(self.fill(existing, self.commits[0]).returncode, 0)
        for revision in ('HEAD', self.commits[0][:12], '-' + self.commits[0][1:], ''):
            with self.subTest(revision=revision):
                self.assertNotEqual(self.fill(self.base / ('m3' + str(len(revision))), revision).returncode, 0)
        self.assertNotEqual(self.fill(self.base / 'm4').returncode, 0, 'at least one commit')


if __name__ == '__main__':
    unittest.main()
