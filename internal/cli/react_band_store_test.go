package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S13 (CLI part): another holder of the store lock past the wait yields
// store_locked, exit 0, no evaluation, and no file change under .autopus/.
func TestReactBand_StoreLockedExitsZeroWithoutChanges(t *testing.T) {
	t.Parallel()
	p := newBandFixtureProject(t)
	holder, err := filelock.Acquire(context.Background(), p.metrics(healthband.LockFile), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, holder.Unlock()) })
	before := bandTreeHashes(t, p.dir)

	run := p.run(emptyRunList(), nil, "--format", "json")

	require.NoError(t, run.err)
	envelope := decodeBandEnvelope(t, run.stdout)
	assert.Equal(t, []string{"store_locked"}, envelope.Data.Reasons)
	assert.Empty(t, envelope.Checks)
	assert.Equal(t, before, bandTreeHashes(t, p.dir))

	text := p.run(emptyRunList(), nil)
	require.NoError(t, text.err)
	assert.Contains(t, text.stdout, "reason: store_locked\n")
}

// A store band cannot use, such as a symlinked .autopus/metrics, ends no
// evaluation: band writes nothing through the link, still prints what it
// learned, and exits non-zero; the JSON envelope carries the error.
func TestReactBand_UnusableStoreExitsNonZeroAfterTheReport(t *testing.T) {
	t.Parallel()
	p := newBandCmdProject(t)
	elsewhere := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(p.dir, ".autopus"), 0o700))
	if err := os.Symlink(elsewhere, filepath.Join(p.dir, ".autopus", "metrics")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	run := p.run(emptyRunList(), nil, "--format", "json")

	require.Error(t, run.err)
	assert.True(t, isJSONFatalError(run.err), "the envelope already reported the error")
	var envelope struct {
		Status string            `json:"status"`
		Error  *jsonErrorPayload `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(run.stdout), &envelope), run.stdout)
	assert.Equal(t, "error", envelope.Status)
	require.NotNil(t, envelope.Error)
	assert.Equal(t, "band_failed", envelope.Error.Code)
	assert.Contains(t, run.stdout, `"series": []`, "no series is an empty list, not null")
	entries, err := os.ReadDir(elsewhere)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is created through the symlink")

	text := p.run(emptyRunList(), nil)
	require.Error(t, text.err)
	assert.Equal(t, "ci: 0 runs, 0 trusted, 0 new\n", text.stdout)
}
