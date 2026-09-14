package orchestra

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Prompt-file transport must degrade to direct input instead of dispatching a
// provider with a half-written or missing prompt file.

func TestWritePromptMarkdown_FailsWhenPromptOrResponseDirIsUnusable(t *testing.T) {
	t.Parallel()
	provider := ProviderConfig{Name: "codex", Binary: "codex"}

	fileAsWorkingDir := filepath.Join(t.TempDir(), "working")
	require.NoError(t, os.WriteFile(fileAsWorkingDir, []byte("not a dir"), 0o600))
	_, _, _, err := writePromptMarkdown(fileAsWorkingDir, provider, 1, "body")
	require.ErrorContains(t, err, "create prompt dir")

	blockedResponses := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(blockedResponses, filepath.Dir(responseFilesDir)), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(blockedResponses, responseFilesDir), []byte("x"), 0o600))
	_, _, _, err = writePromptMarkdown(blockedResponses, provider, 1, "body")
	require.ErrorContains(t, err, "create response dir")

	readOnlyPrompts := t.TempDir()
	promptDir := filepath.Join(readOnlyPrompts, promptFilesDir)
	require.NoError(t, os.MkdirAll(promptDir, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(readOnlyPrompts, responseFilesDir), 0o700))
	require.NoError(t, os.Chmod(promptDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(promptDir, 0o700) })
	_, _, _, err = writePromptMarkdown(readOnlyPrompts, provider, 1, "body")
	require.ErrorContains(t, err, "create prompt file")
}

func TestPanePromptText_FallsBackToDirectPromptWhenFileTransportFails(t *testing.T) {
	t.Parallel()
	broken := filepath.Join(t.TempDir(), "working")
	require.NoError(t, os.WriteFile(broken, []byte("not a dir"), 0o600))

	instruction, path, responsePath := panePromptText(
		OrchestraConfig{WorkingDir: broken}, ProviderConfig{Name: "codex"}, 1, "full prompt body")
	assert.Equal(t, "full prompt body", instruction, "the raw prompt must still reach the provider")
	assert.Empty(t, path)
	assert.Empty(t, responsePath)
}

func TestWritePromptMarkdown_NormalizesRoundAndBlankWorkingDir(t *testing.T) {
	workingDir := t.TempDir()
	t.Chdir(workingDir)

	path, responsePath, instruction, err := writePromptMarkdown("  ", ProviderConfig{Name: "cl/aude"}, 0, "body")
	require.NoError(t, err)
	defer cleanupPromptFiles([]string{path, responsePath})

	assert.Contains(t, filepath.Base(path), "claude-round-1-", "a non-positive round is normalized to round 1")
	assert.True(t, filepath.IsAbs(path), "a blank working dir resolves against the process directory")
	assert.Contains(t, instruction, path)
	assert.Contains(t, instruction, responsePath)
	assert.NotContains(t, filepath.Base(path), "cl/aude", "provider names must be sanitized into the file name")
}

func TestCleanupPromptFiles_SkipsBlankPathsAndRemovesTheRest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	present := filepath.Join(dir, "prompt.md")
	require.NoError(t, os.WriteFile(present, []byte("body"), 0o600))

	cleanupPromptFiles([]string{"", "   ", present})

	_, err := os.Stat(present)
	assert.True(t, os.IsNotExist(err))
}

func TestReadResponseFile_RejectsBlankPathAndUnreadableFile(t *testing.T) {
	t.Parallel()

	output, ok := readResponseFile("   ")
	assert.False(t, ok)
	assert.Empty(t, output)

	output, ok = readResponseFile(filepath.Join(t.TempDir(), "absent.md"))
	assert.False(t, ok)
	assert.Empty(t, output)

	unterminated := filepath.Join(t.TempDir(), "partial.md")
	require.NoError(t, os.WriteFile(unterminated, []byte(responseBeginMarker+"\nhalf written\n"), 0o600))
	output, ok = readResponseFile(unterminated)
	assert.False(t, ok, "a response without its end marker must not be consumed")
	assert.Empty(t, output)
}
