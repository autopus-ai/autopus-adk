"""Pilot C trial environment: overrides seen in Bash commands, self-test verdicts, and (macOS) the
sandboxed xcrun database, here-documents and the per-trial module cache clone."""
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

import claude_agent
import stream_c
import trial_env_c


class OverrideTests(unittest.TestCase):
    def test_assignments_are_found_and_reads_are_not(self):
        self.assertEqual(trial_env_c.env_overrides('export CGO_ENABLED=0 GOCACHE=$PWD/.c TMPDIR=$PWD/.t && go vet'),
                         ['CGO_ENABLED', 'GOCACHE', 'TMPDIR'])
        self.assertEqual(trial_env_c.env_overrides('GOFLAGS=-mod=mod go test ./...; go env -w GOMODCACHE=/x'),
                         ['GOFLAGS', 'GOMODCACHE'])
        for quiet in ('echo $TMPDIR', 'go env GOCACHE', 'GOTMPDIR=/x go build', 'ls "${TMPDIR}"'):
            self.assertEqual(trial_env_c.env_overrides(quiet), [], quiet)

    def test_stream_records_overrides_per_command(self):
        with tempfile.TemporaryDirectory() as d:
            events = Path(d) / 'events.jsonl'
            uses = [{'type': 'tool_use', 'name': 'Bash', 'input': {'command': c}}
                    for c in ('go test ./...', 'export GOCACHE=/w/c && go test ./...')]
            events.write_text(json.dumps({'type': 'assistant', 'message': {'content': uses}}) + '\n')
            observed = stream_c.parse_stream(events)
            self.assertEqual(observed['env_overrides'],
                             [{'vars': ['GOCACHE'], 'command': 'export GOCACHE=/w/c && go test ./...'}])


class SelftestVerdictTests(unittest.TestCase):
    ENV = {'GOCACHE': '/t/gocache', 'GOMODCACHE': '/t/gomodcache', 'GOTMPDIR': '/t/tmp'}
    OK = {'exit_code': 0, 'timed_out': False}

    def test_a_denial_on_stderr_fails_even_with_exit_zero(self):
        err = "git: error: couldn't create cache file '/var/folders/x/T/xcrun_db-a' (errno=Operation not permitted)"
        self.assertTrue(trial_env_c.judge('git_status', self.OK, '', err, None, self.ENV))
        self.assertTrue(trial_env_c.judge('zsh_heredoc', self.OK, '', "can't create temp file", 'ok', self.ENV))
        self.assertEqual(trial_env_c.judge('zsh_heredoc', self.OK, 'zsh-heredoc-ok\n', '', 'zsh-heredoc-ok',
                                           self.ENV), [])

    def test_go_env_must_name_the_trial_caches_and_cgo_on(self):
        good = '/t/gocache\n/t/gomodcache\n/t/tmp\n1\n'
        self.assertEqual(trial_env_c.judge('go_env', self.OK, good, '', None, self.ENV), [])
        self.assertTrue(trial_env_c.judge('go_env', self.OK, good.replace('\n1\n', '\n0\n'), '', None, self.ENV))
        self.assertTrue(trial_env_c.judge('go_env', {'exit_code': 1, 'timed_out': False}, good, '', None, self.ENV))

    def test_steps_cover_the_required_checks(self):
        names = [name for name, _, _ in trial_env_c.selftest_steps('/bin/zsh', ['internal/cli'])]
        self.assertEqual(names, ['go_build', 'go_test_compile', 'zsh_heredoc', 'bash_heredoc', 'git_status',
                                 'cc_compile', 'go_env'])


@unittest.skipUnless(sys.platform == 'darwin', 'Seatbelt and xcrun are macOS-only')
class SandboxedEnvironmentTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp()).resolve()
        self.addCleanup(shutil.rmtree, self.root, True)
        self.out = self.root / 'out'
        self.shell = claude_agent.write_shell_wrapper(self.out / 'bin' / 'shell' / 'zsh')

    def trial(self, name: str) -> Path:
        trial = self.out / 'trials' / name
        (trial / 'tmp').mkdir(parents=True)
        return trial

    def run_in(self, trial: Path, argv: list, extra: dict) -> subprocess.CompletedProcess:
        wrapped = claude_agent.sandboxed(argv, trial, self.out, self.out / 'bin', str(trial / 'gomodcache'), {},
                                         str(Path.home()), [])
        env = {'PATH': '/usr/bin:/bin', 'HOME': str(trial), 'TMPDIR': str(trial / 'tmp'), **extra}
        return subprocess.run(wrapped, cwd=trial, env=env, capture_output=True, text=True, timeout=120)

    def test_xcrun_db_in_the_trial_temp_silences_the_shim_and_stays_private(self):
        first, second = self.trial('a'), self.trial('b')
        bare = self.run_in(first, ['/usr/bin/git', '--version'], {})
        self.assertIn("couldn't create cache file", bare.stderr)
        fixed = self.run_in(first, ['/usr/bin/git', '--version'], trial_env_c.temp_env(first / 'tmp'))
        self.assertEqual((fixed.returncode, fixed.stderr), (0, ''))
        self.assertTrue((first / 'tmp' / 'xcrun_db').is_file())
        peek = self.run_in(second, ['/bin/cat', str(first / 'tmp' / 'xcrun_db')], trial_env_c.temp_env(second / 'tmp'))
        self.assertNotEqual(peek.returncode, 0)

    def test_zsh_and_bash_here_documents_write_into_the_trial(self):
        trial = self.trial('h')
        script = 'cat <<EOF\nheredoc-ok\nEOF'
        bare = self.run_in(trial, ['/bin/zsh', '-f', '-c', script], {})
        self.assertIn("can't create temp file", bare.stderr)
        for argv, extra in (([str(self.shell), '-c', script], {}),
                            ([str(self.shell), '-c', script], trial_env_c.temp_env(trial / 'tmp')),
                            (['/bin/bash', '-c', script], {})):
            done = self.run_in(trial, argv, extra)
            self.assertEqual((done.returncode, done.stdout, done.stderr), (0, 'heredoc-ok\n', ''), argv)

    def test_module_cache_clone_is_writable_and_leaves_the_source_alone(self):
        source = self.out / 'cache' / 'modcache' / 'cache' / 'download' / 'm' / '@v'
        source.mkdir(parents=True)
        (source / 'v1.info').write_text('{}')
        trial = self.trial('m')
        clone = trial_env_c.clone_modcache(self.out / 'cache' / 'modcache', trial / 'gomodcache')
        self.assertTrue(clone['apfs_clone'])
        target = trial / 'gomodcache' / 'cache' / 'download' / 'm' / '@v' / 'v2.info'
        wrote = self.run_in(trial, ['/bin/sh', '-c', 'echo x > "$1"', 'w', str(target)], {})
        self.assertEqual(wrote.returncode, 0, wrote.stderr)
        denied = self.run_in(trial, ['/bin/sh', '-c', 'echo x > "$1"', 'w', str(source / 'v2.info')], {})
        self.assertNotEqual(denied.returncode, 0)
        self.assertEqual(sorted(p.name for p in source.iterdir()), ['v1.info'])


if __name__ == '__main__':
    unittest.main()
