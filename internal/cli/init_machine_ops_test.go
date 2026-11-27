package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// init writes machine_ops: false only for a project with no autopus.yaml
// here or above; a re-init or a nested init keeps the existing value.
func TestInitMachineOps(t *testing.T) {
	fresh := t.TempDir()
	got := initMachineOps(fresh)
	require.NotNil(t, got)
	assert.False(t, *got, "a new project starts with the verification core")

	write := func(dir, extra string) {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"),
			[]byte("mode: full\nproject_name: x\nplatforms:\n  - claude-code\n"+extra), 0o600))
	}
	legacy := t.TempDir()
	write(legacy, "")
	assert.Nil(t, initMachineOps(legacy), "a re-init keeps an unset key unset (on)")

	core := t.TempDir()
	write(core, "machine_ops: false\n")
	got = initMachineOps(core)
	require.NotNil(t, got)
	assert.False(t, *got, "a re-init keeps false")

	unreadable := t.TempDir()
	write(unreadable, "future_key_x: 1\n")
	assert.Nil(t, initMachineOps(unreadable), "a file this binary cannot load is not a new project")

	mono := t.TempDir()
	write(mono, "machine_ops: true\n")
	pkg := filepath.Join(mono, "packages", "web")
	require.NoError(t, os.MkdirAll(pkg, 0o755))
	got = initMachineOps(pkg)
	require.NotNil(t, got)
	assert.True(t, *got, "a package inherits the repository's value")
}

// With machine_ops off, a CPU-gate hook does not carry the disk check, and
// the automatic reclaim stays off.
func TestMachineOpsOff_DiskGateAndReclaim(t *testing.T) {
	dir := writeDiskProject(t, "machine_ops: false\nhooks:\n  cpu_gate: true\n")
	withDiskFree(t, gib(4), "/fixture")
	withMachineLoad(t, 0, 24) // the CPU half of the hook must not read this machine
	assert.Empty(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go test ./...", nil)),
		"the disk check is off with machine_ops off")

	withTempDiskState(t)
	spawned := withAutoSpawn(t)
	core := writeDiskProject(t, "machine_ops: false\n")
	assert.False(t, maybeStartAutoReclaim(core))
	assert.Empty(t, *spawned)
	on := writeDiskProject(t, "machine_ops: false\ndisk:\n  reclaim:\n    auto: true\n")
	assert.True(t, maybeStartAutoReclaim(on), "an explicit disk.reclaim.auto wins")
}
