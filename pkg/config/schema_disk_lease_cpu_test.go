package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// CPU lease keys: cpu_slots and dir load, and absent keys keep the defaults
// (0 = the logical CPU count, "" = the per-user state dir).
func TestDiskConf_LeaseCPUKeys(t *testing.T) {
	t.Parallel()
	cfg, err := config.LoadPreview(writeDiskConfig(t, "disk:\n  lease: {cpu_slots: 12, dir: /var/tmp/autopus-lease, aging_minutes: 0}\n"))
	require.NoError(t, err)
	eff := cfg.Disk.Effective().Lease
	assert.Equal(t, 12, eff.CPUSlots)
	assert.Zero(t, eff.AgingMinutes, "0 turns aging off")
	assert.Equal(t, "/var/tmp/autopus-lease", eff.Dir)

	def, err := config.LoadPreview(writeDiskConfig(t, ""))
	require.NoError(t, err)
	assert.Zero(t, def.Disk.Effective().Lease.CPUSlots)
	assert.Empty(t, def.Disk.Effective().Lease.Dir)
}

func TestDiskConf_LeaseCPUBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ body, want string }{
		{"disk:\n  lease: {cpu_slots: -1}\n", "disk.lease.cpu_slots must be >= 0"},
		{"disk:\n  lease: {cpu_slots: 100000}\n", "disk.lease.cpu_slots must be <= 4096"},
		{"disk:\n  lease: {dir: relative/dir}\n", "disk.lease.dir must be an absolute path"},
		{"disk:\n  lease: {aging_minutes: -1}\n", "disk.lease.aging_minutes must be >= 0"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadPreview(writeDiskConfig(t, tc.body))
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
