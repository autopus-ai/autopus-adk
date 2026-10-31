package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// diskS5Block is the exact stdout of the S5 idle dry run.
func diskS5Block(f diskS5) string {
	return "disk trim: run=dry-run mode=idle free=100.0 GiB ci=forced-idle rules=go_build_large_files(>50 MiB, idle>120m) go_build_files(>1 MiB, idle>120m) go_build_work_dirs(idle>360m)\n" +
		"  roots: cache=" + f.cache + " work=" + f.work + "\n" +
		"  note: idle>120m guarantees 60m without use; mtime trimming bounds the risk to a running build, it does not remove it\n" +
		"  go_build_large_files: candidates=2 size=120.0 MiB\n" +
		"  go_build_files: candidates=1 size=2.0 MiB\n" +
		"  go_build_work_dirs: candidates=1 size=3.0 MiB\n" +
		"  skipped_symlink=3 (go_build_cache=2, go_build_work_dirs=1)\n" +
		"disk trim: would reclaim 125.0 MiB (dry-run; pass --apply to delete)\n"
}

func decodeDiskTrim(t *testing.T, stdout string) (diskEnvelope, diskTrimData) {
	t.Helper()
	envelope := decodeDiskEnvelope(t, stdout)
	var data diskTrimData
	require.NoError(t, json.Unmarshal(envelope.Data, &data))
	return envelope, data
}

// S5: the idle dry run prints exactly the block, --list prints each
// candidate under its rule, and --json carries the same values; the exec
// recorder sees no go subprocess while $GOCACHE is set.
func TestDiskTrim_S5DryRunBlockListAndJSON(t *testing.T) {
	f := newDiskS5(t, false)
	recorder := withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	project := writeDiskProject(t, "")

	run := runDiskCLI(t, "disk", "trim", "--dir", project, "--ci", "idle")
	require.Equal(t, 0, run.code, run.stderr)
	assert.Equal(t, diskS5Block(f), run.stdout)
	assert.Empty(t, run.stderr)

	list := runDiskCLI(t, "disk", "trim", "--dir", project, "--ci", "idle", "--list")
	assert.Equal(t, []string{
		"  go_build_large_files: candidates=2 size=120.0 MiB", "    " + f.entry(3), "    " + f.entry(8),
		"  go_build_files: candidates=1 size=2.0 MiB", "    " + f.entry(1),
		"  go_build_work_dirs: candidates=1 size=3.0 MiB", "    go-build111",
	}, strings.Split(list.stdout, "\n")[3:10])

	jsonRun := runDiskCLI(t, "disk", "trim", "--dir", project, "--ci", "idle", "--list", "--json")
	require.Equal(t, 0, jsonRun.code, jsonRun.stderr)
	assert.Empty(t, jsonRun.stderr)
	envelope, data := decodeDiskTrim(t, jsonRun.stdout)
	assert.Equal(t, "ok", envelope.Status)
	assert.Equal(t, []any{"dry-run", "idle", true, (*diskRefusal)(nil)}, []any{data.Run, data.Mode, data.Active, data.Refused})
	assert.Equal(t, diskCIData{State: "forced-idle", Source: "flag"}, data.CI)
	assert.Equal(t, diskRoots{Cache: diskRootData{Path: f.cache, Available: true}, Work: diskRootData{Path: f.work, Available: true}}, data.Roots)
	require.Len(t, data.Rules, 3)
	assert.Equal(t, []string{f.entry(3), f.entry(8)}, *data.Rules[0].Files)
	assert.Equal(t, []any{"go_build_files", 1, []string{f.entry(1)}}, []any{data.Rules[1].ID, data.Rules[1].Candidates, *data.Rules[1].Files})
	assert.Equal(t, diskspace.Skipped{Symlink: diskspace.RootCounts{Cache: 2, WorkDir: 1}}, data.Skipped)
	assert.InDelta(t, 131072000, data.TotalBytes, 8*4096)
	assert.Equal(t, int64(107374182400), data.FreeBytes)
	assert.Equal(t, int64(10737418240), data.FloorBytes)
	for _, call := range recorder.calls {
		assert.False(t, strings.HasPrefix(call, "go "), call)
		assert.NotContains(t, call, "clean")
	}
}

// S5 modes and S12 gating: each configuration and seam picks its header,
// rules, and total; start_below_gb leaves only the inactive line.
func TestDiskTrim_ModesAndGating(t *testing.T) {
	f := newDiskS5(t, false)
	withDiskSeams(t, f)
	for _, tc := range []struct {
		name, section, ci string
		free              float64
		contains          []string
		exact, stderr     string
	}{
		{name: "ci", ci: "running", free: 100, contains: []string{"run=dry-run mode=ci free=100.0 GiB ci=forced-running", "go_build_files(>1 MiB, idle>180m)", "would reclaim 123.0 MiB"}},
		{name: "emergency", ci: "running", free: 5, contains: []string{"mode=emergency free=5.0 GiB", "go_build_files(>1 MiB, idle>120m)", "would reclaim 125.0 MiB"}},
		// S5: emergency_idle_minutes 60 adds h2 and h4 to the files rule and
		// warns about the key.
		{name: "emergency 60", section: "disk:\n  trim: {go_build: {emergency_idle_minutes: 60}}\n", ci: "running", free: 5,
			contains: []string{"mode=emergency free=5.0 GiB", "go_build_files(>1 MiB, idle>60m)", "go_build_files: candidates=3 size=64.0 MiB",
				"note: idle>60m guarantees 0m without use;", "would reclaim 187.0 MiB"},
			stderr: "disk trim: warning: disk.trim.go_build.emergency_idle_minutes=60 is below 120 and may delete an entry a running build has already looked up\n"},
		{name: "inactive", section: "disk:\n  trim: {start_below_gb: 80}\n", ci: "idle", free: 100, exact: "disk trim: inactive (free above start_below_gb)\n"},
		{name: "just below start", section: "disk:\n  trim: {start_below_gb: 80}\n", ci: "idle", free: 79.9, contains: []string{"mode=idle free=79.9 GiB", "would reclaim 125.0 MiB"}},
		// S12 row 3: with the large rule off, h3 and h8 fall to the files rule.
		{name: "large rule off", section: "disk:\n  trim: {go_build: {large_file_below_gb: 60}}\n", ci: "idle", free: 70,
			contains: []string{"rules=go_build_files(>1 MiB, idle>120m) go_build_work_dirs(idle>360m)\n", "go_build_files: candidates=3 size=122.0 MiB", "would reclaim 125.0 MiB"}},
		{name: "large rule on", section: "disk:\n  trim: {go_build: {large_file_below_gb: 60}}\n", ci: "idle", free: 59, contains: []string{"go_build_large_files: candidates=2 size=120.0 MiB"}},
	} {
		withDiskFree(t, gib(tc.free), diskDataMount)
		run := runDiskCLI(t, "disk", "trim", "--dir", writeDiskProject(t, tc.section), "--ci", tc.ci)
		require.Equal(t, 0, run.code, tc.name+": "+run.stderr)
		if tc.exact != "" {
			assert.Equal(t, tc.exact, run.stdout, tc.name)
		}
		assert.Equal(t, tc.stderr, run.stderr, tc.name)
		for _, want := range tc.contains {
			assert.Contains(t, run.stdout, want, tc.name)
		}
	}
	withDiskFree(t, gib(100), diskDataMount)
	run := runDiskCLI(t, "disk", "trim", "--json", "--dir", writeDiskProject(t, "disk:\n  trim: {start_below_gb: 80}\n"), "--ci", "idle")
	_, data := decodeDiskTrim(t, run.stdout)
	assert.False(t, data.Active)
	assert.Empty(t, data.Rules)
}

// S1/REQ-DISK-CFG-04: every idle key below 120 is named with its value by
// trim and status, on stderr in text mode and as envelope warnings in JSON.
func TestDiskTrim_WarnsAboutIdleThresholdsBelow120(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	project := writeDiskProject(t, "disk:\n  trim: {go_build: {idle_minutes: 60, large_file_idle_minutes: 90}}\n")
	want := "disk trim: warning: disk.trim.go_build.idle_minutes=60 is below 120 and may delete an entry a running build has already looked up\n" +
		"disk trim: warning: disk.trim.go_build.large_file_idle_minutes=90 is below 120 and may delete an entry a running build has already looked up\n"

	for _, sub := range []string{"trim", "status"} {
		run := runDiskCLI(t, "disk", sub, "--dir", project, "--ci", "idle")
		assert.Equal(t, 0, run.code, sub)
		assert.Equal(t, want, run.stderr, sub)

		jsonRun := runDiskCLI(t, "disk", sub, "--dir", project, "--ci", "idle", "--json")
		assert.Empty(t, jsonRun.stderr, sub)
		envelope := decodeDiskEnvelope(t, jsonRun.stdout)
		require.Len(t, envelope.Warnings, 2, sub)
		for _, warning := range envelope.Warnings {
			assert.Equal(t, "idle_threshold_below_120", warning.Code, sub)
			assert.Contains(t, want, warning.Message, sub)
		}
	}
}

// S5/REQ-DISK-TRIM-07: an unset $GOCACHE asks go exactly once, `go env
// GOCACHE`, and no run ever passes clean to anything.
func TestDiskTrim_AsksGoEnvOnlyWhenGOCACHEIsUnset(t *testing.T) {
	f := newDiskS5(t, true)
	recorder := withDiskSeams(t, f)
	t.Setenv("GOCACHE", "")
	recorder.goCache = f.cache
	withDiskFree(t, gib(100), diskDataMount)

	run := runDiskCLI(t, "disk", "trim", "--dir", writeDiskProject(t, ""), "--ci", "idle")

	require.Equal(t, 0, run.code, run.stderr)
	assert.Contains(t, run.stdout, "roots: cache="+f.cache+" ")
	assert.Equal(t, []string{"go env GOCACHE"}, recorder.calls)
	assert.Equal(t, [][]string{{"GOTOOLCHAIN=local", "GOWORK=off"}}, recorder.envs)
}
