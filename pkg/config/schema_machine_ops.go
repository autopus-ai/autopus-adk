package config

// MachineOpsEnabled reports machine_ops: unset counts as on.
func (c *HarnessConfig) MachineOpsEnabled() bool {
	return c == nil || c.MachineOps == nil || *c.MachineOps
}

// EffectiveHooks is Hooks with the machine-ops defaults applied: with
// machine_ops off, a disk gate the project did not set is off.
func (c *HarnessConfig) EffectiveHooks() HooksConf {
	h := c.Hooks
	if !c.MachineOpsEnabled() && h.DiskGate == nil {
		off := false
		h.DiskGate = &DiskGateConf{Enabled: &off}
	}
	return h
}

// AutoReclaimEnabled is disk.reclaim.auto with the machine-ops default: with
// machine_ops off, an unset value is off.
func (c *HarnessConfig) AutoReclaimEnabled() bool {
	if c.Disk.Reclaim.Auto == nil && !c.MachineOpsEnabled() {
		return false
	}
	return c.Disk.Effective().Reclaim.Auto
}
