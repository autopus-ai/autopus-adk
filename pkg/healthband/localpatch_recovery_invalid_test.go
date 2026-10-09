//go:build unix

package healthband

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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

// Only the hanging call has a short timeout. Under load a git call that
// succeeds can take seconds, and a 2 s GitTimeout on every call stopped the
// repository resolution instead (recovery_skipped, no result) or a
// configuration read (kept as git_config_unsafe). Here every other call,
// the claim, and the step itself have budgets that no load reaches, so the
// one call that can end at a timeout is the hanging git status.
func TestRecoverLocalPatches_GitCallPastItsTimeout_KeepsTheWorktree(t *testing.T) {
	t.Parallel()
	const (
		statusTimeout = time.Second
		stopGrace     = 100 * time.Millisecond
		hangFor       = 5 * time.Minute // the hanging call's natural end
	)
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	w.checkedOut()
	// git status in the band worktree hangs; every other command runs real
	// git, which the trace records.
	real, err := exec.LookPath("git")
	require.NoError(t, err)
	hang := w.f.script("git-hang", "case \" $* \" in *\" status --porcelain \"*) exec sleep "+
		strconv.Itoa(int(hangFor/time.Second))+";; esac\nexec '"+real+"' \"$@\"\n")
	opts := RecoveryOptions{Git: w.git, CacheDir: w.cache, Now: func() time.Time { return lpLease.Add(time.Second) }}
	opts.Git.Binary, opts.Git.StopGrace = hang, stopGrace
	opts.GitTimeout, opts.ClaimBudget = 2*time.Minute, 10*time.Minute
	opts.gitCallTimeout = func(args []string) time.Duration {
		if args[0] == "status" {
			return statusTimeout
		}
		return 0 // GitTimeout
	}
	start := w.mark()
	began := time.Now()
	// The world's one-minute context also covers its setup, so the step
	// runs on the test's own context.
	report, err := w.store.RecoverLocalPatches(t.Context(), opts)
	elapsed := time.Since(began)
	require.NoError(t, err)

	require.Len(t, report.Results, 1, "reasons %v", report.Reasons)
	assert.Equal(t, ClaimFailedPrefix+LocalPatchCodeInterrupted, report.Results[0].Status)
	assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeIncomplete}}, report.Results[0].Kept)
	// ls-files -s is the last call before git status, which never reached
	// real git: the stopped call is the hanging one.
	assert.Equal(t, 1, w.count(start, "ls-files -s -z"), "recovery reached git status")
	assert.Zero(t, w.count(start, "status"), "git status ran only as the hang")
	// Wall-clock bounds that hold on any machine: the step lasts at least
	// until the status timeout's SIGTERM, which load can only delay, and a
	// status that ran to its natural end would have exited 0 with a clean
	// worktree that Cleanup Rule 3 removes.
	assert.GreaterOrEqual(t, elapsed, statusTimeout-stopGrace, "git status held until its timeout")
	assert.Less(t, elapsed, hangFor, "git status is stopped at its timeout, not waited out")
	assert.True(t, w.admin())
	_, err = os.Lstat(w.paths.Worktree)
	assert.NoError(t, err)
}
