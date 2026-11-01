package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

const diskBaseConfig = "mode: full\nproject_name: disk\nplatforms:\n  - claude-code\n"

var diskTopLevelKey = regexp.MustCompile(`(?m)^disk:`)

func writeDiskConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(diskBaseConfig+body), 0o600))
	return dir
}

func readDiskConfig(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	return string(data)
}

// S1: an absent disk key resolves to the documented defaults, and a config at
// defaults never gains the key, neither generated nor saved after a load.
func TestDiskConf_DefaultsResolveAndStayOutOfSavedConfig(t *testing.T) {
	t.Parallel()
	dir := writeDiskConfig(t, "")

	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	eff := cfg.Disk.Effective()

	assert.Equal(t, int64(10737418240), eff.FloorBytes())
	assert.Equal(t, 10, eff.MinFreeGB)
	assert.Empty(t, eff.Reserves)
	assert.Equal(t, 0, eff.StartBelowGB)
	assert.Equal(t, []string{"Runner.Worker"}, eff.CIProcessNames)
	assert.Equal(t, config.DiskGoBuildEffective{
		FileMB: 1, IdleMinutes: 120, CIIdleMinutes: 180, EmergencyIdleMinutes: 120,
		LargeFileMB: 50, LargeFileIdleMinutes: 120, LargeFileBelowGB: 0,
	}, eff.GoBuild)
	assert.Equal(t, 360, eff.WorkDirIdleMinutes)
	assert.Empty(t, eff.BelowRecommendedIdle(), "no default idle threshold is below the safe hour")

	require.NoError(t, config.Save(dir, cfg))
	assert.NotRegexp(t, diskTopLevelKey, readDiskConfig(t, dir), "load-save round trip of defaults")

	marshaled, err := yaml.Marshal(config.DefaultFullConfig("disk"))
	require.NoError(t, err)
	assert.NotRegexp(t, diskTopLevelKey, string(marshaled), "generated autopus.yaml")
	assert.Equal(t, config.DiskConf{}, config.DefaultFullConfig("disk").Disk)
}

// S1: each invalid section fails the load with the dotted key and its bound.
func TestDiskConf_InvalidValuesFailLoadNamingKeyAndBound(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		body string
		want []string
	}{
		{"disk:\n  min_free_gb: -1\n", []string{"disk.min_free_gb must be >= 0"}},
		{"disk:\n  reserves: {release: 0}\n", []string{"disk.reserves.release must be a positive integer"}},
		{"disk:\n  reserves: {release: -3}\n", []string{"disk.reserves.release must be a positive integer"}},
		{"disk:\n  reserves: {\"Release Build\": 30}\n", []string{`disk.reserves name "Release Build" must match`}},
		{"disk:\n  trim: {go_build: {idle_minutes: 30}}\n", []string{"disk.trim.go_build.idle_minutes must be >= 60"}},
		{"disk:\n  trim: {go_build: {idle_minutes: 0}}\n", []string{"disk.trim.go_build.idle_minutes must be >= 60"}},
		{"disk:\n  trim: {go_build: {ci_idle_minutes: 59}}\n", []string{"disk.trim.go_build.ci_idle_minutes must be >= 60"}},
		{"disk:\n  trim: {go_build: {emergency_idle_minutes: 1}}\n", []string{"disk.trim.go_build.emergency_idle_minutes must be >= 60"}},
		{"disk:\n  trim: {go_build: {large_file_idle_minutes: 10}}\n", []string{"disk.trim.go_build.large_file_idle_minutes must be >= 60"}},
		{"disk:\n  trim: {go_build_work_dirs: {idle_minutes: 59}}\n", []string{"disk.trim.go_build_work_dirs.idle_minutes must be >= 60"}},
		{"disk:\n  trim: {go_build: {file_mb: 0}}\n", []string{"disk.trim.go_build.file_mb must be >= 1"}},
		{"disk:\n  trim: {go_build: {large_file_mb: 0}}\n", []string{"disk.trim.go_build.large_file_mb must be >= 1"}},
		{"disk:\n  trim: {go_build: {large_file_below_gb: -1}}\n", []string{"disk.trim.go_build.large_file_below_gb must be >= 0"}},
		{"disk:\n  trim: {start_below_gb: -5}\n", []string{"disk.trim.start_below_gb must be >= 0"}},
		{"disk:\n  trim: {ci_process_names: [\"bin/Runner.Worker\"]}\n", []string{"disk.trim.ci_process_names entries must be bare executable names"}},
		{"disk:\n  trim: {ci_process_names: [\"\"]}\n", []string{"disk.trim.ci_process_names entries must be bare executable names"}},
		{"disk:\n  trim: {go_build: {idle_min: 120}}\n", []string{"idle_min", "unknown keys are rejected"}},
	} {
		t.Run(tc.body, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadPreview(writeDiskConfig(t, tc.body))
			require.Error(t, err)
			for _, want := range tc.want {
				assert.ErrorContains(t, err, want)
			}
		})
	}
}

// S1: a floor and two reserves load as configured, in bytes through the accessor.
func TestDiskConf_FloorAndReservesResolveToBytes(t *testing.T) {
	t.Parallel()
	cfg, err := config.LoadPreview(writeDiskConfig(t, "disk:\n  min_free_gb: 45\n  reserves: {release: 30, visual-linux: 37}\n"))
	require.NoError(t, err)
	eff := cfg.Disk.Effective()

	assert.Equal(t, int64(48318382080), eff.FloorBytes())
	assert.Equal(t, map[string]int{"release": 30, "visual-linux": 37}, eff.Reserves)
	release, ok := eff.ReserveBytes("release")
	assert.True(t, ok)
	assert.Equal(t, int64(30)*(1<<30), release)
	_, ok = eff.ReserveBytes("weekly")
	assert.False(t, ok)
}

// Explicit values, explicit zeros, and an explicit empty process list survive a
// save and resolve as written rather than as the default.
func TestDiskConf_ExplicitValuesSurviveSave(t *testing.T) {
	t.Parallel()
	dir := writeDiskConfig(t, "disk:\n"+
		"  min_free_gb: 0\n"+
		"  trim:\n"+
		"    start_below_gb: 80\n"+
		"    ci_process_names: []\n"+
		"    go_build: {idle_minutes: 60, file_mb: 2, large_file_below_gb: 100}\n"+
		"    go_build_work_dirs: {idle_minutes: 720}\n")

	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	require.NoError(t, config.Save(dir, cfg))
	reloaded, err := config.LoadPreview(dir)
	require.NoError(t, err)
	eff := reloaded.Disk.Effective()

	assert.Equal(t, 0, eff.MinFreeGB)
	assert.Equal(t, 80, eff.StartBelowGB)
	assert.Empty(t, eff.CIProcessNames)
	assert.NotNil(t, eff.CIProcessNames, "an explicit [] is not the default list")
	assert.Equal(t, 60, eff.GoBuild.IdleMinutes)
	assert.Equal(t, 2, eff.GoBuild.FileMB)
	assert.Equal(t, 100, eff.GoBuild.LargeFileBelowGB)
	assert.Equal(t, 180, eff.GoBuild.CIIdleMinutes, "an unset sibling keeps its default")
	assert.Equal(t, 720, eff.WorkDirIdleMinutes)
	assert.Regexp(t, diskTopLevelKey, readDiskConfig(t, dir))
}

// Effective hands out copies, so a caller cannot change the defaults or the
// loaded config through the resolved maps and slices.
func TestDiskConf_EffectiveReturnsCopies(t *testing.T) {
	t.Parallel()
	conf := config.DiskConf{Reserves: map[string]int{"release": 30}}
	eff := conf.Effective()
	eff.Reserves["release"] = 1
	eff.CIProcessNames[0] = "mutated"

	assert.Equal(t, 30, conf.Reserves["release"])
	assert.Equal(t, []string{"Runner.Worker"}, config.DiskConf{}.Effective().CIProcessNames)
}

// S1/REQ-DISK-CFG-04: a threshold from Go's one-hour refresh interval up to
// two hours loads, and is reported with its value so a command can warn.
func TestDiskConf_BelowRecommendedIdleNamesKeysBelowTwoHours(t *testing.T) {
	t.Parallel()
	cfg, err := config.LoadPreview(writeDiskConfig(t, "disk:\n  trim:\n"+
		"    go_build: {idle_minutes: 60, large_file_idle_minutes: 90, emergency_idle_minutes: 119, ci_idle_minutes: 120}\n"+
		"    go_build_work_dirs: {idle_minutes: 61}\n"))
	require.NoError(t, err)

	assert.Equal(t, []config.DiskIdleThreshold{
		{Key: "disk.trim.go_build.idle_minutes", Minutes: 60},
		{Key: "disk.trim.go_build.emergency_idle_minutes", Minutes: 119},
		{Key: "disk.trim.go_build.large_file_idle_minutes", Minutes: 90},
		{Key: "disk.trim.go_build_work_dirs.idle_minutes", Minutes: 61},
	}, cfg.Disk.Effective().BelowRecommendedIdle())
}
