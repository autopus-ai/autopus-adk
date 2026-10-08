//go:build unix

package healthband

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-SIGMABAND-002 S9: records that disagree with the Derived paths end
// failed:record_invalid and touch nothing.
func TestRecoverLocalPatches_RecordsThatDisagreeWithTheDerivation_AreRecordInvalid(t *testing.T) {
	t.Parallel()
	other := "99999999e5f60708a1b2c3d4e5f60708"
	cases := map[string]func(w *lpWorld) (claim LocalPatchRecord, cache string){
		"worktree_path names the user's checkout": func(w *lpWorld) (LocalPatchRecord, string) {
			claim := lpClaim(w.lp.LocalPatchLocation, lpLease)
			claim.WorktreePath = w.dir
			return claim, w.cache
		},
		"key ends in another claim id": func(w *lpWorld) (LocalPatchRecord, string) {
			claim := lpClaim(w.lp.LocalPatchLocation, lpLease)
			claim.Key = LocalPatchKey(lpSeries, lpEpisode, other)
			paths := w.lp.Paths(claim.Key)
			claim.WorktreePath, claim.PatchPath, claim.Branch = paths.Worktree, paths.Patch, paths.Branch
			return claim, w.cache
		},
		"user cache directory changed since the claim": func(w *lpWorld) (LocalPatchRecord, string) {
			return lpClaim(w.lp.LocalPatchLocation, lpLease), filepath.Join(w.f.root, "other-cache")
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newLPWorld(t, nil)
			claim, cache := tamper(w)
			appendPhaseA(t, w.store, lpDecision(), claim)
			w.checkout()
			checkout, band := lpCheckoutHashes(t, w.dir), lpTreeHashes(t, w.lp.Path)
			start := w.mark()
			report := w.recover(lpLease.Add(time.Second), func(opts *RecoveryOptions) { opts.CacheDir = cache })
			require.Len(t, report.Results, 1)
			result := report.Results[0]
			assert.Equal(t, ClaimFailedPrefix+LocalPatchCodeRecordInvalid, result.Status)
			assert.Empty(t, result.WorktreePath+result.PatchPath+result.Branch, "no stored or derived path is reported")
			assert.Equal(t, checkout, lpCheckoutHashes(t, w.dir), "the user's checkout is byte-identical")
			assert.Equal(t, band, lpTreeHashes(t, w.lp.Path), "every artifact is byte-identical")
			assert.True(t, w.admin())
			for _, command := range []string{"worktree remove", "update-ref", "status"} {
				assert.Zero(t, w.count(start, command), command)
			}
		})
	}
}

// SPEC-SIGMABAND-002 S9: recovery runs every git call outside the store
// lock, and a git call that outlives its timeout keeps the artifact.
func TestRecoverLocalPatches_GitCalls_RunOutsideTheStoreLock(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.throughPatchDone()
	calls, insideLock := 0, 0
	git := w.git
	git.Environ = func() []string {
		calls++
		if lock, err := filelock.Acquire(context.Background(), w.store.Path(LockFile), 0); err != nil {
			insideLock++
		} else {
			_ = lock.Unlock()
		}
		return slices.Clone(w.f.setupEnv)
	}
	w.git = git
	assert.Equal(t, ClaimDone, w.recovered().Status)
	assert.Positive(t, calls)
	assert.Zero(t, insideLock, "no git call ran while the store lock was held")
}

// Not parallel: the short timeout covers every git call, so the test runs
// without the load of the parallel tests.
func TestRecoverLocalPatches_GitCallPastItsTimeout_KeepsTheWorktree(t *testing.T) {
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	w.checkedOut()
	// git status in the band worktree hangs; every other command runs.
	real, err := exec.LookPath("git")
	require.NoError(t, err)
	hang := w.f.script("git-hang", "case \" $* \" in *\" status --porcelain \"*) exec sleep 30;; esac\nexec '"+real+"' \"$@\"\n")
	began := time.Now()
	report := w.recover(lpLease.Add(time.Second), func(opts *RecoveryOptions) {
		opts.Git.Binary, opts.Git.StopGrace, opts.GitTimeout = hang, 100*time.Millisecond, 2*time.Second
	})
	assert.Less(t, time.Since(began), 20*time.Second, "the hanging call is stopped at its timeout")
	require.Len(t, report.Results, 1)
	assert.Equal(t, ClaimFailedPrefix+LocalPatchCodeInterrupted, report.Results[0].Status)
	assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeIncomplete}}, report.Results[0].Kept)
	assert.True(t, w.admin())
	_, err = os.Lstat(w.paths.Worktree)
	assert.NoError(t, err)
}
