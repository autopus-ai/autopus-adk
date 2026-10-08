package cli

import (
	"context"
	"errors"
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
	policy := healthband.PatchPolicy{Git: bandPolicyGit(r.p.git), Decide: r.p.decide}
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
	intent := healthband.NewLocalPatchStage(r.s.target.claimID, healthband.StageApplyIntent)
	intent.DiffSHA256, intent.Tree = lpSHA256([]byte(r.diff)), r.tree
	if err := r.p.appendStage(ctx, r.s, intent); err != nil {
		return healthband.LocalPatchCodeRecordUnavailable
	}
	if err := r.s.dir.CreateFile(r.s.key+".diff", []byte(r.diff)); err != nil {
		return healthband.PatchCodeInvalid
	}
	if _, err := run.Run(gctx, "apply", "--index", r.s.paths.Diff); err != nil {
		return healthband.PatchCodeInvalid
	}
	if out, err := run.Run(gctx, "write-tree"); err != nil || strings.TrimSuffix(string(out), "\n") != r.tree {
		return healthband.PatchCodeInvalid
	}
	done := healthband.NewLocalPatchStage(r.s.target.claimID, healthband.StageApplyDone)
	done.Tree = r.tree
	if err := r.p.appendStage(ctx, r.s, done); err != nil {
		return healthband.LocalPatchCodeRecordUnavailable
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
	done := healthband.NewLocalPatchStage(r.s.target.claimID, healthband.StageCommitDone)
	done.CommitOID = r.commit
	if err := r.p.appendStage(ctx, r.s, done); err != nil {
		return healthband.LocalPatchCodeRecordUnavailable
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
	intent := healthband.NewLocalPatchStage(r.s.target.claimID, healthband.StageBranchIntent)
	intent.CommitOID = r.commit
	if err := r.p.appendStage(ctx, r.s, intent); err != nil {
		return healthband.LocalPatchCodeRecordUnavailable
	}
	if _, err := r.p.git.In(r.s.worktree).Run(gctx, "update-ref", "--no-deref", r.s.paths.Ref, r.commit, ""); err != nil {
		return lpCodeBranchFailed
	}
	if err := r.p.appendStage(ctx, r.s, healthband.NewLocalPatchStage(r.s.target.claimID, healthband.StageBranchDone)); err != nil {
		return healthband.LocalPatchCodeRecordUnavailable
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
	name, temp, sum := filepath.Base(r.s.paths.Patch), filepath.Base(r.s.paths.PatchTemp), lpSHA256(out)
	intent := healthband.NewLocalPatchStage(r.s.target.claimID, healthband.StagePatchIntent)
	intent.Path, intent.PatchSHA256 = r.s.paths.Patch, sum
	if err := r.p.appendStage(ctx, r.s, intent); err != nil {
		return healthband.LocalPatchCodeRecordUnavailable
	}
	if present, err := r.s.dir.Exists(name); err != nil || present {
		return lpCodePatchFileFailed // never replace a file that appeared since step 1
	}
	if err := r.s.dir.CreateFile(temp, out); err != nil {
		return lpCodePatchFileFailed
	}
	if err := r.s.dir.RenameFile(temp, name); err != nil {
		return lpCodePatchFileFailed
	}
	done := healthband.NewLocalPatchStage(r.s.target.claimID, healthband.StagePatchDone)
	done.PatchSHA256 = sum
	if err := r.p.appendStage(ctx, r.s, done); err != nil {
		return healthband.LocalPatchCodeRecordUnavailable
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
