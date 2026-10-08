package healthband

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"time"
)

// Cleanup Rules (REQ-11): one set for the live run and recovery, in the
// order 3, 1, 2, so the worktree check still finds the diff file. Only the
// artifacts that the claim's intent records name are considered, at their
// derived paths, and every git call has a LocalPatchGitTimeout timeout. An
// artifact that a rule cannot prove to be band's own and unchanged is kept
// and listed with its reason; no rule ever runs git worktree prune, unlock,
// or a second --force.

// CleanupLocalPatch applies the Cleanup Rules to the claim of records (its
// prep and stage records) after a live failure and returns the kept
// artifacts for the result's kept[]. git runs in the user's checkout. A
// record set without a valid key touches nothing.
func CleanupLocalPatch(ctx context.Context, git GitPolicyRunner, dir *LocalPatchDir, records []LocalPatchRecord) []LocalPatchKept {
	c := newLPCleaner(git, dir, lpFactsOf(records), LocalPatchGitTimeout)
	if !localPatchKeyPattern.MatchString(c.facts.key) {
		return nil
	}
	c.rules(ctx)
	return c.kept
}

// RemoveStoppedCheckout removes a step-2 checkout that the live run stopped
// or that exited non-zero, after its worktree_failed record: one git
// worktree remove --force (the entry is unlocked once git worktree add
// --no-checkout has returned, and only band's partial checkout is there),
// then the empty <lp>/<key>/. The Cleanup Rule 3 tests do not apply. A
// removal that git refuses or that is stopped keeps the entry with reason
// worktree_incomplete.
func RemoveStoppedCheckout(ctx context.Context, git GitPolicyRunner, dir *LocalPatchDir, key string) []LocalPatchKept {
	c := newLPCleaner(git, dir, lpFacts{key: key}, LocalPatchGitTimeout)
	if !localPatchKeyPattern.MatchString(key) {
		return []LocalPatchKept{{ArtifactWorktree, KeptWorktreeIncomplete}}
	}
	c.removeWorktree(ctx)
	return c.kept
}

// WorktreeStatusSHA256 is the worktree_done status_sha256: the SHA-256 of
// the git status --porcelain -z --untracked-files=all --ignored output
// followed by the git diff --no-ext-diff --no-textconv --binary output, both
// run in the worktree of wt.
func WorktreeStatusSHA256(ctx context.Context, wt GitPolicyRunner) (string, error) {
	hash := sha256.New()
	if err := wt.RunTo(ctx, hash, gitStatusArgs...); err != nil {
		return "", err
	}
	if err := wt.RunTo(ctx, hash, gitDiffArgs...); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

var (
	gitStatusArgs = []string{"status", "--porcelain", "-z", "--untracked-files=all", "--ignored"}
	gitDiffArgs   = []string{"diff", "--no-ext-diff", "--no-textconv", "--binary"}
)

type lpCleaner struct {
	git     GitPolicyRunner // the user's checkout
	dir     *LocalPatchDir
	paths   LocalPatchPaths
	facts   lpFacts
	timeout time.Duration
	kept    []LocalPatchKept
}

func newLPCleaner(git GitPolicyRunner, dir *LocalPatchDir, facts lpFacts, timeout time.Duration) *lpCleaner {
	return &lpCleaner{git: git, dir: dir, paths: dir.Paths(facts.key), facts: facts, timeout: timeout}
}

func (c *lpCleaner) keep(artifact, reason string) {
	c.kept = append(c.kept, LocalPatchKept{Artifact: artifact, Reason: reason})
}

// run runs one git command of a rule under its own timeout.
func (c *lpCleaner) run(ctx context.Context, git GitPolicyRunner, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return git.Run(ctx, args...)
}

// rules runs Cleanup Rules 3, 1, and 2.
func (c *lpCleaner) rules(ctx context.Context) {
	c.worktreeRule(ctx)
	c.branchRule(ctx)
	c.fileRule()
}

// complete reports the complete patch_intent set of the Recovery State
// Table: the patch file at its patch_intent hash, the branch at the claim
// commit, and a clean worktree whose HEAD is the claim commit.
func (c *lpCleaner) complete(ctx context.Context) bool {
	sum, found, err := c.dir.fileHash(filepath.Base(c.paths.Patch))
	if err != nil || !found || sum != c.facts.patchSHA || c.facts.commitOID == "" || c.branch(ctx) != lpBranchAtClaim {
		return false
	}
	state := c.inspect(ctx)
	return state.present && state.removable && state.head == c.facts.commitOID
}

// patchDone is the patch_done row: everything stays, the diff file goes when
// its hash matches, and each artifact that differs from its record is
// listed.
func (c *lpCleaner) patchDone(ctx context.Context) {
	if state := c.inspect(ctx); state.present && (!state.removable || state.head != c.facts.commitOID) {
		reason := state.reason
		if state.removable {
			reason = KeptHeadUnrecognized
		}
		c.keep(ArtifactWorktree, reason)
	}
	if c.branch(ctx) == lpBranchMoved {
		c.keep(ArtifactBranch, KeptBranchMoved)
	}
	if sum, found, err := c.dir.fileHash(filepath.Base(c.paths.Patch)); found && (err != nil || sum != c.facts.patchSHA) {
		c.keep(ArtifactPatchFile, KeptPatchModified)
	}
	if c.facts.has[StageApplyIntent] {
		c.removeIfHash(filepath.Base(c.paths.Diff), c.facts.diffSHA, ArtifactDiffFile)
	}
}

// worktreeRule is Cleanup Rule 3.
func (c *lpCleaner) worktreeRule(ctx context.Context) {
	if !c.facts.has[StageWorktreeIntent] {
		return
	}
	state := c.inspect(ctx)
	switch {
	case !state.present:
		c.dir.removeEmptyKeyDir(c.facts.key)
	case state.removable:
		c.removeWorktree(ctx)
	default:
		c.keep(ArtifactWorktree, state.reason)
	}
}

func (c *lpCleaner) removeWorktree(ctx context.Context) {
	if _, err := c.run(ctx, c.git, "worktree", "remove", "--force", c.paths.Worktree); err != nil {
		c.keep(ArtifactWorktree, KeptWorktreeIncomplete)
		return
	}
	c.dir.removeEmptyKeyDir(c.facts.key)
}

// branchRule is Cleanup Rule 1: the branch is deleted only while it is no
// symbolic ref and points at the claim commit, by update-ref --no-deref -d
// with that commit as the old value.
func (c *lpCleaner) branchRule(ctx context.Context) {
	if !c.facts.has[StageBranchIntent] {
		return
	}
	switch c.branch(ctx) {
	case lpBranchAbsent:
	case lpBranchAtClaim:
		if _, err := c.run(ctx, c.git, "update-ref", "--no-deref", "-d", c.paths.Ref, c.facts.commitOID); err != nil {
			c.keep(ArtifactBranch, KeptBranchMoved)
		}
	default:
		c.keep(ArtifactBranch, KeptBranchMoved)
	}
}

type lpBranchState int

const (
	lpBranchAbsent lpBranchState = iota
	lpBranchAtClaim
	lpBranchMoved // a symbolic ref, another OID, or a check that failed
)

func (c *lpCleaner) branch(ctx context.Context) lpBranchState {
	if _, err := c.run(ctx, c.git, "symbolic-ref", "-q", "--no-recurse", c.paths.Ref); GitExitCode(err) != 1 {
		return lpBranchMoved
	}
	out, err := c.run(ctx, c.git, "rev-parse", "--verify", "--quiet", c.paths.Ref)
	switch {
	case GitExitCode(err) == 1:
		return lpBranchAbsent
	case err == nil && c.facts.commitOID != "" && gitLine(out) == c.facts.commitOID:
		return lpBranchAtClaim
	}
	return lpBranchMoved
}

// fileRule is Cleanup Rule 2: the patch file and its temp file go only when
// their SHA-256 equals the patch_intent hash, the diff file only when it
// equals the apply_intent hash, so an edited file is kept and no git version
// or configuration change can reclassify an unmodified one.
func (c *lpCleaner) fileRule() {
	if c.facts.has[StagePatchIntent] {
		c.removeIfHash(filepath.Base(c.paths.Patch), c.facts.patchSHA, ArtifactPatchFile)
		c.removeIfHash(filepath.Base(c.paths.PatchTemp), c.facts.patchSHA, ArtifactPatchTemp)
	}
	if c.facts.has[StageApplyIntent] {
		c.removeIfHash(filepath.Base(c.paths.Diff), c.facts.diffSHA, ArtifactDiffFile)
	}
}

func (c *lpCleaner) removeIfHash(name, want, artifact string) {
	sum, found, err := c.dir.fileHash(name)
	if !found && err == nil {
		return
	}
	if err != nil || sum != want || c.dir.removeFile(name) != nil {
		c.keep(artifact, KeptPatchModified)
	}
}

// statusMatches reports whether status, followed by the worktree's diff,
// hashes to the worktree_done status_sha256.
func (c *lpCleaner) statusMatches(ctx context.Context, wt GitPolicyRunner, status []byte) bool {
	if c.facts.statusSHA == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	hash := sha256.New()
	_, _ = hash.Write(status)
	if err := wt.RunTo(ctx, io.Writer(hash), gitDiffArgs...); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == c.facts.statusSHA
}
