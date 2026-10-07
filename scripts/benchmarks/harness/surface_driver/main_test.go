package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
	"github.com/insajin/autopus-adk/pkg/version"
)

// repoRoot is the module root, four directories above this package.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	return root
}

// committedSet is the committed golden set; the live runner passes its pins.
func committedSet(t *testing.T) *harneval.Set {
	t.Helper()
	set, err := harneval.LoadSet(repoRoot(t))
	require.NoError(t, err)
	return set
}

// pinArgs renders pins as the runner passes them, with the generator version
// given separately because the linker, not the flag, carries it.
func pinArgs(root string, pins harneval.Pins, output, generatorVersion string) []string {
	catalog := ""
	if pins.CodexModelCatalog != "" {
		catalog = filepath.Join(root, filepath.FromSlash(pins.CodexModelCatalog))
	}
	return []string{"--output", output, "--project-name", pins.ProjectName, "--generator-version", generatorVersion,
		"--codex-model-catalog", catalog, "--codex-cli-version", pins.CodexCLIVersion,
		"--opencode-cli-version", pins.OpencodeCLIVersion}
}

// inProcessDigest is the digest of the default surface harneval.Generate
// writes in-process, under its sentinel, for the set's pins.
func inProcessDigest(t *testing.T, set *harneval.Set) string {
	t.Helper()
	generation, err := harneval.Generate(context.Background(), set, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = generation.Close() })
	digest, err := harneval.SurfaceDigest(generation.Surfaces[""].Root)
	require.NoError(t, err)
	return digest
}

// TestRun_InProcess_WritesTheHarnevalDefaultSurface: with the version this
// test binary carries as the pin, the driver writes byte for byte the default
// surface the deterministic lane generates, under an empty PATH and HOME.
func TestRun_InProcess_WritesTheHarnevalDefaultSurface(t *testing.T) {
	root, set := repoRoot(t), committedSet(t)
	set.Manifest.Pins.GeneratorVersion = version.Version()
	want := inProcessDigest(t, set)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	output := filepath.Join(t.TempDir(), "surface")
	var stdout bytes.Buffer

	require.NoError(t, run(pinArgs(root, set.Manifest.Pins, output, version.Version()), &stdout))

	got, err := harneval.SurfaceDigest(output)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	var summary map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	assert.Equal(t, map[string]any{"generator_version": version.Version(),
		"platforms": []any{"claude-code", "codex", "antigravity-cli", "opencode", "omp"}}, summary)
}

// TestBuiltDriver_LinkedVersion_WritesTheHarnevalDefaultSurface builds the
// driver the way the live runner does and runs it with only an empty PATH,
// HOME, and TMPDIR. The version linked in with -X writes the surface the
// in-process codex.WithPluginBaseVersion pin writes, so the deterministic
// lane's surface digest and the live lane's candidate digest agree. A pin
// other than the linked-in version is refused.
func TestBuiltDriver_LinkedVersion_WritesTheHarnevalDefaultSurface(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go command builds the driver")
	}
	root, set := repoRoot(t), committedSet(t)
	pins := set.Manifest.Pins
	binary := filepath.Join(t.TempDir(), "surface_driver")
	build := exec.Command(gobin, "build", "-trimpath",
		"-ldflags=-X github.com/insajin/autopus-adk/pkg/version.version="+pins.GeneratorVersion, "-o", binary, ".")
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=readonly -buildvcs=false", "GOTOOLCHAIN=local")
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))
	want := inProcessDigest(t, set)
	isolated := []string{"PATH=" + t.TempDir(), "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir()}
	output := filepath.Join(t.TempDir(), "surface")

	driver := exec.Command(binary, pinArgs(root, pins, output, pins.GeneratorVersion)...)
	driver.Env = isolated
	out, err = driver.CombinedOutput()

	require.NoError(t, err, string(out))
	got, err := harneval.SurfaceDigest(output)
	require.NoError(t, err)
	assert.Equal(t, want, got, "the -X pin and the in-process WithPluginBaseVersion pin write one surface")
	other := filepath.Join(t.TempDir(), "other")
	refused := exec.Command(binary, pinArgs(root, pins, other, "v9.9.9")...)
	refused.Env = isolated
	out, err = refused.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(out), `linked-in version "`+pins.GeneratorVersion+`" is not the pinned generator version "v9.9.9"`)
	assert.NoDirExists(t, other)
}

// TestRun_RefusesWithoutWritingAnything: missing pins, stray arguments, an
// unlinked version, an existing output, and an unreadable catalog all fail
// before any surface file is written.
func TestRun_RefusesWithoutWritingAnything(t *testing.T) {
	t.Parallel()
	pins := harneval.Pins{GeneratorVersion: version.Version(), ProjectName: "p", CodexCLIVersion: "codex-cli 0.160.0",
		OpencodeCLIVersion: "1.18.7"}
	valid := func(output string) []string { return pinArgs("", pins, output, version.Version()) }
	tests := []struct {
		name, fragment string
		args           func(output string) []string
	}{
		{"no pins", "are required", func(output string) []string { return []string{"--output", output} }},
		{"stray argument", `unexpected argument "extra"`, func(output string) []string { return append(valid(output), "extra") }},
		{"unknown flag", "flag provided but not defined: -model", func(output string) []string { return append(valid(output), "--model", "m") }},
		{"unlinked version", `is not the pinned generator version "v0.0.0-unlinked"`, func(output string) []string {
			return pinArgs("", pins, output, "v0.0.0-unlinked")
		}},
		{"unreadable catalog", "codex model catalog:", func(output string) []string {
			return append(valid(output), "--codex-model-catalog", filepath.Join(filepath.Dir(output), "missing.json"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			output := filepath.Join(t.TempDir(), "surface")

			err := run(tt.args(output), io.Discard)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.fragment)
			assert.NoDirExists(t, output)
		})
	}
	existing := t.TempDir()
	err := run(valid(existing), io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output:")
	entries, readErr := os.ReadDir(existing)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "an existing output directory is left untouched")
}
