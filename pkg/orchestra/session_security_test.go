package orchestra

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSessionDirectory = "autopus-orchestra-sessions"

func isolatedSessionPath(t *testing.T, id string) string {
	t.Helper()
	return filepath.Join(os.TempDir(), testSessionDirectory, id+".json")
}

func TestSessionPersistence_InvalidIDsFailClosed(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	invalidIDs := []string{"", ".", "..", "../victim", "/tmp/victim", `a\b`, "a/b", "has space"}
	for _, id := range invalidIDs {
		t.Run(id, func(t *testing.T) {
			session := OrchestraSession{ID: id, CreatedAt: time.Now()}

			assert.ErrorContains(t, SaveSession(session), "invalid session ID")
			_, loadErr := LoadSession(id)
			assert.ErrorContains(t, loadErr, "invalid session ID")
			assert.ErrorContains(t, RemoveSession(id), "invalid session ID")
		})
	}
}

func TestSaveSession_UsesPrivateDedicatedDirectory(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	session := OrchestraSession{ID: "private-dir", CreatedAt: time.Now()}

	require.NoError(t, SaveSession(session))
	t.Cleanup(func() { _ = RemoveSession(session.ID) })

	dirInfo, err := os.Stat(filepath.Dir(isolatedSessionPath(t, session.ID)))
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(isolatedSessionPath(t, session.ID))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
}

func TestSaveSession_ExistingFileIsNeverOverwritten(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	path := isolatedSessionPath(t, "existing")
	require.NoError(t, os.Mkdir(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))

	err := SaveSession(OrchestraSession{ID: "existing", CreatedAt: time.Now()})

	require.Error(t, err)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(data))
}

func TestSessionPersistence_SymlinksFailClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated Windows privileges")
	}
	t.Setenv("TMPDIR", t.TempDir())
	dir := filepath.Join(os.TempDir(), testSessionDirectory)
	require.NoError(t, os.Mkdir(dir, 0o700))
	target := filepath.Join(os.TempDir(), "outside-session.json")
	require.NoError(t, os.WriteFile(target, []byte(`{"id":"linked"}`), 0o600))
	link := isolatedSessionPath(t, "linked")
	require.NoError(t, os.Symlink(target, link))

	assert.Error(t, SaveSession(OrchestraSession{ID: "linked", CreatedAt: time.Now()}))
	_, loadErr := LoadSession("linked")
	assert.Error(t, loadErr)
	assert.Error(t, RemoveSession("linked"))

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, `{"id":"linked"}`, string(data))
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
}

func TestSessionPersistence_SymlinkedDirectoryFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated Windows privileges")
	}
	t.Setenv("TMPDIR", t.TempDir())
	target := filepath.Join(os.TempDir(), "outside-session-dir")
	require.NoError(t, os.Mkdir(target, 0o700))
	require.NoError(t, os.Symlink(target, filepath.Join(os.TempDir(), testSessionDirectory)))

	err := SaveSession(OrchestraSession{ID: "linked-dir", CreatedAt: time.Now()})

	require.ErrorContains(t, err, "not a private directory")
	_, statErr := os.Stat(filepath.Join(target, "linked-dir.json"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
