package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// withAutoSpawn records the automatic reclaims the Stop hook would start.
func withAutoSpawn(t *testing.T) *[]string {
	t.Helper()
	var spawned []string
	prev := diskSpawnAutoReclaim
	diskSpawnAutoReclaim = func(dir string) error { spawned = append(spawned, dir); return nil }
	t.Cleanup(func() { diskSpawnAutoReclaim = prev })
	return &spawned
}

func withTempDiskState(t *testing.T) {
	t.Helper()
	prev := diskStatePath
	dir := filepath.Join(t.TempDir(), "state", "disk")
	diskStatePath = func() string { return dir }
	t.Cleanup(func() { diskStatePath = prev })
}

func TestAutoReclaim_RateLimitedPerUserAndOptOut(t *testing.T) {
	withTempDiskState(t)
	spawned := withAutoSpawn(t)
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	prevNow := diskNow
	diskNow = func() time.Time { return now }
	t.Cleanup(func() { diskNow = prevNow })
	dir := writeDiskProject(t, "")

	assert.True(t, maybeStartAutoReclaim(dir))
	now = now.Add(10 * time.Minute)
	assert.False(t, maybeStartAutoReclaim(writeDiskProject(t, "")), "the interval is shared by every project of the user")
	now = now.Add(21 * time.Minute)
	assert.True(t, maybeStartAutoReclaim(dir), "30 minutes after the last start, by default")

	now = now.Add(time.Hour)
	assert.False(t, maybeStartAutoReclaim(writeDiskProject(t, "disk:\n  reclaim: {auto: false}\n")))
	t.Setenv("AUTOPUS_DISK_AUTO_RECLAIM", "off")
	assert.False(t, maybeStartAutoReclaim(dir))
	assert.Equal(t, []string{dir, dir}, *spawned)
}

// The Stop hook starts it even when verification itself is off or has
// nothing to check.
func TestVerifyStop_StartsAutoReclaim(t *testing.T) {
	withTempDiskState(t)
	spawned := withAutoSpawn(t)
	dir := writeDiskProject(t, "")
	t.Setenv("AUTOPUS_VERIFY_STOP", "off")
	cmd := newVerifyStopCmd()
	cmd.SetIn(strings.NewReader(`{"session_id":"s","cwd":"` + dir + `"}`))
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{dir}, *spawned)
}

// --auto removes the merged, clean linked worktree, keeps the idle
// scratchpad session and large files, and logs an auto_reclaim event.
func TestDiskReclaim_AutoKeepsToTheSafeSubset(t *testing.T) {
	rf := newReclaimFixture(t)
	diskGitT(t, rf.repo, "update-ref", "refs/remotes/origin/main", "main")
	large := filepath.Join(rf.scratch, "-other", "backup.img")
	writeSparse(t, large, diskspace.GiB)
	old := rf.now.Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(large, old, old))

	dry := runDiskCLI(t, "disk", "reclaim", "--dir", rf.repo, "--json")
	require.Equal(t, 0, dry.code, dry.stderr)
	var data diskReclaimData
	require.NoError(t, json.Unmarshal(decodeDiskEnvelope(t, dry.stdout).Data, &data))
	assert.Len(t, data.LargeFiles.Candidates, 1)
	assert.Len(t, data.Scratchpads.Candidates, 1)
	assert.Equal(t, data.Worktrees.Bytes+data.Regenerable.Bytes+data.Scratchpads.Bytes, data.TotalBytes,
		"large files count only with --include-large-files")

	auto := runDiskCLI(t, "disk", "reclaim", "--auto", "--apply", "--include-large-files", "--dir", rf.repo)
	require.Equal(t, 0, auto.code, auto.stderr+auto.stdout)
	assert.NoDirExists(t, rf.harness)
	assert.NoDirExists(t, rf.elsewhere, "merged into origin/main, clean, idle: removed wherever it is")
	assert.FileExists(t, large, "the automatic reclaim never removes large files")
	assert.DirExists(t, rf.session, "nor session scratchpads")
	assert.Contains(t, runDiskCLI(t, "disk", "log").stdout, "auto_reclaim")

	keep := runDiskCLI(t, "disk", "reclaim", "--apply", "--dir", rf.repo)
	require.Equal(t, 0, keep.code, keep.stderr)
	assert.FileExists(t, large, "--apply alone lists large files but keeps them")
	assert.Contains(t, keep.stdout, "only with --include-large-files")
	gone := runDiskCLI(t, "disk", "reclaim", "--apply", "--include-large-files", "--dir", rf.repo)
	require.Equal(t, 0, gone.code, gone.stderr)
	assert.NoFileExists(t, large)
}

func writeSparse(t *testing.T, path string, size int64) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(size))
	require.NoError(t, f.Close())
}

// Status measures the detected consumers and ranks their growth against the
// previous status run.
func TestDiskStatus_ExtraConsumersAndGrowth(t *testing.T) {
	f := newDiskS5(t, true)
	withDiskSeams(t, f)
	withDiskFree(t, gib(100), diskDataMount)
	home := filepath.Join(f.base, "home")
	image := filepath.Join(home, ".colima", "default", "diffdisk")
	require.NoError(t, os.MkdirAll(filepath.Dir(image), 0o755))
	require.NoError(t, os.WriteFile(image, make([]byte, 64*1024), 0o644))
	prevHome := diskConsumerHome
	diskConsumerHome = func() string { return home }
	t.Cleanup(func() { diskConsumerHome = prevHome })
	project := writeDiskProject(t, "")

	first := runDiskCLI(t, "disk", "status", "--dir", project, "--ci", "idle")
	require.Equal(t, 0, first.code, first.stderr)
	assert.Contains(t, first.stdout, "  colima: ")
	assert.Contains(t, first.stdout, "consumer growth: no change recorded yet")

	require.NoError(t, os.WriteFile(image, make([]byte, 8*diskspace.MiB), 0o644))
	later := f.now.Add(10 * time.Minute)
	diskNow = func() time.Time { return later }
	run := runDiskCLI(t, "disk", "status", "--dir", project, "--ci", "idle", "--json")
	require.Equal(t, 0, run.code, run.stderr)
	var data diskStatusData
	require.NoError(t, json.Unmarshal(decodeDiskEnvelope(t, run.stdout).Data, &data))
	require.NotEmpty(t, data.Growth)
	assert.Equal(t, diskspace.ConsumerColima, data.Growth[0].Name)
	assert.Greater(t, data.Growth[0].DeltaBytes, 7*diskspace.MiB)
	require.Len(t, data.Consumers.Extra, 1)
	assert.Equal(t, image[:len(image)-len("/default/diffdisk")], data.Consumers.Extra[0].Path)

	text := runDiskCLI(t, "disk", "status", "--dir", project, "--ci", "idle")
	assert.Contains(t, text.stdout, "consumer growth since ")
	assert.Contains(t, text.stdout, "  colima +")
}

// The per-prompt alert names the top grower from the ledger.
func TestStickyDiskAlert_NamesTheTopGrower(t *testing.T) {
	withTempDiskState(t)
	withDiskFree(t, gib(5), "/fixture")
	now := time.Now().UTC().Truncate(time.Second)
	prevNow := diskNow
	diskNow = func() time.Time { return now }
	t.Cleanup(func() { diskNow = prevNow })
	state, err := openDiskState()
	require.NoError(t, err)
	for _, s := range []diskspace.ConsumerSample{
		{Time: now.Add(-52 * time.Minute), Bytes: map[string]int64{"colima": 2 * diskspace.GiB, "npm_cache": diskspace.GiB}},
		{Time: now.Add(-time.Minute), Bytes: map[string]int64{"colima": 40 * diskspace.GiB, "npm_cache": diskspace.GiB}},
	} {
		_, err := diskspace.RecordConsumerSample(state, s)
		require.NoError(t, err)
	}
	dir := writeDiskProject(t, "")
	alert := stickyDiskAlert(dir, config.DiskConf{}.Effective())
	assert.True(t, strings.HasSuffix(alert, " Top grower: colima +38.0 GiB in 52m."), alert)
}
