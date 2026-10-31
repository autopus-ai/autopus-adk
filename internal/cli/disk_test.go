package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

const diskDataMount = "/System/Volumes/Data"

// diskResult is one `auto ...` run mapped to its process outcome the way
// Execute maps it: the exit code, and stderr as runRootCommand leaves it.
type diskResult struct {
	code           int
	stdout, stderr string
}

func runDiskCLI(t *testing.T, args ...string) diskResult {
	t.Helper()
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	code, _ := runRootCommand(context.Background(), root, &stderr)
	return diskResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// withDiskFree pins the free-space reading of every disk command and of the
// doctor disk row for the rest of the test.
func withDiskFree(t *testing.T, availBytes int64, mount string) {
	t.Helper()
	withDiskFreeFunc(t, func(string) (diskspace.Space, error) {
		return diskspace.Space{AvailBytes: availBytes, BlockUnit: 4096, Mount: mount}, nil
	})
}

func withDiskFreeFunc(t *testing.T, free func(string) (diskspace.Space, error)) {
	t.Helper()
	previous := diskFree
	diskFree = free
	t.Cleanup(func() { diskFree = previous })
}

// gib converts a fractional GiB count to bytes at run time, as the tables of
// acceptance.md state free space.
func gib(v float64) int64 { return int64(v * float64(diskspace.GiB)) }

// writeDiskProject creates a project whose autopus.yaml holds the given disk
// section below a minimal valid harness config.
func writeDiskProject(t *testing.T, diskSection string) string {
	t.Helper()
	dir := t.TempDir()
	body := "mode: full\nproject_name: disk\nplatforms:\n  - claude-code\n" + diskSection
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(body), 0o600))
	return dir
}

// diskEnvelope is the shared JSON envelope with data left raw.
type diskEnvelope struct {
	Status string `json:"status"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Warnings []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"warnings"`
	Data json.RawMessage `json:"data"`
}

func decodeDiskEnvelope(t *testing.T, stdout string) diskEnvelope {
	t.Helper()
	var envelope diskEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), stdout)
	return envelope
}

// diskS5 is the cache root R and work dir root W of acceptance S5 plus the
// outside/ targets of its symlinks, all canonical.
type diskS5 struct {
	now                        time.Time
	base, cache, work, outside string
	h                          [11]string
}

func (f diskS5) entry(n int) string     { return f.h[n][:2] + "/" + f.h[n] + "-d" }
func (f diskS5) path(rel string) string { return filepath.Join(f.cache, filepath.FromSlash(rel)) }

// diskAged writes fully allocated files and symlinks and stamps mtimes
// relative to now, failing the test on the first error.
type diskAged struct {
	t   *testing.T
	now time.Time
}

func (a diskAged) file(path string, size int64, age time.Duration) {
	require.NoError(a.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(a.t, os.WriteFile(path, bytes.Repeat([]byte{'x'}, int(size)), 0o644))
	a.stamp(path, age)
}

func (a diskAged) stamp(path string, age time.Duration) {
	require.NoError(a.t, os.Chtimes(path, a.now.Add(-age), a.now.Add(-age)))
}

func (a diskAged) link(target, path string) {
	require.NoError(a.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(a.t, os.Symlink(target, path))
}

// newDiskS5 builds the S5 fixture; light leaves out the 60 MiB entries for
// tests that check routing, not totals.
func newDiskS5(t *testing.T, light bool) diskS5 {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	f := diskS5{now: time.Now().Truncate(time.Second), base: base, cache: filepath.Join(base, "R"),
		work: filepath.Join(base, "W"), outside: filepath.Join(base, "outside")}
	for i, prefix := range []string{"", "aa", "aa", "bb", "bb", "cc", "cc", "dd", "ff", "ff", "ff"} {
		if i > 0 {
			f.h[i] = fmt.Sprintf("%s%062x", prefix, i)
		}
	}
	a, m := diskAged{t: t, now: f.now}, time.Minute
	big := int64(60 * diskspace.MiB)
	if light {
		big = 2 * diskspace.MiB
	}
	a.file(filepath.Join(f.cache, "README"), diskspace.KiB, 0)
	a.file(filepath.Join(f.cache, "trim.txt"), diskspace.KiB, 0)
	a.file(f.path(f.entry(1)), 2*diskspace.MiB, 130*m)
	a.file(f.path(f.entry(2)), 2*diskspace.MiB, 90*m)
	a.file(f.path(f.entry(3)), big, 130*m)
	a.file(f.path(f.entry(4)), big, 100*m)
	a.file(f.path(f.entry(5)), 512*diskspace.KiB, 500*m)
	a.file(filepath.Join(f.cache, "cc", f.h[6]+"-a"), 100, 500*m)
	a.file(filepath.Join(f.outside, "big.bin"), 2*diskspace.MiB, 500*m)
	a.link(filepath.Join(f.outside, "big.bin"), f.path(f.entry(7)))
	a.file(filepath.Join(f.cache, "ee", "stray.bin"), 5*diskspace.MiB, 500*m)
	for n, age := range map[int]time.Duration{8: 130 * m, 9: 30 * m, 10: 130 * m} {
		a.file(filepath.Join(f.path(f.entry(n)), "probe.test"), big, 500*m)
		if n == 10 {
			a.link(filepath.Join(f.outside, "big.bin"), filepath.Join(f.path(f.entry(n)), "link"))
		}
		a.stamp(f.path(f.entry(n)), age)
	}
	a.file(filepath.Join(f.work, "go-build111", "b001", "pkg.a"), 3*diskspace.MiB, 400*m)
	a.stamp(filepath.Join(f.work, "go-build111", "b001"), 400*m)
	a.stamp(filepath.Join(f.work, "go-build111"), 400*m)
	a.file(filepath.Join(f.work, "go-build222", "b001", "x.o"), diskspace.MiB, 10*m)
	a.stamp(filepath.Join(f.work, "go-build222", "b001"), 400*m)
	a.stamp(filepath.Join(f.work, "go-build222"), 400*m)
	a.file(filepath.Join(f.outside, "linked", "big.bin"), 2*diskspace.MiB, 500*m)
	a.link(filepath.Join(f.outside, "linked"), filepath.Join(f.work, "go-build333"))
	a.file(filepath.Join(f.work, "go-build555", "pkg.a"), 100*diskspace.KiB, 100*m)
	a.stamp(filepath.Join(f.work, "go-build555"), 100*m)
	for _, dir := range []string{"msgmcp-railway-roles1", "notes"} {
		require.NoError(t, os.MkdirAll(filepath.Join(f.work, dir), 0o755))
		a.stamp(filepath.Join(f.work, dir), 400*m)
	}
	return f
}

// diskRecorder is the diskRunner seam of a test: it records every program a
// disk command starts, answers `go env GOCACHE` from goCache, and runs git
// without the developer's git configuration.
type diskRecorder struct {
	mu      sync.Mutex
	calls   []string
	goCache string
}

func (r *diskRecorder) run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	r.mu.Unlock()
	switch {
	case name == "go" && strings.Join(args, " ") == "env GOCACHE" && r.goCache != "":
		return []byte(r.goCache + "\n"), nil
	case name != "git":
		return nil, errors.New("not available in tests: " + name)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	return cmd.Output()
}

// withDiskSeams points every disk seam at the fixture: GOCACHE and GOTMPDIR,
// the clock, a scratchpad root under the test's own temp dir, a process
// lister that sees no CI job, and the recorder. It returns the recorder.
func withDiskSeams(t *testing.T, f diskS5) *diskRecorder {
	t.Helper()
	t.Setenv("GOCACHE", f.cache)
	t.Setenv("GOTMPDIR", f.work)
	recorder := &diskRecorder{}
	scratch := filepath.Join(f.base, "scratch", "claude-test")
	prevNow, prevScratch, prevRunner, prevLister := diskNow, diskScratchpad, diskRunner, diskLister
	t.Cleanup(func() { diskNow, diskScratchpad, diskRunner, diskLister = prevNow, prevScratch, prevRunner, prevLister })
	diskNow = func() time.Time { return f.now }
	diskScratchpad = func(int) string { return scratch }
	diskRunner = recorder.run
	diskLister = func(diskspace.Runner) diskspace.ProcessLister {
		return diskspace.ListerFunc(func(context.Context) ([]string, error) { return []string{"/bin/zsh"}, nil })
	}
	return recorder
}

// withDiskHook installs the trim hook between a candidate's scan and its
// removal for the rest of the test.
func withDiskHook(t *testing.T, hook func(rule, rel string)) {
	t.Helper()
	previous := diskBeforeRemove
	diskBeforeRemove = hook
	t.Cleanup(func() { diskBeforeRemove = previous })
}
