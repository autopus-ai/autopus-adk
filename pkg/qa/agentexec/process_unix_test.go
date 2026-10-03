//go:build !windows

package agentexec

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realRunner spawns real processes but reads the override from argv, never
// from the real environment.
func realRunner(argv string) Runner {
	r := NewRunner()
	r.Getenv = func(k string) string {
		if k == OverrideEnv {
			return argv
		}
		return ""
	}
	return r
}

func TestRun_RealProcess_DeliversPromptOnStdin(t *testing.T) {
	t.Parallel()
	const prompt = "hello agent\nsecond line"
	resp, err := realRunner(`["/bin/sh","-c","cat"]`).Run(context.Background(),
		Request{Target: TargetGemini, Mode: ModeGenerate, Prompt: prompt, Timeout: 10 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, prompt, resp.Stdout)
	assert.Equal(t, 0, resp.ExitCode)
	assert.Equal(t, []string{"/bin/sh", "-c", "cat"}, resp.Argv)
	assert.Positive(t, resp.Duration)
}

func TestRun_RealProcess_RunsInWorkDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	resp, err := realRunner(`["/bin/sh","-c","pwd -P"]`).Run(context.Background(),
		Request{Target: TargetClaude, Mode: ModeEdit, Prompt: "p", WorkDir: dir, Timeout: 10 * time.Second})
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, want, strings.TrimSpace(resp.Stdout))
}

func TestRun_RealProcess_NonZeroExitKeepsOutput(t *testing.T) {
	t.Parallel()
	resp, err := realRunner(`["/bin/sh","-c","echo out; echo err >&2; exit 3"]`).Run(context.Background(),
		Request{Target: TargetClaude, Mode: ModeGenerate, Prompt: "p", Timeout: 10 * time.Second})
	var agentErr *AgentError
	require.ErrorAs(t, err, &agentErr)
	assert.Equal(t, CodeFailed, agentErr.Code)
	assert.Equal(t, 3, agentErr.ExitCode)
	assert.NoError(t, agentErr.Err, "a plain exit status is not a runner error")
	assert.Equal(t, "out\n", resp.Stdout)
	assert.Equal(t, "err\n", resp.Stderr)
}

func TestRun_RealProcess_MissingBinaryIsSetupGap(t *testing.T) {
	t.Parallel()
	_, err := realRunner(`["/nonexistent/autopus-agent"]`).Run(context.Background(),
		Request{Target: TargetClaude, Mode: ModeGenerate, Prompt: "p"})
	assert.Equal(t, CodeCLIMissing, ErrorCode(err))
}

// The second script keeps sh alive as the parent of sleep, so only a
// process-group kill releases the pipes before sleep would end on its own.
func TestRun_RealProcess_TimeoutKillsProcessGroup(t *testing.T) {
	t.Parallel()
	for _, script := range []string{"sleep 5", "sleep 5; echo unreachable"} {
		t.Run(script, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			resp, err := realRunner(`["/bin/sh","-c","`+script+`"]`).Run(context.Background(),
				Request{Target: TargetClaude, Mode: ModeGenerate, Prompt: "p", Timeout: 200 * time.Millisecond})
			elapsed := time.Since(start)

			var agentErr *AgentError
			require.ErrorAs(t, err, &agentErr)
			assert.Equal(t, "qa_agent_timeout", agentErr.Code)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Less(t, elapsed, 3*time.Second, "the agent must be killed at the deadline")
			assert.NotContains(t, resp.Stdout, "unreachable")
		})
	}
}

// A zero Runner falls back to the real environment and process defaults.
func TestRun_ZeroRunner_UsesProcessDefaults(t *testing.T) {
	t.Setenv(OverrideEnv, `["/bin/sh","-c","cat"]`)
	resp, err := Runner{}.Run(context.Background(),
		Request{Target: TargetCodex, Mode: ModeGenerate, Prompt: "zero value", Timeout: 10 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, "zero value", resp.Stdout)
}
