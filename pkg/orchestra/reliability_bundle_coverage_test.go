package orchestra

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reliability artifacts must be pruned without ever deleting a run that is still
// active, and receipt persistence failures must degrade silently rather than
// break the run.

func TestPruneReliabilityArtifacts_KeepsActiveAndNewestRuns(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	old := time.Now().Add(-72 * time.Hour)

	makeRun := func(name string, mod time.Time, active bool) string {
		dir := filepath.Join(base, name)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		if active {
			require.NoError(t, os.WriteFile(filepath.Join(dir, reliabilityActiveMarkerName), []byte("now"), 0o600))
		}
		require.NoError(t, os.Chtimes(dir, mod, mod))
		return dir
	}

	stale := makeRun("stale", old, false)
	activeButOld := makeRun("active-old", old, true)
	newest := makeRun("newest", time.Now(), false)
	middle := makeRun("middle", time.Now().Add(-time.Minute), false)
	loose := filepath.Join(base, "loose.json")
	require.NoError(t, os.WriteFile(loose, []byte("{}"), 0o600))

	require.NoError(t, pruneReliabilityArtifacts(base, 1, 24*time.Hour, time.Hour))

	assert.NoDirExists(t, stale, "a run older than the max age is removed")
	assert.DirExists(t, activeButOld, "an active marker protects a run from age-based pruning")
	assert.DirExists(t, newest, "the newest run is kept")
	assert.NoDirExists(t, middle, "runs beyond the retention count are removed")
	assert.FileExists(t, loose, "non-run files are left alone")
}

func TestPruneReliabilityArtifacts_ToleratesMissingBaseAndZeroGrace(t *testing.T) {
	t.Parallel()
	base := t.TempDir()

	require.NoError(t, pruneReliabilityArtifacts(filepath.Join(base, "absent"), 2, time.Hour, time.Hour))

	dir := filepath.Join(base, "active-run")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, reliabilityActiveMarkerName), []byte("now"), 0o600))
	// With no grace window, the active marker carries no protection.
	assert.False(t, isActiveReliabilityRun(dir, time.Now(), 0))
	assert.True(t, isActiveReliabilityRun(dir, time.Now(), time.Hour))
	assert.False(t, isActiveReliabilityRun(filepath.Join(base, "absent"), time.Now(), time.Hour))
}

func TestReliabilityRuntimeRoot_FallsBackToTempWhenHomeIsUnavailable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("HOME", "")

	root, err := reliabilityRuntimeRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "autopus-runtime", "orchestra"), root)
	assert.DirExists(t, root)

	store, err := newReliabilityStore("run/1")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(store.artifactDir(), reliabilityActiveMarkerName))
	assert.NotContains(t, filepath.Base(store.artifactDir()), "/", "run IDs are sanitized into the dir name")
}

func TestNewReliabilityStore_FailsWhenNoRuntimeRootIsUsable(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))
	t.Setenv("TMPDIR", blocked)
	t.Setenv("HOME", "")

	_, err := reliabilityRuntimeRoot()
	require.ErrorContains(t, err, "create reliability root")

	store, err := newReliabilityStore("run-1")
	require.Error(t, err, "receipt storage must fail loudly when no root can be created")
	assert.Nil(t, store)
}

func TestReliabilityStore_WriteJSONDegradesInsteadOfFailingTheRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := &reliabilityStore{runID: "run-1", dir: dir}

	// A directory standing where the receipt file belongs blocks both write attempts.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "preflight-p1.json"), 0o700))
	path := store.recordPreflight(ProviderPreflightReceipt{Provider: "p1", Status: "fail"})
	assert.Empty(t, path, "an unwritable receipt yields no path")
	assert.True(t, store.degraded)

	ok := store.recordPreflight(ProviderPreflightReceipt{Provider: "p2", Status: "pass"})
	assert.NotEmpty(t, ok, "degradation of one receipt must not disable the rest")

	summary := store.summary("")
	assert.Equal(t, 1, summary.PreflightFailures, "non-pass preflight receipts are counted")
	assert.Equal(t, dir, summary.ArtifactDir)
	assert.Equal(t, "run-1", summary.RunID)
}
