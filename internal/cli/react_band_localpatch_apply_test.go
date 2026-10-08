//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalPatchPatch_ConfigGainedDuringRequest_UnsafeAtStepEight(t *testing.T) {
	w := newLPPatchWorld(t, lpS13Harness())
	w.git(w.repo, "config", "filter.mark.clean", "tools/clean.py")
	objects := w.git(w.repo, "count-objects", "-v")
	result := w.run(t)
	assert.Equal(t, "failed:git_config_unsafe:filter.mark.clean", result.Status)
	assert.Equal(t, objects, w.git(w.repo, "count-objects", "-v"), "no object before the step-8 check")
	assert.NotContains(t, w.ledger.trail(), "stage:apply_intent")
	assert.Equal(t, []string{lpPatchClaimID}, w.cleaner.calls)
}

func TestLocalPatchPatch_RecordFaults_RecordUnavailable(t *testing.T) {
	for _, phase := range []string{
		lpPhaseMessage, lpPhaseApplyIntent, lpPhaseApplyDone, lpPhaseCommitDone,
		lpPhaseBranchIntent, lpPhaseBranchDone, lpPhasePatchIntent, lpPhasePatchDone,
	} {
		t.Run(phase, func(t *testing.T) {
			w := newLPPatchWorld(t, lpS13Harness())
			w.ledger.failPhase = phase
			result := w.run(t)
			assert.Equal(t, "failed:record_unavailable", result.Status)
			assert.Equal(t, []string{lpPatchClaimID}, w.cleaner.calls)
			if phase == lpPhaseApplyIntent {
				_, err := os.Lstat(filepath.Join(w.lp(), lpKey+".diff"))
				assert.True(t, os.IsNotExist(err), "no diff file without its intent record")
			}
			if phase == lpPhasePatchIntent {
				_, err := os.Lstat(filepath.Join(w.lp(), lpKey+".patch.tmp-a1b2c3d4"))
				assert.True(t, os.IsNotExist(err), "no patch file without its intent record")
			}
		})
	}
}

func TestLocalPatchPatch_BranchAndPatchFileFailures(t *testing.T) {
	w := newLPPatchWorld(t, lpS13Harness())
	w.patcher.beforeCommit = func(string) { w.git(w.repo, "branch", "autopus/band/"+lpKey) }
	assert.Equal(t, "failed:branch_failed", w.run(t).Status, "update-ref creates the branch only while it is absent")

	w = newLPPatchWorld(t, lpS13Harness())
	w.patcher.beforeCommit = func(string) {
		require.NoError(t, os.WriteFile(filepath.Join(w.lp(), lpKey+".patch"), []byte("reviewer"), 0o600))
	}
	assert.Equal(t, "failed:patch_file_failed", w.run(t).Status)
	data, err := os.ReadFile(filepath.Join(w.lp(), lpKey+".patch"))
	require.NoError(t, err)
	assert.Equal(t, "reviewer", string(data), "an existing patch file is never replaced")
}

func TestLocalPatchPrepare_WorktreeRecordFaults(t *testing.T) {
	f := newLPFixture(t, nil)
	f.ledger.failPhase = lpPhaseWorktreeDone
	s := f.patcher.prepare(t.Context(), f.target())
	assert.Equal(t, lpCodeRecordUnavailable, s.code)
	assert.Equal(t, []string{lpPatchClaimID}, f.cleaner.calls, "the worktree goes through the Cleanup Rules")
	_, err := os.Lstat(filepath.Join(f.lp(), lpKey))
	assert.True(t, os.IsNotExist(err))
	f.patcher.release(s)

	f = newLPFixture(t, nil)
	f.useGitWrapper("add128")
	f.ledger.failPhase = lpPhaseWorktreeFailed
	s = f.patcher.prepare(t.Context(), f.target())
	assert.Equal(t, lpCodeRecordUnavailable, s.code)
	f.patcher.release(s)

	f = newLPFixture(t, nil)
	require.NoError(t, os.Remove(f.lp()))
	require.NoError(t, os.WriteFile(f.lp(), nil, 0o600))
	s = f.patcher.prepare(t.Context(), f.target())
	assert.Equal(t, lpCodeCacheUnavailable, s.code, "a file in place of <lp>")
}

func TestLocalPatchPatch_GitFaults(t *testing.T) {
	cases := []struct{ part, want string }{
		{"apply --cached", "failed:patch_invalid"},
		{"apply --index", "failed:patch_invalid"},
		{" commit --no-verify", "failed:commit_failed"},
		{"cat-file commit", "failed:commit_tree_mismatch"},
		{"format-patch --stdout", "failed:patch_file_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.part, func(t *testing.T) {
			w := newLPPatchWorld(t, lpS13Harness())
			log := w.useGitWrapper("pass")
			w.failGit(tc.part)
			if !assert.Equal(t, tc.want, w.run(t).Status) {
				t.Log(strings.Join(lpGitLog(t, log), "\n"))
			}
			if tc.part != "format-patch --stdout" { // after step 10 the branch is Cleanup Rule 1's (T2)
				assert.Empty(t, w.git(w.repo, "for-each-ref", "refs/heads/autopus/band/"))
			}
		})
	}
}

func TestLocalPatchPrepare_GitFaults(t *testing.T) {
	cases := []struct{ part, want string }{
		{"config --list", "git_config_unsafe:config_unreadable"},
		{"ls-tree -r -l", "worktree_too_large"},
		{"status --porcelain", "worktree_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.part, func(t *testing.T) {
			f := newLPFixture(t, nil)
			f.useGitWrapper("pass")
			f.failGit(tc.part)
			s := f.patcher.prepare(t.Context(), f.target())
			assert.Equal(t, tc.want, s.code)
			_, err := os.Lstat(filepath.Join(f.lp(), lpKey))
			assert.True(t, os.IsNotExist(err), "no worktree is left")
			f.patcher.release(s)
		})
	}
}

func TestLocalPatchStatfs_RealFileSystem(t *testing.T) {
	t.Parallel()
	space, err := lpStatfs(t.TempDir())
	require.NoError(t, err)
	assert.Positive(t, space.unit)
	assert.Positive(t, space.avail)
	_, err = lpStatfs(filepath.Join(t.TempDir(), "absent"))
	assert.Error(t, err)
}
