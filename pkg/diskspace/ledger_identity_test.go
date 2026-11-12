package diskspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func idleCwds(context.Context) ([]string, error) { return []string{}, nil }

// identityRepo is a repository whose .gitignore ignores .env and
// node_modules, and whose origin/main is its main.
func identityRepo(t *testing.T) (repo, base string) {
	t.Helper()
	base = canonicalTempDir(t)
	repo = filepath.Join(base, "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitT(t, repo, "init", "--initial-branch=main")
	writeFile(t, filepath.Join(repo, ".gitignore"), ".env\nnode_modules/\n")
	writeFile(t, filepath.Join(repo, "package.json"), "{}\n")
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-m", "seed")
	gitT(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	return repo, base
}

func scanIdentity(t *testing.T, repo string, now time.Time, ledger []LedgerEntry) WorktreeReclaimScan {
	t.Helper()
	return ScanWorktreeReclaim(context.Background(), WorktreeReclaimInput{Run: hermeticRunner, ProjectDir: repo,
		Cwd: repo, Now: now, IdleMinutes: 360, UID: os.Getuid(), Ledger: ledger, Cwds: idleCwds})
}

// A released entry outlives a worktree removed outside reclaim. A new
// worktree at the same path, even with the freed inode number, is neither
// live nor released, so reclaim never selects it on the old release.
func TestLedger_ReleaseDoesNotCarryToRecreatedWorktree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, base := identityRepo(t)
	wt := filepath.Join(base, "wt")
	gitT(t, repo, "worktree", "add", "-b", "feat", wt, "main")
	state := testState(t)
	require.NoError(t, RegisterWorktree(ctx, state, wt, "agent-1", time.Now()))
	found, err := ReleaseWorktree(ctx, state, wt, time.Now())
	require.NoError(t, err)
	require.True(t, found)

	gitT(t, repo, "worktree", "remove", wt)
	gitT(t, repo, "worktree", "add", "-b", "feat2", wt, "main")
	writeFile(t, filepath.Join(wt, ".env"), "KEY=1\n")
	reuseInode(t, state, wt)
	entries, err := LoadLedger(state)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.False(t, entries[0].Live())

	scan := scanIdentity(t, repo, time.Now(), entries)
	assert.Empty(t, scan.Candidates)
	counters := ApplyWorktrees(ctx, hermeticRunner, []ReclaimCandidate{{Kind: ReclaimWorktree, Path: wt,
		Reason: ReasonReleased, MainPath: repo, ID: pathID(wt), Head: strings.TrimSpace(gitT(t, wt, "rev-parse", "HEAD"))}}, &state, os.Getuid(), nil)
	assert.Equal(t, 0, counters.Worktrees, "the apply-time recheck refuses the stale release too")
	assert.FileExists(t, filepath.Join(wt, ".env"))
}

// An entry written before identity tokens existed is never live: it is not
// trusted to own, let alone release, whatever is at its path now.
func TestLedger_EntryWithoutTokenIsNotLive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, wt, _ := ledgerRepo(t)
	state := testState(t)
	require.NoError(t, RegisterWorktree(ctx, state, wt, "", time.Now()))
	entries, err := LoadLedger(state)
	require.NoError(t, err)
	require.True(t, entries[0].Live())
	entries[0].Token = ""
	data, err := json.Marshal(ledgerDoc{Version: ledgerVersion, Entries: entries})
	require.NoError(t, err)
	require.NoError(t, state.WriteFileAtomic(ledgerFile, data))
	entries, err = LoadLedger(state)
	require.NoError(t, err)
	assert.False(t, entries[0].Live())

	require.NoError(t, RegisterWorktree(ctx, state, wt, "", time.Now()))
	entries, err = LoadLedger(state)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.True(t, entries[0].Live(), "registering again restores it")
}

// A released worktree checked out on another branch afterwards is in use
// again: the release no longer applies.
func TestLedger_ReleaseRequiresTheSameHead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, base := identityRepo(t)
	wt := filepath.Join(base, "wt")
	gitT(t, repo, "worktree", "add", "-b", "feat", wt, "main")
	state := testState(t)
	require.NoError(t, RegisterWorktree(ctx, state, wt, "", time.Now()))
	_, err := ReleaseWorktree(ctx, state, wt, time.Now())
	require.NoError(t, err)
	entries, err := LoadLedger(state)
	require.NoError(t, err)
	require.True(t, entries[0].ReleaseHolds())
	scan := scanIdentity(t, repo, time.Now(), entries)
	require.Len(t, scan.Candidates, 1)
	assert.Equal(t, ReasonReleased, scan.Candidates[0].Reason)

	gitT(t, wt, "checkout", "-b", "other")
	assert.False(t, entries[0].ReleaseHolds())
	scan = scanIdentity(t, repo, time.Now(), entries)
	assert.Empty(t, scan.Candidates, "recent, so not merged_idle either")
	counters := ApplyWorktrees(ctx, hermeticRunner, []ReclaimCandidate{{Kind: ReclaimWorktree, Path: wt,
		Reason: ReasonReleased, MainPath: repo, ID: pathID(wt), Head: strings.TrimSpace(gitT(t, wt, "rev-parse", "HEAD"))}}, &state, os.Getuid(), nil)
	assert.Equal(t, 0, counters.Worktrees)
	assert.DirExists(t, wt)
}

// A merged, clean, idle worktree the user made outside the harness
// directories is kept while it holds an ignored file such as .env, which
// `git worktree remove` would delete; ignored build output alone does not
// keep it.
func TestMergedLinked_IgnoredFilesKeepTheWorktree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, base := identityRepo(t)
	keep := filepath.Join(base, "review")
	gitT(t, repo, "worktree", "add", "-b", "review", keep, "origin/main")
	writeFile(t, filepath.Join(keep, ".env"), "KEY=1\n")
	built := filepath.Join(base, "built")
	gitT(t, repo, "worktree", "add", "-b", "built", built, "origin/main")
	writeFile(t, filepath.Join(built, "node_modules", "x", "index.js"), "x\n")

	scan := scanIdentity(t, repo, time.Now().Add(7*time.Hour), nil)
	require.Len(t, scan.Candidates, 1)
	assert.Equal(t, built, scan.Candidates[0].Path)
	assert.Equal(t, ReasonMergedLinked, scan.Candidates[0].Reason)
	assert.Equal(t, 1, scan.Skipped[SkipIgnored])

	// Written between the scan and the apply: the recheck keeps it.
	writeFile(t, filepath.Join(built, ".env"), "KEY=2\n")
	counters := ApplyWorktrees(ctx, hermeticRunner, scan.Candidates, nil, os.Getuid(), nil)
	assert.Equal(t, 0, counters.Worktrees)
	assert.Equal(t, 1, counters.SkippedChanged)
	assert.FileExists(t, filepath.Join(keep, ".env"))
	assert.FileExists(t, filepath.Join(built, ".env"))
}

func TestRegenerableEntry(t *testing.T) {
	t.Parallel()
	root := canonicalTempDir(t)
	writeFile(t, filepath.Join(root, "web", "package.json"), "{}")
	assert.True(t, regenerableEntry(root, "web/node_modules/"))
	assert.True(t, regenerableEntry(root, "web/dist/app.js"))
	assert.False(t, regenerableEntry(root, "node_modules/"), "no manifest beside it")
	assert.False(t, regenerableEntry(root, "web/.env"))
	assert.False(t, regenerableEntry(root, "web/dist"), "a file named like a build directory")
}
