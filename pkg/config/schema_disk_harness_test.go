package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The disk harness keys load, resolve, and validate.
func TestDiskConf_HarnessKeys(t *testing.T) {
	t.Parallel()
	cfg, err := config.LoadPreview(writeDiskConfig(t,
		"disk:\n  reclaim: {auto: false, auto_interval_minutes: 5, large_file_idle_minutes: 120}\n"+
			"  consumers: {extra_roots: [/opt/runner/_work]}\nhooks:\n  disk_gate: false\n"))
	require.NoError(t, err)
	eff := cfg.Disk.Effective()
	assert.False(t, eff.Reclaim.Auto)
	assert.Equal(t, 5, eff.Reclaim.AutoIntervalMinutes)
	assert.Equal(t, 120, eff.Reclaim.LargeFileIdleMinutes)
	assert.Equal(t, []string{"/opt/runner/_work"}, eff.ExtraRoots)
	assert.False(t, cfg.Hooks.IsDiskGateEnabled())
	assert.True(t, config.HooksConf{}.IsDiskGateEnabled(), "unset disk_gate means on")

	mapped, err := config.LoadPreview(writeDiskConfig(t,
		"hooks:\n  disk_gate:\n    extra_commands: [make release, bazel build]\n"))
	require.NoError(t, err)
	assert.True(t, mapped.Hooks.IsDiskGateEnabled(), "the mapping form without enabled stays on")
	assert.Equal(t, []string{"make release", "bazel build"}, mapped.Hooks.DiskGateExtraCommands())
	off, err := config.LoadPreview(writeDiskConfig(t, "hooks:\n  disk_gate: {enabled: false}\n"))
	require.NoError(t, err)
	assert.False(t, off.Hooks.IsDiskGateEnabled())

	for _, tc := range []struct{ body, want string }{
		{"hooks:\n  disk_gate: maybe\n", "hooks.disk_gate must be true, false, or a mapping"},
		{"hooks:\n  disk_gate: {extra_comands: [x]}\n", "extra_comands"},
		{"hooks:\n  disk_gate: {extra_commands: ['  ']}\n", "hooks.disk_gate.extra_commands entries must not be empty"},
		{"disk:\n  reclaim: {auto_interval_minutes: 4}\n", "disk.reclaim.auto_interval_minutes must be >= 5"},
		{"disk:\n  reclaim: {large_file_idle_minutes: 30}\n", "disk.reclaim.large_file_idle_minutes must be >= 60"},
		{"disk:\n  consumers: {extra_roots: [relative/dir]}\n", "disk.consumers.extra_roots entries must be absolute"},
	} {
		_, err := config.LoadPreview(writeDiskConfig(t, tc.body))
		assert.ErrorContains(t, err, tc.want, tc.body)
	}
}
