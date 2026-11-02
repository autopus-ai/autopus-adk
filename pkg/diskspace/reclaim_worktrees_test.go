package diskspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// s4Repo builds acceptance S4: worktrees a through h of one repository, with
// e and f registered and f released. It returns the repository, the paths,
// and the state dir.
func s4Repo(t *testing.T) (string, map[string]string, StateDir) {
	t.Helper()
	base := canonicalTempDir(t)
	repo := filepath.Join(base, "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitT(t, repo, "init", "--initial-branch=main")
	writeFile(t, filepath.Join(repo, ".gitignore"), "target/\n")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "seed\n")
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-m", "seed")
	wt := map[string]string{}
	for _, name := range []string{"a", "b", "c", "g", "h"} {
		wt[name] = filepath.Join(repo, ".worktrees", name)
	}
	for _, name := range []string{"d", "e", "f"} {
		wt[name] = filepath.Join(base, "elsewhere", name)
	}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		gitT(t, repo, "worktree", "add", "-b", "br-"+name, wt[name], "main")
	}
	a := &aged{now: time.Now()}
	a.file(filepath.Join(wt["a"], "target", "debug", "blob.bin"), 3*MiB, 0)
	require.NoError(t, a.err)
	writeFile(t, filepath.Join(wt["c"], "untracked.txt"), "u\n")
	writeFile(t, filepath.Join(wt["f"], "wip.txt"), "wip\n")
	gitT(t, wt["f"], "add", ".")
	gitT(t, wt["f"], "commit", "-m", "wip")
	gitT(t, repo, "worktree", "lock", "--reason", "busy", wt["g"])

	state := testState(t)
	ctx := context.Background()
	require.NoError(t, RegisterWorktree(ctx, state, wt["e"], "agent", time.Now()))
	require.NoError(t, RegisterWorktree(ctx, state, wt["f"], "agent", time.Now()))
	_, err := ReleaseWorktree(ctx, state, wt["f"], time.Now())
	require.NoError(t, err)
	return repo, wt, state
}

func s4Input(t *testing.T, repo string, wt map[string]string, state StateDir) WorktreeReclaimInput {
	t.Helper()
	ledger, err := LoadLedger(state)
	require.NoError(t, err)
	// Every fixture file is minutes old; the clock runs 400 minutes ahead,
	// and b gets a file written 100 minutes before it.
	now := time.Now().Add(400 * time.Minute)
	recent := now.Add(-100 * time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(wt["b"], "tracked.txt"), recent, recent))
	return WorktreeReclaimInput{Run: hermeticRunner, ProjectDir: repo, Cwd: filepath.Join(wt["h"]),
		Now: now, IdleMinutes: 360, UID: os.Getuid(), Ledger: ledger}
}

func candidatePaths(cands []ReclaimCandidate) []string {
	var out []string
	for _, c := range cands {
		out = append(out, c.Path)
	}
	slices.Sort(out)
	return out
}

// SPEC-DISK-002 S4: the fixed selection table.
func TestScanWorktreeReclaim_S4Table(t *testing.T) {
	t.Parallel()
	repo, wt, state := s4Repo(t)
	scan := ScanWorktreeReclaim(context.Background(), s4Input(t, repo, wt, state))

	want := []string{wt["a"], wt["e"], wt["f"]}
	slices.Sort(want)
	assert.Equal(t, want, candidatePaths(scan.Candidates))
	reasons := map[string]string{}
	for _, c := range scan.Candidates {
		reasons[c.Path] = c.Reason
	}
	assert.Equal(t, ReasonMergedIdle, reasons[wt["a"]])
	assert.Equal(t, ReasonMergedIdle, reasons[wt["e"]])
	assert.Equal(t, ReasonReleased, reasons[wt["f"]])
	assert.Equal(t, map[string]int{SkipRecent: 1, SkipDirty: 1, SkipNotOwned: 1, SkipLocked: 1, SkipCwd: 1}, scan.Skipped)
	assert.False(t, scan.Owned[wt["d"]])
	assert.True(t, scan.Owned[wt["b"]])
}

// SPEC-DISK-002 S4: apply removes exactly the candidates, keeps branches, and
// drops their ledger entries.
func TestApplyWorktrees_S4RemovesCandidatesKeepsBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, wt, state := s4Repo(t)
	scan := ScanWorktreeReclaim(ctx, s4Input(t, repo, wt, state))

	counters := ApplyWorktrees(ctx, hermeticRunner, scan.Candidates, &state, os.Getuid(), nil)
	assert.Equal(t, 3, counters.Worktrees)
	assert.Zero(t, counters.Failed)
	assert.Positive(t, counters.DeletedBytes)
	for _, name := range []string{"a", "e", "f"} {
		assert.NoDirExists(t, wt[name])
	}
	for _, name := range []string{"b", "c", "d", "g", "h"} {
		assert.DirExists(t, wt[name])
	}
	list := gitT(t, repo, "worktree", "list", "--porcelain")
	assert.NotContains(t, list, wt["a"])
	branches := gitT(t, repo, "branch", "--list")
	for _, name := range []string{"br-a", "br-e", "br-f"} {
		assert.Contains(t, branches, name)
	}
	ledger, err := LoadLedger(state)
	require.NoError(t, err)
	assert.Empty(t, ledger)
}

// SPEC-DISK-002 S4: a file added between scan and apply keeps the worktree.
func TestApplyWorktrees_ChangedSinceScanIsSkipped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, wt, state := s4Repo(t)
	scan := ScanWorktreeReclaim(ctx, s4Input(t, repo, wt, state))

	counters := ApplyWorktrees(ctx, hermeticRunner, scan.Candidates, &state, os.Getuid(), func(path string) {
		if path == wt["a"] {
			writeFile(t, filepath.Join(path, "late.txt"), "late\n")
		}
	})
	assert.Equal(t, 1, counters.SkippedChanged)
	assert.Equal(t, 2, counters.Worktrees)
	assert.DirExists(t, wt["a"])
}

// A released detached worktree with commits off the base is kept.
func TestWorktreeVerdict_DetachedReleasedUnmergedIsKept(t *testing.T) {
	t.Parallel()
	released := time.Now()
	entry := LedgerEntry{ReleasedAt: &released}
	_, skip := worktreeVerdict(Worktree{Clean: True, Merged: False}, entry, true, "/w", "/elsewhere", time.Now(), 360, time.Now())
	assert.Equal(t, SkipDetached, skip)
	reason, _ := worktreeVerdict(Worktree{Clean: True, Merged: False, Branch: "wip"}, entry, true, "/w", "/elsewhere", time.Now(), 360, time.Now())
	assert.Equal(t, ReasonReleased, reason)
}

// SPEC-DISK-002 REQ-DISK2-REG-04: a pipeline worktree directly under the
// temp dir is a harness worktree; the same name elsewhere is not.
func TestIsHarnessWorktree_PipelineTempWorktree(t *testing.T) {
	t.Parallel()
	tmp := canonicalOrSelf(os.TempDir())
	assert.True(t, isHarnessWorktree("/repo", filepath.Join(tmp, "autopus-worktree-12345")))
	assert.False(t, isHarnessWorktree("/repo", filepath.Join(tmp, "autopus-worktree-x")))
	assert.False(t, isHarnessWorktree("/repo", filepath.Join(tmp, "sub", "autopus-worktree-12345")))
	assert.True(t, isHarnessWorktree("/repo", "/repo/.worktrees/a"))
}

// SPEC-DISK-002 review F-03: a clean commit, a re-registration that clears
// the release, or fresh activity after the scan each keep the worktree.
func TestApplyWorktrees_RecheckEverySelectionGate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, wt, state := s4Repo(t)
	scan := ScanWorktreeReclaim(ctx, s4Input(t, repo, wt, state))
	require.Len(t, scan.Candidates, 3)

	counters := ApplyWorktrees(ctx, hermeticRunner, scan.Candidates, &state, os.Getuid(), func(path string) {
		switch path {
		case wt["a"]:
			// A clean commit: HEAD moves and is no longer merged.
			writeFile(t, filepath.Join(path, "more.txt"), "more\n")
			gitT(t, path, "add", ".")
			gitT(t, path, "commit", "-m", "more")
		case wt["e"]:
			// Fresh activity after the scan's idle cutoff.
			future := time.Now().Add(500 * time.Minute)
			require.NoError(t, os.Chtimes(filepath.Join(path, "tracked.txt"), future, future))
		case wt["f"]:
			// Registered again: the release no longer holds.
			require.NoError(t, RegisterWorktree(ctx, state, path, "agent", time.Now()))
		}
	})
	assert.Equal(t, 3, counters.SkippedChanged)
	assert.Zero(t, counters.Worktrees)
	for _, name := range []string{"a", "e", "f"} {
		assert.DirExists(t, wt[name])
	}
}

// SPEC-DISK-002 review F-15: a registered worktree of another repository is
// reclaimed from a project that is not its repository.
func TestScanWorktreeReclaim_RegisteredWorktreeOfAnotherRepository(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	project, _, _ := ledgerRepo(t)
	otherRepo, otherWT, _ := ledgerRepo(t)
	state := testState(t)
	require.NoError(t, RegisterWorktree(ctx, state, otherWT, "", time.Now()))
	ledger, err := LoadLedger(state)
	require.NoError(t, err)

	scan := ScanWorktreeReclaim(ctx, WorktreeReclaimInput{Run: hermeticRunner, ProjectDir: project, Cwd: project,
		Now: time.Now().Add(400 * time.Minute), IdleMinutes: 360, UID: os.Getuid(), Ledger: ledger})
	assert.Equal(t, []string{otherWT}, candidatePaths(scan.Candidates))
	counters := ApplyWorktrees(ctx, hermeticRunner, scan.Candidates, &state, os.Getuid(), nil)
	assert.Equal(t, 1, counters.Worktrees)
	assert.NotContains(t, gitT(t, otherRepo, "worktree", "list"), otherWT)
}

// SPEC-DISK-002 review r2: a released worktree that detaches and loses its
// branch after the scan is kept, because its commits would become
// unreachable.
func TestApplyWorktrees_ReleasedLosingItsBranchIsKept(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, wt, state := s4Repo(t)
	scan := ScanWorktreeReclaim(ctx, s4Input(t, repo, wt, state))
	var released []ReclaimCandidate
	for _, c := range scan.Candidates {
		if c.Reason == ReasonReleased {
			released = append(released, c)
		}
	}
	require.Len(t, released, 1)
	counters := ApplyWorktrees(ctx, hermeticRunner, released, &state, os.Getuid(), func(path string) {
		gitT(t, path, "checkout", "--detach")
		gitT(t, repo, "branch", "-D", "br-f")
	})
	assert.Equal(t, 1, counters.SkippedChanged)
	assert.DirExists(t, wt["f"])
}
