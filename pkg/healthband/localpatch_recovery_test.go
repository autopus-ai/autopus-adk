//go:build unix

package healthband

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var lpLease = lpT0.Add(1800 * time.Second)

func (w *lpWorld) recover(at time.Time, edits ...func(*RecoveryOptions)) RecoveryReport {
	w.t.Helper()
	opts := RecoveryOptions{Git: w.git, CacheDir: w.cache, Now: func() time.Time { return at }}
	for _, edit := range edits {
		edit(&opts)
	}
	report, err := w.store.RecoverLocalPatches(w.ctx, opts)
	require.NoError(w.t, err)
	return report
}

// results returns every result record of the claim.
func (w *lpWorld) results() []LocalPatchRecord {
	w.t.Helper()
	var results []LocalPatchRecord
	for _, record := range w.records() {
		if record.Kind == LocalPatchKindResult {
			results = append(results, record)
		}
	}
	return results
}

func TestRecoverLocalPatches_NoLog_TakesNoLockAndDoesNothing(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	report := w.recover(lpLease.Add(time.Hour))
	assert.Empty(t, report.Reasons)
	for _, name := range []string{RecoveryLockFile, LocalPatchEventsFile} {
		_, err := os.Lstat(w.store.Path(name))
		assert.ErrorIs(t, err, os.ErrNotExist, name)
	}
}

func TestRecoverLocalPatches_LeasePassedWithoutResult_EndsInterruptedOnce(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	assert.Empty(t, w.recover(lpLease).Results, "a lease that has not passed is a live claim")
	report := w.recover(lpLease.Add(time.Second))
	require.Len(t, report.Results, 1)
	results := w.results()
	require.Len(t, results, 1)
	assert.Equal(t, ClaimFailedPrefix+LocalPatchCodeInterrupted, results[0].Status)
	assert.True(t, results[0].Recovered)
	assert.Equal(t, w.base, results[0].BaseSHA)
	assert.Empty(t, w.recover(lpLease.Add(time.Hour)).Results, "an ended claim is never retried")
	assert.Len(t, w.results(), 1)
	log, err := w.store.ReadLocalPatchLog()
	require.NoError(t, err)
	decision, ok := lpDecider(func(d *LocalPatchDecider) { d.Log = log }).Decide([]Event{lpEvent(50, "1050", 3, ActionSuppressed, lpEpisode)}, 0)
	require.True(t, ok)
	assert.Equal(t, LocalPatchSkippedAlreadyPatched, decision.Record.Reason, "the next plan does not claim the episode again")
	_, err = os.Lstat(w.paths.Lock)
	assert.ErrorIs(t, err, os.ErrNotExist, "recovery unlinks the key lock after the result")
}

func TestRecoverLocalPatches_RecoveryLockHeld_ReportsRecoveryLockedAndAppendsNothing(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	held, err := filelock.Acquire(context.Background(), w.store.Path(RecoveryLockFile), 0)
	require.NoError(t, err)
	defer func() { require.NoError(t, held.Unlock()) }()
	report := w.recover(lpLease.Add(time.Second), func(opts *RecoveryOptions) { opts.LockWait = 20 * time.Millisecond })
	assert.Equal(t, []string{ReasonRecoveryLocked}, report.Reasons)
	assert.Empty(t, w.results())
}

func TestRecoverLocalPatches_KeyLockHeldByALiveProcess_IsSkippedUntilReleased(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	live, err := w.lp.AcquireKeyLock(context.Background(), lpFixtureK)
	require.NoError(t, err)
	report := w.recover(lpLease.Add(time.Second))
	assert.Equal(t, []string{ReasonRecoveryKeyLocked}, report.Reasons)
	assert.Equal(t, []string{lpClaimID}, report.KeyLocked)
	assert.Empty(t, w.results())
	require.NoError(t, live.Release())
	report = w.recover(lpLease.Add(2 * time.Second))
	assert.Empty(t, report.Reasons)
	require.Len(t, w.results(), 1)
}

// SPEC-SIGMABAND-002 S9 (a), CD-3 M7: a recovery whose first read saw no
// result gets the key lock only after the live claim appended its done
// result and unlinked the lock; its re-read finds the result and it acts on
// nothing.
func TestRecoverLocalPatches_StaleSnapshot_ReReadFindsTheLiveResult(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.throughPatchDone()
	worktree, patch := lpTreeHashes(t, w.paths.Worktree), lpTreeHashes(t, w.lp.Path)
	start := w.mark()
	report := w.recover(lpLease.Add(time.Second), func(opts *RecoveryOptions) {
		opts.beforeKeyLock = func(claimID string) {
			done := NewLocalPatchResult(claimID, "")
			appended, err := w.store.AppendLocalPatchResult(context.Background(), done)
			require.NoError(t, err)
			require.True(t, appended)
		}
	})
	assert.Empty(t, report.Results)
	results := w.results()
	require.Len(t, results, 1)
	assert.False(t, results[0].Recovered)
	assert.Equal(t, worktree, lpTreeHashes(t, w.paths.Worktree))
	assert.Equal(t, patch, lpTreeHashes(t, w.lp.Path))
	assert.Equal(t, w.commit, gpTrim([]byte(w.f.git(w.f.setupEnv, w.dir, "rev-parse", w.paths.Ref))))
	for _, command := range []string{"worktree remove", "update-ref", "status", "symbolic-ref"} {
		assert.Zero(t, w.count(start, command), "recovery ran no %s for the ended claim", command)
	}
}

// SPEC-SIGMABAND-002 S9 (b), CD-3 M7: a live claim paused after its claim
// record and before its key lock until its lease passed resumes after
// recovery ended it: it takes the key lock, its store-locked re-read finds
// the result, and it writes no prep.
func TestRecoverLocalPatches_ClaimPausedBeforeItsKeyLock_ResumesIntoTheResult(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	appendPhaseA(t, w.store, lpDecision(), lpClaim(w.lp.LocalPatchLocation, lpLease))
	require.Len(t, w.recover(lpLease.Add(time.Second)).Results, 1)
	start := w.mark()
	lock, err := w.lp.AcquireKeyLock(w.ctx, lpFixtureK)
	require.NoError(t, err)
	appended, err := w.store.AppendLocalPatchPrep(w.ctx, lpPrep(LocalPatchCodeOK))
	require.NoError(t, err)
	assert.False(t, appended, "the re-read finds the result, so the claim stops")
	require.NoError(t, lock.Release())
	kinds := map[string]int{}
	for _, record := range w.records() {
		kinds[record.Kind]++
	}
	assert.Equal(t, map[string]int{LocalPatchKindClaim: 1, LocalPatchKindResult: 1}, kinds)
	assert.Zero(t, w.count(start, "worktree"))
}

// F4: a repository that git cannot resolve leaves the step not ready, so no
// claim is touched, and the run reports recovery_skipped, which makes the
// skip visible; a later run that resolves the repository recovers.
func TestRecoverLocalPatches_UnresolvedRepository_ReportsRecoverySkipped(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.claimed(lpLease)
	report := w.recover(lpLease.Add(time.Second), func(opts *RecoveryOptions) { opts.Git = opts.Git.In(t.TempDir()) })
	assert.Equal(t, []string{ReasonRecoverySkipped}, report.Reasons)
	assert.Empty(t, w.results())
	require.Len(t, w.recover(lpLease.Add(2*time.Second)).Results, 1, "a later run recovers")
}
