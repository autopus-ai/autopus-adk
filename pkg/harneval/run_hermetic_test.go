package harneval

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
)

// bookkeeping lists the excluded bookkeeping files of a generated surface.
func bookkeeping(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	require.NoError(t, filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, current)
		if err == nil && isBookkeeping(filepath.ToSlash(rel)) {
			found = append(found, filepath.ToSlash(rel))
		}
		return err
	}))
	return found
}

// TestRun_S2_PinnedRun_IsByteIdenticalAcrossHomesAndTempRoots is the S2
// in-process oracle over the production pipeline (template check, pinned
// adapters, evaluator): two runs under different HOME and TMPDIR give the same
// document once produced_at is removed, with an empty sentinel log, although
// the real surface carries timestamped bookkeeping. Not parallel: it sets
// HOME and TMPDIR for the process.
func TestRun_S2_PinnedRun_IsByteIdenticalAcrossHomesAndTempRoots(t *testing.T) {
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()
	f.freshTemplates()
	var documents []string
	for range 2 {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("TMPDIR", t.TempDir())
		var excluded []string
		observe := func(generation *Generation) error {
			excluded = bookkeeping(t, generation.Surfaces[""].Root)
			return nil
		}

		result, err := Run(context.Background(), f.root, RunOptions{Mutate: observe})

		require.NoError(t, err)
		require.Equal(t, StatusPass, result.Status, "%v %v %v", result.FailureReasons, result.Details, result.Notes)
		assert.Empty(t, result.SentinelLog)
		assert.NotEmpty(t, excluded, "the real surface writes bookkeeping the digest must skip")
		assert.Regexp(t, `^[0-9a-f]{64}$`, result.SurfaceDigest)
		assert.Equal(t, oracleSetDigest, result.SetDigest)
		result.ProducedAt = ""
		data, err := EncodeResult(result)
		require.NoError(t, err)
		documents = append(documents, string(data))
	}
	assert.Equal(t, documents[0], documents[1])
}

// TestRun_S2_UnpinnedCodexProbe_IsTheOnlyReason is the S2 negative at the run
// seam: codex without its pins probes the host and the run stops there.
func TestRun_S2_UnpinnedCodexProbe_IsTheOnlyReason(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()
	unpinned := func(root string, _ Pins, _ []byte) []adapter.PlatformAdapter {
		return []adapter.PlatformAdapter{codex.NewWithRoot(root)}
	}

	result := fakeRun(t, f.root, RunOptions{Adapters: unpinned})

	assert.Equal(t, []string{ReasonHostProbeUnpinned}, result.FailureReasons)
	assert.Equal(t, []string{"codex"}, result.Details)
	assert.Contains(t, result.SentinelLog, "codex debug models")
	assert.Empty(t, result.Transitions)
	assert.Empty(t, result.SurfaceDigest)
}

func TestRun_AdapterError_IsGenerationFailedWithThePlatform(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()
	refusing := func(string, Pins, []byte) []adapter.PlatformAdapter {
		return []adapter.PlatformAdapter{refusingAdapter{}}
	}

	result := fakeRun(t, f.root, RunOptions{Adapters: refusing})

	assert.Equal(t, []string{ReasonGenerationFailed}, result.FailureReasons)
	assert.Equal(t, []string{"codex"}, result.Details)
	assert.Contains(t, strings.Join(result.Notes, "\n"), "placement refused")
}

// TestRun_MutationSeam_ActsBetweenGenerationAndAssertions: deleting the router
// after generation regresses every standard task, and the surface digest is
// taken from the surface the assertions saw.
func TestRun_MutationSeam_ActsBetweenGenerationAndAssertions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()
	removeRouter := func(generation *Generation) error {
		return os.Remove(filepath.Join(generation.Surfaces[""].Root, filepath.FromSlash(routerPath)))
	}

	clean := fakeRun(t, f.root, RunOptions{})
	mutated := fakeRun(t, f.root, RunOptions{Mutate: removeRouter})

	assert.Equal(t, []Transition{
		{TaskID: "GT-FIX-A", Kind: TransitionRegression},
		{TaskID: "GT-FIX-B", Kind: TransitionRegression},
		{TaskID: "GT-FIX-C", Kind: TransitionRegression},
	}, mutated.Transitions)
	assert.Equal(t, []string{ReasonRegression}, mutated.FailureReasons)
	requireRate(t, 0, mutated.PassRate)
	assert.InDelta(t, -1, mutated.RegressionDelta, 1e-9)
	assert.NotEqual(t, clean.SurfaceDigest, mutated.SurfaceDigest)
}

// TestRun_SeamOrDigestFailure_IsAnErrorNotAResult: when no document can be
// produced, Run returns an error instead of a result that could read as pass.
func TestRun_SeamOrDigestFailure_IsAnErrorNotAResult(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()
	broken := errors.New("mutation refused")
	options := func(mutate func(*Generation) error) RunOptions {
		return RunOptions{Adapters: fakeAdapters, StaleCheck: noStaleTemplates, Mutate: mutate}
	}

	result, err := Run(context.Background(), f.root, options(func(*Generation) error { return broken }))
	assert.Nil(t, result)
	assert.ErrorIs(t, err, broken)

	symlink := func(generation *Generation) error {
		root := generation.Surfaces[""].Root
		return os.Symlink(filepath.Join(root, filepath.FromSlash(routerPath)), filepath.Join(root, "link"))
	}
	result, err = Run(context.Background(), f.root, options(symlink))
	assert.Nil(t, result)
	assert.Error(t, err, "a non-regular generated file cannot be digested")
}

// TestRun_NoTempDirectory_IsAnError covers an infrastructure failure inside
// generation. Not parallel: it points TMPDIR at a missing directory.
func TestRun_NoTempDirectory_IsAnError(t *testing.T) {
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	result, err := Run(context.Background(), f.root, RunOptions{Adapters: fakeAdapters, StaleCheck: noStaleTemplates})

	assert.Nil(t, result)
	assert.Error(t, err)
}
