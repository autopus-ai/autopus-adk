package cli

// SPEC-PANERM-001 S7 and RFP-2: binary O is `auto` v0.50.123 built from
// c447badc (spec.md Outcome Boundary). Since W2 the in-tree loader no longer
// carries O's schema, so "binary O loads every written file" runs O itself.
// The helpers are exported for the cli_test oracles of the same package.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

const panermCommitO = "c447badc28e393b19984d2eeea159a81d609acd9"

// BuildPanermBinaryO returns the path of binary O. AUTOPUS_PANERM_O_BIN names
// a prebuilt one; otherwise O is built from this repository's git object store
// with the module cache only (go.mod and go.sum equal those of c447badc).
// Without git or the commit (a shallow clone) it logs why and returns "", and
// the O steps skip once the assertions before them have run.
func BuildPanermBinaryO(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("AUTOPUS_PANERM_O_BIN"); bin != "" {
		return bin
	}
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repo := filepath.Join(filepath.Dir(file), "..", "..")
	if out, err := exec.Command("git", "-C", repo, "cat-file", "-e", panermCommitO+"^{commit}").CombinedOutput(); err != nil {
		t.Logf("binary O needs commit %s in the local git object store: %v %s", panermCommitO[:8], err, out)
		return ""
	}
	archive := filepath.Join(t.TempDir(), "o.tar")
	src, bin := t.TempDir(), filepath.Join(t.TempDir(), "auto-o")
	runPanermStep(t, repo, nil, "git", "archive", "--format=tar", "-o", archive, panermCommitO)
	runPanermStep(t, src, nil, "tar", "-xf", archive)
	runPanermStep(t, src, []string{"GOPROXY=off", "GOWORK=off", "GOFLAGS=-mod=readonly"},
		"go", "build", "-trimpath", "-o", bin, "./cmd/auto")
	return bin
}

func runPanermStep(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v:\n%s", name, args, out)
}

// RequirePanermOLoads asserts that binary O loads each file as autopus.yaml
// with exit status 0. `auto check --arch --quiet` decodes the file strictly
// with O's schema and exits 1 on any key O does not declare; the file is
// copied alone into a fresh directory, so nothing else decides the status.
func RequirePanermOLoads(t *testing.T, bin string, files ...string) {
	t.Helper()
	skipWithoutPanermO(t, bin)
	for _, file := range files {
		out, err := panermOLoad(t, bin, file)
		require.NoError(t, err, "binary O must load %s:\n%s", file, out)
	}
}

func skipWithoutPanermO(t *testing.T, bin string) {
	t.Helper()
	if bin == "" {
		t.Skip("binary O is unavailable (see the BuildPanermBinaryO log); the steps before this one ran")
	}
}

func panermOLoad(t *testing.T, bin, file string) (string, error) {
	t.Helper()
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), data, 0o644))
	cmd := exec.Command(bin, "check", "--arch", "--quiet")
	cmd.Dir = dir
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "TMPDIR=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// requirePanermORejectsATypo pins the probe as a discriminator: O rejects
// C3's typo with its strict-decode error, so an exit status 0 means a load.
func requirePanermORejectsATypo(t *testing.T, bin string) {
	t.Helper()
	skipWithoutPanermO(t, bin)
	out, err := panermOLoad(t, bin, filepath.Join(legacyPaneFixtures, "c3.yaml"))
	require.Error(t, err, "binary O must reject C3")
	require.Contains(t, out, "field pane_argz not found in type config.ProviderEntry")
}
