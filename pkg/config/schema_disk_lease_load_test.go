package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// Load admission keys: absent keys keep 0.85, 8 GiB, and 10 minutes; 0
// turns each off.
func TestDiskConf_LeaseLoadKeys(t *testing.T) {
	t.Parallel()
	def, err := config.LoadPreview(writeDiskConfig(t, ""))
	require.NoError(t, err)
	eff := def.Disk.Effective().Lease
	assert.InDelta(t, 0.85, eff.MaxLoadRatio, 1e-9)
	assert.Equal(t, 8, eff.MinFreeMemGB)
	assert.Equal(t, 10, eff.MaxLoadWaitMinutes)

	cfg, err := config.LoadPreview(writeDiskConfig(t,
		"disk:\n  lease: {max_load_ratio: 0, min_free_mem_gb: 0, max_load_wait_minutes: 0}\n"))
	require.NoError(t, err)
	off := cfg.Disk.Effective().Lease
	assert.Zero(t, off.MaxLoadRatio)
	assert.Zero(t, off.MinFreeMemGB)
	assert.Zero(t, off.MaxLoadWaitMinutes)

	set, err := config.LoadPreview(writeDiskConfig(t, "disk:\n  lease: {max_load_ratio: 1.5, min_free_mem_gb: 16}\n"))
	require.NoError(t, err)
	assert.InDelta(t, 1.5, set.Disk.Effective().Lease.MaxLoadRatio, 1e-9)
	assert.Equal(t, 16, set.Disk.Effective().Lease.MinFreeMemGB)
}

func TestDiskConf_LeaseLoadBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ body, want string }{
		{"disk:\n  lease: {max_load_ratio: -0.1}\n", "disk.lease.max_load_ratio must be from 0 to 64"},
		{"disk:\n  lease: {max_load_ratio: 65}\n", "disk.lease.max_load_ratio must be from 0 to 64"},
		{"disk:\n  lease: {max_load_ratio: .nan}\n", "disk.lease.max_load_ratio must be from 0 to 64"},
		{"disk:\n  lease: {min_free_mem_gb: -1}\n", "disk.lease.min_free_mem_gb must be from 0 to"},
		{"disk:\n  lease: {max_load_wait_minutes: -1}\n", "disk.lease.max_load_wait_minutes must be >= 0"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadPreview(writeDiskConfig(t, tc.body))
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
