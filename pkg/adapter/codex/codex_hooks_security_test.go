package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateHooks_RejectsHooksJSONSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".codex"), 0o755))
	victim := filepath.Join(t.TempDir(), "victim.json")
	require.NoError(t, os.WriteFile(victim, []byte("preserve-hooks-json"), 0o600))
	requireSymlink(t, victim, filepath.Join(root, ".codex", "hooks.json"))

	_, err := NewWithRoot(root).generateHooks(config.DefaultFullConfig("test"))
	require.Error(t, err)
	assertFileContent(t, victim, "preserve-hooks-json")
}

// A symlinked .codex directory would redirect hooks.json out of the
// repository, so generation fails at the write and the link target stays empty.
func TestGenerateHooks_RefusesToWriteThroughASymlinkedCodexDir(t *testing.T) {
	t.Parallel()
	root, outside := t.TempDir(), t.TempDir()
	requireSymlink(t, outside, filepath.Join(root, ".codex"))

	_, err := NewWithRoot(root).generateHooks(config.DefaultFullConfig("test"))

	require.EqualError(t, err, "codex hooks.json 쓰기 실패: managed directory must not be a symlink: .codex")
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// No hook script is installed since the completion hooks were retired
// (SPEC-PANERM-001 REQ-12), so generation writes nothing below .codex/hooks,
// even through a symlinked parent.
func TestGenerateHooks_WritesNothingBelowCodexHooks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".codex"), 0o755))
	outside := t.TempDir()
	requireSymlink(t, outside, filepath.Join(root, ".codex", "hooks"))

	_, err := NewWithRoot(root).generateHooks(config.DefaultFullConfig("test"))
	require.NoError(t, err)
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func requireSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(data))
}
