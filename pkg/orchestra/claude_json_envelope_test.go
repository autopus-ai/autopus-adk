package orchestra

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakeClaude writes an executable named claude that drains stdin, prints
// a stdout warning line like Claude Code does without stdin data, and then
// prints stdout.
func writeFakeClaude(t *testing.T, stdout string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake claude fixture requires a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\ncat >/dev/null\necho 'Warning: no stdin data received in 3s, proceeding without it.'\n" +
		"printf '%s\\n' " + shellSingleQuote(stdout) + "\n"
	writeExecutableFixture(t, path, script, 0o755)
	return path
}

func claudeJSONProvider(binary string) ProviderConfig {
	return ProviderConfig{
		Name: "claude", Binary: binary,
		Args: []string{"--print", "--model", "claude-opus-5-5", "--effort", "max", "--output-format", "json"},
	}
}

func TestSubprocessBackend_ClaudeJSON_ReturnsResultTextAndActualUsage(t *testing.T) {
	t.Parallel()
	binary := writeFakeClaude(t, claudeJSONFixture)

	resp, err := NewSubprocessBackendImpl().Execute(context.Background(), ProviderRequest{
		Provider: "claude", Prompt: "review", Role: "reviewer", Timeout: time.Minute,
		Config: claudeJSONProvider(binary),
	})

	require.NoError(t, err)
	assert.Equal(t, "VERDICT: PASS", resp.Output)
	require.Len(t, resp.Usage, 1)
	assert.Equal(t, telemetry.UsageStatusActual, resp.Usage[0].UsageStatus)
	assert.Equal(t, int64(54), *resp.Usage[0].OutputTokensTotal)
}

func TestRunProvider_ClaudeJSON_ReturnsResultText(t *testing.T) {
	t.Parallel()
	binary := writeFakeClaude(t, claudeJSONFixture)

	resp, err := runProvider(context.Background(), claudeJSONProvider(binary), "review")

	require.NoError(t, err)
	assert.Equal(t, "VERDICT: PASS", resp.Output)
	assert.True(t, resp.UsageCapability.Observed)
}

func TestRunProvider_ClaudeJSONErrorResult_Fails(t *testing.T) {
	t.Parallel()
	binary := writeFakeClaude(t, `{"type":"result","subtype":"error_during_execution","is_error":true}`)

	_, err := runProvider(context.Background(), claudeJSONProvider(binary), "review")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error_during_execution")
}
