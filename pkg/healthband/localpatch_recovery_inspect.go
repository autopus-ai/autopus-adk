package healthband

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// Cleanup Rule 3's worktree tests. Only the admin entry whose path equals
// the derived worktree path is considered. The locked check comes first and
// alone decides an entry left by an interrupted git worktree add; an entry
// without an index file is removable only while its directory holds just its
// .git file; and before any command reads the worktree's files, Git
// Execution Policy item 3 runs inside it.

// lpWorktreeState classifies the claim's worktree.
type lpWorktreeState struct {
	present   bool   // an admin entry or a worktree directory exists
	removable bool   // band may remove it with one git worktree remove --force
	reason    string // keep reason while present and not removable
	head      string // the entry's HEAD
}

func (c *lpCleaner) inspect(ctx context.Context) lpWorktreeState {
	keep := func(reason string) lpWorktreeState { return lpWorktreeState{present: true, reason: reason} }
	list, err := c.run(ctx, c.git, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return keep(KeptWorktreeIncomplete)
	}
	absent, onlyGitFile, contentErr := c.dir.worktreeContents(c.facts.key)
	entry, found := findWorktree(parseWorktreeList(list), c.paths.Worktree)
	if !found {
		if absent && contentErr == nil {
			return lpWorktreeState{}
		}
		return keep(KeptWorktreeIncomplete)
	}
	admin, known := adminDirFor(c.dir.CommonDir, c.paths.Worktree)
	if !known || entry.locked || exists(filepath.Join(admin, "locked")) || contentErr != nil {
		return keep(KeptWorktreeIncomplete)
	}
	if !exists(filepath.Join(admin, "index")) {
		if absent || onlyGitFile {
			return lpWorktreeState{present: true, removable: true, head: entry.head}
		}
		return keep(KeptWorktreeIncomplete) // a checkout that a crash of band interrupted
	}
	if absent {
		return lpWorktreeState{present: true, removable: true, head: entry.head} // only the admin entry is left
	}
	state := c.inspectCheckout(ctx, entry.head)
	state.present, state.head = true, entry.head
	return state
}

// inspectCheckout runs the tests over a checked-out worktree whose HEAD is
// head: configuration, HEAD, index flags, gitlinks, and status.
func (c *lpCleaner) inspectCheckout(ctx context.Context, head string) lpWorktreeState {
	wt := c.git.In(c.paths.Worktree)
	configCtx, cancel := context.WithTimeout(ctx, c.timeout)
	code, err := wt.CheckConfig(configCtx)
	cancel()
	switch {
	case err != nil:
		return lpWorktreeState{reason: KeptWorktreeIncomplete}
	case code != "":
		return lpWorktreeState{reason: KeptGitConfigUnsafe}
	case !c.knownHead(ctx, head):
		return lpWorktreeState{reason: KeptHeadUnrecognized}
	}
	flags, err := c.run(ctx, wt, "ls-files", "-v", "-z")
	if err != nil {
		return lpWorktreeState{reason: KeptWorktreeIncomplete}
	}
	stages, err := c.run(ctx, wt, "ls-files", "-s", "-z")
	if err != nil {
		return lpWorktreeState{reason: KeptWorktreeIncomplete}
	}
	if indexFlagged(flags) {
		return lpWorktreeState{reason: KeptWorktreeModified}
	}
	for _, path := range gitlinkPaths(stages) {
		if !c.dir.gitlinkClear(c.facts.key, path) {
			return lpWorktreeState{reason: KeptWorktreeModified}
		}
	}
	status, err := c.run(ctx, wt, gitStatusArgs...)
	if err != nil {
		return lpWorktreeState{reason: KeptWorktreeIncomplete}
	}
	if len(status) == 0 || c.statusMatches(ctx, wt, status) || head == c.facts.baseSHA && c.indexAtExpected(ctx, wt, status) {
		return lpWorktreeState{removable: true}
	}
	return lpWorktreeState{reason: KeptWorktreeModified}
}

// indexAtExpected is the third removal case: before the commit, a status of
// only staged entries while the index tree is the apply_intent expected
// tree, so the staged entries are exactly the diff's paths.
func (c *lpCleaner) indexAtExpected(ctx context.Context, wt GitPolicyRunner, status []byte) bool {
	if c.facts.expectedTree == "" || !onlyStaged(status) {
		return false
	}
	tree, err := c.run(ctx, wt, "write-tree")
	return err == nil && gitLine(tree) == c.facts.expectedTree
}

// knownHead reports a HEAD at the base SHA or at the claim commit: the
// commit_done or branch_intent OID, or, without one, the commit whose only
// parent is the base, whose tree is the expected tree, and whose message
// hashes to the message stage (found even after a crash before
// commit_done). A commit a user amended to another tree is never it.
func (c *lpCleaner) knownHead(ctx context.Context, head string) bool {
	switch {
	case !validGitOID(head):
		return false
	case head == c.facts.baseSHA:
		return true
	case c.facts.commitOID != "":
		return head == c.facts.commitOID
	}
	if c.facts.baseSHA == "" || c.facts.expectedTree == "" || c.facts.messageSHA == "" {
		return false
	}
	out, err := c.run(ctx, c.git, "cat-file", "commit", head)
	if err != nil {
		return false
	}
	commit, ok := parseCommit(out)
	sum := sha256.Sum256(commit.message)
	if !ok || len(commit.parents) != 1 || commit.parents[0] != c.facts.baseSHA || commit.tree != c.facts.expectedTree ||
		hex.EncodeToString(sum[:]) != c.facts.messageSHA {
		return false
	}
	c.facts.commitOID = head
	return true
}
