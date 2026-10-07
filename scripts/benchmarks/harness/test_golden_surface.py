"""Per-revision arm surfaces of the golden mode (SPEC-HARNEVAL-001 REQ-HE-07, T13).

The surface driver build and run, the baseline_ref_unsupported refusal before any agent call, the
runner wiring without --surfaces, and the bootstrap: the driver builds and runs at v0.50.122. The
build and the run go through grader.sb, so the tests that really build need macOS sandbox-exec.
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

import golden
import golden_protocol as gp
import golden_surface as gs
import grader
import prepare_grader
from test_golden_fixture import argv, build_world, calls, git
import workspace
from test_golden_runner import fake_digests

HERE = Path(__file__).resolve().parent
CHECKOUT = HERE.parents[2]
PINS = json.loads((CHECKOUT / 'evals/harness/manifest.json').read_text())['pins']
# One entry file per platform (claude-code, codex, antigravity-cli, opencode, omp) and the saved config.
ENTRIES = ('.claude/settings.json', '.codex/config.toml', '.gemini/settings.json', 'opencode.json', '.omp/commands',
           'autopus.yaml')
PLUGIN = '.autopus/plugins/auto/.codex-plugin/plugin.json'
SANDBOXED = sys.platform == 'darwin' and os.path.exists(grader.SANDBOX) and bool(shutil.which('go'))


def resolves(revision: str) -> bool:
    return subprocess.run(['git', 'rev-parse', '--verify', '--quiet', revision + '^{commit}'], cwd=CHECKOUT,
                          capture_output=True).returncode == 0


def elsewhere_tree(root: Path) -> Path:
    """A build root whose tree is a module without the driver API, holding a stale driver package."""
    package = root / 'src' / 'scripts/benchmarks/harness/surface_driver'
    package.mkdir(parents=True)
    (root / 'src' / 'go.mod').write_text('module example.com/elsewhere\n\ngo 1.21\n')
    (package / 'main.go').write_text('package main\n\nfunc main() { panic("revision driver") }\n')
    (package / 'extra.go').write_text('package main\n')
    return package


class DriverBuildTests(unittest.TestCase):
    def test_the_build_is_trimmed_offline_cgo_free_and_links_the_pinned_version(self):
        self.assertEqual(gs.build_command('v0.50.123', Path('/b/driver')),
                         ['go', 'build', '-trimpath',
                          '-ldflags=-X github.com/insajin/autopus-adk/pkg/version.version=v0.50.123',
                          '-o', '/b/driver', './scripts/benchmarks/harness/surface_driver'])
        with self.assertRaises(gs.SurfaceError) as caught:
            gs.build_command('v0.50.123 -X main.x=y', Path('/b/driver'))
        self.assertEqual(caught.exception.stage, 'build')
        env = gs.build_env(Path('/s/driver/build'), Path('/toolchain/bin/go'), Path('/s/driver/modcache'))
        self.assertEqual(env, {'PATH': '/toolchain/bin', 'HOME': '/s/driver/build/home', 'TMPDIR': '/s/driver/build/tmp',
                               'GOPATH': '/s/driver/build/gopath', 'GOCACHE': '/s/driver/build/gocache',
                               'GOMODCACHE': '/s/driver/modcache', 'GOFLAGS': '-mod=readonly -buildvcs=false',
                               'GOPROXY': 'off', 'GOSUMDB': 'off', 'GOWORK': 'off', 'GOTOOLCHAIN': 'local',
                               'CGO_ENABLED': '0', 'PWD': '/s/driver/build/src'})

    @unittest.skipUnless(shutil.which('go'), 'go answers GOROOT and downloads the modules')
    def test_the_build_runs_under_grader_sb_writing_only_its_build_root(self):
        launched = []

        def launch(argv, cwd, stdout, stderr, timeout):
            launched.append((argv, cwd))
            return {'exit_code': 0, 'timed_out': False}
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            elsewhere_tree(base / 'build')
            with mock.patch.object(gs.grader, 'run_sandboxed', side_effect=launch):
                binary = gs.build_driver(base / 'build', 'v0.50.123', base / 'modcache', base / 'download')
        go, goroot = grader.toolchain()
        sandbox_argv, cwd = launched[0]
        split = sandbox_argv.index(grader.SANDBOX)
        self.assertEqual((sandbox_argv[:2], cwd, binary), (['/usr/bin/env', '-i'], base / 'build' / 'src',
                                                            base / 'build' / 'surface_driver'))
        self.assertEqual(sandbox_argv[split:split + 9], [grader.SANDBOX, '-f', str(grader.PROFILE),
                                                         '-D', 'GRADE_ROOT=' + str(base / 'build'),
                                                         '-D', 'MODCACHE=' + str(base / 'modcache'),
                                                         '-D', 'GOROOT=' + str(goroot)])
        self.assertEqual(sandbox_argv[-7:], [str(go), 'build', '-trimpath',
                                             '-ldflags=-X github.com/insajin/autopus-adk/pkg/version.version=v0.50.123',
                                             '-o', str(base / 'build' / 'surface_driver'),
                                             './scripts/benchmarks/harness/surface_driver'])
        self.assertIn('CGO_ENABLED=0', sandbox_argv[2:split])
        self.assertIn('GOPROXY=off', sandbox_argv[2:split])

    @unittest.skipUnless(SANDBOXED, 'requires macOS sandbox-exec and the Go toolchain')
    def test_the_arm_driver_package_holds_only_this_checkout_driver(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            package = elsewhere_tree(base / 'build')
            with self.assertRaises(gs.SurfaceError) as caught:
                gs.build_driver(base / 'build', 'v0.50.123', base / 'modcache', base / 'download')
            self.assertEqual(sorted(path.name for path in package.iterdir()), ['main.go'])
            self.assertEqual((package / 'main.go').read_bytes(), gs.DRIVER.read_bytes())
        self.assertEqual(caught.exception.stage, 'build')
        self.assertIn('cannot find module providing package github.com/insajin/autopus-adk/pkg/',
                      caught.exception.detail)


class BaselineRefTests(unittest.TestCase):
    """Without --surfaces, a baseline ref the driver cannot be built from refuses the session first."""

    def world(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        return build_world(Path(directory.name), ['alpha'])

    def refuse(self, world, steps=None, *extra):
        steps = steps or golden.Steps(platform='darwin', set_digests=fake_digests)
        with self.assertRaises(golden.Refusal) as caught:
            golden.run_session(golden.parse(argv(world, '--auto', 'unused', *extra, surfaces=False)), steps)
        self.assertEqual(calls(world), [])
        return caught.exception

    @unittest.skipUnless(SANDBOXED, 'requires macOS sandbox-exec and the Go toolchain')
    def test_a_baseline_ref_without_the_driver_api_is_unsupported(self):
        world = self.world()
        git(world.repo, 'tag', 'v0.50.123')
        refused = self.refuse(world)
        self.assertEqual(refused.reason, 'baseline_ref_unsupported')
        self.assertTrue(refused.detail.startswith('baseline surface at v0.50.123: build: '), refused.detail)
        self.assertIn('github.com/insajin/autopus-adk/pkg/', refused.detail)
        self.assertEqual(list(world.session.iterdir()), [])

    def test_a_baseline_ref_missing_from_the_repository_is_unsupported(self):
        world = self.world()
        refused = self.refuse(world)
        self.assertEqual(refused.reason, 'baseline_ref_unsupported')
        self.assertTrue(refused.detail.startswith('baseline surface at v0.50.123: revision: '), refused.detail)

    def test_without_surfaces_each_arm_comes_from_its_revision_with_the_candidate_pins(self):
        world, requested = self.world(), []

        def surface(repo, revision, pins, set_root, destination, scratch, proxy=None):
            requested.append((Path(repo), revision, pins, Path(set_root)))
            (destination / '.codex').mkdir(parents=True)
            (destination / 'AGENTS.md').write_text('surface of ' + revision + '\n')
            return destination

        def prepare(*_args):
            raise prepare_grader.PrepareError('stop once the protocol is frozen')
        steps = golden.Steps(platform='darwin', set_digests=fake_digests, surface=surface, prepare=prepare)
        self.assertEqual(self.refuse(world, steps).reason, 'oracle_calibration_failed')
        pins = world.manifest['pins']
        self.assertEqual(requested, [(world.repo, 'v0.50.123', pins, world.root), (world.repo, 'HEAD', pins, world.root)])
        protocol = json.loads((world.session / 'protocol.json').read_text())
        for arm, revision in (('baseline', 'v0.50.123'), ('candidate', 'HEAD')):
            with tempfile.TemporaryDirectory() as directory:
                (Path(directory) / '.codex').mkdir()
                (Path(directory) / 'AGENTS.md').write_text('surface of ' + revision + '\n')
                self.assertEqual(protocol[arm + '_surface_digest'], gp.surface_digest(Path(directory)), arm)

    def test_a_candidate_ref_the_driver_cannot_serve_is_invalid(self):
        def surface(repo, revision, pins, set_root, destination, scratch, proxy=None):
            if revision != 'v0.50.123':
                raise gs.SurfaceError('generate', 'adapter failed')
            destination.mkdir(parents=True)
            return destination
        steps = golden.Steps(platform='darwin', set_digests=fake_digests, surface=surface)
        refused = self.refuse(self.world(), steps, '--candidate-ref', 'main')
        self.assertEqual((refused.reason, refused.detail), ('invalid', 'candidate surface at main: generate: adapter failed'))


@unittest.skipUnless(SANDBOXED and resolves('v0.50.122'), 'needs macOS sandbox-exec, go and the v0.50.122 tag')
class BootstrapTests(unittest.TestCase):
    def test_v0_50_122_builds_the_driver_and_writes_every_platform_with_the_pinned_version(self):
        # The build is offline under grader.sb: every module comes from the local module cache through the
        # trusted download, and the build writes only its build root.
        with tempfile.TemporaryDirectory() as directory:
            base, digests = Path(directory).resolve(), []
            self.addCleanup(prepare_grader.remove_tree, base)
            for name in ('first', 'second'):
                driver = base / name / 'driver'
                surface = gs.arm_surface(CHECKOUT, 'v0.50.122', PINS, CHECKOUT, base / name / 'surface', driver)
                for entry in ENTRIES:
                    self.assertTrue((surface / entry).exists(), entry)
                version = json.loads((surface / PLUGIN).read_text())['version']
                self.assertRegex(version, r'^0\.50\.123\+codex\.harness-golden\.[0-9a-f]{12}$')
                self.assertIn('project_name: harness-golden\n', (surface / 'autopus.yaml').read_text())
                self.assertFalse((driver / 'build' / 'src').exists(), 'the extracted revision is removed')
                self.assertFalse((driver / 'modcache').exists(), 'the session module cache is removed')
                self.assertTrue(any((driver / 'build' / 'gocache').iterdir()), 'the build cache is the session one')
                digests.append(gp.surface_digest(surface))
        self.assertRegex(digests[0], r'^[0-9a-f]{64}$')
        self.assertEqual(digests[0], digests[1], 'two builds in different paths write one surface')


@unittest.skipIf(SANDBOXED, 'BootstrapTests builds the driver at v0.50.122 under grader.sb')
@unittest.skipUnless(shutil.which('go') and resolves('v0.50.122'), 'needs go and the v0.50.122 tag')
class DriverSourceTests(unittest.TestCase):
    def test_the_driver_source_builds_at_v0_50_122(self):
        # Without sandbox-exec no arm surface can be built, yet the driver source must keep compiling against
        # the oldest supported revision; a plain build keeps that guard where BootstrapTests cannot run.
        with tempfile.TemporaryDirectory() as directory:
            tree = Path(directory) / 'src'
            workspace.snapshot(CHECKOUT, 'v0.50.122', tree)
            package = tree / gs.DRIVER_PACKAGE
            shutil.rmtree(package, ignore_errors=True)
            package.mkdir(parents=True)
            shutil.copyfile(gs.DRIVER, package / 'main.go')
            env = {**os.environ, 'GOWORK': 'off', 'GOFLAGS': '-mod=readonly -buildvcs=false', 'CGO_ENABLED': '0'}
            completed = subprocess.run(gs.build_command('v0.50.123', Path(directory) / 'driver'), cwd=tree, env=env,
                                       capture_output=True, text=True, timeout=1800)
        self.assertEqual(completed.returncode, 0, completed.stderr[-2000:])


if __name__ == '__main__':
    unittest.main()
