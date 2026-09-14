package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Only prune entries may delete: an emit/retain row sharing the loop must never
// take a user file with it.
func TestPruneManagedPaths_IgnoresNonPruneActions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	keep := filepath.Join(root, "keep.md")
	require.NoError(t, os.WriteFile(keep, []byte("kept"), 0o644))

	backupDir := ""
	require.NoError(t, PruneManagedPaths(root, []ManifestDiffEntry{
		{Path: "keep.md", Action: ManifestActionEmit, OldChecksum: Checksum("kept")},
		{Path: "keep.md", Action: ManifestActionRetain, OldChecksum: Checksum("kept")},
	}, &backupDir))

	assert.FileExists(t, keep)
	assert.Empty(t, backupDir, "no prune happened, so no backup dir should be created")
}

// An unmodified managed file is compiler-owned: removed with no backup, and the
// emptied directory chain collapses up to -- but never including -- the root.
func TestPruneManagedPaths_RemovesUnmodifiedFileAndEmptyParents(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "gen.md"), []byte("managed"), 0o644))

	backupDir := ""
	require.NoError(t, PruneManagedPaths(root, []ManifestDiffEntry{
		{Path: filepath.Join("a", "b", "gen.md"), Action: ManifestActionPrune, OldChecksum: Checksum("managed")},
	}, &backupDir))

	assert.NoDirExists(t, filepath.Join(root, "a"))
	assert.DirExists(t, root, "the workspace root is never an empty-parent candidate")
	assert.Empty(t, backupDir, "an unmodified managed file needs no backup")
}

// A sibling file in the parent directory pins that directory: prune must not
// widen into a directory the user still owns content in.
func TestPruneManagedPaths_KeepsParentHoldingUserFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "shared")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gen.md"), []byte("managed"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "user.md"), []byte("mine"), 0o644))

	backupDir := ""
	require.NoError(t, PruneManagedPaths(root, []ManifestDiffEntry{
		{Path: filepath.Join("shared", "gen.md"), Action: ManifestActionPrune, OldChecksum: Checksum("managed")},
	}, &backupDir))

	assert.NoFileExists(t, filepath.Join(dir, "gen.md"))
	assert.FileExists(t, filepath.Join(dir, "user.md"))
}

// User edits to a managed file are recoverable: the changed bytes land in a
// lazily created backup dir before the file is removed.
func TestPruneManagedPaths_BacksUpUserModifiedFileBeforeRemoval(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "edited.md"), []byte("user edit"), 0o644))

	backupDir := ""
	require.NoError(t, PruneManagedPaths(root, []ManifestDiffEntry{
		{Path: "edited.md", Action: ManifestActionPrune, OldChecksum: Checksum("original")},
	}, &backupDir))

	require.NotEmpty(t, backupDir, "a modified file must force backup dir creation")
	assert.NoFileExists(t, filepath.Join(root, "edited.md"))
	data, err := os.ReadFile(filepath.Join(backupDir, "edited.md"))
	require.NoError(t, err)
	assert.Equal(t, "user edit", string(data))
}

// Without anywhere to put the backup, prune fails closed: the user's edited
// bytes must still be on disk after the error.
func TestPruneManagedPaths_RefusesModifiedFileWhenBackupUnavailable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "edited.md")
	require.NoError(t, os.WriteFile(target, []byte("user edit"), 0o644))

	err := PruneManagedPaths(root, []ManifestDiffEntry{
		{Path: "edited.md", Action: ManifestActionPrune, OldChecksum: Checksum("original")},
	}, nil)

	require.Error(t, err)
	assert.ErrorContains(t, err, "backup dir unavailable")
	assert.FileExists(t, target)
}

// A missing prune target is not an error; later entries in the same batch must
// still be processed.
func TestPruneManagedPaths_SkipsMissingTargetAndContinues(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	present := filepath.Join(root, "present.md")
	require.NoError(t, os.WriteFile(present, []byte("managed"), 0o644))

	backupDir := ""
	require.NoError(t, PruneManagedPaths(root, []ManifestDiffEntry{
		{Path: "absent.md", Action: ManifestActionPrune, OldChecksum: Checksum("gone")},
		{Path: "present.md", Action: ManifestActionPrune, OldChecksum: Checksum("managed")},
	}, &backupDir))

	assert.NoFileExists(t, present)
}

// os.Remove follows parent-directory symlinks, so a link planted above a
// managed path would delete outside the workspace unless the resolved target is
// contained.
func TestPruneManagedPaths_RefusesPathEscapingViaParentSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	external := filepath.Join(outside, "victim.md")
	require.NoError(t, os.WriteFile(external, []byte("outside-owned"), 0o644))
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	backupDir := ""
	err := PruneManagedPaths(root, []ManifestDiffEntry{
		{Path: filepath.Join("link", "victim.md"), Action: ManifestActionPrune},
	}, &backupDir)

	require.Error(t, err)
	assert.ErrorContains(t, err, "outside workspace")
	assert.FileExists(t, external)
}

// Escaping and absolute spellings are rejected before any filesystem mutation.
func TestSafePruneFilePath_RejectsUnsafeSpellings(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, rel := range map[string]string{
		"self":     ".",
		"parent":   filepath.Join("..", "escape.md"),
		"absolute": filepath.Join(root, "abs.md"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := SafePruneFilePath(root, rel)
			require.Error(t, err)
			assert.ErrorContains(t, err, "unsafe prune path")
		})
	}
}

// A workspace that does not exist yet yields an empty prune set rather than an
// error: Generate reads a permission preimage before creating the root.
func TestSafePruneFilePath_MissingRootYieldsNoTarget(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "not-created")

	target, err := SafePruneFilePath(root, "gen.md")

	require.NoError(t, err)
	assert.Empty(t, target)
}
