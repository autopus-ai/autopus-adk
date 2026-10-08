//go:build unix

package healthband

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-SIGMABAND-002 S9: the Recovery State Table row by row, each with a
// crash injected after one step of the Local Patch Flow.

// recovered runs recovery after the claim's lease and returns its one
// result.
func (w *lpWorld) recovered() LocalPatchRecord {
	w.t.Helper()
	w.recover(lpLease.Add(time.Second))
	results := w.results()
	require.Len(w.t, results, 1)
	assert.True(w.t, results[0].Recovered)
	return results[0]
}

func (w *lpWorld) assertExists(paths ...string) {
	w.t.Helper()
	for _, path := range paths {
		_, err := os.Lstat(path)
		assert.NoError(w.t, err, path)
	}
}

func TestRecoveryTable_CrashAfterThePatchRename_EndsDoneByThePatchIntentHash(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	w.checkedOut()
	w.messaged()
	w.intended()
	w.applied()
	w.stage(StageApplyDone, func(r *LocalPatchRecord) { r.Tree = w.expected })
	w.committed()
	w.stage(StageCommitDone, func(r *LocalPatchRecord) { r.CommitOID = w.commit })
	w.branched()
	w.stage(StageBranchDone, nil)
	w.patched()
	// format.* settings that change a fresh format-patch cannot reclassify
	// the file: band compares the patch_intent hash.
	w.f.git(w.f.setupEnv, w.dir, "config", "format.signature", "changed")
	w.f.git(w.f.setupEnv, w.dir, "config", "format.headers", "X-Changed: 1")
	start := w.mark()
	result := w.recovered()
	assert.Equal(t, ClaimDone, result.Status)
	assert.Empty(t, result.Kept)
	assert.Equal(t, w.commit, result.CommitSHA)
	assert.Equal(t, w.base, result.BaseSHA)
	assert.Equal(t, w.paths.Worktree, result.WorktreePath)
	assert.Zero(t, w.count(start, "format-patch"))
	w.assertExists(w.paths.Worktree, w.paths.Patch)
	_, err := os.Lstat(w.paths.Diff)
	assert.ErrorIs(t, err, os.ErrNotExist, "the diff file goes")
	assert.True(t, w.admin())
	assert.Equal(t, w.commit, gpTrim([]byte(w.f.git(w.f.setupEnv, w.dir, "rev-parse", w.paths.Ref))))
}

func TestRecoveryTable_CrashAfterPatchDone_KeepsAndListsAnEditedPatchFile(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.throughPatchDone()
	require.NoError(t, os.WriteFile(w.paths.Patch, []byte("reviewer notes\n"), 0o600))
	result := w.recovered()
	assert.Equal(t, ClaimDone, result.Status)
	assert.Equal(t, []LocalPatchKept{{ArtifactPatchFile, KeptPatchModified}}, result.Kept)
	data, err := os.ReadFile(w.paths.Patch)
	require.NoError(t, err)
	assert.Equal(t, "reviewer notes\n", string(data))
	w.assertExists(w.paths.Worktree)
	_, err = os.Lstat(w.paths.Diff)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRecoveryTable_CrashBeforePatchIntentEnds_RunsTheCleanupRules(t *testing.T) {
	t.Parallel()
	cases := map[string]func(w *lpWorld){
		"after git apply --index, before apply_done": func(w *lpWorld) { w.checkedOut(); w.messaged(); w.intended(); w.applied() },
		"after git commit, before commit_done": func(w *lpWorld) {
			w.checkedOut()
			w.messaged()
			w.intended()
			w.applied()
			w.stage(StageApplyDone, func(r *LocalPatchRecord) { r.Tree = w.expected })
			w.committed()
		},
		"after update-ref, before branch_done": func(w *lpWorld) {
			w.checkedOut()
			w.messaged()
			w.intended()
			w.applied()
			w.committed()
			w.stage(StageCommitDone, func(r *LocalPatchRecord) { r.CommitOID = w.commit })
			w.branched()
		},
		"right after git worktree add --no-checkout": func(w *lpWorld) {
			w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
			w.f.must(w.git.Run(w.ctx, "worktree", "add", "--no-checkout", "--detach", w.paths.Worktree, w.base))
		},
		"after worktree_intent, before any add": func(w *lpWorld) {
			w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
		},
	}
	for name, crash := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newLPWorld(t, nil)
			w.claimed(lpLease)
			crash(w)
			start := w.mark()
			result := w.recovered()
			assert.Equal(t, ClaimFailedPrefix+LocalPatchCodeInterrupted, result.Status)
			assert.Empty(t, result.Kept)
			w.assertGone()
			deletes := 0
			if w.f.git(w.f.setupEnv, w.dir, "for-each-ref", "--format=%(refname)", w.paths.Ref) == "" && w.commit != "" {
				deletes = w.count(start, "update-ref --no-deref -d "+w.paths.Ref+" "+w.commit)
			}
			assert.Equal(t, map[bool]int{true: 1, false: 0}[name == "after update-ref, before branch_done"], deletes,
				"only a branch_intent claim deletes its branch, at the claim commit")
		})
	}
}

func TestRecoveryTable_WorktreeFailedLast_EndsWithItsCode(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
	w.stage(StageWorktreeFailed, func(r *LocalPatchRecord) { r.Code = LocalPatchCodeWorktreeFailed })
	result := w.recovered()
	assert.Equal(t, ClaimFailedPrefix+LocalPatchCodeWorktreeFailed, result.Status)
	w.assertGone()
}

// A tier-2 diagnosis with the flag on has no local_patch claim: its records
// and its result are keyed by its diagnose claim id.
func TestRecoveryTable_DiagnosisOnlyCrashAfterWorktreeDone_RemovesItsWorktree(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	key := LocalPatchKey(lpSeries, lpEpisode, lpDiagnoseID)
	w.paths = w.lp.Paths(key)
	due := DueClaim{Claim: Claim{ID: lpDiagnoseID, LeaseUntil: lpT0.Add(990 * time.Second)}, Series: lpSeries, EpisodeID: lpEpisode}
	appended, err := w.store.AppendLocalPatchPrep(w.ctx, NewLocalPatchPrep(due, "", key, w.base, LocalPatchCodeOK))
	require.NoError(t, err)
	require.True(t, appended)
	stage := func(phase string, set func(*LocalPatchRecord)) {
		record := NewLocalPatchStage(lpDiagnoseID, phase)
		set(&record)
		require.NoError(t, w.store.AppendLocalPatchStage(w.ctx, record))
	}
	stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
	w.f.must(w.git.Run(w.ctx, "worktree", "add", "--no-checkout", "--detach", w.paths.Worktree, w.base))
	w.f.must(w.wt().Run(w.ctx, "reset", "--hard", "--no-recurse-submodules", "--quiet"))
	sum, err := WorktreeStatusSHA256(w.ctx, w.wt())
	require.NoError(t, err)
	stage(StageWorktreeDone, func(r *LocalPatchRecord) { r.StatusSHA256 = sum })
	assert.Empty(t, w.recover(lpT0.Add(990*time.Second)).Results, "the diagnose lease has not passed")
	report := w.recover(lpT0.Add(991 * time.Second))
	require.Len(t, report.Results, 1)
	log, err := w.store.ReadLocalPatchLog()
	require.NoError(t, err)
	result, found := log.Result(lpDiagnoseID)
	require.True(t, found)
	assert.Equal(t, ClaimFailedPrefix+LocalPatchCodeInterrupted, result.Status)
	w.assertGone()
}
