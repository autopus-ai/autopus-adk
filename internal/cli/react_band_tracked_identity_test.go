package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Review round 3 (L3 residual): store_tracked judges the store by identity,
// not by spelling. On a file system that folds ſ (U+017F) to s, as APFS does,
// .autopus/metricſ/ and .autopuſ/metrics/ are the store directory itself, so
// a file git tracks there is a tracked store file. A case skips where the file
// system keeps that spelling apart from .autopus/metrics.
func TestReactBandGH_StoreTrackedMatchesAFoldVariantOfTheStore(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{".autopus/metricſ", ".autopuſ/metrics", ".AUTOPUſ/Metricſ"} {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			repo, git := bandGitRepo(t)
			bandStoreFile(t, repo, dir, healthband.CIRunsFile)
			if _, err := os.Stat(filepath.Join(repo, ".autopus", "metrics", healthband.CIRunsFile)); err != nil {
				t.Skipf("this file system keeps %s apart from .autopus/metrics", dir)
			}
			git("add", "-f", "--", dir+"/"+healthband.CIRunsFile)

			assert.True(t, newBandGHClient(execBandRunner{}).storeTracked(context.Background(), repo))
		})
	}
}

// Review round 3: inherited global pathspec switches cannot turn the check
// off. GIT_LITERAL_PATHSPECS=1 made git read the magic pathspec as a file name
// and list nothing; band drops it from the call.
func TestReactBandGH_StoreTrackedIgnoresAnInheritedLiteralPathspecSwitch(t *testing.T) {
	repo, git := bandGitRepo(t)
	bandStoreFile(t, repo, ".autopus/metrics", healthband.CIRunsFile)
	git("add", "-f", ".autopus/metrics/ci-runs.jsonl")
	t.Setenv("GIT_LITERAL_PATHSPECS", "1")

	assert.True(t, newBandGHClient(execBandRunner{}).storeTracked(context.Background(), repo))
}

// The call inherits the environment without the four global pathspec
// switches, matched in any letter case as Windows matches keys.
func TestReactBandGH_StoreTrackedDropsTheGlobalPathspecSwitches(t *testing.T) {
	t.Parallel()
	runner := &fakeBandRunner{}
	client := newBandGHClient(runner)
	client.environ = func() []string {
		return []string{"PATH=/usr/bin", "GIT_LITERAL_PATHSPECS=1", "GIT_GLOB_PATHSPECS=1", "git_noglob_pathspecs=1", "GIT_ICASE_PATHSPECS=1", "GIT_PAGER=cat"}
	}

	assert.False(t, client.storeTracked(context.Background(), t.TempDir()))
	calls := runner.recorded("git")
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"PATH=/usr/bin", "GIT_PAGER=cat"}, calls[0].env)
}

// git lists every path under any .autopu*/metric* spelling. A listed path is
// a store file when its entry at the store's depth is the store directory on
// disk, and also whenever that cannot be settled, fail-closed: a stat of
// either path fails, a record lacks two components, or the last record is
// cut short. Only an entry that exists apart from an existing store is not.
func TestReactBandGH_StoreTrackedJudgesListedPathsByIdentity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		dirs    []string // made under the project before the check
		listing string
		tracked bool
	}{
		{"another directory beside the store", []string{".autopus/metrics", ".autopus/metrics-archive"}, ".autopus/metrics-archive/notes.md\x00", false},
		{"a file deeper in the store", []string{".autopus/metrics/pending"}, ".autopus/metrics/pending/c1.json\x00", true},
		{"a listed directory missing from disk", []string{".autopus/metrics"}, ".autopus/metrics-archive/notes.md\x00", true},
		{"a store not made yet", []string{".autopus/metrics-archive"}, ".autopus/metrics-archive/notes.md\x00", true},
		{"a record without two components", []string{".autopus/metrics"}, ".autopus\x00", true},
		{"a record cut short", []string{".autopus/metrics", ".autopus/metrics-archive"}, ".autopus/metrics-archive/notes.md", true},
	} {
		project := t.TempDir()
		for _, dir := range tc.dirs {
			require.NoError(t, os.MkdirAll(filepath.Join(project, filepath.FromSlash(dir)), 0o700))
		}
		runner := &fakeBandRunner{answers: map[string]fakeBandAnswer{bandTrackedArgv: {stdout: tc.listing}}}

		assert.Equal(t, tc.tracked, newBandGHClient(runner).storeTracked(context.Background(), project), tc.name)
	}
}

// A listed directory spelled apart from the store that resolves to it is the
// store. The link stands in for a file system that folds the spelling.
func TestReactBandGH_StoreTrackedCountsAListedAliasOfTheStore(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".autopus", "metrics"), 0o700))
	if err := os.Symlink("metrics", filepath.Join(project, ".autopus", "metrics-alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	runner := &fakeBandRunner{answers: map[string]fakeBandAnswer{bandTrackedArgv: {stdout: ".autopus/metrics-alias/ci-runs.jsonl\x00"}}}

	assert.True(t, newBandGHClient(runner).storeTracked(context.Background(), project))
}

// Review round 3: the wider pathspec also lists a directory that only shares
// the prefix. It is not the store, so band still runs.
func TestReactBandGH_StoreTrackedIgnoresAnotherTrackedDirectory(t *testing.T) {
	t.Parallel()
	repo, git := bandGitRepo(t)
	bandStoreFile(t, repo, ".autopus/metrics", healthband.CIRunsFile)
	bandStoreFile(t, repo, ".autopus/metrics-archive", "notes.md")
	git("add", "-f", ".autopus/metrics-archive/notes.md")

	assert.False(t, newBandGHClient(execBandRunner{}).storeTracked(context.Background(), repo))
}

// A symlink git tracks at the store path is a tracked store as well: the
// store-level pathspec lists the link itself, and it resolves to the store.
func TestReactBandGH_StoreTrackedCountsATrackedLinkAtTheStorePath(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("git checks symlinks out as plain files without core.symlinks")
	}
	repo, git := bandGitRepo(t)
	bandStoreFile(t, repo, "planted", healthband.CIRunsFile)
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".autopus"), 0o700))
	require.NoError(t, os.Symlink("../planted", filepath.Join(repo, ".autopus", "metrics")))
	git("add", "-f", "planted/ci-runs.jsonl", ".autopus/metrics")

	assert.True(t, newBandGHClient(execBandRunner{}).storeTracked(context.Background(), repo))
}
