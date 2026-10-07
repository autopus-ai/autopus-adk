package orchestra

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeByStrategy_AllStrategies covers all branches of mergeByStrategy.
func TestMergeByStrategy_AllStrategies(t *testing.T) {
	t.Parallel()

	responses := []ProviderResponse{
		{Provider: "p1", Output: "output1", Duration: time.Second},
		{Provider: "p2", Output: "output2", Duration: 2 * time.Second},
	}
	cfg := OrchestraConfig{
		Providers: []ProviderConfig{
			{Name: "p1", Binary: "echo"},
			{Name: "p2", Binary: "echo"},
		},
		Strategy: StrategyConsensus,
	}

	tests := []struct {
		name     string
		strategy Strategy
		wantSub  string
	}{
		{"pipeline", StrategyPipeline, "파이프라인"},
		{"fastest", StrategyFastest, "최속 응답"},
		{"consensus", StrategyConsensus, ""},
		{"debate", StrategyDebate, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			merged, summary := mergeByStrategy(tt.strategy, responses, cfg)
			assert.NotEmpty(t, merged+summary)
			if tt.wantSub != "" {
				assert.Contains(t, summary, tt.wantSub)
			}
		})
	}
}

// TestMergeByStrategy_Fastest_EmptyResponses covers the empty fastest branch.
func TestMergeByStrategy_Fastest_EmptyResponses(t *testing.T) {
	t.Parallel()
	merged, summary := mergeByStrategy(StrategyFastest, nil, OrchestraConfig{})
	assert.Empty(t, merged)
	assert.Equal(t, "응답 없음", summary)
}

// TestHasSentinel_Found covers the case where sentinel IS present in the file.
func TestHasSentinel_Found(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	require.NoError(t, os.WriteFile(path, []byte("some output\n"+sentinel+"\n"), 0o600))
	assert.True(t, hasSentinel(path))
}

// TestHasSentinel_NotFound covers the case where sentinel is absent.
func TestHasSentinel_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	require.NoError(t, os.WriteFile(path, []byte("no marker here\n"), 0o600))
	assert.False(t, hasSentinel(path))
}

// TestHasSentinel_FileNotExists covers the error path (file does not exist).
func TestHasSentinel_FileNotExists(t *testing.T) {
	t.Parallel()
	assert.False(t, hasSentinel("/tmp/nonexistent-autopus-test-file.out"))
}

// TestBuildPaneCommand_PromptViaArgs covers the PromptViaArgs=false (stdin) branch for gemini.
func TestBuildPaneCommand_PromptViaArgs(t *testing.T) {
	t.Parallel()
	p := ProviderConfig{Name: "gemini", Binary: "gemini", Args: []string{"-m", "gemini-3.1-pro-preview", "-p", ""}, PromptViaArgs: false}
	cmd := buildPaneCommand(p, "hello world", "/tmp/test.out")
	// gemini now uses stdin heredoc mode (PromptViaArgs=false)
	assert.Contains(t, cmd, "hello world")
	assert.Contains(t, cmd, "tee '/tmp/test.out'")
	assert.Contains(t, cmd, "PROMPT_EOF")
}

func TestBuildPaneCommand_PromptViaArgsInjectsEmptySlot(t *testing.T) {
	t.Parallel()
	p := ProviderConfig{Name: "gemini", Binary: "agy", Args: []string{"--print", ""}, PromptViaArgs: true}
	cmd := buildPaneCommand(p, "hello world", "/tmp/test.out")

	assert.Contains(t, cmd, "'agy' '--print' 'hello world' | tee '/tmp/test.out'")
	assert.NotContains(t, cmd, "'--print' '' 'hello world'")
}

// TestBuildPaneCommand_StdinMode covers the PromptViaArgs=false branch.
func TestBuildPaneCommand_StdinMode(t *testing.T) {
	t.Parallel()
	p := ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"--model", "opus"}, PromptViaArgs: false}
	cmd := buildPaneCommand(p, "test prompt", "/tmp/test.out")
	assert.Contains(t, cmd, "PROMPT_EOF")
	assert.Contains(t, cmd, "test prompt")
	assert.Contains(t, cmd, "tee '/tmp/test.out'")
}

func TestBuildPaneCommand_StdinModeUsesPipelineGrouping(t *testing.T) {
	t.Parallel()
	p := ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"--model", "opus"}, PromptViaArgs: false}
	cmd := buildPaneCommand(p, "test prompt", "/tmp/test.out")
	assert.Contains(t, cmd, ") | tee '/tmp/test.out'")
	assert.NotContains(t, cmd, "\n | tee ")
}

// TestReadOutputFile_StripsSentinel covers sentinel stripping in readOutputFile.
func TestReadOutputFile_StripsSentinel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	require.NoError(t, os.WriteFile(path, []byte("real output\n"+sentinel+"\n"), 0o600))
	output := readOutputFile(path)
	assert.Equal(t, "real output", output)
}

// TestReadOutputFile_FileNotExists covers the error branch of readOutputFile.
func TestReadOutputFile_FileNotExists(t *testing.T) {
	t.Parallel()
	assert.Empty(t, readOutputFile("/tmp/nonexistent-autopus-test-file-read.out"))
}

// TestWaitForSentinel_Success covers the happy path where sentinel appears.
func TestWaitForSentinel_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(path, []byte("output\n"+sentinel+"\n"), 0o600)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	assert.NoError(t, waitForSentinel(ctx, path))
}

// TestWaitForSentinel_Timeout covers the timeout path.
func TestWaitForSentinel_Timeout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, waitForSentinel(ctx, path), context.DeadlineExceeded)
}

// TestPaneArgs_FallbackToArgs covers the fallback to Args when PaneArgs is unset.
func TestPaneArgs_FallbackToArgs(t *testing.T) {
	t.Parallel()
	p := ProviderConfig{Args: []string{"--model", "opus"}}
	got := paneArgs(p)
	assert.Equal(t, []string{"--model", "opus"}, got)
}

// TestPaneArgs_EmptyBothNil covers nil Args and nil PaneArgs.
func TestPaneArgs_EmptyBothNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, paneArgs(ProviderConfig{}))
	assert.Empty(t, paneArgs(ProviderConfig{Args: []string{}}))
}

// TestRunPaneOrchestra_DefaultTimeout covers the default timeout (0 -> 120) branch.
// Uses a tight context to avoid actually waiting 120 seconds.
func TestRunPaneOrchestra_DefaultTimeout(t *testing.T) {
	t.Parallel()
	mock := newCmuxMock()
	cfg := OrchestraConfig{
		Providers:      []ProviderConfig{echoProvider("p1")},
		Strategy:       StrategyConsensus,
		Prompt:         "test",
		TimeoutSeconds: 0,
		Terminal:       mock,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := RunPaneOrchestra(ctx, cfg)
	require.NoError(t, err)
	assert.NotNil(t, result)
}
