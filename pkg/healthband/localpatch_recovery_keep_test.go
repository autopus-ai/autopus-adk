//go:build unix

package healthband

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-SIGMABAND-002 S9: worktrees that recovery cannot prove to be band's
// own and unchanged are kept, listed in kept[], and left byte-identical.

// keptAfterRecovery recovers the claim and returns its kept[] after
// checking the interrupted state and that the worktree is unchanged.
func (w *lpWorld) keptAfterRecovery() []LocalPatchKept {
	w.t.Helper()
	before := lpTreeHashes(w.t, w.paths.KeyDir)
	result := w.recovered()
	assert.Equal(w.t, ClaimFailedPrefix+LocalPatchCodeInterrupted, result.Status)
	assert.Equal(w.t, before, lpTreeHashes(w.t, w.paths.KeyDir), "the kept worktree is byte-identical")
	return result.Kept
}

func TestRecoveryKeep_InterruptedAddWithItsLockedFile_IsWorktreeIncomplete(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	user := filepath.Join(w.f.root, "user-wt")
	w.f.git(w.f.setupEnv, w.dir, "worktree", "add", "-q", "--detach", user, w.base)
	w.claimed(lpLease)
	w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
	w.f.git(w.f.setupEnv, w.dir, "worktree", "add", "-q", "--no-checkout", "--detach", w.paths.Worktree, w.base)
	w.f.git(w.f.setupEnv, w.dir, "worktree", "lock", "--reason", "initializing", w.paths.Worktree)
	admin, found := adminDirFor(filepath.Join(w.dir, ".git"), w.paths.Worktree)
	require.True(t, found)
	require.NoError(t, os.WriteFile(filepath.Join(admin, "index.lock"), nil, 0o600))
	locked, err := os.ReadFile(filepath.Join(admin, "locked"))
	require.NoError(t, err)
	require.Equal(t, "initializing\n", string(locked))
	start := w.mark()
	assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeIncomplete}}, w.keptAfterRecovery())
	assert.Zero(t, w.count(start, "worktree remove"))
	assert.Zero(t, w.count(start, "worktree unlock"))
	_, err = w.f.gitErr(w.f.setupEnv, w.dir, "worktree", "remove", "--force", w.paths.Worktree)
	assert.Equal(t, 128, exitCodeOf(err), "one --force refuses a locked entry")
	w.f.git(w.f.setupEnv, w.dir, "worktree", "remove", "--force", "--force", w.paths.Worktree)
	assert.False(t, w.admin())
	_, registered := findWorktree(parseWorktreeList([]byte(w.f.git(w.f.setupEnv, w.dir, "worktree", "list", "--porcelain", "-z"))), user)
	assert.True(t, registered, "another worktree of the user stays registered")
}

func TestRecoveryKeep_PartialCheckoutWithoutIndex_IsWorktreeIncomplete(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
	w.f.must(w.git.Run(w.ctx, "worktree", "add", "--no-checkout", "--detach", w.paths.Worktree, w.base))
	w.f.writeFiles(w.paths.Worktree, map[string]string{"pkg/foo/foo.go": "package foo\n"})
	assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeIncomplete}}, w.keptAfterRecovery())
	assert.True(t, w.admin())
}

func TestRecoveryKeep_HiddenChanges_AreWorktreeModified(t *testing.T) {
	t.Parallel()
	cases := map[string]func(w *lpWorld){
		"assume-unchanged bit and an edit": func(w *lpWorld) {
			w.f.git(w.f.setupEnv, w.paths.Worktree, "update-index", "--assume-unchanged", "pkg/foo/foo.go")
			require.NoError(w.t, os.WriteFile(filepath.Join(w.paths.Worktree, "pkg/foo/foo.go"), []byte("package foo // user\n"), 0o644))
		},
		"skip-worktree bit and a deletion": func(w *lpWorld) {
			w.f.git(w.f.setupEnv, w.paths.Worktree, "update-index", "--skip-worktree", "pkg/foo/bar.go")
			require.NoError(w.t, os.Remove(filepath.Join(w.paths.Worktree, "pkg/foo/bar.go")))
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newLPWorld(t, nil)
			w.claimed(lpLease)
			w.checkedOut()
			change(w)
			status := w.f.git(w.f.setupEnv, w.paths.Worktree, "status", "--porcelain", "--untracked-files=all", "--ignored")
			require.Empty(t, status, "git status shows nothing")
			assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeModified}}, w.keptAfterRecovery())
		})
	}
}

func TestRecoveryKeep_FileInsideAnUninitializedGitlink_IsWorktreeModified(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.f.git(w.f.setupEnv, w.dir, "update-index", "--add", "--cacheinfo", "160000,"+w.base+",pkg/sub")
	w.f.git(w.f.setupEnv, w.dir, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "gitlink")
	w.base = gpTrim(w.f.must(w.git.Run(w.ctx, "rev-parse", "HEAD")))
	w.claimed(lpLease)
	w.checkedOut()
	notes := filepath.Join(w.paths.Worktree, "pkg", "sub", "notes.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(notes), 0o755))
	require.NoError(t, os.WriteFile(notes, []byte("user notes\n"), 0o600))
	require.Empty(t, w.f.git(w.f.setupEnv, w.paths.Worktree, "status", "--porcelain", "--untracked-files=all", "--ignored"),
		"git status does not list a file inside the gitlink directory")
	assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeModified}}, w.keptAfterRecovery())
	data, err := os.ReadFile(notes)
	require.NoError(t, err)
	assert.Equal(t, "user notes\n", string(data))
}

func TestRecoveryKeep_UnsafeConfigurationBeforeRecovery_IsGitConfigUnsafeWithoutStatus(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	w.checkedOut()
	w.messaged()
	w.f.git(w.f.setupEnv, w.dir, "config", "filter.mark.clean", "tools/clean.py")
	start := w.mark()
	assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptGitConfigUnsafe}}, w.keptAfterRecovery())
	assert.Zero(t, w.count(start, "status"), "no command reads the worktree's files")
}

// exitCodeOf returns the exit code of a raw git error, or -1.
func exitCodeOf(err error) int {
	if exit, ok := err.(interface{ ExitCode() int }); ok {
		return exit.ExitCode()
	}
	return -1
}
