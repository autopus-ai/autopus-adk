package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// SPEC-DISK-002 S1: absent reclaim and lease keys resolve to their defaults,
// and a config at defaults still writes no disk key.
func TestDiskConf_ReclaimAndLeaseDefaults(t *testing.T) {
	t.Parallel()
	dir := writeDiskConfig(t, "")

	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	eff := cfg.Disk.Effective()

	assert.Equal(t, config.DiskReclaimEffective{WorktreeIdleMinutes: 360, ScratchpadIdleMinutes: 1440, Auto: true, AutoIntervalMinutes: 30, LargeFileIdleMinutes: 1440}, eff.Reclaim)
	assert.Equal(t, config.DiskLeaseEffective{MaxConcurrent: 0, WaitMinutes: 10, ReclaimOnShortage: true}, eff.Lease)

	require.NoError(t, config.Save(dir, cfg))
	assert.NotRegexp(t, diskTopLevelKey, readDiskConfig(t, dir))
}

// SPEC-DISK-002 S1: explicit values load, including an explicit false.
func TestDiskConf_ReclaimAndLeaseExplicitValues(t *testing.T) {
	t.Parallel()
	cfg, err := config.LoadPreview(writeDiskConfig(t,
		"disk:\n  reclaim: {worktree_idle_minutes: 60, scratchpad_idle_minutes: 90}\n"+
			"  lease: {max_concurrent: 2, wait_minutes: 0, reclaim_on_shortage: false}\n"))
	require.NoError(t, err)
	eff := cfg.Disk.Effective()

	assert.Equal(t, config.DiskReclaimEffective{WorktreeIdleMinutes: 60, ScratchpadIdleMinutes: 90, Auto: true, AutoIntervalMinutes: 30, LargeFileIdleMinutes: 1440}, eff.Reclaim)
	assert.Equal(t, config.DiskLeaseEffective{MaxConcurrent: 2, WaitMinutes: 0, ReclaimOnShortage: false}, eff.Lease)
}

// SPEC-DISK-002 S1: each out-of-bound value fails the load naming the key.
func TestDiskConf_ReclaimAndLeaseBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ body, want string }{
		{"disk:\n  reclaim: {worktree_idle_minutes: 59}\n", "disk.reclaim.worktree_idle_minutes must be >= 60"},
		{"disk:\n  reclaim: {scratchpad_idle_minutes: 30}\n", "disk.reclaim.scratchpad_idle_minutes must be >= 60"},
		{"disk:\n  lease: {max_concurrent: -1}\n", "disk.lease.max_concurrent must be >= 0"},
		{"disk:\n  lease: {wait_minutes: -1}\n", "disk.lease.wait_minutes must be >= 0"},
		{"disk:\n  lease: {max_parallel: 1}\n", "max_parallel"},
		{"disk:\n  reclaim: {scratchpad_idle_minutes: 153722868}\n", "disk.reclaim.scratchpad_idle_minutes must be <= 5256000"},
		{"disk:\n  trim: {go_build: {idle_minutes: 153722868}}\n", "disk.trim.go_build.idle_minutes must be <= 5256000"},
		{"disk:\n  min_free_gb: 8589934592\n", "disk.min_free_gb must be <= 1048576"},
		{"disk:\n  reserves: {release: 8589934592}\n", "disk.reserves.release must be <= 1048576"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadPreview(writeDiskConfig(t, tc.body))
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
