package diskspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ledgerRepo is a repository with one linked worktree, and a plain directory.
func ledgerRepo(t *testing.T) (repo, wt, plain string) {
	t.Helper()
	base := canonicalTempDir(t)
	repo = filepath.Join(base, "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitT(t, repo, "init", "--initial-branch=main")
	writeFile(t, filepath.Join(repo, "f.txt"), "seed\n")
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-m", "seed")
	wt = filepath.Join(base, "wt")
	gitT(t, repo, "worktree", "add", "-b", "feat", wt, "main")
	plain = filepath.Join(base, "plain")
	require.NoError(t, os.MkdirAll(plain, 0o755))
	return repo, wt, plain
}

func testState(t *testing.T) StateDir {
	t.Helper()
	state, err := OpenStateDir(StateDirPath(canonicalTempDir(t), ""), os.Getuid())
	require.NoError(t, err)
	return state
}

// SPEC-DISK-002 S3: only the top of a linked worktree resolves.
func TestResolveLinkedWorktree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, wt, plain := ledgerRepo(t)

	got, err := ResolveLinkedWorktree(ctx, hermeticRunner, wt)
	require.NoError(t, err)
	assert.Equal(t, wt, got)

	writeFile(t, filepath.Join(wt, "sub", "x"), "x")
	for _, dir := range []string{repo, plain, filepath.Join(wt, "sub"), filepath.Join(wt, "missing")} {
		_, err := ResolveLinkedWorktree(ctx, hermeticRunner, dir)
		require.ErrorIs(t, err, ErrNotLinkedWorktree, dir)
	}
}

// SPEC-DISK-002 S3: register twice keeps one entry; release marks it; a
// recreated path makes the entry stale and the next write drops it.
func TestLedger_RegisterReleaseStale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, wt, _ := ledgerRepo(t)
	state := testState(t)
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

	require.NoError(t, RegisterWorktree(ctx, state, wt, "agent-1", now))
	require.NoError(t, RegisterWorktree(ctx, state, wt, "agent-1\x1b[31m", now.Add(time.Minute)))
	entries, err := LoadLedger(state)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "agent-1[31m", entries[0].Owner, "control characters are dropped")
	assert.True(t, entries[0].Live())
	assert.False(t, entries[0].Released())

	found, err := ReleaseWorktree(ctx, state, wt, now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.True(t, found)
	entries, _ = LoadLedger(state)
	require.True(t, entries[0].Released())

	found, err = ReleaseWorktree(ctx, state, filepath.Join(wt, "nope"), now)
	require.NoError(t, err)
	assert.False(t, found)

	gitT(t, repo, "worktree", "remove", wt)
	gitT(t, repo, "worktree", "add", "-b", "feat2", wt, "main")
	entries, _ = LoadLedger(state)
	require.Len(t, entries, 1)
	// Some filesystems (ext4) hand the recreated directory the freed inode
	// number; emulate that so the identity token alone must tell them apart.
	reuseInode(t, state, wt)
	entries, _ = LoadLedger(state)
	assert.False(t, entries[0].Live(), "a recreated path is a different directory")

	other := filepath.Join(filepath.Dir(wt), "wt2")
	gitT(t, repo, "worktree", "add", "-b", "feat3", other, "main")
	require.NoError(t, RegisterWorktree(ctx, state, other, "", now))
	entries, _ = LoadLedger(state)
	require.Len(t, entries, 1, "the stale entry was dropped on write")
	assert.Equal(t, other, entries[0].Path)

	require.NoError(t, ForgetWorktree(ctx, state, other))
	entries, _ = LoadLedger(state)
	assert.Empty(t, entries)
}

// reuseInode rewrites every ledger entry of path to the device and inode
// path has now, as a filesystem that reuses freed inode numbers would.
func reuseInode(t *testing.T, state StateDir, path string) {
	t.Helper()
	entries, err := LoadLedger(state)
	require.NoError(t, err)
	id := pathID(path)
	for i := range entries {
		if entries[i].Path == path {
			entries[i].Dev, entries[i].Ino = id.Dev, id.Ino
		}
	}
	data, err := json.Marshal(ledgerDoc{Version: ledgerVersion, Entries: entries})
	require.NoError(t, err)
	require.NoError(t, state.WriteFileAtomic(ledgerFile, data))
}

// SPEC-DISK-002 S3: a corrupted ledger is refused and left byte-identical.
func TestLedger_CorruptedIsNeverOverwritten(t *testing.T) {
	t.Parallel()
	_, wt, _ := ledgerRepo(t)
	state := testState(t)
	require.NoError(t, os.WriteFile(state.Join(ledgerFile), []byte("{"), 0o600))

	err := RegisterWorktree(context.Background(), state, wt, "", time.Now())
	require.ErrorIs(t, err, ErrLedgerUnreadable)
	data, readErr := os.ReadFile(state.Join(ledgerFile))
	require.NoError(t, readErr)
	assert.Equal(t, "{", string(data))
}

// SPEC-DISK-002 S3: concurrent registers of different worktrees both land.
func TestLedger_ConcurrentRegistersSerialize(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, wt, _ := ledgerRepo(t)
	others := []string{wt}
	for i := range 5 {
		path := filepath.Join(filepath.Dir(wt), "c"+string(rune('a'+i)))
		gitT(t, repo, "worktree", "add", "-b", "c"+string(rune('a'+i)), path, "main")
		others = append(others, path)
	}
	state := testState(t)
	var wg sync.WaitGroup
	for _, path := range others {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, RegisterWorktree(ctx, state, path, "", time.Now()))
		}()
	}
	wg.Wait()
	entries, err := LoadLedger(state)
	require.NoError(t, err)
	assert.Len(t, entries, len(others))
}

// cleanOwner keeps at most 200 bytes on a rune boundary.
func TestCleanOwner(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("가", 100)
	got := cleanOwner(long)
	assert.LessOrEqual(t, len(got), ledgerOwnerMax)
	assert.True(t, strings.HasPrefix(long, got))
	assert.Equal(t, "ab", cleanOwner("a\nb"))
}

// SPEC-DISK-002 review F-02: a worktree another uid owns cannot be registered.
func TestLedger_RefusesForeignOwnedWorktree(t *testing.T) {
	t.Parallel()
	_, wt, _ := ledgerRepo(t)
	foreign := testState(t)
	foreign.uid = os.Getuid() + 1
	err := RegisterWorktree(context.Background(), foreign, wt, "", time.Now())
	require.ErrorIs(t, err, ErrNotOwnedWorktree)
}
