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

	// The automatic reclaim reads its settings at the root but runs in the
	// session's cwd.
	spawned := withAutoSpawn(t)
	require.True(t, maybeStartAutoReclaim(sub))
	assert.Equal(t, []string{sub}, *spawned)
}

// The automatic reclaim of a session whose cwd is inside a harness worktree
// that has no autopus.yaml of its own (the file is untracked in the main
// checkout) runs there, without --dir, so the worktree holding the session
// is never a candidate, though the config comes from the main checkout.
func TestDiskSubdir_AutoReclaimKeepsTheSessionWorktree(t *testing.T) {
	rf := newReclaimFixture(t)
	withTempDiskState(t)
	diskGitT(t, rf.repo, "update-ref", "refs/remotes/origin/main", "main")
	require.NoError(t, os.WriteFile(filepath.Join(rf.repo, "autopus.yaml"),
		[]byte("mode: full\nproject_name: rr\nplatforms:\n  - claude-code\n"), 0o600))
	require.NoFileExists(t, filepath.Join(rf.harness, "autopus.yaml"))
	sub := filepath.Join(rf.harness, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))

	spawned := withAutoSpawn(t)
	require.True(t, maybeStartAutoReclaim(sub))
	require.Equal(t, []string{sub}, *spawned, "the reclaim runs in the session's cwd")
	cmd := autoReclaimCommand("auto", sub)
	assert.Equal(t, sub, cmd.Dir)
	assert.Equal(t, []string{"auto", "disk", "reclaim", "--auto", "--apply"}, cmd.Args, "no --dir")

	// What the spawned child does: run in the session's cwd without --dir.
	t.Chdir(sub)
	prevGetwd := diskGetwd
	diskGetwd = func() (string, error) { return sub, nil }
	t.Cleanup(func() { diskGetwd = prevGetwd })
	out := runDiskCLI(t, "disk", "reclaim", "--auto", "--apply")
	require.Equal(t, 0, out.code, out.stderr+out.stdout)
	assert.DirExists(t, rf.harness, "the worktree holding the session is kept")
}
