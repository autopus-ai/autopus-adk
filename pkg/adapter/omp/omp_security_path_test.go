package omp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// TestOMPClean_DoesNotFollowSymlinkedManifestPath is the L-2 regression. The
// existence probe uses Lstat but the checksum and backup read used os.ReadFile,
// which follows links, so a symlink planted at a manifest path copied the
// target's contents into .autopus/backup/.
func TestOMPClean_DoesNotFollowSymlinkedManifestPath(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)
	secret := filepath.Join(dir, "outside-secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("SUPER-SECRET-VALUE\n"), 0o600))

	victim := filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"branding.md")
	require.NoError(t, os.Remove(victim))
	require.NoError(t, os.Symlink(secret, victim))

	require.Error(t, NewWithRoot(dir).Clean(context.Background()),
		"an unverifiable managed path must fail the whole destructive preflight")

	assert.FileExists(t, secret, "the symlink target itself must never be deleted")
	data, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, "SUPER-SECRET-VALUE\n", string(data))

	assert.NotContains(t, strings.Join(backupPaths(t, dir), "\n"), "branding.md",
		"a symlinked manifest path must not be copied into the backup directory")
	assert.FileExists(t, filepath.Join(dir, ".omp", "commands", "auto.md"), "preflight failure must precede every mutation")
}

// TestOMPWriteMapping_RejectsSymlinkedTarget covers the write half of L-2: a
// symlink planted where omp is about to write must not redirect the write onto
// the link target.
func TestOMPWriteMapping_RejectsSymlinkedTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(root, "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("original\n"), 0o600))

	linkDir := filepath.Join(root, ompRuleDir)
	require.NoError(t, os.MkdirAll(linkDir, 0o755))
	link := filepath.Join(linkDir, ompRuleFilePrefix+"branding.md")
	require.NoError(t, os.Symlink(outside, link))

	err := writeMapping(root, adapter.FileMapping{
		TargetPath:      filepath.Join(ompRuleDir, ompRuleFilePrefix+"branding.md"),
		OverwritePolicy: adapter.OverwriteAlways,
		Content:         []byte("managed body\n"),
	})
	require.Error(t, err, "writing through a symlink must be refused")

	data, readErr := os.ReadFile(outside)
	require.NoError(t, readErr)
	assert.Equal(t, "original\n", string(data), "the link target must be untouched")
}

func backupPaths(t *testing.T, root string) []string {
	t.Helper()

	paths := make([]string, 0)
	for rel := range backupCopies(t, root) {
		paths = append(paths, rel)
	}
	return paths
}

// TestOMPClean_DoesNotDeleteThroughParentDirectorySymlink is the F-1 regression.
// The earlier symlink fix gated only the checksum/backup read; os.Remove still
// ran unconditionally, and it resolves PARENT-directory symlinks. A manifest
// path whose parent is a link therefore deleted a file outside the workspace,
// silently, because `auto platform remove omp` discards Clean's error.
func TestOMPClean_DoesNotDeleteThroughParentDirectorySymlink(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)

	outside := t.TempDir()
	victim := filepath.Join(outside, "auto.md")
	require.NoError(t, os.WriteFile(victim, []byte("OUTSIDE-WORKSPACE\n"), 0o600))

	// Replace the managed commands directory with a link to the outside directory,
	// so every .omp/commands/<name>.md manifest path resolves out of the workspace.
	commandsDir := filepath.Join(dir, ".omp", "commands")
	require.NoError(t, os.RemoveAll(commandsDir))
	require.NoError(t, os.Symlink(outside, commandsDir))

	require.Error(t, NewWithRoot(dir).Clean(context.Background()),
		"a parent symlink must fail the whole destructive preflight")

	assert.FileExists(t, victim, "a file outside the workspace must survive Clean")
	data, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "OUTSIDE-WORKSPACE\n", string(data))
	assert.DirExists(t, outside, "the linked-to directory must survive too")
	assert.FileExists(t, filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"branding.md"),
		"preflight failure must precede every mutation")
}

// TestOMPClean_SkipsSymlinkedEntryWithoutUnlinking pins the containment choice:
// an entry that fails the workspace check aborts the complete clean plan.
func TestOMPClean_SkipsSymlinkedEntryWithoutUnlinking(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.md")
	require.NoError(t, os.WriteFile(target, []byte("secret\n"), 0o600))

	link := filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"branding.md")
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink(target, link))

	require.Error(t, NewWithRoot(dir).Clean(context.Background()))

	assert.FileExists(t, target, "the link target is never touched")
	info, err := os.Lstat(link)
	require.NoError(t, err, "the unverifiable entry is skipped, not unlinked")
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	assert.FileExists(t, filepath.Join(dir, ".omp", "commands", "auto.md"), "preflight failure must precede every mutation")
}

// TestOMPWriteMapping_RejectsEscapingRelativePath is the F-4 regression: the
// Generate write path accepted `..` segments that Update already refused.
func TestOMPWriteMapping_RejectsEscapingRelativePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "escaped.md")
	t.Cleanup(func() { _ = os.Remove(outside) })

	err := writeMapping(root, adapter.FileMapping{
		TargetPath:      filepath.Join("..", "escaped.md"),
		OverwritePolicy: adapter.OverwriteAlways,
		Content:         []byte("escaped\n"),
	})
	require.Error(t, err, "a path escaping the workspace must be refused")
	assert.NoFileExists(t, outside)
}

// TestOMPClean_DoesNotDeleteManifestThroughSymlinkedParent covers the manifest
// removal at the end of Clean, which ran unguarded while every other removal
// went through containment. os.Remove resolves parent-directory symlinks, so a
// linked .autopus/ deleted a manifest outside the workspace.
func TestOMPClean_DoesNotDeleteManifestThroughSymlinkedParent(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)
	outside := t.TempDir()

	// Relocate the whole runtime directory and link to it, so every .autopus/
	// path resolves out of the workspace.
	autopusDir := filepath.Join(dir, ".autopus")
	entries, err := os.ReadDir(autopusDir)
	require.NoError(t, err)
	for _, e := range entries {
		data, readErr := os.ReadFile(filepath.Join(autopusDir, e.Name()))
		if readErr != nil {
			continue
		}
		require.NoError(t, os.WriteFile(filepath.Join(outside, e.Name()), data, 0o644))
	}
	require.NoError(t, os.RemoveAll(autopusDir))
	require.NoError(t, os.Symlink(outside, autopusDir))

	manifestOutside := filepath.Join(outside, "omp-manifest.json")
	require.FileExists(t, manifestOutside, "the relocated manifest is the bait")

	require.Error(t, NewWithRoot(dir).Clean(context.Background()),
		"a symlinked manifest parent must fail the whole destructive preflight")

	assert.FileExists(t, manifestOutside,
		"a manifest reached through a symlinked parent must not be deleted")
	assert.FileExists(t, filepath.Join(dir, ".omp", "commands", "auto.md"), "preflight failure must precede every mutation")
}

// TestOMPGenerate_IgnoresSymlinkedUserConfig proves a plain OMP install does
// not read or rewrite user config, including a symlinked config path.
func TestOMPGenerate_IgnoresSymlinkedUserConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	secret := filepath.Join(dir, "outside-secret.yml")
	require.NoError(t, os.WriteFile(secret, []byte("apiKey: SUPER-SECRET\n"), 0o600))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".omp"), 0o755))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, configFile)))

	harness := config.DefaultFullConfig("omp-sweep")
	harness.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, harness))

	_, err := NewWithRoot(dir).Generate(context.Background(), harness)
	require.NoError(t, err)

	data, readErr := os.ReadFile(secret)
	require.NoError(t, readErr)
	assert.Equal(t, "apiKey: SUPER-SECRET\n", string(data),
		"the link target must be neither rewritten nor truncated")
}

// TestOMPClean_SkipsWorkspaceInternalParentSymlink is the O-1 regression. A
// parent symlink that stays INSIDE the workspace passes containment (the
// resolved target is still under the root) but fails the symlink check, and the
// check previously gated only the backup. The entry was therefore deleted
// through the link with no backup taken. Preflight now rejects the complete plan.
func TestOMPClean_SkipsWorkspaceInternalParentSymlink(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)

	// Move the managed rule directory aside, still inside the workspace, and
	// leave a link where the manifest expects it.
	rulesDir := filepath.Join(dir, ompRuleDir)
	realDir := filepath.Join(dir, ".omp", "real-rules")
	require.NoError(t, os.Rename(rulesDir, realDir))
	require.NoError(t, os.Symlink(realDir, rulesDir))

	// Make one entry checksum-mismatched so the old code would have wanted a
	// backup, then skipped it and deleted anyway.
	victim := filepath.Join(realDir, ompRuleFilePrefix+"branding.md")
	require.NoError(t, os.WriteFile(victim, []byte("USER EDITED CONTENT\n"), 0o644))

	require.Error(t, NewWithRoot(dir).Clean(context.Background()))

	data, err := os.ReadFile(victim)
	require.NoError(t, err,
		"an entry reached through a symlink must not be deleted without a backup")
	assert.Equal(t, "USER EDITED CONTENT\n", string(data))
	assert.FileExists(t, filepath.Join(dir, ".omp", "commands", "auto.md"), "preflight failure must precede every mutation")
}
