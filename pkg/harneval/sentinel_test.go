package harneval

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Not parallel: it reads PATH and HOME outside the sentinel lock, and Go never
// runs a sequential top-level test alongside a parallel one.
func TestWithSentinel_RecordsHostInvocationsAndRestoresEnvironment(t *testing.T) {
	pathBefore, homeBefore := os.Getenv("PATH"), os.Getenv("HOME")
	var homeEntries []os.DirEntry

	invocations, err := withSentinel(func() error {
		entries, err := os.ReadDir(os.Getenv("HOME"))
		homeEntries = entries
		require.NoError(t, err)
		// A probe that reaches a host CLI hits the recording stand-in and fails.
		assert.Error(t, exec.Command("codex", "debug", "models").Run())
		assert.Error(t, exec.Command("opencode", "--version").Run())
		_, lookErr := exec.LookPath("ls")
		assert.Error(t, lookErr, "PATH holds only the sentinels")
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"codex debug models", "opencode --version"}, invocations)
	assert.Empty(t, homeEntries, "HOME is an empty directory during generation")
	assert.Equal(t, pathBefore, os.Getenv("PATH"))
	assert.Equal(t, homeBefore, os.Getenv("HOME"))
	assert.Equal(t, []string{"codex", "opencode"}, invokedBinaries(invocations))
}

func TestWithSentinel_EveryProbedBinaryHasAStandIn(t *testing.T) {
	t.Parallel()
	names := []string{"codex", "opencode", "claude", "agy", "gemini", "omp", "git"}
	assert.Equal(t, names, sentinelBinaries)

	invocations, err := withSentinel(func() error {
		for _, name := range names {
			_ = exec.Command(name, "--version").Run()
		}
		return nil
	})

	require.NoError(t, err)
	assert.Len(t, invocations, len(names))
	assert.Equal(t, []string{"agy", "claude", "codex", "gemini", "git", "omp", "opencode"}, invokedBinaries(invocations))
}

func TestWithSentinel_QuietRunHasNoInvocationsAndKeepsError(t *testing.T) {
	t.Parallel()
	invocations, err := withSentinel(func() error { return os.ErrPermission })
	require.ErrorIs(t, err, os.ErrPermission)
	assert.Empty(t, invocations)
}
