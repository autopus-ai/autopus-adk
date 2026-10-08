//go:build unix

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

func TestLocalPatchPrepare_Ready_RecordsAndWorktree(t *testing.T) {
	f := newLPFixture(t, nil)
	before := f.git(f.repo, "status", "--porcelain")
	s := f.patcher.prepare(context.Background(), f.target())
	require.True(t, s.ready(), "code %q", s.code)
	assert.Equal(t, []string{"prep", "stage:worktree_intent", "stage:worktree_done"}, f.ledger.trail())
	worktree := filepath.Join(f.lp(), lpKey, "worktree")
	prep := f.ledger.prep()
	assert.Equal(t, healthband.LocalPatchRecord{
		Kind: healthband.LocalPatchKindPrep, Series: lpSeries, EpisodeID: "e1042", DiagnoseClaimID: lpDiagnoseClaimID,
		LeaseUntil: lpT0.Add(990 * time.Second), ClaimID: lpPatchClaimID, Key: lpKey, BaseSHA: f.base, Code: "ok",
	}, lpUnsealed(prep))
	assert.Equal(t, worktree, f.ledger.stage(healthband.StageWorktreeIntent).Path)
	empty := sha256.Sum256(nil)
	assert.Equal(t, hex.EncodeToString(empty[:]), f.ledger.stage(healthband.StageWorktreeDone).StatusSHA256)
	assert.Equal(t, worktree, s.worktree)
	data, err := os.ReadFile(filepath.Join(worktree, "pkg", "foo", "foo.go"))
	require.NoError(t, err)
	assert.Equal(t, lpFooGo, string(data))
	assert.Equal(t, f.base+"\n", f.git(worktree, "rev-parse", "HEAD"))
	assert.Equal(t, before, f.git(f.repo, "status", "--porcelain"))
	info, err := os.Lstat(f.lp())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	assert.False(t, f.absent(lpKey+".lock"), "the key lock is held until the last result")
	f.patcher.release(s)
	assert.True(t, f.absent(lpKey+".lock"), "the key lock is unlinked after the result")
}

func TestLocalPatchPrepare_StepOneCodes(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f *lpFixture)
		want  string
	}{
		{"default branch missing", func(f *lpFixture) { f.patcher.defaultBranch = "release" }, "base_unavailable"},
		{"default branch fails check-ref-format", func(f *lpFixture) { f.patcher.defaultBranch = "bad..name" }, "base_unavailable"},
		{"existing key directory", func(f *lpFixture) {
			require.NoError(t, os.MkdirAll(filepath.Join(f.lp(), lpKey), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(f.lp(), lpKey, "user.txt"), []byte("mine"), 0o600))
		}, "artifact_exists"},
		{"existing patch file", func(f *lpFixture) {
			require.NoError(t, os.WriteFile(filepath.Join(f.lp(), lpKey+".patch"), []byte("mine"), 0o600))
		}, "artifact_exists"},
		{"existing branch", func(f *lpFixture) { f.git(f.repo, "branch", "autopus/band/"+lpKey) }, "artifact_exists"},
		{"existing symbolic ref", func(f *lpFixture) {
			f.git(f.repo, "symbolic-ref", "refs/heads/autopus/band/"+lpKey, "refs/heads/main")
		}, "artifact_exists"},
		{"five kept keys", func(f *lpFixture) {
			for _, key := range []string{"k1", "k2", "k3"} {
				require.NoError(t, os.MkdirAll(filepath.Join(f.lp(), key), 0o700))
			}
			for _, key := range []string{"k4", "k5", "k1"} {
				require.NoError(t, os.WriteFile(filepath.Join(f.lp(), key+".patch"), nil, 0o600))
			}
		}, "cap_reached"},
		{"unsafe filter in the user's checkout", func(f *lpFixture) { f.git(f.repo, "config", "filter.mark.clean", "tools/clean.py") },
			"git_config_unsafe:filter.mark.clean"},
		{"free space one block short", func(f *lpFixture) {
			// Two one-block blobs plus 512 MiB need 131,074 blocks of 4096.
			f.patcher.statfs = func(string) (lpDiskSpace, error) { return lpDiskSpace{avail: 131_073, unit: 4096}, nil }
		}, "disk_insufficient"},
		{"no <lp> location", func(f *lpFixture) { f.patcher.location = nil }, "cache_unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLPFixture(t, nil)
			lp := f.lp()
			tc.setup(f)
			s := f.patcher.prepare(context.Background(), f.target())
			assert.False(t, s.ready())
			assert.Equal(t, tc.want, s.code)
			assert.Equal(t, []string{"prep"}, f.ledger.trail())
			assert.Equal(t, tc.want, f.ledger.prep().Code)
			_, err := os.Lstat(filepath.Join(lp, lpKey, "worktree"))
			assert.True(t, os.IsNotExist(err), "no worktree add")
			f.patcher.release(s)
		})
	}
}

func TestLocalPatchPrepare_FreeSpaceExactlyEnough_Passes(t *testing.T) {
	f := newLPFixture(t, nil)
	f.patcher.statfs = func(string) (lpDiskSpace, error) { return lpDiskSpace{avail: 131_074, unit: 4096}, nil }
	s := f.patcher.prepare(context.Background(), f.target())
	assert.True(t, s.ready(), "code %q", s.code)
	f.patcher.release(s)
}

func TestLPTreeSizes_Bounds(t *testing.T) {
	t.Parallel()
	record := "100644 blob 0123456789abcdef0123456789abcdef01234567       1\tf\x00"
	sizes, ok := lpTreeSizes([]byte(strings.Repeat(record, 1000)))
	require.True(t, ok)
	assert.Equal(t, uint64(132_072), lpBlocksNeeded(sizes, 4096), "S3: 1,000 one-byte blobs need 512 MiB + 1,000 blocks")
	_, ok = lpTreeSizes([]byte(strings.Repeat(record, lpMaxEntries+1)))
	assert.False(t, ok, "200,001 entries are worktree_too_large")
	big := "100644 blob 0123456789abcdef0123456789abcdef01234567 2147483649\tbig\x00"
	_, ok = lpTreeSizes([]byte(big))
	assert.False(t, ok, "more than 2 GiB is worktree_too_large")
	gitlink := "160000 commit 0123456789abcdef0123456789abcdef01234567       -\tsub\x00"
	sizes, ok = lpTreeSizes([]byte(gitlink))
	assert.True(t, ok)
	assert.Equal(t, []uint64{0}, sizes)
	_, ok = lpTreeSizes([]byte("garbage\x00"))
	assert.False(t, ok)
}

func TestLocalPatchPrepare_ResultFoundByReRead_StopsWithoutRecord(t *testing.T) {
	f := newLPFixture(t, nil)
	ended := healthband.NewLocalPatchResult(lpPatchClaimID, healthband.LocalPatchCodeInterrupted)
	ended.Recovered = true
	appended, err := f.ledger.store.AppendLocalPatchResult(context.Background(), ended)
	require.NoError(t, err)
	require.True(t, appended)
	s := f.patcher.prepare(context.Background(), f.target())
	assert.True(t, s.stopped)
	assert.False(t, s.ready())
	assert.Equal(t, []string{"result"}, f.ledger.trail(), "no prep or stage record")
	assert.True(t, f.absent(lpKey+".lock"), "the key lock file is unlinked")
	assert.True(t, f.absent(lpKey), "no worktree")
	_, written := f.patcher.patch(context.Background(), s, localPatchInput{})
	assert.False(t, written)
	assert.Equal(t, []string{"result"}, f.ledger.trail())
}

func TestLocalPatchPrepare_KeyLockHeldByAnotherProcess_Stops(t *testing.T) {
	f := newLPFixture(t, nil)
	held, err := filelock.Acquire(context.Background(), filepath.Join(f.lp(), lpKey+".lock"), 0)
	require.NoError(t, err)
	defer func() { _ = held.Unlock() }()
	// A lock file present at step 1 is an artifact (checked before the lock).
	s := f.patcher.prepare(context.Background(), f.target())
	assert.Equal(t, healthband.LocalPatchCodeArtifactExists, s.code)
	assert.Equal(t, []string{"prep"}, f.ledger.trail())
	assert.True(t, f.absent(lpKey), "no worktree")

	// A lock taken between that check and the key lock stops the claim.
	dir, code, err := f.patcher.location.Open(context.Background(), f.patcher.git.In(f.repo))
	require.NoError(t, err)
	require.Empty(t, code)
	raced := &localPatchSetup{target: f.target(), key: lpKey, dir: dir}
	assert.Empty(t, f.patcher.lockKey(context.Background(), raced))
	assert.True(t, raced.stopped)
	assert.Nil(t, raced.lock)
	f.patcher.release(raced)
}

func TestLocalPatchPrepare_RecordFaults(t *testing.T) {
	f := newLPFixture(t, nil)
	f.ledger.failPrep = true
	s := f.patcher.prepare(context.Background(), f.target())
	assert.Equal(t, healthband.LocalPatchCodeRecordUnavailable, s.code)
	assert.False(t, s.prepped)
	f.patcher.release(s)

	f = newLPFixture(t, nil)
	f.ledger.failPhase = healthband.StageWorktreeIntent
	s = f.patcher.prepare(context.Background(), f.target())
	assert.Equal(t, healthband.LocalPatchCodeRecordUnavailable, s.code)
	assert.Equal(t, []string{"prep"}, f.ledger.trail())
	assert.True(t, f.absent(lpKey), "no artifact without its intent record")
	f.patcher.release(s)
}

func TestLocalPatchPrepare_LeaseShort_LeaseExhausted(t *testing.T) {
	f := newLPFixture(t, nil)
	f.patcher.now = func() time.Time { return lpT0.Add(900 * time.Second) } // 90 s left of 990 s
	s := f.patcher.prepare(context.Background(), f.target())
	assert.Equal(t, healthband.LocalPatchCodeLeaseExhausted, s.code)
	assert.Empty(t, f.ledger.trail())
	result, written := f.patcher.patch(context.Background(), s, localPatchInput{})
	require.True(t, written)
	assert.Equal(t, "failed:lease_exhausted", result.Status)
}
