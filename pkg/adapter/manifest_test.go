package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/version"
)

func TestNewManifest(t *testing.T) {
	t.Parallel()
	m := NewManifest("claude-code")
	assert.Equal(t, "1.0.0", m.Version)
	assert.Equal(t, "claude-code", m.Platform)
	assert.NotEmpty(t, m.GeneratedAt)
	assert.NotNil(t, m.Files)
}

func TestManifestFromFiles(t *testing.T) {
	t.Parallel()
	pf := &PlatformFiles{
		Files: []FileMapping{
			{TargetPath: ".claude/CLAUDE.md", Checksum: "abc123", OverwritePolicy: OverwriteMarker},
			{TargetPath: ".claude/rules/autopus/lore.md", Checksum: "def456", OverwritePolicy: OverwriteAlways},
		},
	}
	m := ManifestFromFiles("claude-code", pf)
	assert.Equal(t, "claude-code", m.Platform)
	assert.Len(t, m.Files, 2)
	assert.Equal(t, "abc123", m.Files[".claude/CLAUDE.md"].Checksum)
	assert.Equal(t, OverwriteMarker, m.Files[".claude/CLAUDE.md"].Policy)
}

func TestLoadManifest_NotExist(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	m, err := LoadManifest(tmp, "claude-code")
	require.NoError(t, err)
	assert.Nil(t, m, "non-existent manifest should return nil, not error")
}

func TestLoadManifest_RoundTrip(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	original := NewManifest("codex")
	original.Files["some/file.md"] = ManifestFile{Checksum: "xyz789", Policy: OverwriteNever}

	require.NoError(t, original.Save(tmp))

	loaded, err := LoadManifest(tmp, "codex")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "codex", loaded.Platform)
	assert.Equal(t, "xyz789", loaded.Files["some/file.md"].Checksum)
	assert.Equal(t, OverwriteNever, loaded.Files["some/file.md"].Policy)
}

func TestLoadManifest_CorruptJSON(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir := filepath.Join(tmp, manifestDir)
	require.NoError(t, os.MkdirAll(dir, 0755))
	path := filepath.Join(dir, "claude-code-"+manifestFile)
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0644))

	_, err := LoadManifest(tmp, "claude-code")
	require.Error(t, err)
}

func TestChecksum(t *testing.T) {
	t.Parallel()
	// Same input must produce same output.
	c1 := Checksum("hello world")
	c2 := Checksum("hello world")
	assert.Equal(t, c1, c2)
	// Different input must produce different output.
	c3 := Checksum("hello world!")
	assert.NotEqual(t, c1, c3)
	// Known SHA256 of empty string.
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", Checksum(""))
}

func TestResolveAction_MarkerPolicy(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// Marker policy always overwrites, regardless of file existence or manifest state.
	action := ResolveAction(tmp, "any/file.md", OverwriteMarker, nil)
	assert.Equal(t, ActionOverwrite, action)
}

func TestResolveAction_NoManifest_FileExists(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "existing.md")
	require.NoError(t, os.WriteFile(path, []byte("content"), 0644))

	action := ResolveAction(tmp, "existing.md", OverwriteAlways, nil)
	assert.Equal(t, ActionOverwrite, action)
}

func TestResolveAction_NoManifest_FileAbsent(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	action := ResolveAction(tmp, "missing.md", OverwriteAlways, nil)
	assert.Equal(t, ActionCreate, action)
}

func TestResolveAction_WithManifest_UserDeleted(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// File does not exist on disk but was managed in old manifest → Skip
	old := NewManifest("claude-code")
	old.Files["deleted.md"] = ManifestFile{Checksum: "aaa", Policy: OverwriteAlways}

	action := ResolveAction(tmp, "deleted.md", OverwriteAlways, old)
	assert.Equal(t, ActionSkip, action)
}

func TestResolveAction_WithManifest_NewFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// File does not exist and was never managed → Create
	old := NewManifest("claude-code")

	action := ResolveAction(tmp, "newfile.md", OverwriteAlways, old)
	assert.Equal(t, ActionCreate, action)
}

func TestResolveAction_WithManifest_Unchanged(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	content := "unchanged content"
	path := filepath.Join(tmp, "file.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	old := NewManifest("claude-code")
	old.Files["file.md"] = ManifestFile{Checksum: Checksum(content), Policy: OverwriteAlways}

	action := ResolveAction(tmp, "file.md", OverwriteAlways, old)
	assert.Equal(t, ActionOverwrite, action)
}

func TestResolveAction_WithManifest_UserModified(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.md")
	require.NoError(t, os.WriteFile(path, []byte("modified by user"), 0644))

	old := NewManifest("claude-code")
	old.Files["file.md"] = ManifestFile{Checksum: Checksum("original content"), Policy: OverwriteAlways}

	action := ResolveAction(tmp, "file.md", OverwriteAlways, old)
	assert.Equal(t, ActionBackup, action)
}

func TestResolveAction_WithManifest_UnmanagedExistingFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "unmanaged.md")
	require.NoError(t, os.WriteFile(path, []byte("some content"), 0644))

	// File exists but was NOT in the old manifest → Backup (preserve user file)
	old := NewManifest("claude-code")

	action := ResolveAction(tmp, "unmanaged.md", OverwriteAlways, old)
	assert.Equal(t, ActionBackup, action)
}

func TestBackupFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// Create source file.
	srcContent := []byte("source content")
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "src.md"), srcContent, 0644))

	backupDir := filepath.Join(tmp, "backup")
	backupPath, err := BackupFile(tmp, "src.md", backupDir)
	require.NoError(t, err)
	assert.FileExists(t, backupPath)

	got, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, srcContent, got)
}

func TestBackupFile_SourceNotExist(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	_, err := BackupFile(tmp, "nonexistent.md", filepath.Join(tmp, "backup"))
	require.Error(t, err)
}

func TestCreateBackupDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir, err := CreateBackupDir(tmp)
	require.NoError(t, err)
	assert.DirExists(t, dir)
	// Dir should be inside .autopus/backup/
	assert.Contains(t, dir, filepath.Join(tmp, manifestDir, "backup"))
}

func TestManifestSave_RecordsGenerator(t *testing.T) {
	dir := t.TempDir()
	m := NewManifest("claude-code")
	require.NoError(t, m.Save(dir))
	loaded, err := LoadManifest(dir, "claude-code")
	require.NoError(t, err)
	assert.Equal(t, version.Version(), loaded.Generator)
	assert.NotEmpty(t, loaded.Generator)
}
