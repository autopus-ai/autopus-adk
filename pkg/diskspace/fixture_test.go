package diskspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// blockTolerance is the acceptance tolerance on a fixture byte total: one
// 4096-byte block per fixture file.
const blockTolerance = 4096

// sharedBase holds the fixtures read-only tests share; TestMain removes it.
var sharedBase string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "diskspace-shared-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sharedBase, _ = filepath.EvalSymlinks(dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// s5Fixture is the cache root R and work dir root W of acceptance S5, both
// canonical, so a path the code prints equals the path built here.
type s5Fixture struct {
	now     time.Time
	cache   string
	work    string
	outside string
	h       [11]string // h[1]..h[10]: the <hN> names
}

var (
	sharedOnce sync.Once
	shared     s5Fixture
	sharedErr  error
)

// sharedS5 is one S5 fixture for tests that only read it.
func sharedS5(t *testing.T) s5Fixture {
	t.Helper()
	sharedOnce.Do(func() { shared, sharedErr = buildS5(filepath.Join(sharedBase, "s5")) })
	require.NoError(t, sharedErr)
	return shared
}

func hexName(prefix string, n int) string { return fmt.Sprintf("%s%062x", prefix, n) }

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

// aged writes fully allocated files and stamps mtimes relative to now.
type aged struct {
	now time.Time
	err error
}

func (a *aged) file(path string, size int64, age time.Duration) {
	if a.err == nil {
		a.err = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if a.err == nil {
		a.err = os.WriteFile(path, bytes.Repeat([]byte{'x'}, int(size)), 0o644)
	}
	a.stamp(path, age)
}

func (a *aged) stamp(path string, age time.Duration) {
	if a.err == nil {
		a.err = os.Chtimes(path, a.now.Add(-age), a.now.Add(-age))
	}
}

func (a *aged) link(target, path string) {
	if a.err == nil {
		a.err = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if a.err == nil {
		a.err = os.Symlink(target, path)
	}
}

func buildS5(base string) (s5Fixture, error) {
	f := s5Fixture{now: time.Now().Truncate(time.Second), cache: filepath.Join(base, "R"),
		work: filepath.Join(base, "W"), outside: filepath.Join(base, "outside")}
	for i, prefix := range []string{"", "aa", "aa", "bb", "bb", "cc", "cc", "dd", "ff", "ff", "ff"} {
		if i > 0 {
			f.h[i] = hexName(prefix, i)
		}
	}
	minute := time.Minute
	a := &aged{now: f.now}
	a.file(filepath.Join(f.cache, "README"), KiB, 0)
	a.file(filepath.Join(f.cache, "trim.txt"), KiB, 0)
	for _, e := range []struct {
		n    int
		size int64
		age  time.Duration
	}{{1, 2 * MiB, 130 * minute}, {2, 2 * MiB, 90 * minute}, {3, 60 * MiB, 130 * minute}, {4, 60 * MiB, 100 * minute}, {5, 512 * KiB, 500 * minute}} {
		a.file(f.path(f.entry(e.n)), e.size, e.age)
	}
	a.file(filepath.Join(f.cache, "cc", f.h[6]+"-a"), 100, 500*minute)
	a.file(filepath.Join(f.outside, "big.bin"), 2*MiB, 500*minute)
	a.link(filepath.Join(f.outside, "big.bin"), f.path(f.entry(7)))
	a.file(filepath.Join(f.cache, "ee", "stray.bin"), 5*MiB, 500*minute)
	for _, e := range []struct {
		n   int
		age time.Duration
	}{{8, 130 * minute}, {9, 30 * minute}, {10, 130 * minute}} {
		a.file(filepath.Join(f.path(f.entry(e.n)), "probe.test"), 60*MiB, 500*minute)
		if e.n == 10 {
			a.link(filepath.Join(f.outside, "big.bin"), filepath.Join(f.path(f.entry(e.n)), "link"))
		}
		a.stamp(f.path(f.entry(e.n)), e.age)
	}
	a.file(filepath.Join(f.work, "go-build111", "b001", "pkg.a"), 3*MiB, 400*minute)
	a.stamp(filepath.Join(f.work, "go-build111", "b001"), 400*minute)
	a.stamp(filepath.Join(f.work, "go-build111"), 400*minute)
	a.file(filepath.Join(f.work, "go-build222", "b001", "x.o"), MiB, 10*minute)
	a.stamp(filepath.Join(f.work, "go-build222", "b001"), 400*minute)
	a.stamp(filepath.Join(f.work, "go-build222"), 400*minute)
	a.file(filepath.Join(f.outside, "linked", "big.bin"), 2*MiB, 500*minute)
	a.link(filepath.Join(f.outside, "linked"), filepath.Join(f.work, "go-build333"))
	a.file(filepath.Join(f.work, "go-build555", "pkg.a"), 100*KiB, 100*minute)
	a.stamp(filepath.Join(f.work, "go-build555"), 100*minute)
	for _, dir := range []string{"msgmcp-railway-roles1", "notes"} {
		if a.err == nil {
			a.err = os.MkdirAll(filepath.Join(f.work, dir), 0o755)
		}
		a.stamp(filepath.Join(f.work, dir), 400*minute)
	}
	return f, a.err
}

// entry is the cache-relative path of <hN>-d.
func (f s5Fixture) entry(n int) string { return f.h[n][:2] + "/" + f.h[n] + "-d" }

func (f s5Fixture) path(rel string) string { return filepath.Join(f.cache, filepath.FromSlash(rel)) }

// roots are the fixture roots as the Locator would resolve them.
func (f s5Fixture) roots(t *testing.T) (Root, Root) {
	t.Helper()
	l := Locator{UID: os.Getuid()}
	return l.inspect(KindCache, f.cache), l.inspect(KindWorkDir, f.work)
}

// snapshot maps every path under roots to its kind, size, content hash, and
// mtime, so a test can prove a run left everything else untouched.
func snapshot(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range roots {
		require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			switch {
			case err != nil:
				return err
			case info.Mode()&os.ModeSymlink != 0:
				target, _ := os.Readlink(path)
				out[path] = "symlink->" + target
			case info.IsDir():
				out[path] = "dir"
			default:
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				out[path] = fmt.Sprintf("file %d %x %d", info.Size(), sha256.Sum256(data), info.ModTime().UnixNano())
			}
			return nil
		}))
	}
	return out
}

// execRecorder is a Runner that records every program it is asked to start
// and answers from a table keyed by the joined command line.
type execRecorder struct {
	mu      sync.Mutex
	calls   []string
	answers map[string]string
}

func (r *execRecorder) run(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	r.mu.Lock()
	r.calls = append(r.calls, line)
	r.mu.Unlock()
	if out, ok := r.answers[line]; ok {
		return []byte(out), nil
	}
	return nil, errors.New("not answered: " + line)
}
