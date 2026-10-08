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

// SPEC-SIGMABAND-002 S9 (rev 11): the live run removes a step-2 checkout
// that it stopped or that exited non-zero with one git worktree remove
// --force and the empty <lp>/<key>/, so the checkout takes no retention
// slot and the next run's tier-3 opening still gets a claim.
func TestRemoveStoppedCheckout_PartialCheckout_TakesNoRetentionSlot(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	for _, key := range []string{"k-1", "k-2", "k-3"} {
		require.NoError(t, os.Mkdir(filepath.Join(w.lp.Path, key), 0o700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(w.lp.Path, "k-4.patch"), nil, 0o600))
	w.claimed(lpLease)
	w.stage(StageWorktreeIntent, func(r *LocalPatchRecord) { r.Path = w.paths.Worktree })
	w.f.must(w.git.Run(w.ctx, "worktree", "add", "--no-checkout", "--detach", w.paths.Worktree, w.base))
	// A checkout stopped part way: some files of the base, an index lock, and
	// no index file.
	w.f.writeFiles(w.paths.Worktree, map[string]string{"pkg/foo/foo.go": "package foo\n"})
	admin, found := adminDirFor(filepath.Join(w.dir, ".git"), w.paths.Worktree)
	require.True(t, found)
	require.NoError(t, os.WriteFile(filepath.Join(admin, "index.lock"), nil, 0o600))
	w.stage(StageWorktreeFailed, func(r *LocalPatchRecord) { r.Code = LocalPatchCodeWorktreeFailed })
	kept, err := w.lp.KeptKeys()
	require.NoError(t, err)
	require.Equal(t, 5, kept, "the stopped checkout holds a slot until it is removed")
	start := w.mark()
	assert.Empty(t, RemoveStoppedCheckout(w.ctx, w.git, w.lp, lpFixtureK))
	assert.Equal(t, []string{"worktree remove --force " + w.paths.Worktree}, w.invocations(start))
	assert.False(t, w.admin())
	_, err = os.Lstat(w.paths.KeyDir)
	assert.ErrorIs(t, err, os.ErrNotExist)
	kept, err = w.lp.KeptKeys()
	require.NoError(t, err)
	assert.Equal(t, 4, kept)
	decision, ok := lpDecider(func(d *LocalPatchDecider) { d.Kept = kept }).Decide([]Event{lpEvent(60, "1060", 3, ActionDiagnose, "e1060")}, 0)
	require.True(t, ok)
	assert.Equal(t, LocalPatchDecideClaim, decision.Record.Decision, "the next tier-3 opening gets a claim")
}

func TestRemoveStoppedCheckout_RemovalGitRefuses_KeepsWorktreeIncomplete(t *testing.T) {
	t.Parallel()
	w := newLPWorld(t, nil)
	w.f.git(w.f.setupEnv, w.dir, "worktree", "add", "-q", "--no-checkout", "--detach", w.paths.Worktree, w.base)
	w.f.git(w.f.setupEnv, w.dir, "worktree", "lock", w.paths.Worktree)
	assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeIncomplete}}, RemoveStoppedCheckout(w.ctx, w.git, w.lp, lpFixtureK))
	assert.True(t, w.admin())
	assert.Equal(t, []LocalPatchKept{{ArtifactWorktree, KeptWorktreeIncomplete}}, RemoveStoppedCheckout(w.ctx, w.git, w.lp, "../x"))
}

func TestLocalPatchLeaseCovers_GroupPlusCleanupAndMargin(t *testing.T) {
	t.Parallel()
	lease := lpT0.Add(LocalPatchClaimBudget)
	assert.Equal(t, 990*time.Second, LocalPatchDiagnoseBudget)
	assert.Equal(t, 810*time.Second, LocalPatchClaimBudget)
	// S9: A diagnose, A local_patch, B diagnose, C diagnose, C local_patch.
	var chained time.Duration
	var leases []time.Duration
	for _, budget := range []time.Duration{LocalPatchDiagnoseBudget, LocalPatchClaimBudget, LocalPatchDiagnoseBudget,
		LocalPatchDiagnoseBudget, LocalPatchClaimBudget} {
		chained += budget
		leases = append(leases, chained/time.Second)
	}
	assert.Equal(t, []time.Duration{990, 1800, 2790, 3780, 4590}, leases)
	assert.True(t, LocalPatchLeaseCovers(lpT0, lease, LocalPatchRequestDeadline))
	assert.True(t, LocalPatchLeaseCovers(lease.Add(-(30+60+60)*time.Second), lease, LocalPatchBranchDeadline))
	assert.False(t, LocalPatchLeaseCovers(lease.Add(-(30+60+60-1)*time.Second), lease, LocalPatchBranchDeadline))
}
