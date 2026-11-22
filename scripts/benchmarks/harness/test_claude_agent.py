"""Claude Code driver contracts: arm settings, isolation, environment, argv and the sandbox wrapper."""
import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

import claude_agent


class SettingsTests(unittest.TestCase):
    def test_only_instructions_disables_hooks(self):
        self.assertTrue(claude_agent.arm_settings('instructions')['disableAllHooks'])
        self.assertNotIn('disableAllHooks', claude_agent.arm_settings('native'))
        self.assertNotIn('disableAllHooks', claude_agent.arm_settings('full'))
        with self.assertRaises(ValueError):
            claude_agent.arm_settings('reduced')

    def test_argv_uses_verified_flags_and_inline_settings(self):
        argv = claude_agent.claude_argv('instructions', 'opus', 'high')
        for flag in ('-p', '--verbose', '--include-hook-events', '--no-session-persistence', '--strict-mcp-config'):
            self.assertIn(flag, argv)
        self.assertEqual(argv[argv.index('--output-format') + 1], 'stream-json')
        self.assertEqual(argv[argv.index('--setting-sources') + 1], 'project')
        self.assertEqual(argv[argv.index('--permission-mode') + 1], 'bypassPermissions')
        self.assertEqual(json.loads(argv[argv.index('--settings') + 1])['disableAllHooks'], True)
        self.assertEqual(argv[argv.index('--disallowedTools') + 1:][:2], ['WebFetch', 'WebSearch'])
        self.assertNotIn('--dangerously-skip-permissions', argv)


class IsolationTests(unittest.TestCase):
    def test_home_has_no_login_arm_user_settings_and_a_trusted_workspace(self):
        with tempfile.TemporaryDirectory() as d:
            work = Path(d) / 'workspace'
            work.mkdir()
            home = claude_agent.isolated_home(Path(d), 'instructions', work)
            config = Path(home['CLAUDE_CONFIG_DIR'])
            self.assertEqual(sorted(p.name for p in config.iterdir()), ['.claude.json', 'settings.json'])
            # A nested `claude -p` reads user settings, so the arm's hook switch must be there too,
            # with the web tools denied and project MCP servers off.
            user = json.loads((config / 'settings.json').read_text())
            self.assertTrue(user['disableAllHooks'])
            self.assertEqual(user['permissions']['deny'], ['WebFetch', 'WebSearch'])
            self.assertIs(user['enableAllProjectMcpServers'], False)
            state = json.loads((config / '.claude.json').read_text())
            self.assertTrue(state['projects'][str(work.resolve())]['hasTrustDialogAccepted'])
            with self.assertRaises(FileExistsError):
                claude_agent.isolated_home(Path(d), 'full', work)

    def test_environment_is_an_allowlist(self):
        base = {'PATH': '/opt/homebrew/bin', 'ANTHROPIC_API_KEY': 'leak', 'GITHUB_TOKEN': 'x', 'SSH_AUTH_SOCK': '/s',
                'ORCA_SESSION': 'o', 'AWS_PROFILE': 'p', 'LANG': 'C', 'USER': 'u', 'CLAUDE_CONFIG_DIR': '/account'}
        env = claude_agent.agent_env(base, {'HOME': '/h', 'CLAUDE_CONFIG_DIR': '/h/.claude'}, Path('/t'), 'oauth',
                                     'tok', [Path('/pinned/bin')])
        self.assertEqual(env['CLAUDE_CODE_OAUTH_TOKEN'], 'tok')
        for key in ('ANTHROPIC_API_KEY', 'GITHUB_TOKEN', 'SSH_AUTH_SOCK', 'ORCA_SESSION', 'AWS_PROFILE'):
            self.assertNotIn(key, env)
        self.assertEqual(env['CLAUDE_CONFIG_DIR'], '/h/.claude')
        self.assertEqual((env['TMPDIR'], env['LANG'], env['USER']), ('/t', 'C', 'u'))
        self.assertEqual(env['PATH'], os.pathsep.join(['/pinned/bin', *claude_agent.SYSTEM_PATH]))

    def test_credential_file_must_be_private_and_single(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'token'
            path.write_text('abc\n')
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                claude_agent.read_credential(path)
            path.chmod(0o600)
            self.assertEqual(claude_agent.read_credential(path), 'abc')
            path.write_text('a b')
            with self.assertRaises(ValueError):
                claude_agent.read_credential(path)


class ShellWrapperTests(unittest.TestCase):
    # The wrapper execs zsh, which pilot C's macOS hosts ship and Linux CI runners lack.
    @unittest.skipUnless(shutil.which('zsh'), 'zsh is not installed')
    def test_login_flags_are_dropped_and_no_startup_file_runs(self):
        with tempfile.TemporaryDirectory() as d:
            shell = claude_agent.write_shell_wrapper(Path(d) / 'bin' / 'zsh')
            env = {'PATH': '/pinned:/usr/bin:/bin', 'HOME': d, 'ZDOTDIR': d}
            Path(d, '.zshrc').write_text('export PATH=/from-zshrc:$PATH\n')
            Path(d, '.zprofile').write_text('export PATH=/from-zprofile:$PATH\n')
            for args in (['-c', '-l', 'echo "$PATH"'], ['-l', '-c', 'echo "$PATH"'], ['-lc', 'echo "$PATH"'],
                         ['--login', '-c', 'echo "$PATH"']):
                out = subprocess.run([str(shell), *args], env=env, capture_output=True, text=True, check=True)
                self.assertEqual(out.stdout.strip(), '/pinned:/usr/bin:/bin', args)


class SandboxTests(unittest.TestCase):
    def test_wrapper_params_and_refusals(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d).resolve()
            out, mod, tool = root / 'out', root / 'mod', root / 'tools' / 'pkg' / 'bin' / 'claude'
            trial = out / 'trials' / 't'
            trial.mkdir(parents=True)
            argv = claude_agent.sandboxed(['claude'], trial, out, out / 'bin', str(mod), {'claude': str(tool)}, d,
                                          [root / 'repo'])
            self.assertEqual(argv[:3], [claude_agent.SANDBOX, '-f', str(claude_agent.PROFILE)])
            for param in ('TRIAL=' + str(trial), 'OUTPUT=' + str(out), 'TOOL0=' + str(tool.parents[1]),
                          'PROTECTED0=' + str(root / 'repo'), 'TOOL3=' + str(trial / '.unused-slot')):
                self.assertIn(param, argv)
            self.assertEqual(argv[-1], 'claude')
            with self.assertRaises(ValueError):
                claude_agent.sandboxed(['claude'], trial, out, out / 'bin', str(mod), {}, d, [out])
            with self.assertRaises(ValueError):
                claude_agent.sandboxed(['claude'], root / 'elsewhere', out, out / 'bin', str(mod), {}, d, [])
            with self.assertRaises(ValueError):
                claude_agent.sandboxed(['claude'], trial, out, out / 'bin', str(out / 'mod'), {}, d, [])
            # The trial's own module cache clone lies inside the trial and is accepted.
            own = claude_agent.sandboxed(['claude'], trial, out, out / 'bin', str(trial / 'gomodcache'), {}, d, [])
            self.assertIn('MODCACHE=' + str(trial / 'gomodcache'), own)

    def test_profile_closes_reads_sockets_and_signals(self):
        profile = claude_agent.PROFILE.read_text()
        for rule in ('(deny file-write*)', '(deny network-outbound (remote unix-socket))', '(deny signal)',
                     '(allow signal (target same-sandbox))', '(subpath "/Users")', '(subpath "/private/var/folders")',
                     '(global-name "com.apple.SecurityServer")', '(deny lsopen)', '(deny user-preference-write)'):
            self.assertIn(rule, profile)
        writes = profile.split('(allow file-write*')[1].split('\n\n')[0]
        self.assertNotIn('/private/var/folders', writes)
        self.assertNotIn('(subpath "/private/tmp")', writes)
        self.assertNotIn('/private/tmp/claude-', profile)


if __name__ == '__main__':
    unittest.main()
