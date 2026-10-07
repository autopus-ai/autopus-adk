package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandTrackedArgv is the read-only check that the store is not committed.
const bandTrackedArgv = "git ls-files -z -- .autopus/metrics"

// Security L3: a metric store that git tracks came with the repository, not
// from this machine's runs, so band refuses to read or write it: the run
// fails closed with reason store_tracked, takes no lock, changes no file,
// and runs no gh, with or without --dry-run.
func TestReactBand_RefusesATrackedMetricsStore(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--no-fetch"}, {"--dry-run"}, nil} {
		p := newBandCmdProject(t)
		p.store(healthband.CIRunsFile, bandO2CI())
		before := bandTreeHashes(t, p.dir)
		runner := emptyRunList()
		runner.answers[bandTrackedArgv] = fakeBandAnswer{stdout: ".autopus/metrics/ci-runs.jsonl\x00"}

		run := p.run(runner, nil, append(args, "--format", "json")...)

		require.Error(t, run.err, args)
		envelope := decodeBandEnvelope(t, run.stdout)
		assert.Equal(t, "error", envelope.Status, args)
		assert.Contains(t, envelope.Data.Reasons, healthband.ReasonStoreTracked, args)
		assert.Equal(t, before, bandTreeHashes(t, p.dir), args)
		assert.NoFileExists(t, p.metrics(healthband.LockFile), args)
		assert.Equal(t, []string{bandTrackedArgv}, runner.argvs("git"), args)
		assert.Empty(t, runner.argvs("gh"), args)
	}
}

// The check asks git itself: an untracked store is fine, a store file in the
// index is tracked, and a directory outside any repository is not tracked.
func TestReactBandGH_StoreTrackedAsksGit(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	client := newBandGHClient(execBandRunner{})
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init", "-q").Run())
	store := filepath.Join(repo, ".autopus", "metrics", "ci-runs.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(store), 0o700))
	require.NoError(t, os.WriteFile(store, []byte("{}\n"), 0o600))

	assert.False(t, client.storeTracked(context.Background(), repo), "untracked")
	require.NoError(t, exec.Command("git", "-C", repo, "add", "-f", ".autopus/metrics/ci-runs.jsonl").Run())
	assert.True(t, client.storeTracked(context.Background(), repo), "in the index")
	assert.False(t, client.storeTracked(context.Background(), t.TempDir()), "no repository")
}
