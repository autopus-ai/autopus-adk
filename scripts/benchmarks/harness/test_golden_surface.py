"""Per-revision arm surfaces of the golden mode (SPEC-HARNEVAL-001 REQ-HE-07, T13).

The surface driver build and run, the baseline_ref_unsupported refusal before any agent call, the
runner wiring without --surfaces, and the bootstrap: the driver builds and runs at v0.50.122.
"""
import json
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

import golden
import golden_protocol as gp
import golden_surface as gs
import prepare_grader
from test_golden_fixture import argv, build_world, calls, git
from test_golden_runner import fake_digests

HERE = Path(__file__).resolve().parent
CHECKOUT = HERE.parents[2]
PINS = json.loads((CHECKOUT / 'evals/harness/manifest.json').read_text())['pins']
# One entry file per platform (claude-code, codex, antigravity-cli, opencode, omp) and the saved config.
ENTRIES = ('.claude/settings.json', '.codex/config.toml', '.gemini/settings.json', 'opencode.json', '.omp/commands',
           'autopus.yaml')
PLUGIN = '.autopus/plugins/auto/.codex-plugin/plugin.json'


def resolves(revision: str) -> bool:
    return subprocess.run(['git', 'rev-parse', '--verify', '--quiet', revision + '^{commit}'], cwd=CHECKOUT,
                          capture_output=True).returncode == 0


class DriverBuildTests(unittest.TestCase):
    def test_the_build_is_trimmed_and_links_the_pinned_version_into_pkg_version(self):
        self.assertEqual(gs.build_command('v0.50.123', Path('/b/driver')),
                         ['go', 'build', '-trimpath',
                          '-ldflags=-X github.com/insajin/autopus-adk/pkg/version.version=v0.50.123',
                          '-o', '/b/driver', './scripts/benchmarks/harness/surface_driver'])
        with self.assertRaises(gs.SurfaceError) as caught:
            gs.build_command('v0.50.123 -X main.x=y', Path('/b/driver'))
        self.assertEqual(caught.exception.stage, 'build')
        env = gs.build_env({'PATH': '/toolchain', 'GOWORK': '/elsewhere/go.work', 'GOFLAGS': '-mod=mod'})
        self.assertEqual(env, {'PATH': '/toolchain', 'GOWORK': 'off', 'GOFLAGS': '-mod=readonly -buildvcs=false',
                               'GOTOOLCHAIN': 'local'})

    @unittest.skipUnless(shutil.which('go'), 'go builds the driver')
    def test_the_arm_driver_package_holds_only_this_checkout_driver(self):
        with tempfile.TemporaryDirectory() as directory:
            tree = Path(directory) / 'src'
            package = tree / 'scripts/benchmarks/harness/surface_driver'
            package.mkdir(parents=True)
            (tree / 'go.mod').write_text('module example.com/elsewhere\n\ngo 1.21\n')
            (package / 'main.go').write_text('package main\n\nfunc main() { panic("revision driver") }\n')
            (package / 'extra.go').write_text('package main\n')
            with self.assertRaises(gs.SurfaceError) as caught:
                gs.build_driver(tree, 'v0.50.123', Path(directory) / 'driver')
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

    @unittest.skipUnless(shutil.which('go'), 'go builds the driver')
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

        def surface(repo, revision, pins, set_root, destination, scratch):
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
        def surface(repo, revision, pins, set_root, destination, scratch):
            if revision != 'v0.50.123':
                raise gs.SurfaceError('generate', 'adapter failed')
            destination.mkdir(parents=True)
            return destination
        steps = golden.Steps(platform='darwin', set_digests=fake_digests, surface=surface)
        refused = self.refuse(self.world(), steps, '--candidate-ref', 'main')
        self.assertEqual((refused.reason, refused.detail), ('invalid', 'candidate surface at main: generate: adapter failed'))


@unittest.skipUnless(shutil.which('go') and resolves('v0.50.122'), 'needs go and the v0.50.122 tag')
class BootstrapTests(unittest.TestCase):
    def test_v0_50_122_builds_the_driver_and_writes_every_platform_with_the_pinned_version(self):
        with tempfile.TemporaryDirectory() as directory:
            base, digests = Path(directory), []
            for name in ('first', 'second'):
                surface = gs.arm_surface(CHECKOUT, 'v0.50.122', PINS, CHECKOUT, base / name / 'surface', base / name / 'driver')
                for entry in ENTRIES:
                    self.assertTrue((surface / entry).exists(), entry)
                version = json.loads((surface / PLUGIN).read_text())['version']
                self.assertRegex(version, r'^0\.50\.123\+codex\.harness-golden\.[0-9a-f]{12}$')
                self.assertIn('project_name: harness-golden\n', (surface / 'autopus.yaml').read_text())
                self.assertFalse((base / name / 'driver' / 'src').exists(), 'the extracted revision is removed')
                digests.append(gp.surface_digest(surface))
        self.assertRegex(digests[0], r'^[0-9a-f]{64}$')
        self.assertEqual(digests[0], digests[1], 'two builds in different paths write one surface')


if __name__ == '__main__':
    unittest.main()
