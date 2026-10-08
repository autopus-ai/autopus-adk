package cli

import (
	"context"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Local Patch Flow step 2: the worktree of a flag-on diagnosis at
// <lp>/<key>/worktree/, added without a checkout so that Git Execution
// Policy item 3 runs inside it before any file of the base is written, then
// checked out by the reset that a default worktree add runs as its child.

// addWorktree is step 2 up to worktree_done: the intent record, the add
// without checkout, item 3 inside the new worktree, the checkout, and the
// status hash. gctx bounds the git commands, ctx the records.
func (p *bandLocalPatcher) addWorktree(gctx, ctx context.Context, s *localPatchSetup) {
	path, claimID := s.paths.Worktree, s.target.recordClaimID()
	intent := healthband.NewLocalPatchStage(claimID, healthband.StageWorktreeIntent)
	intent.Path = path
	if err := p.appendStage(ctx, s, intent); err != nil {
		s.code = healthband.LocalPatchCodeRecordUnavailable
		return
	}
	// <lp>/<key>/ is created first with mode 0700 and without following a
	// link, so git creates the worktree inside a directory band owns.
	if err := s.dir.MakeKeyDir(s.key); err != nil {
		p.setupFailed(ctx, s, healthband.LocalPatchCodeWorktreeFailed, false)
		return
	}
	if _, err := p.git.In(p.checkout).Run(gctx, "worktree", "add", "--no-checkout", "--detach", path, s.baseSHA); err != nil {
		p.setupFailed(ctx, s, healthband.LocalPatchCodeWorktreeFailed, false)
		return
	}
	run := p.git.In(path)
	if code, err := run.CheckConfig(gctx); err != nil || code != "" {
		p.setupFailed(ctx, s, defaultString(code, healthband.GitConfigUnsafePrefix+healthband.GitConfigUnreadable), false)
		return
	}
	if _, err := run.Run(gctx, "reset", "--hard", "--no-recurse-submodules", "--quiet"); err != nil {
		p.setupFailed(ctx, s, healthband.LocalPatchCodeWorktreeFailed, true)
		return
	}
	sum, err := healthband.WorktreeStatusSHA256(gctx, run)
	if err != nil {
		p.setupFailed(ctx, s, healthband.LocalPatchCodeWorktreeFailed, true)
		return
	}
	done := healthband.NewLocalPatchStage(claimID, healthband.StageWorktreeDone)
	done.StatusSHA256 = sum
	if err := p.appendStage(ctx, s, done); err != nil {
		s.code = healthband.LocalPatchCodeRecordUnavailable
		s.kept = append(s.kept, p.cleanup(ctx, s, s.target.diagnose.LeaseUntil, p.groups.cleanup)...)
		return
	}
	s.worktree = path
}

// setupFailed records worktree_failed with its code and cleans up: a
// checkout that the live run stopped or that failed is removed at once
// with one git worktree remove --force (checkout); every other failure goes
// through the Cleanup Rules.
func (p *bandLocalPatcher) setupFailed(ctx context.Context, s *localPatchSetup, code string, checkout bool) {
	s.code = code
	failed := healthband.NewLocalPatchStage(s.target.recordClaimID(), healthband.StageWorktreeFailed)
	failed.Code = code
	if err := p.appendStage(ctx, s, failed); err != nil {
		s.code = healthband.LocalPatchCodeRecordUnavailable
	}
	lease := s.target.diagnose.LeaseUntil
	if !checkout {
		s.kept = append(s.kept, p.cleanup(ctx, s, lease, p.groups.cleanup)...)
		return
	}
	cctx, cancel := p.group(ctx, lease, p.groups.cleanup)
	defer cancel()
	s.kept = append(s.kept, healthband.RemoveStoppedCheckout(cctx, p.git.In(p.checkout), s.dir, s.key)...)
}
