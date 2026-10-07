"""grader.sb denies file reads below the homes and temporary roots, except the grade root, the session
module cache and the Go toolchain, while an oracle still builds and passes (T12 handover, Phase 4 review).

No real credential file is opened: the stores are fake files under a fake account home, and the real
account is probed with access(2) only, which reports readable or not without reading a byte.
"""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import grader
import prepare_grader

# The probe opens each target (lists a *_dir, calls access(2) on a *_access) and logs the error; an
# open_* target must be readable, every other one must not.
PROBE = '''package probe

import (
\t"os"
\t"os/exec"
\t"strings"
\t"syscall"
\t"testing"
)

func TestCredentials(t *testing.T) {
\tdata, err := os.ReadFile("targets.txt")
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tfor _, line := range strings.Split(strings.TrimSpace(string(data)), "\\n") {
\t\tname, path, _ := strings.Cut(line, "=")
\t\tswitch {
\t\tcase strings.HasSuffix(name, "_dir"):
\t\t\t_, err = os.ReadDir(path)
\t\tcase strings.HasSuffix(name, "_access"):
\t\t\terr = syscall.Access(path, 4) // R_OK
\t\tdefault:
\t\t\t_, err = os.ReadFile(path)
\t\t}
\t\tt.Logf("PROBE %s=%v", name, err)
\t}
\terr = exec.Command("/usr/bin/security", "list-keychains").Run()
\tt.Logf("PROBE keychain_ipc=%v", err)
}
'''
# Fake stores below a fake account home: the ones the review named, the ones grader.sb listed before the
# deny-by-default rule, and an ordinary file, which that rule closes as well.
STORES = {'netrc': '.netrc', 'npmrc': '.npmrc', 'git_credentials': '.git-credentials',
          'gemini': '.gemini/oauth_creds.json', 'opencode': '.local/share/opencode/auth.json',
          'gcloud': '.config/gcloud/application_default_credentials.json', 'gh_hosts': '.config/gh/hosts.yml',
          'kube': '.kube/config', 'pypirc': '.pypirc', 'zsh_history': '.zsh_history', 'bash_history': '.bash_history',
          'app_support': 'Library/Application Support/Code/User/globalStorage/state.vscdb',
          'keychain_file': 'Library/Keychains/login.keychain-db', 'ssh_key': '.ssh/id_canary', 'aws': '.aws/credentials',
          'claude_credentials': '.claude/.credentials.json', 'claude_json': '.claude.json', 'omp': '.omp/agent/auth.json',
          'gnupg': '.gnupg/private-keys-v1.d/key', 'docker': '.docker/config.json', 'default_codex': '.codex/auth.json',
          'plain': 'notes/plain.txt'}
# The same kinds of store below the real account home, probed with access(2) only.
REAL_STORES = ('.netrc', '.npmrc', '.git-credentials', '.gemini', '.local/share/opencode', '.config', '.kube',
               '.pypirc', '.zsh_history', '.bash_history', 'Library/Application Support', 'Library/Keychains', '.ssh',
               '.aws', '.codex', '.claude', '.claude.json', '.docker', '.gnupg', '.omp')
PARAMETERS = ('GRADE_ROOT', 'MODCACHE', 'GOROOT', 'ACCOUNT_HOME', 'CODEX_HOME_DIR')


def probe_module(source: Path) -> None:
    source.mkdir(parents=True)
    (source / 'go.mod').write_text('module probe\n\ngo 1.21\n')
    (source / 'probe_test.go').write_text(PROBE)


@unittest.skipUnless(sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and shutil.which('go'),
                     'requires macOS sandbox-exec and the Go toolchain')
class CredentialDenyTests(unittest.TestCase):
    def grade(self, session: Path, targets: dict, roots=None) -> tuple:
        """Prepare the probe module in session, write targets into the grade root and grade the probe."""
        probe_module(session / 'source')
        prepared = prepare_grader.prepare(session / 'source', session / 'grader', ['./...'], 'off')
        root = prepare_grader.new_grade(session / 'grade', session / 'source', Path(prepared['warm_cache']))
        controls = {'open_ws_file': root / 'ws' / 'go.mod', 'open_modcache_dir': Path(prepared['modcache']),
                    'open_toolchain_file': Path(prepared['goroot']) / 'VERSION', 'open_ws_access': root / 'ws' / 'go.mod'}
        rows = {**targets, **controls}
        (root / 'ws' / 'targets.txt').write_text(''.join(f'{name}={path}\n' for name, path in rows.items()))
        with mock.patch.object(grader, 'credential_roots', return_value=roots or grader.credential_roots()):
            result = grader.grade(root, ['go', 'test', '-count=1', '.', '-run', '^TestCredentials$'],
                                  ['TestCredentials'], prepared, session / 'out.jsonl', session / 'out.stderr', 120)
        output = ''.join(json.loads(line).get('Output', '') for line in (session / 'out.jsonl').read_text().splitlines())
        self.assertEqual(result['signal'], 'accepted', output + (session / 'out.stderr').read_text())
        probe = dict(re.findall(r'PROBE (\w+)=(.*)', output))
        for name in controls:
            with self.subTest(name):
                self.assertEqual(probe[name], '<nil>', 'the grade root, module cache and toolchain stay readable')
        self.assertNotEqual(probe['keychain_ipc'], '<nil>', 'no Mach lookup reaches the Security server')
        return probe

    def test_stores_below_the_account_home_are_unreadable_while_an_oracle_inside_it_builds(self):
        # The session lies inside the denied account home, so the oracle builds only through the
        # GRADE_ROOT and MODCACHE openings. /private/tmp is a denied temporary root as well.
        with tempfile.TemporaryDirectory(dir='/private/tmp') as directory:
            base = Path(directory).resolve()
            self.addCleanup(prepare_grader.remove_tree, base)
            account, codex_home = base / 'account', base / 'account' / 'Library' / 'orca codex' / 'home'
            closed = {name: account / relative for name, relative in STORES.items()}
            closed.update(codex_home=codex_home / 'auth.json', neighbor=base / 'other-session' / 'records.jsonl')
            for path in closed.values():
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text('CANARY')
            closed.update(account_dir=account, codex_home_dir=codex_home)
            probe = self.grade(account / 'sessions' / 'one', closed, (str(account), str(codex_home)))
        for name in closed:
            with self.subTest(name):
                self.assertIn('operation not permitted', probe[name])

    def test_the_real_account_stores_are_closed_without_reading_them(self):
        account, codex_home = grader.credential_roots()
        targets = {'home_access': account, 'codex_home_access': codex_home, 'system_keychains_access': '/Library/Keychains'}
        targets.update((re.sub(r'\W', '_', relative.strip('.')) + '_access', os.path.join(account, relative))
                       for relative in REAL_STORES)
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            self.addCleanup(prepare_grader.remove_tree, base)
            probe = self.grade(base / 'session', targets)
        for name in targets:
            with self.subTest(name):
                self.assertNotEqual(probe[name], '<nil>', 'denied, or absent on this host')

    def test_a_missing_parameter_refuses_to_start(self):
        given = {name: '/private/var/empty' for name in PARAMETERS}
        for missing in PARAMETERS:
            with self.subTest(missing=missing):
                defines = [arg for name, value in given.items() if name != missing for arg in ('-D', name + '=' + value)]
                completed = subprocess.run([grader.SANDBOX, '-f', str(grader.PROFILE), *defines, '/usr/bin/true'],
                                           capture_output=True)
                self.assertEqual(completed.returncode, 65)


if __name__ == '__main__':
    unittest.main()
