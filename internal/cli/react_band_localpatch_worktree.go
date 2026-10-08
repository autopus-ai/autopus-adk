package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

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
	path := p.cache.path(filepath.Join(s.key, "worktree"))
	intent := localPatchStage{ClaimID: s.target.recordClaimID(), Phase: lpPhaseWorktreeIntent, Path: path}
	if err := p.ledger.AppendStage(ctx, intent); err != nil {
		s.code = lpCodeRecordUnavailable
		return
	}
	// <lp>/<key>/ is created first with mode 0700 and without following a
	// link, so git creates the worktree inside a directory band owns.
	if err := p.cache.mkdirKey(s.key); err != nil {
		p.setupFailed(ctx, s, lpCodeWorktreeFailed, false)
		return
	}
	if _, err := p.git.In(p.checkout).Run(gctx, "worktree", "add", "--no-checkout", "--detach", path, s.baseSHA); err != nil {
		p.setupFailed(ctx, s, lpCodeWorktreeFailed, false)
		return
	}
	run := p.git.In(path)
	if code, err := run.CheckConfig(gctx); err != nil || code != "" {
		if code == "" {
			code = healthband.GitConfigUnsafePrefix + healthband.GitConfigUnreadable
		}
		p.setupFailed(ctx, s, code, false)
		return
	}
	if _, err := run.Run(gctx, "reset", "--hard", "--no-recurse-submodules", "--quiet"); err != nil {
		p.setupFailed(ctx, s, lpCodeWorktreeFailed, true)
		return
	}
	sum, err := p.statusHash(gctx, path)
	if err != nil {
		p.setupFailed(ctx, s, lpCodeWorktreeFailed, true)
		return
	}
	done := localPatchStage{ClaimID: s.target.recordClaimID(), Phase: lpPhaseWorktreeDone, StatusSHA256: sum}
	if err := p.ledger.AppendStage(ctx, done); err != nil {
		s.code = lpCodeRecordUnavailable
		s.kept = append(s.kept, p.cleanup(ctx, s, s.target.diagnose.LeaseUntil)...)
		return
	}
	s.worktree = path
}

// statusHash is the SHA-256 of the git status output followed by the git
// diff output of the worktree (CD-3 M5), streamed into the hash.
func (p *bandLocalPatcher) statusHash(ctx context.Context, worktree string) (string, error) {
	sum, run := sha256.New(), p.git.In(worktree)
	if err := run.RunTo(ctx, sum, "status", "--porcelain", "-z", "--untracked-files=all", "--ignored"); err != nil {
		return "", err
	}
	if err := run.RunTo(ctx, sum, "diff", "--no-ext-diff", "--no-textconv", "--binary"); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// setupFailed records worktree_failed with its code and cleans up: a
// checkout that the live run stopped or that failed is removed at once
// with one git worktree remove --force (live); every other failure goes
// through the Cleanup Rules.
func (p *bandLocalPatcher) setupFailed(ctx context.Context, s *localPatchSetup, code string, live bool) {
	s.code = code
	failed := localPatchStage{ClaimID: s.target.recordClaimID(), Phase: lpPhaseWorktreeFailed, Code: code}
	if err := p.ledger.AppendStage(ctx, failed); err != nil {
		s.code = lpCodeRecordUnavailable
	}
	lease := s.target.diagnose.LeaseUntil
	if !live {
		s.kept = append(s.kept, p.cleanup(ctx, s, lease)...)
		return
	}
	cctx, cancel := p.group(ctx, lease, p.groups.cleanup)
	defer cancel()
	path := p.cache.path(filepath.Join(s.key, "worktree"))
	if _, err := p.git.In(p.checkout).Run(cctx, "worktree", "remove", "--force", path); err != nil {
		s.kept = append(s.kept, localPatchKept{Artifact: path, Reason: lpKeptWorktreeIncomplete})
		return
	}
	_ = p.cache.removeEmpty(s.key)
}
