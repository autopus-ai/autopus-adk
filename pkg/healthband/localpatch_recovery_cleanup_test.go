//go:build unix

package healthband

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (w *lpWorld) cleanup() []LocalPatchKept {
	w.t.Helper()
	return CleanupLocalPatch(w.ctx, w.git, w.lp, w.records())
}

// assertGone checks that no worktree, key directory, branch, patch file, or
// diff file of the claim remains.
func (w *lpWorld) assertGone() {
	w.t.Helper()
	assert.False(w.t, w.admin(), "no admin entry")
	for _, path := range []string{w.paths.KeyDir, w.paths.Patch, w.paths.PatchTemp, w.paths.Diff} {
		_, err := os.Lstat(path)
		assert.ErrorIs(w.t, err, os.ErrNotExist, path)
	}
	_, err := w.f.gitErr(w.f.setupEnv, w.dir, "rev-parse", "--verify", "--quiet", w.paths.Ref)
	assert.Error(w.t, err, "no branch")
}

func TestCleanupLocalPatch_RefusalBeforeAnyObject_RemovesTheCleanWorktree(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpT0.Add(1800 * time.Second))
	w.checkedOut()
	w.messaged()
	start := w.mark()
	assert.Empty(t, w.cleanup())
	w.assertGone()
	assert.Equal(t, 1, w.count(start, "worktree remove --force "+w.paths.Worktree))
	assert.Zero(t, w.count(start, "update-ref"), "no branch intent, so rule 1 runs nothing")
}

func TestCleanupLocalPatch_EOLWorktree_IsRemovedByItsStatusHashUnlessEdited(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]bool{"fresh": false, "edited": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newLPWorld(t, nil)
			// The CRLF blob is committed before the attribute that makes a
			// fresh checkout of it show as modified.
			// Each step stages only its own file: a racily clean w.go would
			// otherwise be renormalized by the second add.
			for _, file := range [][2]string{{"pkg/foo/w.go", "package foo\r\n"}, {".gitattributes", "*.go text eol=lf\n"}} {
				w.f.writeFiles(w.dir, map[string]string{file[0]: file[1]})
				w.f.git(w.f.setupEnv, w.dir, "add", "--", file[0])
				w.f.git(w.f.setupEnv, w.dir, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "eol")
			}
			w.base = gpTrim(w.f.must(w.git.Run(w.ctx, "rev-parse", "HEAD")))
			w.claimed(lpT0.Add(1800 * time.Second))
			w.checkout()
			// A stat change makes git hash the file again, as it does once the
			// checkout is no longer racily clean.
			later := time.Now().Add(time.Hour)
			require.NoError(t, os.Chtimes(filepath.Join(w.paths.Worktree, "pkg/foo/w.go"), later, later))
			w.worktreeDone()
			status := w.f.must(w.wt().Run(w.ctx, "status", "--porcelain", "-z", "--untracked-files=all", "--ignored"))
			require.Equal(t, " M pkg/foo/w.go\x00", string(status), "a fresh band worktree shows the eol change")
			if edit {
				require.NoError(t, os.WriteFile(filepath.Join(w.paths.Worktree, "pkg/foo/w.go"), []byte("package foo // edited\n"), 0o644))
				assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeModified}}, w.cleanup())
				assert.True(t, w.admin())
				return
			}
			assert.Empty(t, w.cleanup())
			w.assertGone()
		})
	}
}

func TestCleanupLocalPatch_NoCheckoutWorktree_IsRemovedEvenUnderAnUnsafeInclude(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpT0.Add(1800 * time.Second))
	w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
	w.f.must(w.git.Run(w.ctx, "worktree", "add", "--no-checkout", "--detach", w.paths.Worktree, w.base))
	w.f.git(w.f.setupEnv, w.dir, "config", "filter.mark.smudge", "tools/smudge.sh")
	w.stage(StageWorktreeFailed, func(r *LocalPatchRecord) { r.Code = "git_config_unsafe:filter.mark.smudge" })
	start := w.mark()
	assert.Empty(t, w.cleanup())
	w.assertGone()
	assert.Zero(t, w.count(start, "status"), "a worktree that holds only its .git file is never read")
}

func TestCleanupLocalPatch_AfterApplyIndex_IndexTreeAtTheExpectedTree_RemovesIt(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpT0.Add(1800 * time.Second))
	w.checkedOut()
	w.messaged()
	w.intended()
	w.applied()
	assert.Empty(t, w.cleanup())
	w.assertGone()
}

func TestCleanupLocalPatch_AfterCommitBeforeCommitDone_FindsTheClaimCommit(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpT0.Add(1800 * time.Second))
	w.checkedOut()
	w.messaged()
	w.intended()
	w.applied()
	w.stage(StageApplyDone, func(r *LocalPatchRecord) { r.Tree = w.expected })
	w.committed()
	assert.Empty(t, w.cleanup())
	w.assertGone()
}

func TestCleanupLocalPatch_BranchAtTheClaimCommit_IsDeletedWithUpdateRefD(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.throughPatchDone()
	start := w.mark()
	assert.Empty(t, w.cleanup())
	w.assertGone()
	assert.Equal(t, 1, w.count(start, "update-ref --no-deref -d "+w.paths.Ref+" "+w.commit))
}

func TestCleanupLocalPatch_UserChangedArtifacts_AreKeptWithTheirReasons(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		change func(w *lpWorld)
		want   []LocalPatchKept
	}{
		"branch moved": {func(w *lpWorld) { w.f.git(w.f.setupEnv, w.dir, "update-ref", w.paths.Ref, w.base) },
			[]LocalPatchKept{{ArtifactBranch, KeptBranchMoved}}},
		"symbolic ref at the band name": {func(w *lpWorld) {
			w.f.git(w.f.setupEnv, w.dir, "update-ref", "refs/heads/user", w.commit)
			w.f.git(w.f.setupEnv, w.dir, "update-ref", "-d", w.paths.Ref)
			w.f.git(w.f.setupEnv, w.dir, "symbolic-ref", w.paths.Ref, "refs/heads/user")
		}, []LocalPatchKept{{ArtifactBranch, KeptBranchMoved}}},
		"edited patch file": {func(w *lpWorld) { require.NoError(w.t, os.WriteFile(w.paths.Patch, []byte("edited"), 0o600)) },
			[]LocalPatchKept{{ArtifactPatchFile, KeptPatchModified}}},
		"edited diff file": {func(w *lpWorld) { require.NoError(w.t, os.WriteFile(w.paths.Diff, []byte("edited"), 0o600)) },
			[]LocalPatchKept{{ArtifactDiffFile, KeptPatchModified}}},
		"staged user file": {func(w *lpWorld) {
			require.NoError(w.t, os.WriteFile(filepath.Join(w.paths.Worktree, "pkg/foo/bar.go"), []byte("package foo\n"), 0o644))
			w.f.git(w.f.setupEnv, w.paths.Worktree, "add", "pkg/foo/bar.go")
		}, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeModified}}},
		"ignored secret.env": {func(w *lpWorld) {
			require.NoError(w.t, os.WriteFile(filepath.Join(w.paths.Worktree, "secret.env"), []byte("TOKEN=1"), 0o600))
		}, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeModified}}},
		"HEAD elsewhere": {func(w *lpWorld) {
			w.f.git(w.f.setupEnv, w.paths.Worktree, "-c", "core.hooksPath=/dev/null", "commit", "-q", "--allow-empty", "-m", "user")
		}, []LocalPatchKept{{ArtifactWorktree, KeptHeadUnrecognized}}},
		"commit amended to another tree": {func(w *lpWorld) {
			require.NoError(w.t, os.WriteFile(filepath.Join(w.paths.Worktree, "pkg/foo/bar.go"), []byte("package foo\n"), 0o644))
			w.f.git(w.f.setupEnv, w.paths.Worktree, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-a", "--amend", "--no-edit")
		}, []LocalPatchKept{{ArtifactWorktree, KeptHeadUnrecognized}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newLPWorld(t, nil)
			w.throughPatchDone()
			tc.change(w)
			before := lpTreeHashes(t, w.paths.Worktree)
			assert.Equal(t, tc.want, w.cleanup())
			if tc.want[0].Artifact == ArtifactWorktree {
				assert.True(t, w.admin())
				assert.Equal(t, before, lpTreeHashes(t, w.paths.Worktree), "a kept worktree stays byte-identical")
			}
			if user, err := w.f.gitErr(w.f.setupEnv, w.dir, "rev-parse", "--verify", "--quiet", "refs/heads/user"); err == nil {
				assert.Equal(t, w.commit, gpTrim([]byte(user)), "the user branch behind a symbolic ref stays")
			}
		})
	}
}
