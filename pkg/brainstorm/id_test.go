package brainstorm_test

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/filelock"
)

// perUserCache points the per-user cache directory at a fresh temp dir.
func perUserCache(t *testing.T) (string, brainstorm.Options) {
	t.Helper()
	dir := t.TempDir()
	return dir, brainstorm.Options{CacheDir: func() (string, error) { return dir, nil }}
}

func write(t *testing.T, base, start string, opts brainstorm.Options) brainstorm.Result {
	t.Helper()
	result, err := brainstorm.Write(context.Background(), filepath.Join(base, filepath.FromSlash(start)), o2Request(), opts)
	require.NoError(t, err, start)
	return result
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(data)
}

// S9: each start on a fresh copy gets one above the highest BS-BAND ID of
// its scope, the file lands in the start's own brainstorms directory, and
// the per-user lock is the only lock file.
func TestWrite_S9AllocatesAboveTheHighestIDInEveryTopology(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, top, start, want string
		build                  func(*testing.T) string
		newDirs                []string
	}{
		{name: "single repo", top: "P", start: "P", want: "P/.autopus/brainstorms/BS-BAND-005.md", build: topologyP},
		{name: "module of a meta root", top: "W", start: "W/module", want: "W/module/.autopus/brainstorms/BS-BAND-013.md", build: topologyW},
		{name: "meta root itself", top: "W", start: "W", want: "W/.autopus/brainstorms/BS-BAND-013.md", build: topologyW},
		{name: "project no repository lists", top: "H", start: "H/work/proj", want: "H/work/proj/.autopus/brainstorms/BS-BAND-003.md", build: topologyH},
		{name: "meta root inside an outer repository", top: "O", start: "O/W2", want: "O/W2/.autopus/brainstorms/BS-BAND-021.md",
			build: topologyO, newDirs: []string{"O/W2/.autopus", "O/W2/.autopus/brainstorms"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := tc.build(t)
			requireProbeA3(t, base, tc.top)
			cache, opts := perUserCache(t)
			before := snapshot(t, base)

			result := write(t, base, tc.start, opts)

			assert.Equal(t, tc.want, rel(t, base, result.Path)[0])
			assert.Equal(t, strings.TrimSuffix(path.Base(tc.want), ".md"), result.ID)
			assert.Equal(t, append(tc.newDirs, tc.want), added(before, snapshot(t, base)), "only the BS appears in the fixture")
			assert.Equal(t, []string{"autopus", "autopus/bs-band.lock"}, snapshot(t, cache), "the only lock is the per-user one")
			content := readFile(t, result.Path)
			assert.True(t, strings.HasPrefix(content, "# "+result.ID+": ci.failure_rate:CI tier 2 anomaly (e1042)\n"))
			assert.Empty(t, brainstorm.Validate([]byte(content)))
		})
	}
}

// S10: the --from-idea lookup glob */.autopus/brainstorms/<ID>.md from the
// meta root resolves to the module's BS.
func TestWrite_S10ModuleBSIsFoundFromTheMetaRoot(t *testing.T) {
	t.Parallel()
	base := topologyW(t)
	_, opts := perUserCache(t)

	result := write(t, base, "W/module", opts)

	found, err := filepath.Glob(filepath.Join(base, "W", "*", ".autopus", "brainstorms", "BS-BAND-013.md"))
	require.NoError(t, err)
	assert.Equal(t, []string{result.Path}, found)
}

// S9: nested repositories share one scope, so sequential starts in N/m/x
// and N/m continue one sequence.
func TestWrite_S9NestedStartsContinueOneSequence(t *testing.T) {
	t.Parallel()
	base := topologyN(t)
	requireProbeA3(t, base, "N")
	cache, opts := perUserCache(t)
	before := snapshot(t, base)

	first := write(t, base, "N/m/x", opts)
	second := write(t, base, "N/m", opts)

	assert.Equal(t, "N/m/x/.autopus/brainstorms/BS-BAND-011.md", rel(t, base, first.Path)[0])
	assert.Equal(t, "N/m/.autopus/brainstorms/BS-BAND-012.md", rel(t, base, second.Path)[0])
	assert.Equal(t, []string{"N/m/.autopus/brainstorms/BS-BAND-012.md", "N/m/x/.autopus", "N/m/x/.autopus/brainstorms",
		"N/m/x/.autopus/brainstorms/BS-BAND-011.md"}, added(before, snapshot(t, base)), "no lock file inside N")
	assert.Equal(t, []string{"autopus", "autopus/bs-band.lock"}, snapshot(t, cache))
}

// S9: a BS-BAND-013.md created between the scan and the create moves the
// writer to 014 and stays byte-identical.
func TestWrite_S9CollisionAfterTheScanTakesTheNextID(t *testing.T) {
	t.Parallel()
	base := topologyW(t)
	_, opts := perUserCache(t)
	calls := 0
	opts = brainstorm.WithBeforeCreate(opts, func(target string) {
		if calls++; calls == 1 {
			require.NoError(t, os.WriteFile(target, []byte("written by hand\n"), 0o644))
		}
	})

	result := write(t, base, "W/module", opts)

	assert.Equal(t, "BS-BAND-014", result.ID)
	assert.Equal(t, "written by hand\n", readFile(t, filepath.Join(base, "W", "module", ".autopus", "brainstorms", "BS-BAND-013.md")))
	assert.True(t, strings.HasPrefix(readFile(t, result.Path), "# BS-BAND-014: "), "line 1 names the ID it got")
}

// S9: five consecutive collisions record bs_id_exhausted and write no file.
func TestWrite_S9FiveConsecutiveCollisionsExhaustTheIDs(t *testing.T) {
	t.Parallel()
	base := topologyP(t)
	_, opts := perUserCache(t)
	var tried []string
	opts = brainstorm.WithBeforeCreate(opts, func(target string) {
		tried = append(tried, rel(t, base, target)[0])
		require.NoError(t, os.WriteFile(target, []byte("taken\n"), 0o644))
	})
	before := snapshot(t, base)

	_, err := brainstorm.Write(context.Background(), filepath.Join(base, "P"), o2Request(), opts)

	require.ErrorIs(t, err, brainstorm.ErrIDExhausted)
	assert.Equal(t, "bs_id_exhausted", brainstorm.Reason(err))
	require.Len(t, tried, brainstorm.MaxIDAttempts)
	assert.Equal(t, "P/.autopus/brainstorms/BS-BAND-005.md", tried[0])
	assert.Equal(t, "P/.autopus/brainstorms/BS-BAND-009.md", tried[4])
	assert.Equal(t, tried, added(before, snapshot(t, base)))
	for _, name := range tried {
		assert.Equal(t, "taken\n", readFile(t, filepath.Join(base, name)), "the writer wrote none of them")
	}
}

// BS Root Resolution item 4: the allocation waits at most LockWait for the
// per-user lock; a timeout writes nothing and names bs_lock_timeout.
func TestWrite_LockTimeoutWritesNoBS(t *testing.T) {
	t.Parallel()
	require.Equal(t, 30*time.Second, brainstorm.LockWait)
	base := topologyP(t)
	cache, opts := perUserCache(t)
	holder, err := filelock.Acquire(context.Background(), filepath.Join(cache, "autopus", "bs-band.lock"), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, holder.Unlock()) })
	opts.LockWait = 100 * time.Millisecond
	before := snapshot(t, base)

	started := time.Now()
	_, err = brainstorm.Write(context.Background(), filepath.Join(base, "P"), o2Request(), opts)

	require.ErrorIs(t, err, brainstorm.ErrLockTimeout)
	assert.Equal(t, "bs_lock_timeout", brainstorm.Reason(err))
	assert.GreaterOrEqual(t, time.Since(started), 100*time.Millisecond)
	assert.Empty(t, added(before, snapshot(t, base)))
}

// Without an options override the lock sits in os.UserCacheDir().
func TestWrite_DefaultLockIsInTheUserCacheDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg-cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "local-app-data"))
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	base := topologyP(t)

	write(t, base, "P", brainstorm.Options{})

	assert.FileExists(t, filepath.Join(cache, "autopus", "bs-band.lock"))
}

// BS Root Resolution item 4: without a usable user cache directory the
// lock is <root>/.autopus/brainstorms/.bs-band.lock.
func TestWrite_FallbackLockSitsInTheRootBrainstorms(t *testing.T) {
	t.Parallel()
	for name, cacheDir := range map[string]func() (string, error){
		"unavailable": func() (string, error) { return "", os.ErrNotExist },
		"relative":    func() (string, error) { return "cache", nil },
	} {
		base := topologyW(t)

		result := write(t, base, "W/module", brainstorm.Options{CacheDir: cacheDir})

		assert.Equal(t, "BS-BAND-013", result.ID, name)
		assert.FileExists(t, filepath.Join(base, "W", ".autopus", "brainstorms", ".bs-band.lock"), name)
	}
}

// The scan from the root covers depth 8; a deeper start is refused before
// any lock or file is written.
func TestWrite_ScanLimitCoversDepthEightAndRefusesDeeperStarts(t *testing.T) {
	t.Parallel()
	base, dirs := chain(t, 9)
	deep := filepath.Join(base, filepath.FromSlash(dirs[8]), ".autopus", "brainstorms")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(deep, "BS-BAND-030.md"), []byte("existing\n"), 0o644))
	cache, opts := perUserCache(t)

	assert.Equal(t, "BS-BAND-031", write(t, base, dirs[0], opts).ID, "the root sees depth 8")

	before, locks := snapshot(t, base), snapshot(t, cache)
	_, err := brainstorm.Write(context.Background(), filepath.Join(base, filepath.FromSlash(dirs[9])), o2Request(), opts)
	require.ErrorIs(t, err, brainstorm.ErrScopeTooDeep)
	assert.Equal(t, "bs_scope_too_deep", brainstorm.Reason(err))
	assert.Empty(t, added(before, snapshot(t, base)))
	assert.Equal(t, locks, snapshot(t, cache))
}

// A request outside the contract is refused before the lock.
func TestWrite_InvalidRequestTakesNoLock(t *testing.T) {
	t.Parallel()
	base := topologyP(t)
	cache, opts := perUserCache(t)
	req := o2Request()
	req.EpisodeID = "not an episode"

	_, err := brainstorm.Write(context.Background(), filepath.Join(base, "P"), req, opts)

	require.ErrorIs(t, err, brainstorm.ErrInvalidRequest)
	assert.Equal(t, "bs_invalid_request", brainstorm.Reason(err))
	assert.Empty(t, snapshot(t, cache))
}

// A symlinked brainstorms directory would redirect the write outside the
// project; it is refused and nothing lands behind it.
func TestWrite_RefusesASymlinkedBrainstormsDirectory(t *testing.T) {
	t.Parallel()
	base := topologyP(t)
	target := filepath.Join(base, "P", ".autopus", "brainstorms")
	elsewhere := t.TempDir()
	require.NoError(t, os.RemoveAll(target))
	if err := os.Symlink(elsewhere, target); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, opts := perUserCache(t)

	_, err := brainstorm.Write(context.Background(), filepath.Join(base, "P"), o2Request(), opts)

	require.Error(t, err)
	assert.Equal(t, "bs_write_failed", brainstorm.Reason(err))
	assert.Empty(t, snapshot(t, elsewhere))
	assert.Empty(t, brainstorm.Reason(nil))
}
