package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// diskTree lists every path under roots with its kind, so a test can prove
// what a run removed and that nothing else moved.
func diskTree(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range roots {
		require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			out[path] = info.Mode().Type().String() + " " + strconv.FormatInt(info.Size(), 10) + " " + strconv.FormatInt(info.ModTime().UnixNano(), 10)
			if info.IsDir() {
				out[path] = "dir"
			}
			return nil
		}))
	}
	return out
}

func diskWithout(state map[string]string, removed ...string) map[string]string {
	out := map[string]string{}
	for path, value := range state {
		keep := true
		for _, gone := range removed {
			keep = keep && path != gone && !strings.HasPrefix(path, gone+string(filepath.Separator))
		}
		if keep {
			out[path] = value
		}
	}
	return out
}

func diskSummary(stdout string) string {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	return lines[len(lines)-1]
}

// S5: --apply in the idle run deletes exactly h1, h3, h8, and go-build111,
// prints the reclaimed line, and exits 0.
func TestDiskTrimApply_S5DeletesExactlyThePlan(t *testing.T) {
	f := newDiskS5(t, false)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	before := diskTree(t, f.cache, f.work, f.outside)

	run := runDiskCLI(t, "disk", "trim", "--dir", writeDiskProject(t, ""), "--ci", "idle", "--apply")

	require.Equal(t, 0, run.code, run.stderr)
	assert.Empty(t, run.stderr)
	assert.True(t, strings.HasPrefix(run.stdout, "disk trim: run=apply mode=idle "), run.stdout)
	assert.Equal(t, "disk trim: reclaimed 125.0 MiB (deleted=4 skipped_changed=0 skipped_symlink=3 skipped_device=0 skipped_owner=0 vanished=0 failed=0)",
		diskSummary(run.stdout))
	assert.Equal(t, diskWithout(before, f.path(f.entry(1)), f.path(f.entry(3)), f.path(f.entry(8)), filepath.Join(f.work, "go-build111")),
		diskTree(t, f.cache, f.work, f.outside))
}

// S5 hooks, one fresh fixture each: a touched h1 survives as changed, an
// unlinked h3 counts as vanished, and a directory swapped inside go-build111
// after its pre-walk fails that candidate with exit 1.
func TestDiskTrimApply_S5RaceHooks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		hook    func(f diskS5) func(rule, rel string)
		code    int
		summary string
		stderr  string
	}{
		{"touch h1", func(f diskS5) func(string, string) {
			return func(_, rel string) {
				if rel == f.entry(1) {
					now := time.Now()
					require.NoError(t, os.Chtimes(f.path(rel), now, now))
				}
			}
		}, 0, "(deleted=3 skipped_changed=1 skipped_symlink=3 skipped_device=0 skipped_owner=0 vanished=0 failed=0)", ""},
		{"unlink h3", func(f diskS5) func(string, string) {
			return func(_, rel string) {
				if rel == f.entry(3) {
					require.NoError(t, os.Remove(f.path(rel)))
				}
			}
		}, 0, "(deleted=3 skipped_changed=0 skipped_symlink=3 skipped_device=0 skipped_owner=0 vanished=1 failed=0)", ""},
		{"swap b001", func(f diskS5) func(string, string) {
			return func(_, rel string) {
				if rel == "go-build111" {
					b001 := filepath.Join(f.work, rel, "b001")
					require.NoError(t, os.Rename(b001, filepath.Join(f.base, "parked")))
					require.NoError(t, os.MkdirAll(b001, 0o755))
				}
			}
		}, 1, "(deleted=3 skipped_changed=0 skipped_symlink=3 skipped_device=0 skipped_owner=0 vanished=0 failed=1)",
			"disk trim: 1 candidate could not be removed\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDiskS5(t, false)
			withDiskSeams(t, f)
			withDiskFree(t, gib(100), diskDataMount)
			withDiskHook(t, tc.hook(f))

			run := runDiskCLI(t, "disk", "trim", "--dir", writeDiskProject(t, ""), "--ci", "idle", "--apply")

			assert.Equal(t, tc.code, run.code)
			assert.True(t, strings.HasSuffix(diskSummary(run.stdout), tc.summary), diskSummary(run.stdout))
			assert.Equal(t, tc.stderr, run.stderr)
			if tc.name == "touch h1" {
				assert.FileExists(t, f.path(f.entry(1)))
			}
		})
	}
}

// REQ-DISK-TRIM-06/10: an unacceptable root refuses both runs with one line
// and deletes nothing from either root; a missing root is only unavailable.
func TestDiskTrim_RefusesAndReportsUnavailableRoots(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, f diskS5) string
	}{
		{"no README", func(t *testing.T, f diskS5) string {
			require.NoError(t, os.Remove(filepath.Join(f.cache, "README")))
			return f.cache + " is not an acceptable cache root (no README marker)"
		}},
		{"work dir is home", func(t *testing.T, f diskS5) string {
			t.Setenv("HOME", f.work)
			return f.work + " is not an acceptable work dir root (home directory)"
		}},
		{"foreign uid", func(t *testing.T, f diskS5) string {
			uid := os.Getuid() + 1
			previous := diskUID
			diskUID = func() int { return uid }
			t.Cleanup(func() { diskUID = previous })
			return f.cache + " is not an acceptable cache root (not owned by uid " + strconv.Itoa(uid) + ")"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDiskS5(t, true)
			withDiskSeams(t, f)
			withDiskFree(t, gib(100), diskDataMount)
			message := tc.setup(t, f)
			before := diskTree(t, f.cache, f.work)
			project := writeDiskProject(t, "")
			for _, args := range [][]string{{"--ci", "idle"}, {"--ci", "idle", "--apply"}} {
				run := runDiskCLI(t, append([]string{"disk", "trim", "--dir", project}, args...)...)
				assert.Equal(t, 1, run.code, args)
				assert.Equal(t, "disk trim: refused: "+message+"\n", run.stderr, args)
				assert.Empty(t, run.stdout, args)
			}
			jsonRun := runDiskCLI(t, "disk", "trim", "--dir", project, "--ci", "idle", "--apply", "--json")
			envelope, data := decodeDiskTrim(t, jsonRun.stdout)
			assert.Equal(t, 1, jsonRun.code)
			assert.Equal(t, "disk_trim_refused", envelope.Error.Code)
			require.NotNil(t, data.Refused)
			// The shared envelope masks the home directory as ~ in every string.
			assert.Equal(t, maskHomePath(message), data.Refused.Message)
			if tc.name == "foreign uid" {
				assert.Positive(t, data.Skipped.Owner, "the foreign root is scanned so its candidates count as owner skips")
			}
			assert.Equal(t, before, diskTree(t, f.cache, f.work))
		})
	}

	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	t.Setenv("GOCACHE", filepath.Join(f.base, "missing"))
	run := runDiskCLI(t, "disk", "trim", "--dir", writeDiskProject(t, ""), "--ci", "idle")
	assert.Equal(t, 0, run.code, run.stderr)
	assert.Contains(t, run.stdout, "  roots: cache=root_missing work="+f.work+"\n  go_build_cache: unavailable (root_missing)\n")
}

// S6: CI detection reaches the header through the lister seam; a lister
// failure is unknown, warns, and still lets the floor select emergency.
func TestDiskTrim_S6CIRows(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	project := writeDiskProject(t, "")
	for _, tc := range []struct {
		rows   []string
		err    error
		free   float64
		flag   string
		header string
	}{
		{rows: []string{"/opt/actions-runner/bin/Runner.Listener", "/opt/actions-runner/bin/Runner.Worker", "/bin/zsh"}, free: 100, header: "mode=ci free=100.0 GiB ci=running "},
		{rows: []string{"/opt/actions-runner/bin/Runner.Listener", "/bin/zsh", "/usr/bin/grep"}, free: 100, header: "mode=idle free=100.0 GiB ci=idle "},
		{rows: []string{"/opt/actions-runner/bin/Runner.Worker.old", "/bin/zsh"}, free: 100, header: "mode=idle free=100.0 GiB ci=idle "},
		{err: errors.New("ps: operation not permitted"), free: 100, header: "mode=ci free=100.0 GiB ci=unknown "},
		{err: errors.New("ps: operation not permitted"), free: 5, header: "mode=emergency free=5.0 GiB ci=unknown "},
		{rows: []string{"/opt/actions-runner/bin/Runner.Worker"}, free: 100, flag: "idle", header: "mode=idle free=100.0 GiB ci=forced-idle "},
		{rows: []string{"/bin/zsh"}, free: 100, flag: "running", header: "mode=ci free=100.0 GiB ci=forced-running "},
	} {
		rows, err := tc.rows, tc.err
		diskLister = func(diskspace.Runner) diskspace.ProcessLister {
			return diskspace.ListerFunc(func(context.Context) ([]string, error) { return rows, err })
		}
		withDiskFree(t, gib(tc.free), diskDataMount)
		args := []string{"disk", "trim", "--dir", project}
		if tc.flag != "" {
			args = append(args, "--ci", tc.flag)
		}
		run := runDiskCLI(t, args...)
		require.Equal(t, 0, run.code, run.stderr)
		assert.Contains(t, run.stdout, tc.header)
		if err != nil {
			assert.Equal(t, "disk trim: ci detection unavailable (ps: operation not permitted); treating CI as running\n", run.stderr)
		}
	}
}

// S2/REQ-DISK-FREE-02: status and trim exit 1 with the single unsupported line.
func TestDiskStatusAndTrim_UnsupportedOS(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFreeFunc(t, func(string) (diskspace.Space, error) { return diskspace.Space{}, diskspace.UnsupportedOn("plan9") })
	for _, sub := range []string{"status", "trim"} {
		run := runDiskCLI(t, "disk", sub, "--dir", writeDiskProject(t, ""))
		assert.Equal(t, 1, run.code, sub)
		assert.Equal(t, "disk: free space measurement is not supported on plan9\n", run.stderr, sub)
		assert.Empty(t, run.stdout, sub)
	}
}
