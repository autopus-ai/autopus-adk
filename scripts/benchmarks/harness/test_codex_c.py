"""Opt-in codex plumbing for pilot C: options, the dedicated login, the per-trial home, rotation and
scrub on every exit path, and which arms get codex; no network, no model call."""
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import codex_c
import run_c

ARGS = ['--repo', '/r', '--output', '/o', '--model', 'opus']


def auth(account='acct-1', refresh='r0', token='t0'):
    return json.dumps({'OPENAI_API_KEY': None, 'auth_mode': 'chatgpt', 'last_refresh': '2026-11-01T00:00:00Z',
                       'tokens': {'account_id': account, 'access_token': token, 'refresh_token': refresh,
                                  'id_token': 'i'}}).encode()


def private(path: Path, data: bytes) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    path.chmod(0o600)
    return path


def codex_ctx(source: Path, dry_run=False) -> dict:
    return {'auth_file': source, 'model': 'm1', 'effort': 'high', 'dry_run': dry_run,
            'config_sha256': codex_c.settings(run_c.parse(ARGS + ['--codex-auth-file', str(source), '--codex-model',
                                                                  'm1']))['config_sha256']}


class OptionTests(unittest.TestCase):
    def test_off_by_default_changes_nothing(self):
        args = run_c.parse(ARGS)
        self.assertEqual(codex_c.tool_names(args), ['go', 'gofmt', 'claude', 'node'])
        self.assertIsNone(codex_c.trial_settings(args))
        self.assertIsNone(codex_c.settings(args))

    def test_codex_is_kept_out_of_the_shared_tools_and_only_codex_arms_use_it(self):
        with tempfile.TemporaryDirectory() as d:
            path = private(Path(d) / 'auth.json', auth(token='SECRET-TOKEN'))
            args = run_c.parse(ARGS + ['--codex-auth-file', str(path), '--tools', 'go,codex,node', '--dry-run'])
            self.assertEqual(codex_c.tool_names(args), ['go', 'node'])
            settings = codex_c.trial_settings(args)
            self.assertTrue(settings['dry_run'])
            self.assertEqual(settings['auth_file'], path.resolve())
            self.assertNotIn('SECRET', json.dumps(codex_c.settings(args)))
            ctx = {'codex': settings}
            self.assertEqual([codex_c.arm_uses(ctx, a) for a in ('native', 'instructions', 'full')],
                             [False, True, True])
            self.assertFalse(codex_c.arm_uses({'codex': None}, 'full'))

    def test_account_login_loose_or_foreign_files_are_refused(self):
        with tempfile.TemporaryDirectory() as d:
            home = Path(d) / 'home'
            account = private(home / '.codex' / 'auth.json', auth())
            link = Path(d) / 'looks-dedicated.json'
            link.symlink_to(account)
            with mock.patch.object(codex_c, '_account_homes', lambda: [(home / '.codex').resolve()]):
                for path in (account, link):
                    with self.assertRaisesRegex(ValueError, 'dedicated'):
                        codex_c.read_auth(path)
                loose = private(Path(d) / 'loose.json', auth())
                loose.chmod(0o644)
                with self.assertRaises(ValueError):
                    codex_c.read_auth(loose)
                with self.assertRaises(ValueError) as caught:
                    codex_c.read_auth(private(Path(d) / 'other.json', b'{"x": "SECRET"}'))
                self.assertNotIn('SECRET', str(caught.exception))
                self.assertEqual(codex_c.read_auth(private(Path(d) / 'ok.json', auth())), auth())


class PrepareRunTests(unittest.TestCase):
    def test_codex_is_linked_apart_and_its_version_recorded(self):
        with tempfile.TemporaryDirectory() as d:
            host = Path(d) / 'host'
            host.mkdir()
            fake = host / 'codex'
            fake.write_text('#!/bin/sh\necho codex-cli 9.9.9\n')
            fake.chmod(0o755)
            ctx = {'codex': {'model': 'm'}, 'output': Path(d) / 'out', 'tools_dir': Path(d) / 'out' / 'bin' / 'tools',
                   'base_env': {'PATH': str(host)}}
            protocol = {'codex': {}}
            codex_c.prepare_run(protocol, ctx)
            self.assertEqual(ctx['codex']['tools_dir'], Path(d) / 'out' / 'bin' / 'tools-codex')
            self.assertEqual(ctx['codex']['tools'], {'codex': os.path.realpath(fake)})
            self.assertEqual(protocol['codex']['version'], 'codex-cli 9.9.9')
            self.assertFalse(ctx['tools_dir'].exists(), 'the shared tools stay without codex')
            ctx['codex'] = {'model': 'm'}
            codex_c.prepare_run(protocol, ctx)
            self.assertEqual(ctx['codex']['tools'], {'codex': os.path.realpath(fake)}, 'a resume reuses the link')

    def test_resume_refuses_a_codex_change(self):
        with tempfile.TemporaryDirectory() as d:
            frozen = {'codex': {'model': 'a', 'effort': 'high'}}
            Path(d, 'protocol.json').write_text(json.dumps(frozen))
            run_c.check_resume(Path(d), dict(frozen))
            for changed in ({'codex': {'model': 'b', 'effort': 'high'}}, {'codex': None}):
                with self.assertRaisesRegex(SystemExit, 'codex'):
                    run_c.check_resume(Path(d), changed)


class HomeTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.root = Path(self.dir.name)
        self.trial = self.root / 'trial'
        self.trial.mkdir()
        self.source = private(self.root / 'auth.json', auth())
        self.codex = codex_ctx(self.source)

    def tearDown(self):
        self.dir.cleanup()

    def test_private_home_with_the_copy_and_the_closed_config(self):
        env = codex_c.install_home(self.trial, self.codex)
        home = Path(env['CODEX_HOME'])
        self.assertEqual(home, self.trial / 'codex-home')
        self.assertEqual(env['CODEX_CA_CERTIFICATE'], codex_c.CA_BUNDLE)
        self.assertEqual(home.stat().st_mode & 0o777, 0o700)
        self.assertEqual((home / 'auth.json').stat().st_mode & 0o777, 0o600)
        self.assertEqual((home / 'auth.json').read_bytes(), self.source.read_bytes())
        config = (home / 'config.toml').read_text()
        for line in ('model = "m1"', 'web_search = "disabled"', 'cli_auth_credentials_store = "file"',
                     'browser_use = false', 'apps = false', 'plugins = false'):
            self.assertIn(line, config)
        with self.assertRaises(FileExistsError):
            codex_c.install_home(self.trial, self.codex)

    def test_dry_run_gets_a_placeholder_never_the_login(self):
        codex_c.install_home(self.trial, codex_ctx(self.source, dry_run=True))
        self.assertEqual((self.trial / 'codex-home' / 'auth.json').read_bytes(), codex_c.PLACEHOLDER)

    def test_rotation_is_detected_and_an_unreadable_copy_is_not_trusted(self):
        codex_c.install_home(self.trial, self.codex)
        copy = self.trial / 'codex-home' / 'auth.json'
        self.assertIs(codex_c.auth_rotated(self.trial, self.codex), False)
        private(copy, auth(token='t1'))
        self.assertIs(codex_c.auth_rotated(self.trial, self.codex), False, 'a new access token alone is not a spend')
        private(copy, auth(refresh='r1', token='t1'))
        self.assertIs(codex_c.auth_rotated(self.trial, self.codex), True)
        # A link, as the file or as the home directory, is never followed out of the trial.
        outside = private(self.root / 'outside' / 'auth.json', self.source.read_bytes())
        copy.unlink()
        copy.symlink_to(outside)
        self.assertEqual(codex_c.auth_rotated(self.trial, self.codex), 'unknown')
        copy.unlink()
        (self.trial / 'codex-home').rename(self.trial / 'moved')
        (self.trial / 'codex-home').symlink_to(outside.parent)
        self.assertEqual(codex_c.auth_rotated(self.trial, self.codex), 'unknown')
        (self.trial / 'codex-home').unlink()
        self.assertEqual(codex_c.auth_rotated(self.trial, self.codex), 'unknown')

    def test_scrub_removes_every_copy_below_the_trial_and_nothing_outside(self):
        codex_c.install_home(self.trial, self.codex)
        stash = self.trial / 'workspace' / 'deep' / 'note.txt'
        private(stash, self.source.read_bytes())
        locked = self.trial / 'locked'
        private(locked / 'x', self.source.read_bytes())
        locked.chmod(0o500)
        outside = private(self.root / 'outside' / 'auth.json', self.source.read_bytes())
        (self.trial / 'link-to-outside').symlink_to(outside)
        removed = codex_c.scrub(self.trial, self.codex)
        self.assertEqual(removed, 3)
        self.assertFalse((self.trial / 'codex-home' / 'auth.json').exists())
        self.assertFalse(stash.exists())
        self.assertFalse((locked / 'x').exists())
        self.assertEqual(outside.read_bytes(), self.source.read_bytes(), 'a link target outside stays')
        self.assertEqual(self.source.read_bytes(), auth(), 'the given login is never written')

    def test_scrub_does_not_reach_through_a_replaced_home(self):
        codex_c.install_home(self.trial, self.codex)
        (self.trial / 'codex-home').rename(self.trial / 'moved')
        outside = private(self.root / 'account' / 'auth.json', auth(refresh='ACCOUNT'))
        (self.trial / 'codex-home').symlink_to(outside.parent)
        codex_c.scrub(self.trial, self.codex)
        self.assertEqual(outside.read_bytes(), auth(refresh='ACCOUNT'))
        self.assertFalse((self.trial / 'moved' / 'auth.json').exists(), 'the moved copy is found by its bytes')


if __name__ == '__main__':
    unittest.main()
