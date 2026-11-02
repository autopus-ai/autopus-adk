package cli

// Fixtures of the SPEC-REVIEWREUSE-001 reuse tests: a review project inside a
// temporary git repository and a readiness-probe counter.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// runFixtureGit runs git in root with a fixed identity and neither hooks nor
// signing, so a host configuration cannot change the fixture history.
func runFixtureGit(t *testing.T, root string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "-c", "user.name=fixture",
		"-c", "user.email=fixture@example.com"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}

func commitFixtureRepo(t *testing.T, root, message string) {
	t.Helper()
	runFixtureGit(t, root, "add", "-A")
	runFixtureGit(t, root, "commit", "-q", "--allow-empty", "-m", message)
}

// reuseFixture is budgetFixture inside a git repository whose first commit
// holds the SPEC, the configuration, and one source file.
func reuseFixture(t *testing.T, budget config.ReviewBudgetConf, edit func(*config.HarnessConfig)) (*readOnlyReviewFixture, *scriptedBudgetBackend) {
	t.Helper()
	fixture, backend := budgetFixture(t, budget, edit)
	require.NoError(t, os.MkdirAll(filepath.Join(fixture.root, "internal"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.root, "internal", "x.go"), []byte("package internal\n"), 0o644))
	runFixtureGit(t, fixture.root, "init", "-q")
	commitFixtureRepo(t, fixture.root, "fixture")
	return fixture, backend
}

// countSpecReviewReadinessProbes counts preflight calls while keeping the
// host-safe skipped result.
func countSpecReviewReadinessProbes(t *testing.T) *atomic.Int32 {
	t.Helper()
	original := reviewReadinessProbe
	t.Cleanup(func() { reviewReadinessProbe = original })
	var calls atomic.Int32
	reviewReadinessProbe = func(ctx context.Context, providers []orchestra.ProviderConfig, opts providerReadinessOptions) []providerReadinessResult {
		calls.Add(1)
		return original(ctx, providers, opts)
	}
	return &calls
}

func readFixtureFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
