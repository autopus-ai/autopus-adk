package orchestra

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadOutputFile_SentinelOnly covers output file with only sentinel and no real content.
func TestReadOutputFile_SentinelOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	require.NoError(t, os.WriteFile(path, []byte(sentinel+"\n"), 0o600))
	assert.Empty(t, readOutputFile(path), "sentinel-only file should return empty output")
}

// TestReadOutputFile_EmptyFile covers an empty output file.
func TestReadOutputFile_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	assert.Empty(t, readOutputFile(path))
}

// TestCleanupPanes_CloseError verifies that a persistent Close error consumes
// the bounded retry budget without preventing temporary-file cleanup.
func TestCleanupPanes_CloseError(t *testing.T) {
	isolateSurfaceTracker(t)

	mock := newCmuxMock()
	mock.closeErr = fmt.Errorf("close failed: connection reset")

	dir := t.TempDir()
	tmpFile := filepath.Join(dir, "cleanup-test.out")
	require.NoError(t, os.WriteFile(tmpFile, []byte("data"), 0o600))

	panes := []paneInfo{
		{paneID: "pane-1", outputFile: tmpFile, provider: ProviderConfig{Name: "p1"}},
	}

	// Cleanup remains best-effort after the bounded Close retry budget expires.
	cleanupPanes(mock, panes)
	assert.Len(t, mock.closeCalls, closePaneSurfaceAttempts,
		"persistent Close errors must stop at the bounded retry budget")
	_, err := os.Stat(tmpFile)
	assert.ErrorIs(t, err, os.ErrNotExist,
		"temp file cleanup must continue after persistent Close errors")
}

// TestCleanupPanes_MultiplePanes verifies cleanup of multiple panes and files.
func TestCleanupPanes_MultiplePanes(t *testing.T) {
	t.Parallel()

	mock := newCmuxMock()
	dir := t.TempDir()

	var panes []paneInfo
	for i := 0; i < 3; i++ {
		f := filepath.Join(dir, fmt.Sprintf("out-%d.txt", i))
		require.NoError(t, os.WriteFile(f, []byte("data"), 0o600))
		panes = append(panes, paneInfo{
			paneID:     terminal.PaneID(fmt.Sprintf("pane-%d", i)),
			outputFile: f,
			provider:   ProviderConfig{Name: fmt.Sprintf("p%d", i)},
		})
	}

	cleanupPanes(mock, panes)
	assert.Len(t, mock.closeCalls, 3)
	for _, pi := range panes {
		_, err := os.Stat(pi.outputFile)
		assert.True(t, os.IsNotExist(err))
	}
}

// TestMergeByStrategy_EmptyResponses_AllStrategies covers empty responses for every strategy.
func TestMergeByStrategy_EmptyResponses_AllStrategies(t *testing.T) {
	t.Parallel()

	for _, s := range []Strategy{StrategyConsensus, StrategyPipeline, StrategyDebate} {
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			merged, summary := mergeByStrategy(s, nil, OrchestraConfig{})
			_ = merged
			_ = summary
		})
	}
}

// TestBuildPaneCommand_SpecialCharsInPrompt verifies prompts with quotes and newlines.
func TestBuildPaneCommand_SpecialCharsInPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prompt string
		viaArg bool
	}{
		{"double quotes via args", `say "hello"`, true},
		{"newline via stdin", "line1\nline2", false},
		{"single quotes via args", "it's a test", true},
		{"backtick via stdin", "run `ls`", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := ProviderConfig{Name: "test", Binary: "test", PromptViaArgs: tt.viaArg}
			cmd := buildPaneCommand(p, tt.prompt, "/tmp/out.txt")
			assert.Contains(t, cmd, sentinel)
			assert.Contains(t, cmd, "tee '/tmp/out.txt'")
		})
	}
}

// TestWaitForSentinel_FileNotExists covers polling when file does not exist.
func TestWaitForSentinel_FileNotExists(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	err := waitForSentinel(ctx, "/tmp/nonexistent-autopus-sentinel-test.out")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestShellEscapeArg verifies shell escaping of arguments.
func TestShellEscapeArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "hello", "'hello'"},
		{"with spaces", "hello world", "'hello world'"},
		{"with single quote", "it's", `'it'\''s'`},
		{"with double quote", `say "hi"`, `'say "hi"'`},
		{"with backtick", "run `cmd`", "'run `cmd`'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, shellEscapeArg(tt.input))
		})
	}
}

// TestUniqueHeredocDelimiter verifies delimiter uniqueness when content contains base.
func TestUniqueHeredocDelimiter(t *testing.T) {
	t.Parallel()

	// Base not in content — use base as-is
	assert.Equal(t, "PROMPT_EOF", uniqueHeredocDelimiter("PROMPT_EOF", "hello", "abc123"))

	// Base in content — append random suffix
	assert.Equal(t, "PROMPT_EOF_abc123", uniqueHeredocDelimiter("PROMPT_EOF", "line with PROMPT_EOF here", "abc123"))
}
