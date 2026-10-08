//go:build unix

package healthband

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoveryTable_PatchDone_ListsEveryArtifactThatDiffersFromItsRecord(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.throughPatchDone()
	w.f.git(w.f.setupEnv, w.paths.Worktree, "-c", "core.hooksPath=/dev/null", "commit", "-q", "--allow-empty", "-m", "user")
	w.f.git(w.f.setupEnv, w.dir, "update-ref", w.paths.Ref, w.base)
	require.NoError(t, os.WriteFile(w.paths.Diff, []byte("edited"), 0o600))
	result := w.recovered()
	assert.Equal(t, ClaimDone, result.Status)
	assert.Equal(t, []LocalPatchKept{
		{ArtifactWorktree, KeptHeadUnrecognized}, {ArtifactBranch, KeptBranchMoved}, {ArtifactDiffFile, KeptPatchModified},
	}, result.Kept)
	w.assertExists(w.paths.Worktree, w.paths.Patch, w.paths.Diff)
}

func TestRecoveryTable_DoneResult_CarriesTheBSIDThatSIGMABAND001Recorded(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.throughPatchDone()
	claim := Claim{ID: lpDiagnoseID, Kind: ClaimKindDiagnose, Owner: lpOwner, LeaseUntil: lpT0.Add(990 * time.Second)}
	event := resultEvent(7, Result{Claim: claim, Series: lpSeries, SampleKey: "1042", EpisodeID: lpEpisode,
		ClaimOutcome: ClaimOutcome{Status: ClaimDone, BSID: "BS-BAND-001"}}, "")
	event.Claims[0].Status = ClaimDone
	data, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(w.store.Path(EventsFile), append(data, '\n'), 0o600))
	result := w.recovered()
	assert.Equal(t, ClaimDone, result.Status)
	assert.Equal(t, "BS-BAND-001", result.BSID)
}

func TestRecoveryTable_PatchPathIsADirectory_KeepsItAsPatchModified(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	w.checkedOut()
	w.messaged()
	w.intended()
	w.applied()
	w.committed()
	w.stage(StageCommitDone, func(r *LocalPatchRecord) { r.CommitOID = w.commit })
	w.branched()
	w.stage(StagePatchIntent, func(r *LocalPatchRecord) { r.Path, r.PatchSHA256 = w.paths.Patch, lpSHA256([]byte("x")) })
	require.NoError(t, os.Mkdir(w.paths.Patch, 0o700))
	result := w.recovered()
	assert.Equal(t, ClaimFailedPrefix+LocalPatchCodeInterrupted, result.Status)
	assert.Equal(t, []LocalPatchKept{{ArtifactPatchFile, KeptPatchModified}}, result.Kept)
	assert.False(t, w.admin(), "the clean worktree and the branch still go")
}

func TestRecoveryTable_PatchEditedBeforePatchDone_IsKeptWhileTheRestIsCleaned(t *testing.T) {
	t.Parallel()
	// Every step up to the patch file rename; the crash came before patch_done.
	w2 := newLPWorld(t, nil)
	w2.claimed(lpLease)
	w2.checkedOut()
	w2.messaged()
	w2.intended()
	w2.applied()
	w2.committed()
	w2.stage(StageCommitDone, func(r *LocalPatchRecord) { r.CommitOID = w2.commit })
	w2.branched()
	w2.patched()
	require.NoError(t, os.WriteFile(w2.paths.Patch, append([]byte("reviewer: "), w2.patch...), 0o600))
	result := w2.recovered()
	assert.Equal(t, ClaimFailedPrefix+LocalPatchCodeInterrupted, result.Status)
	assert.Equal(t, []LocalPatchKept{{ArtifactPatchFile, KeptPatchModified}}, result.Kept)
	assert.False(t, w2.admin())
	data, err := os.ReadFile(w2.paths.Patch)
	require.NoError(t, err)
	assert.Equal(t, append([]byte("reviewer: "), w2.patch...), data)
}

func TestCleanupLocalPatch_RelativeGitdirFile_FindsTheAdminEntry(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
	w.f.git(w.f.setupEnv, w.dir, "-c", "worktree.useRelativePaths=true", "worktree", "add", "-q", "--no-checkout", "--detach", w.paths.Worktree, w.base)
	w.f.must(w.wt().Run(w.ctx, "reset", "--hard", "--no-recurse-submodules", "--quiet"))
	w.worktreeDone()
	admin, found := adminDirFor(filepath.Join(w.dir, ".git"), w.paths.Worktree)
	require.True(t, found)
	gitdir, err := os.ReadFile(filepath.Join(admin, "gitdir"))
	require.NoError(t, err)
	require.False(t, filepath.IsAbs(string(gitdir)), "git wrote a relative gitdir")
	assert.Empty(t, w.cleanup())
	w.assertGone()
}

func TestCleanupLocalPatch_WorktreePathIsAFile_IsKeptIncomplete(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
	require.NoError(t, os.MkdirAll(w.paths.KeyDir, 0o700))
	require.NoError(t, os.WriteFile(w.paths.Worktree, []byte("not a worktree"), 0o600))
	assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeIncomplete}}, w.cleanup())
	assert.Nil(t, CleanupLocalPatch(w.ctx, w.git, w.lp, nil), "no key, no rule")
}

func TestLocalPatchDir_GitlinkClear_RefusesEverythingButAbsentOrEmpty(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	root := filepath.Join(w.paths.Worktree)
	for _, dir := range []string{"empty", "full", "real"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "full", "notes.txt"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "file"), nil, 0o600))
	require.NoError(t, os.Symlink("real", filepath.Join(root, "link")))
	want := map[string]bool{"absent": true, "empty": true, "missing/sub": true, "full": false, "file": false, "link": false,
		"link/sub": false, "../escape": false, "": false, "file/sub": false}
	for path, clear := range want {
		assert.Equal(t, clear, w.lp.gitlinkClear(lpFixtureK, path), path)
	}
}

func TestResolveLocalPatchLocation_OutsideARepositoryOrCacheVariants(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	outside := w.git.In(w.f.root)
	_, err := ResolveLocalPatchLocation(w.ctx, outside, w.cache)
	assert.ErrorIs(t, err, errLocalPatchLocation)
	_, err = WorktreeStatusSHA256(w.ctx, w.git.In(filepath.Join(w.f.root, "missing")))
	assert.Error(t, err)
	loc, err := ResolveLocalPatchLocation(w.ctx, w.git, "")
	require.NoError(t, err)
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	assert.Equal(t, localPatchDirOf(realPathOf(cache), loc.RepoHash), loc.Path)
}

func TestRecoverLocalPatches_SymlinkedMetricsDirectory_IsAnError(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	metrics := w.store.Dir()
	moved := filepath.Join(w.f.root, "metrics-elsewhere")
	require.NoError(t, os.Rename(metrics, moved))
	require.NoError(t, os.Symlink(moved, metrics))
	_, err := w.store.RecoverLocalPatches(w.ctx, RecoveryOptions{Git: w.git, CacheDir: w.cache, Now: func() time.Time { return lpLease.Add(time.Second) }})
	assert.ErrorIs(t, err, errUnsafeStorePath)
	_, err = os.Lstat(filepath.Join(moved, RecoveryLockFile))
	assert.ErrorIs(t, err, os.ErrNotExist)
}
