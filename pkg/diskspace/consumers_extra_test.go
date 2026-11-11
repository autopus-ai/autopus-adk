package diskspace

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sparseFile writes a file of apparent size apparent with only written bytes
// allocated, the shape of a VM disk image.
func sparseFile(t *testing.T, path string, apparent int64, written int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.Create(path)
	require.NoError(t, err)
	_, err = f.Write(make([]byte, written))
	require.NoError(t, err)
	require.NoError(t, f.Truncate(apparent))
	require.NoError(t, f.Close())
}

func byName(sizes []ConsumerSize) map[string]ConsumerSize {
	out := map[string]ConsumerSize{}
	for _, s := range sizes {
		out[s.Name] = s
	}
	return out
}

func TestMeasureExtraConsumers(t *testing.T) {
	t.Parallel()
	home := canonicalTempDir(t)
	sparseFile(t, filepath.Join(home, ".colima", "_lima", "colima", "diffdisk"), 4*GiB, int(2*MiB))
	sparseFile(t, filepath.Join(home, ".lima", "default", "diffdisk"), 2*GiB, int(MiB))
	runner := filepath.Join(home, "runner", "_work")
	a := &aged{now: time.Now()}
	a.file(filepath.Join(runner, "repo", "blob"), 3*MiB, 0)
	require.NoError(t, a.err)
	missing := filepath.Join(home, "absent")
	locked := filepath.Join(home, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	got := byName(MeasureExtraConsumers(ExtraConsumerInput{Home: home, GOOS: "linux", ExtraRoots: []string{runner, missing, locked}, Budget: 5 * time.Second}))

	colima := got[ConsumerColima]
	assert.True(t, colima.Available)
	assert.False(t, colima.Partial)
	assert.GreaterOrEqual(t, colima.Bytes, 2*MiB, "the written part of the image is counted")
	assert.Less(t, colima.Bytes, 64*MiB, "a sparse image counts its allocated, not apparent, size")
	assert.True(t, got[ConsumerLima].Available)
	_, docker := got[ConsumerDockerDesktop]
	assert.False(t, docker, "an absent optional root is left out")

	work := got[ConsumerRootPrefix+runner]
	assert.True(t, work.Available)
	assert.GreaterOrEqual(t, work.Bytes, 3*MiB)
	assert.Equal(t, ReasonRootMissing, got[ConsumerRootPrefix+missing].Reason)
	if os.Getuid() != 0 {
		unreadable := got[ConsumerRootPrefix+locked]
		assert.False(t, unreadable.Available, "an unreadable root is not reported as zero")
		assert.Equal(t, ReasonUnreadable, unreadable.Reason)
	}
}

// Detection is per system and honours the tools' own overrides; nothing
// assumes the machine the test runs on.
func TestMeasureExtraConsumers_DetectsCommonCachesPerSystem(t *testing.T) {
	t.Parallel()
	home := canonicalTempDir(t)
	gomod := filepath.Join(home, "go", "pkg", "mod")
	cargoHome := filepath.Join(home, "custom-cargo")
	for _, dir := range []string{
		gomod, filepath.Join(home, ".npm"), filepath.Join(cargoHome, "registry"), filepath.Join(home, ".gradle", "caches"),
		filepath.Join(home, ".cache", "pip"), filepath.Join(home, "Library", "Caches", "pip"),
		filepath.Join(home, "Library", "Developer", "Xcode", "DerivedData"),
	} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	env := func(key string) string {
		if key == "CARGO_HOME" {
			return cargoHome
		}
		return ""
	}
	names := func(goos string) []string {
		var out []string
		for _, s := range MeasureExtraConsumers(ExtraConsumerInput{Home: home, GOOS: goos, Getenv: env, GoModCache: gomod, Budget: time.Second}) {
			if s.Name != ConsumerDockerRoot {
				out = append(out, s.Name)
			}
		}
		return out
	}
	common := []string{ConsumerGoModCache, ConsumerNpmCache, ConsumerCargoRegistry, ConsumerGradleCache}
	assert.ElementsMatch(t, append(slices.Clone(common), ConsumerPipCache, ConsumerXcodeDerivedData), names("darwin"))
	assert.ElementsMatch(t, append(slices.Clone(common), ConsumerPipCache), names("linux"))
	assert.ElementsMatch(t, common, names("windows"), "an unsupported system measures what it can")
	assert.Empty(t, MeasureExtraConsumers(ExtraConsumerInput{GOOS: "linux"}), "no home, nothing detected")
}

func TestSizeWithin_Bounds(t *testing.T) {
	t.Parallel()
	root := canonicalTempDir(t)
	a := &aged{now: time.Now()}
	for i := 0; i < 600; i++ {
		a.file(filepath.Join(root, "d", "f"+string(rune('a'+i%26))+time.Duration(i).String()), 10, 0)
	}
	require.NoError(t, a.err)
	require.NoError(t, os.Symlink("/", filepath.Join(root, "link")))

	_, partial := sizeWithin(root, time.Time{}, 100)
	assert.True(t, partial, "the entry cap cuts the walk")
	_, partial = sizeWithin(root, time.Now().Add(-time.Second), 1_000_000)
	assert.True(t, partial, "a passed deadline cuts the walk")
	full, partial := sizeWithin(root, time.Now().Add(time.Minute), 1_000_000)
	assert.False(t, partial)
	walked, err := walkAllocated(root, nil)
	require.NoError(t, err)
	assert.Equal(t, walked, full, "an uncut walk matches walkAllocated, the symlink counted as itself")
}

func TestWorktreeUsage_NodeModules(t *testing.T) {
	t.Parallel()
	wt := canonicalTempDir(t)
	a := &aged{now: time.Now()}
	a.file(filepath.Join(wt, "web", "node_modules", "pkg", "index.js"), 2*MiB, 0)
	a.file(filepath.Join(wt, "web", "node_modules", "pkg", "node_modules", "dep", "x.js"), MiB, 0)
	a.file(filepath.Join(wt, "crate", "Cargo.toml"), 10, 0)
	a.file(filepath.Join(wt, "crate", "target", "debug", "bin"), 3*MiB, 0)
	a.file(filepath.Join(wt, "src", "main.go"), 10, 0)
	require.NoError(t, a.err)

	total, cargo, node := worktreeUsage(wt)
	walked, err := walkAllocated(wt, nil)
	require.NoError(t, err)
	assert.Equal(t, walked, total, "nothing is counted twice")
	assert.GreaterOrEqual(t, node, 3*MiB, "nested node_modules are inside the first one")
	assert.Less(t, node, 4*MiB)
	assert.GreaterOrEqual(t, cargo, 3*MiB)
}

func TestScanScratchpads_LargeFiles(t *testing.T) {
	t.Parallel()
	root := canonicalTempDir(t)
	sparseFile(t, filepath.Join(root, "slug", "session", "backup.img"), GiB+1, 4096)
	sparseFile(t, filepath.Join(root, "slug", "session", "small.img"), GiB-1, 4096)
	scan := ScanScratchpads(root, time.Now())
	require.Len(t, scan.LargeFiles, 1)
	assert.Equal(t, filepath.Join(root, "slug", "session", "backup.img"), scan.LargeFiles[0].Path)
}
