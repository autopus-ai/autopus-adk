package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Local Patch Flow steps 7–11: the Patch Policy over the raw reply (no
// command writes an object before it passes), the expected tree in a band
// temp index, git apply --index in the claim's worktree, the commit with
// its object check, the create-only branch, and the canonical format-patch
// file. Every *_intent record precedes the command that creates its
// artifact, and from step 9 on every command names the recorded commit OID,
// never HEAD.

// lpPatchFileBytes bounds the format-patch output (step 11).
const lpPatchFileBytes = 1 << 20

// policy is step 7: Patch Policy items 1–9, ending with git apply --numstat
// --summary -z --check; a fault refuses the diff.
func (r *patchRun) policy(gctx, _ context.Context) string {
	git := r.p.policyGit
	if git == nil {
		git = bandPolicyGit(r.p.git)
	}
	policy := healthband.PatchPolicy{Git: git, Decide: r.p.decide}
	verdict, err := policy.Evaluate(gctx, healthband.PatchPolicyInput{
		Reply: r.reply, BaseSHA: r.s.baseSHA, Worktree: r.s.worktree, Checkout: r.p.checkout,
	})
	switch {
	case err != nil:
		return healthband.PatchCodeInvalid
	case !verdict.Accepted():
		return verdict.Code
	}
	r.diff, r.result.Files = verdict.Diff, verdict.Files
	return ""
}

// apply is step 8: item 3 inside the worktree again, the expected tree of a
// band temp index, apply_intent, the diff file, git apply --index, and the
// index tree check.
func (r *patchRun) apply(gctx, ctx context.Context) string {
	run := r.p.git.In(r.s.worktree)
	if code, err := run.CheckConfig(gctx); err != nil {
		return healthband.GitConfigUnsafePrefix + healthband.GitConfigUnreadable
	} else if code != "" {
		return code
	}
	temp, err := os.MkdirTemp("", "autopus-band-")
	if err != nil {
		return healthband.PatchCodeInvalid
	}
	r.temp = temp
	index := filepath.Join(temp, "index")
	indexed := run.WithIndexFile(index)
	_, err = indexed.Run(gctx, "read-tree", r.s.baseSHA)
	if err == nil {
		_, err = indexed.RunInput(gctx, []byte(r.diff), "apply", "--cached")
	}
	var out []byte
	if err == nil {
		out, err = indexed.Run(gctx, "write-tree")
	}
	_ = os.Remove(index)
	if r.tree = strings.TrimSuffix(string(out), "\n"); err != nil || !lpValidOID(r.tree) {
		return healthband.PatchCodeInvalid
	}
	intent := localPatchStage{ClaimID: r.s.target.claimID, Phase: lpPhaseApplyIntent, DiffSHA256: lpSHA256([]byte(r.diff)), Tree: r.tree}
	if err := r.p.ledger.AppendStage(ctx, intent); err != nil {
		return lpCodeRecordUnavailable
	}
	if err := r.p.cache.create(r.s.key+".diff", []byte(r.diff)); err != nil {
		return healthband.PatchCodeInvalid
	}
	if _, err := run.Run(gctx, "apply", "--index", r.p.cache.path(r.s.key+".diff")); err != nil {
		return healthband.PatchCodeInvalid
	}
	if out, err := run.Run(gctx, "write-tree"); err != nil || strings.TrimSuffix(string(out), "\n") != r.tree {
		return healthband.PatchCodeInvalid
	}
	if err := r.p.ledger.AppendStage(ctx, localPatchStage{ClaimID: r.s.target.claimID, Phase: lpPhaseApplyDone, Tree: r.tree}); err != nil {
		return lpCodeRecordUnavailable
	}
	return ""
}

// commitPatch is step 9: git commit with the -F file, commit_done with the
// OID that git rev-parse HEAD prints, and the object check of that OID:
// exactly one parent, the base SHA, the expected tree, and a message equal
// to the -F file bytes.
func (r *patchRun) commitPatch(gctx, ctx context.Context) string {
	// The band temp directory, with the -F file, goes after step 9.
	defer func() { _ = os.RemoveAll(r.temp); r.temp = "" }()
	if r.p.beforeCommit != nil {
		r.p.beforeCommit(r.s.worktree)
	}
	messageFile := filepath.Join(r.temp, "message")
	if err := lpWriteNew(messageFile, []byte(r.message)); err != nil {
		return lpCodeCommitFailed
	}
	run := r.p.git.In(r.s.worktree)
	if _, err := run.Run(gctx, "commit", "--no-verify", "--cleanup=verbatim", "-F", messageFile); err != nil {
		return lpCodeCommitFailed
	}
	out, err := run.Run(gctx, "rev-parse", "HEAD")
	if r.commit = strings.TrimSuffix(string(out), "\n"); err != nil || !lpValidOID(r.commit) {
		r.commit = ""
		return lpCodeCommitFailed
	}
	if err := r.p.ledger.AppendStage(ctx, localPatchStage{ClaimID: r.s.target.claimID, Phase: lpPhaseCommitDone, CommitOID: r.commit}); err != nil {
		return lpCodeRecordUnavailable
	}
	raw, err := run.Run(gctx, "cat-file", "commit", r.commit)
	if err != nil {
		return lpCodeCommitTreeMismatch
	}
	tree, parents, message, ok := lpParseCommit(string(raw))
	switch {
	case !ok || tree != r.tree || len(parents) != 1 || parents[0] != r.s.baseSHA:
		return lpCodeCommitTreeMismatch
	case message != r.message:
		return lpCodeCommitMessageAltered
	}
	return ""
}

// branch is step 10: branch_intent, then a create-only update-ref that
// never writes through a symbolic ref, then branch_done.
func (r *patchRun) branch(gctx, ctx context.Context) string {
	claimID := r.s.target.claimID
	if err := r.p.ledger.AppendStage(ctx, localPatchStage{ClaimID: claimID, Phase: lpPhaseBranchIntent, CommitOID: r.commit}); err != nil {
		return lpCodeRecordUnavailable
	}
	ref := healthband.BandBranchRef(r.s.key)
	if _, err := r.p.git.In(r.s.worktree).Run(gctx, "update-ref", "--no-deref", ref, r.commit, ""); err != nil {
		return lpCodeBranchFailed
	}
	if err := r.p.ledger.AppendStage(ctx, localPatchStage{ClaimID: claimID, Phase: lpPhaseBranchDone}); err != nil {
		return lpCodeRecordUnavailable
	}
	return ""
}

// patchFile is step 11: the canonical format-patch of base..commit into
// memory (at most 1 MiB), patch_intent with its SHA-256, the bytes to
// <key>.patch.tmp-<c8>, the rename to <key>.patch, and patch_done.
func (r *patchRun) patchFile(gctx, ctx context.Context) string {
	run := r.p.git.In(r.s.worktree)
	run.MaxOutput = lpPatchFileBytes
	out, err := run.Run(gctx, healthband.GitFormatPatchArgs(r.s.baseSHA, r.commit)...)
	if err != nil || len(out) == 0 {
		return lpCodePatchFileFailed
	}
	claimID, name := r.s.target.claimID, r.s.key+".patch"
	sum := lpSHA256(out)
	intent := localPatchStage{ClaimID: claimID, Phase: lpPhasePatchIntent, Path: r.p.cache.path(name), PatchSHA256: sum}
	if err := r.p.ledger.AppendStage(ctx, intent); err != nil {
		return lpCodeRecordUnavailable
	}
	if _, err := r.p.cache.root.Lstat(r.p.cache.relPath(name)); !errors.Is(err, fs.ErrNotExist) {
		return lpCodePatchFileFailed // never replace a file that appeared since step 1
	}
	temp := name + ".tmp-" + claimID[:8]
	if err := r.p.cache.create(temp, out); err != nil {
		return lpCodePatchFileFailed
	}
	if err := r.p.cache.rename(temp, name); err != nil {
		return lpCodePatchFileFailed
	}
	if err := r.p.ledger.AppendStage(ctx, localPatchStage{ClaimID: claimID, Phase: lpPhasePatchDone, PatchSHA256: sum}); err != nil {
		return lpCodeRecordUnavailable
	}
	return ""
}

// lpParseCommit reads a commit object: its one tree, its parents, and the
// message after the header's blank line.
func lpParseCommit(raw string) (tree string, parents []string, message string, ok bool) {
	header, message, found := strings.Cut(raw, "\n\n")
	trees := 0
	for _, line := range strings.Split(header, "\n") {
		if value, isTree := strings.CutPrefix(line, "tree "); isTree {
			tree, trees = value, trees+1
		} else if value, isParent := strings.CutPrefix(line, "parent "); isParent {
			parents = append(parents, value)
		}
	}
	return tree, parents, message, found && trees == 1
}

// lpValidOID admits a full lowercase SHA-1 or SHA-256 object name.
func lpValidOID(oid string) bool {
	return (len(oid) == 40 || len(oid) == 64) && strings.Trim(oid, "0123456789abcdef") == ""
}

// lpWriteNew writes a new 0600 file without following a link.
func lpWriteNew(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|lpNoFollowFlag, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return errors.Join(err, file.Close())
}
