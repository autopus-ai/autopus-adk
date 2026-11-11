package diskspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const lockStart = "Mon Nov  2 16:22:03 2026"

func agentReason(id string, pid string) string {
	return "claude agent agent-" + id + " (pid " + pid + " start " + lockStart + ")"
}

// wideRepo builds a repository whose default remote branch is origin/main,
// with linked worktrees outside and inside the harness directories.
func wideRepo(t *testing.T) (string, map[string]string) {
	t.Helper()
	base := canonicalTempDir(t)
	repo := filepath.Join(base, "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitT(t, repo, "init", "--initial-branch=main")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "seed\n")
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-m", "seed")
	gitT(t, repo, "update-ref", "refs/remotes/origin/main", "main")
	wt := map[string]string{}
	for _, name := range []string{"merged", "dirty", "unmerged", "held"} {
		wt[name] = filepath.Join(base, "elsewhere", name)
	}
	for _, name := range []string{"dead", "deaddirty", "deadunmerged", "live", "liveheld", "liveunmerged", "busy"} {
		wt[name] = filepath.Join(repo, ".claude", "worktrees", "agent-"+name)
	}
	for name, path := range wt {
		gitT(t, repo, "worktree", "add", "-b", "br-"+name, path, "main")
	}
	writeFile(t, filepath.Join(wt["dirty"], "untracked.txt"), "u\n")
	writeFile(t, filepath.Join(wt["deaddirty"], "untracked.txt"), "u\n")
	for _, name := range []string{"unmerged", "deadunmerged", "liveunmerged"} {
		writeFile(t, filepath.Join(wt[name], "wip.txt"), "wip\n")
		gitT(t, wt[name], "add", ".")
		gitT(t, wt[name], "commit", "-m", "wip")
	}
	for _, name := range []string{"dead", "deaddirty", "deadunmerged"} {
		gitT(t, repo, "worktree", "lock", "--reason", agentReason(name, "111"), wt[name])
	}
	for _, name := range []string{"live", "liveheld", "liveunmerged"} {
		gitT(t, repo, "worktree", "lock", "--reason", agentReason(name, "222"), wt[name])
	}
	gitT(t, repo, "worktree", "lock", "--reason", "busy", wt["busy"])
	return repo, wt
}

func fakeAlive(_ context.Context, pid int, _ string) bool { return pid == 222 }

func wideInput(repo string, wt map[string]string, now time.Time, cwds CwdLister) WorktreeReclaimInput {
	return WorktreeReclaimInput{Run: hermeticRunner, ProjectDir: repo, Cwd: repo, Now: now, IdleMinutes: 360,
		UID: os.Getuid(), Cwds: cwds, Alive: fakeAlive}
}

func reasons(cands []ReclaimCandidate) map[string]string {
	out := map[string]string{}
	for _, c := range cands {
		out[c.Path] = c.Reason
	}
	return out
}

func TestScanWorktreeReclaim_WideRules(t *testing.T) {
	t.Parallel()
	repo, wt := wideRepo(t)
	holding := func(context.Context) ([]string, error) {
		return []string{filepath.Join(wt["held"], "sub"), wt["liveheld"], "/"}, nil
	}
	later := time.Now().Add(400 * time.Minute)

	scan := ScanWorktreeReclaim(context.Background(), wideInput(repo, wt, later, holding))
	assert.Equal(t, map[string]string{
		wt["merged"]: ReasonMergedLinked,
		wt["dead"]:   ReasonAgentDead,
		wt["live"]:   ReasonAgentIdle,
	}, reasons(scan.Candidates))
	assert.Equal(t, map[string]int{SkipDirty: 2, SkipNotOwned: 1, SkipCwd: 2, SkipUnmerged: 2, SkipLocked: 1}, scan.Skipped)
	for _, c := range scan.Candidates {
		assert.Equal(t, c.Reason != ReasonMergedLinked, c.Unlock, "only agent worktrees are unlocked: %s", c.Path)
	}

	t.Run("unknown cwds keep what needs them", func(t *testing.T) {
		failing := func(context.Context) ([]string, error) { return nil, errors.New("no lsof") }
		scan := ScanWorktreeReclaim(context.Background(), wideInput(repo, wt, later, failing))
		assert.Equal(t, map[string]string{wt["dead"]: ReasonAgentDead}, reasons(scan.Candidates))
		assert.Equal(t, 4, scan.Skipped[SkipCwdUnknown], "merged, held, live, and liveheld")
	})
	t.Run("a live agent is kept until idle", func(t *testing.T) {
		none := func(context.Context) ([]string, error) { return []string{"/"}, nil }
		scan := ScanWorktreeReclaim(context.Background(), wideInput(repo, wt, time.Now(), none))
		assert.Equal(t, map[string]string{wt["dead"]: ReasonAgentDead}, reasons(scan.Candidates),
			"a dead agent needs no idle wait; a fresh live one, merged by definition, is kept")
		assert.Equal(t, 4, scan.Skipped[SkipRecent])
	})
}

func TestApplyWorktrees_WideRulesRemoveAndKeepBranches(t *testing.T) {
	t.Parallel()
	repo, wt := wideRepo(t)
	none := func(context.Context) ([]string, error) { return []string{"/"}, nil }
	ctx := context.Background()
	scan := ScanWorktreeReclaim(ctx, wideInput(repo, wt, time.Now().Add(400*time.Minute), none))
	require.Len(t, scan.Candidates, 5, "merged, held, dead, live, liveheld")

	// A process enters "held" between the scan and its removal.
	entered := func(context.Context) ([]string, error) { return []string{wt["held"]}, nil }
	for i := range scan.Candidates {
		if scan.Candidates[i].Path == wt["held"] {
			scan.Candidates[i].cwds = entered
		}
	}
	counters := ApplyWorktrees(ctx, hermeticRunner, scan.Candidates, nil, os.Getuid(), nil)
	assert.Equal(t, 4, counters.Worktrees, counters.Failures)
	assert.Equal(t, 1, counters.SkippedChanged)
	for _, name := range []string{"merged", "dead", "live", "liveheld"} {
		assert.NoDirExists(t, wt[name])
		gitT(t, repo, "rev-parse", "--verify", "refs/heads/br-"+name)
	}
	for _, name := range []string{"held", "deaddirty", "deadunmerged", "liveunmerged", "busy", "dirty"} {
		assert.DirExists(t, wt[name])
	}
}

func TestParseAgentLock(t *testing.T) {
	t.Parallel()
	pid, start, ok := parseAgentLock(agentReason("a98567d9afc9d5072", "4450"))
	require.True(t, ok)
	assert.Equal(t, 4450, pid)
	assert.Equal(t, lockStart, start)
	for _, reason := range []string{"", "busy", "claude agent agent-x (pid 0 start t)", "claude agent agent-x (pid abc start t)"} {
		_, _, ok := parseAgentLock(reason)
		assert.False(t, ok, reason)
	}
}

func TestSystemAgentAlive(t *testing.T) {
	t.Parallel()
	ps := func(out string, err error) Runner {
		return func(context.Context, string, []string, string, ...string) ([]byte, error) { return []byte(out), err }
	}
	self := os.Getpid()
	assert.True(t, SystemAgentAlive(ps(" "+lockStart+"\n", nil))(context.Background(), self, lockStart))
	assert.False(t, SystemAgentAlive(ps("Tue Nov  3 01:00:00 2026", nil))(context.Background(), self, lockStart),
		"a reused pid with another start time is not the agent")
	assert.True(t, SystemAgentAlive(ps("", errors.New("no ps")))(context.Background(), self, lockStart),
		"an unreadable start time keeps the worktree")
}

func TestParseLsofNames(t *testing.T) {
	t.Parallel()
	got := parseLsofNames([]byte("p1\nfcwd\nn/\np2\nfcwd\nn/Users/x/wt\nnrelative\n"))
	assert.Equal(t, []string{"/", "/Users/x/wt"}, got)
	assert.True(t, holdsCwd("/Users/x", []string{"/Users/x/wt"}))
	assert.False(t, holdsCwd("/Users/x/w", []string{"/Users/x/wt"}))
}
