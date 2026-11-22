package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Without --dir, `auto disk` run in a subdirectory reads the autopus.yaml of
// the project root, where the disk gate reads it: the lease's CPU slots and
// the check's floor both come from the root config.
func TestDiskSubdir_ReadsProjectRootConfig(t *testing.T) {
	root := cpuLeaseProject(t, "  min_free_gb: 60\n  lease: {cpu_slots: 4}\n", 16)
	sub := filepath.Join(root, "web")
	require.NoError(t, os.Mkdir(sub, 0o755))
	t.Chdir(sub)

	lease := runDiskCLI(t, "disk", "lease", "--cpu", "16", "--", "sh", "-c", printLeaseEnv)
	assert.Equal(t, 0, lease.code, lease.stderr)
	assert.Equal(t, "4 4\n", lease.stdout, "the root's disk.lease.cpu_slots clamps the grant")

	check := runDiskCLI(t, "disk", "check")
	assert.Equal(t, 3, check.code, check.stdout+check.stderr)
	assert.Contains(t, check.stderr, "60.0 GiB required (disk.min_free_gb)")

	// An explicit --dir still names the directory whose config is read.
	explicit := runDiskCLI(t, "disk", "check", "--dir", sub)
	assert.Equal(t, 0, explicit.code, explicit.stdout+explicit.stderr)

	// The automatic reclaim reads and reclaims the root project too.
	spawned := withAutoSpawn(t)
	require.True(t, maybeStartAutoReclaim(sub))
	want, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	assert.Equal(t, []string{want}, *spawned)
}
