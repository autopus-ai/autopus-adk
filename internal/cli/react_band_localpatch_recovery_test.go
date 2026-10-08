//go:build unix

package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// SPEC-SIGMABAND-002 REQ-11 and Data Contracts Derived paths: the records
// that the live executor writes are the ones plan task T2's recovery
// derives, so a claim that the executor left without a result is recovered
// by its Recovery State Table row and never ends failed:record_invalid,
// even when the user cache directory is reached through a symlink.

// useSymlinkedCache points the user cache directory at a symlink to a real
// directory; <lp> is the real path, which git also records for a worktree.
func (f *lpFixture) useSymlinkedCache() {
	real, link := filepath.Join(f.root, "cache-real"), filepath.Join(f.root, "cache-link")
	require.NoError(f.t, os.MkdirAll(real, 0o700))
	require.NoError(f.t, os.Symlink(real, link))
	f.cacheDir = link
	f.patcher.location = f.location(link, f.patcher.git)
}

// appendPhaseARecords appends the decision and claim records of the S4
// tier-3 opening as phase A does: through T2's Decision Table, with the
// claim's paths derived from <lp>.
func (f *lpFixture) appendPhaseARecords() {
	f.t.Helper()
	target := f.target()
	decider := &healthband.LocalPatchDecider{
		Location: *f.patcher.location, Owner: bandDiagnoseOwner, NewClaimID: func() string { return lpPatchClaimID },
	}
	decision, ok := decider.Decide([]healthband.Event{target.diagnose.Event}, 0)
	require.True(f.t, ok)
	require.NotNil(f.t, decision.Claim)
	decision.Claim.LeaseUntil = target.lease
	locked, err := f.ledger.store.Lock(context.Background(), time.Second)
	require.NoError(f.t, err)
	_, err = locked.AppendLocalPatch(decision.Record, *decision.Claim)
	require.NoError(f.t, errorsJoin(err, locked.Unlock()))
}

func errorsJoin(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// recover runs T2's recovery step after the claim's lease has passed.
func (f *lpFixture) recover() healthband.RecoveryReport {
	f.t.Helper()
	after := f.target().lease.Add(time.Second)
	report, err := f.ledger.store.RecoverLocalPatches(context.Background(), healthband.RecoveryOptions{
		Git: f.patcher.git.In(f.repo), CacheDir: f.cacheDir, Now: func() time.Time { return after },
	})
	require.NoError(f.t, err)
	return report
}

func TestLocalPatchRecovery_InterruptedAfterWorktreeDone_RecoveredWithoutRecordInvalid(t *testing.T) {
	f := newLPFixture(t, nil)
	f.useSymlinkedCache()
	f.appendPhaseARecords()
	s := f.patcher.prepare(context.Background(), f.target())
	require.True(t, s.ready(), "code %q", s.code)
	f.patcher.release(s) // the process ends here: no result, the key lock is gone

	report := f.recover()
	require.Len(t, report.Results, 1, "reasons %v", report.Reasons)
	result := report.Results[0]
	assert.Equal(t, lpPatchClaimID, result.ClaimID)
	assert.Equal(t, "failed:interrupted", result.Status, "the worktree_done row, not record_invalid")
	assert.True(t, result.Recovered)
	assert.Empty(t, result.Kept, "the clean worktree is removed by Cleanup Rule 3")
	assert.True(t, f.absent(lpKey), "no <lp>/<key>/ is left")
	assert.NotContains(t, f.git(f.repo, "worktree", "list", "--porcelain"), lpKey)
	assert.Equal(t, []string{"decision", "claim", "prep", "stage:worktree_intent", "stage:worktree_done", "result"}, f.ledger.trail())
}

func TestLocalPatchRecovery_CrashAfterPatchDone_RecoveredDone(t *testing.T) {
	w := newLPPatchWorld(t, lpS13Harness(), (*lpFixture).useSymlinkedCache, (*lpFixture).appendPhaseARecords)
	w.ledger.failResult = true // the process ends before its result
	live := w.run(t)
	require.Equal(t, healthband.ClaimDone, live.Status)
	assert.NotContains(t, w.ledger.trail(), "result")

	report := w.recover()
	require.Len(t, report.Results, 1, "reasons %v", report.Reasons)
	result := report.Results[0]
	assert.Equal(t, healthband.ClaimDone, result.Status, "the patch_done row")
	assert.True(t, result.Recovered)
	assert.Empty(t, result.Kept, "every artifact equals its record")
	assert.Equal(t, live.CommitSHA, result.CommitSHA)
	assert.Equal(t, live.WorktreePath, result.WorktreePath)
	assert.Equal(t, live.PatchPath, result.PatchPath)
	assert.Equal(t, live.Branch, result.Branch)
	assert.False(t, w.absent(lpKey), "the worktree is kept for review")
	assert.False(t, w.absent(lpKey+".patch"))
	assert.Equal(t, live.CommitSHA+"\n", w.git(w.repo, "rev-parse", "refs/heads/autopus/band/"+lpKey))
}
