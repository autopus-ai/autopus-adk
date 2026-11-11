package content

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func diskGateHooks(t *testing.T, cfg config.HooksConf, platform string) int {
	t.Helper()
	hooks, _, err := GenerateHookConfigs(cfg, platform, true)
	require.NoError(t, err)
	n := 0
	for _, hook := range hooks {
		if hook.Command == diskGateCommandLine {
			assert.Equal(t, "PreToolUse", hook.Event)
			assert.Equal(t, "Bash", hook.Matcher)
			assert.Equal(t, diskGateTimeoutSeconds, hook.Timeout)
			n++
		}
	}
	return n
}

func TestDiskGateHook_ClaudeOnlyAndOptOut(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1, diskGateHooks(t, config.HooksConf{}, "claude-code"), "unset means on")
	assert.Equal(t, 1, diskGateHooks(t, config.HooksConf{}, "claude"))
	for _, platform := range []string{"codex", "gemini", "antigravity-cli", "opencode", "omp"} {
		assert.Zero(t, diskGateHooks(t, config.HooksConf{}, platform), platform)
	}
	off := false
	assert.Zero(t, diskGateHooks(t, config.HooksConf{DiskGate: &config.DiskGateConf{Enabled: &off}}, "claude-code"))
	assert.Contains(t, diskGateCommandLine, "auto disk gate")
	assert.Contains(t, diskGateCommandLine, "exit 0", "a gate fault never blocks a call")
}
