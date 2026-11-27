package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMachineOps(t *testing.T) {
	on, off := true, false
	var nilCfg *HarnessConfig
	assert.True(t, nilCfg.MachineOpsEnabled())

	legacy := DefaultFullConfig("legacy")
	assert.True(t, legacy.MachineOpsEnabled(), "a project written before the key keeps the helpers")
	assert.True(t, legacy.EffectiveHooks().IsDiskGateEnabled())
	assert.Equal(t, legacy.Disk.Effective().Reclaim.Auto, legacy.AutoReclaimEnabled())

	core := DefaultFullConfig("core")
	core.MachineOps = &off
	assert.False(t, core.EffectiveHooks().IsDiskGateEnabled())
	assert.False(t, core.AutoReclaimEnabled())
	assert.Nil(t, core.Hooks.DiskGate, "the stored config is not changed")

	core.Hooks.DiskGate = &DiskGateConf{Enabled: &on}
	core.Disk.Reclaim.Auto = &on
	assert.True(t, core.EffectiveHooks().IsDiskGateEnabled(), "an explicit setting wins")
	assert.True(t, core.AutoReclaimEnabled())
}
