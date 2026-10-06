package orchestra

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writePwdRecordingProvider creates a shell provider that records its working
// directory into pwdFile and then answers with output.
func writePwdRecordingProvider(t *testing.T, dir, name, pwdFile, output string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell provider fixture requires a POSIX shell")
	}
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\ncat >/dev/null\npwd > " + shellSingleQuote(pwdFile) + "\nprintf '%s' " + shellSingleQuote(output) + "\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func TestRunOrchestra_ProviderWorkDirIsolatesProcessCwd(t *testing.T) {
	t.Parallel()

	fixtureDir := t.TempDir()
	workDir := t.TempDir()
	pwdFile := filepath.Join(fixtureDir, "pwd.txt")
	binary := writePwdRecordingProvider(t, fixtureDir, "recorder", pwdFile, "isolated answer")
	repoCwd, err := os.Getwd()
	require.NoError(t, err)

	before := time.Now()
	result, err := RunOrchestra(context.Background(), OrchestraConfig{
		Providers:       []ProviderConfig{{Name: "recorder", Binary: binary, SandboxMode: SandboxModeReadOnly}},
		Strategy:        StrategyFastest,
		Prompt:          "where are you",
		TimeoutSeconds:  10,
		ProviderWorkDir: workDir,
		ReadOnly:        true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	recorded, readErr := os.ReadFile(pwdFile)
	require.NoError(t, readErr)
	got := strings.TrimSpace(string(recorded))
	assert.Equal(t, mustEvalSymlinks(t, workDir), mustEvalSymlinks(t, got), "provider must run inside the isolated work dir")
	assert.NotEqual(t, mustEvalSymlinks(t, repoCwd), mustEvalSymlinks(t, got), "provider must not inherit the orchestrator cwd")

	require.Len(t, result.Responses, 1)
	execution := result.Responses[0].Execution
	require.NotNil(t, execution)
	assert.Equal(t, workDir, execution.Cwd)
	assert.Equal(t, SandboxModeReadOnly, execution.SandboxMode)
	assert.Greater(t, execution.PID, 0)
	assert.Equal(t, []string{binary}, execution.Command)
	assert.False(t, execution.StartedAt.Before(before))
	assert.False(t, execution.EndedAt.Before(execution.StartedAt))

	require.NotNil(t, result.RunReceipt)
	require.Len(t, result.RunReceipt.ProviderReceipts, 1)
	receipt := result.RunReceipt.ProviderReceipts[0]
	assert.Equal(t, workDir, receipt.Cwd)
	assert.Equal(t, SandboxModeReadOnly, receipt.SandboxMode)
	assert.Equal(t, execution.PID, receipt.PID)
	assert.Equal(t, []string{binary}, receipt.Command)
	_, parseErr := time.Parse(time.RFC3339Nano, receipt.StartedAt)
	assert.NoError(t, parseErr, "started_at must be RFC3339")
	_, parseErr = time.Parse(time.RFC3339Nano, receipt.EndedAt)
	assert.NoError(t, parseErr, "ended_at must be RFC3339")
}

func TestRunProvider_WithoutWorkDirRecordsInheritedCwd(t *testing.T) {
	t.Parallel()

	fixtureDir := t.TempDir()
	pwdFile := filepath.Join(fixtureDir, "pwd.txt")
	binary := writePwdRecordingProvider(t, fixtureDir, "recorder", pwdFile, "answer")
	cwd, err := os.Getwd()
	require.NoError(t, err)

	resp, err := runProvider(context.Background(), ProviderConfig{Name: "recorder", Binary: binary}, "prompt")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Execution)
	assert.Equal(t, cwd, resp.Execution.Cwd)
	assert.Equal(t, SandboxModeUnrestricted, resp.Execution.SandboxMode)
	recorded, readErr := os.ReadFile(pwdFile)
	require.NoError(t, readErr)
	assert.Equal(t, mustEvalSymlinks(t, cwd), mustEvalSymlinks(t, strings.TrimSpace(string(recorded))))
}

func TestBuildProviderRunReceipts_TimedOutAttemptKeepsLaunchProvenance(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC)
	execution := &ProviderExecution{
		Command: []string{"codex", "exec", "--sandbox", "read-only"}, Cwd: "/tmp/autopus-brainstorm-1",
		PID: 4242, SandboxMode: SandboxModeReadOnly, StartedAt: started, EndedAt: started.Add(30 * time.Second),
	}
	result := finalizeOrchestrationContract(&OrchestraResult{
		Strategy: StrategyDebate, ConfiguredProviders: []string{"codex", "gemini"},
		RoundHistory: [][]ProviderResponse{{{Provider: "gemini", Output: "idea", ExecutedBackend: "subprocess"}}},
		Responses:    []ProviderResponse{{Provider: "gemini", Output: "idea", ExecutedBackend: "subprocess"}},
		FailedProviders: []FailedProvider{{
			Name: "codex", Role: "debater_r1", Attempt: 1, TimedOut: true, FailureClass: "timeout",
			ExecutedBackend: "subprocess", Execution: execution,
		}},
	})

	require.NotNil(t, result.RunReceipt)
	var codex *ProviderRunReceipt
	for index := range result.RunReceipt.ProviderReceipts {
		if result.RunReceipt.ProviderReceipts[index].Provider == "codex" {
			codex = &result.RunReceipt.ProviderReceipts[index]
		}
	}
	require.NotNil(t, codex, "timed-out attempt must still produce a receipt")
	assert.True(t, codex.TimedOut)
	assert.Equal(t, execution.Command, codex.Command)
	assert.Equal(t, "/tmp/autopus-brainstorm-1", codex.Cwd)
	assert.Equal(t, 4242, codex.PID)
	assert.Equal(t, SandboxModeReadOnly, codex.SandboxMode)
	assert.Equal(t, "2026-09-06T10:00:00Z", codex.StartedAt)
	assert.Equal(t, "2026-09-06T10:00:30Z", codex.EndedAt)
}

func TestBuildInteractiveLaunchCommand_ReadOnlyOmitsBypassAndEntersWorkDir(t *testing.T) {
	t.Parallel()

	workDir := "/tmp/autopus-brainstorm-xyz"
	claude := ProviderConfig{Name: "claude", Binary: "claude", PaneArgs: []string{"--model", "opus", "--permission-mode", "plan"}}
	launch := paneLaunchFor(OrchestraConfig{WorkingDir: "/repo", ProviderWorkDir: workDir, ReadOnly: true})

	cmd := buildInteractiveLaunchCommand(claude, "", launch)
	assert.Equal(t, "cd "+shellQuote(workDir)+" && claude --model opus --permission-mode plan", cmd)
	assert.NotContains(t, cmd, "--dangerously-skip-permissions")

	gemini := ProviderConfig{Name: "gemini", Binary: "agy", PromptViaArgs: true}
	geminiCmd := buildInteractiveLaunchCommand(gemini, "prompt", launch)
	assert.NotContains(t, geminiCmd, "--dangerously-skip-permissions")
	assert.True(t, strings.HasPrefix(geminiCmd, "cd "+shellQuote(workDir)+" && agy"), geminiCmd)

	// Without ReadOnly the legacy bypass stays and the pane enters WorkingDir.
	legacy := paneLaunchFor(OrchestraConfig{WorkingDir: "/repo"})
	legacyCmd := buildInteractiveLaunchCommand(claude, "", legacy)
	assert.Contains(t, legacyCmd, "--dangerously-skip-permissions")
	assert.True(t, strings.HasPrefix(legacyCmd, "cd '/repo' && claude"), legacyCmd)
}

func mustEvalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}
