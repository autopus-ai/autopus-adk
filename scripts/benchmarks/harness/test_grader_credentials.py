"""grader.sb keeps the account's credential stores unreadable while the oracle still builds (T12 handover)."""
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

PROBE = '''package probe

import (
\t"os"
\t"os/exec"
\t"strings"
\t"testing"
)

func TestCredentials(t *testing.T) {
\tdata, err := os.ReadFile("targets.txt")
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tfor _, line := range strings.Split(strings.TrimSpace(string(data)), "\\n") {
\t\tname, path, _ := strings.Cut(line, "=")
\t\t_, err := os.ReadFile(path)
\t\tt.Logf("PROBE %s=%v", name, err)
\t}
\terr = exec.Command("/usr/bin/security", "list-keychains").Run()
\tt.Logf("PROBE keychain_ipc=%v", err)
}
'''
SECRETS = {'ssh_key': '.ssh/id_canary', 'gh_hosts': '.config/gh/hosts.yml', 'aws': '.aws/credentials',
           'claude_dir': '.claude/.credentials.json', 'claude_json': '.claude.json', 'omp': '.omp/agent/auth.json',
           'gnupg': '.gnupg/private-keys-v1.d/key', 'docker': '.docker/config.json',
           'keychain_file': 'Library/Keychains/login.keychain-db', 'default_codex': '.codex/auth.json'}


@unittest.skipUnless(sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and shutil.which('go'),
                     'requires macOS sandbox-exec and the Go toolchain')
class CredentialDenyTests(unittest.TestCase):
    def test_credential_stores_are_unreadable_and_other_account_files_stay_readable(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            self.addCleanup(prepare_grader.remove_tree, base)
            account, codex_home = base / 'account', base / 'account' / 'Library' / 'orca codex' / 'home'
            targets = {name: account / relative for name, relative in SECRETS.items()}
            targets.update(codex_home=codex_home / 'auth.json', plain=account / 'notes' / 'plain.txt')
            for path in targets.values():
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text('CANARY')
            source = base / 'source'
            source.mkdir()
            (source / 'go.mod').write_text('module probe\n\ngo 1.21\n')
            (source / 'probe_test.go').write_text(PROBE)
            (source / 'targets.txt').write_text(''.join(f'{name}={path}\n' for name, path in targets.items()))
            prepared = prepare_grader.prepare(source, base / 'grader', ['./...'], 'off')
            root = prepare_grader.new_grade(base / 'grade', source, Path(prepared['warm_cache']))
            with mock.patch.object(grader, 'credential_roots', return_value=(str(account), str(codex_home))):
                result = grader.grade(root, ['go', 'test', '-count=1', '.', '-run', '^TestCredentials$'],
                                      ['TestCredentials'], prepared, base / 'out.jsonl', base / 'out.stderr', 120)
            output = ''.join(json.loads(line).get('Output', '') for line in (base / 'out.jsonl').read_text().splitlines())
            probe = dict(re.findall(r'PROBE (\w+)=(.*)', output))
            self.assertEqual(result['signal'], 'accepted', output)
            for name in [*SECRETS, 'codex_home']:
                with self.subTest(name):
                    self.assertIn('operation not permitted', probe[name])
            self.assertEqual(probe['plain'], '<nil>')
            self.assertNotEqual(probe['keychain_ipc'], '<nil>')

    def test_missing_credential_parameter_refuses_to_start(self):
        argv = ['/usr/bin/sandbox-exec', '-f', str(grader.PROFILE), '-D', 'GRADE_ROOT=/private/var/empty']
        for extra in ([], ['-D', 'ACCOUNT_HOME=/private/var/empty'], ['-D', 'CODEX_HOME_DIR=/private/var/empty']):
            with self.subTest(extra=extra):
                completed = subprocess.run(argv + extra + ['/usr/bin/true'], capture_output=True)
                self.assertEqual(completed.returncode, 65)


if __name__ == '__main__':
    unittest.main()
