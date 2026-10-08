//go:build darwin

package harneval

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runnerArmSurface is the trusted runner's own rebuild of the same arm:
// golden_surface.arm_surface and golden_protocol.surface_digest.
const runnerArmSurface = `import json, sys, tempfile
from pathlib import Path
import golden_protocol, golden_surface
root, commit = Path(sys.argv[1]), sys.argv[2]
pins = json.loads((root / 'evals/harness/manifest.json').read_text())['pins']
with tempfile.TemporaryDirectory() as scratch:
    surface = golden_surface.arm_surface(root, commit, pins, root, Path(scratch) / 'surface', Path(scratch) / 'driver')
    print(golden_protocol.surface_digest(surface))
`

// TestArmSurfaceDigest_BaselineTagMatchesTheTrustedRunner: the signer's
// rebuild of the committed baseline_ref arm gives the digest the trusted
// runner writes into the protocol as baseline_surface_digest, so the
// field-by-field protocol comparison holds for an honest session.
func TestArmSurfaceDigest_BaselineTagMatchesTheTrustedRunner(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the surface driver at the baseline tag twice")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	set, err := LoadSet(root)
	require.NoError(t, err)
	ctx := context.Background()
	ref := set.Manifest.Live.BaselineRef
	commit, err := GitTagCommit(ctx, root, ref, os.Environ())
	if err != nil {
		t.Skipf("baseline tag %s is not present in this clone: %v", ref, err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not on PATH, so the trusted runner's digest cannot be computed")
	}

	got, err := ArmSurfaceDigest(ctx, root, commit, set.Manifest.Pins, ArmSurfaceOptions{Env: os.Environ()})
	require.NoError(t, err)

	cmd := exec.CommandContext(ctx, python, "-c", runnerArmSurface, root, commit)
	cmd.Dir = filepath.Join(root, "scripts", "benchmarks", "harness")
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	want, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(string(want)), got, "baseline %s at %s", ref, commit)
	assert.Regexp(t, `^[0-9a-f]{64}$`, got)
	t.Logf("baseline %s at %s: surface digest %s", ref, commit, got)
}

// TestArmSurfaceDigest_RefusesBeforeAnyProcess: a commit, generator
// version, or catalog path the runner would refuse stops the rebuild first.
func TestArmSurfaceDigest_RefusesBeforeAnyProcess(t *testing.T) {
	t.Parallel()
	pins := Pins{GeneratorVersion: "v0.50.123", ProjectName: "p", CodexCLIVersion: "c", OpencodeCLIVersion: "o"}
	for name, tc := range map[string]struct {
		commit string
		edit   func(*Pins)
		want   string
	}{
		"tag name instead of a commit": {"v0.50.123", func(*Pins) {}, "is not a 40-hex commit"},
		"version with a space":         {strings.Repeat("a", 40), func(p *Pins) { p.GeneratorVersion = "v1 -X x=y" }, "cannot be linked"},
		"catalog outside the tree":     {strings.Repeat("a", 40), func(p *Pins) { p.CodexModelCatalog = "../models.json" }, "not a clean relative path"},
	} {
		pinned := pins
		tc.edit(&pinned)

		_, err := ArmSurfaceDigest(context.Background(), t.TempDir(), tc.commit, pinned, ArmSurfaceOptions{})

		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.want, name)
	}
}
