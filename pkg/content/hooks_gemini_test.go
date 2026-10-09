package content_test

// Gemini CLI reads a command hook timeout in milliseconds and tests a matcher
// as an unanchored regular expression against the tool name (Gemini CLI
// 0.52.0: DEFAULT_HOOK_TIMEOUT 6e4 and `new RegExp(matcher).test(toolName)`,
// recorded in .autopus/specs/SPEC-EDITGUARD-001/evidence/t11-probes.txt). Its
// shell tool is run_shell_command, so a "Bash" matcher never fires there.

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

const (
	archCheckCommand = "auto check --hygiene --arch --quiet --staged --warn-only"
	geminiShellTool  = "run_shell_command"
)

func shellHooksOf(t *testing.T, platform string) map[string]adapter.HookConfig {
	t.Helper()
	cfg := config.HooksConf{PreCommitArch: true, ReactCIFailure: true}
	hooks, _, err := content.GenerateHookConfigs(cfg, platform, true)
	require.NoError(t, err)
	byCommand := map[string]adapter.HookConfig{}
	for _, hook := range hooks {
		switch hook.Command {
		case archCheckCommand, reactCheckCommand:
			byCommand[hook.Command] = hook
		}
	}
	require.Len(t, byCommand, 2, "%s registers the arch and react checks", platform)
	return byCommand
}

func TestGenerateHookConfigs_GeminiShellHooksUseMillisecondsAndTheShellTool(t *testing.T) {
	t.Parallel()

	for _, platform := range []string{"gemini", "gemini-cli"} {
		hooks := shellHooksOf(t, platform)
		assert.Equal(t, adapter.HookConfig{Event: "BeforeTool", Matcher: "^run_shell_command$", Type: "command",
			Command: archCheckCommand, Timeout: 30000}, hooks[archCheckCommand], platform)
		assert.Equal(t, adapter.HookConfig{Event: "AfterTool", Matcher: "^run_shell_command$", Type: "command",
			Command: reactCheckCommand, Timeout: 60000}, hooks[reactCheckCommand], platform)

		matcher := regexp.MustCompile(hooks[archCheckCommand].Matcher)
		assert.True(t, matcher.MatchString(geminiShellTool), "the matcher must select Gemini's shell tool")
		for _, other := range []string{"Bash", "write_file", "mcp_shell_run_shell_command_v2"} {
			assert.False(t, matcher.MatchString(other), "the anchored matcher must not select %q", other)
		}
	}
}

// Every other platform reads the timeout in seconds and keeps its own shell
// matcher.
func TestGenerateHookConfigs_OtherPlatformsKeepSecondTimeouts(t *testing.T) {
	t.Parallel()

	for _, platform := range []string{"claude-code", "codex", "opencode"} {
		hooks := shellHooksOf(t, platform)
		assert.Equal(t, adapter.HookConfig{Event: "PreToolUse", Matcher: "Bash", Type: "command",
			Command: archCheckCommand, Timeout: 30}, hooks[archCheckCommand], platform)
		assert.Equal(t, adapter.HookConfig{Event: "PostToolUse", Matcher: "Bash", Type: "command",
			Command: reactCheckCommand, Timeout: 60}, hooks[reactCheckCommand], platform)
	}

	// Antigravity wraps each command for its JSON stdout protocol.
	hooks, _, err := content.GenerateHookConfigs(config.HooksConf{PreCommitArch: true, ReactCIFailure: true}, "antigravity-cli", true)
	require.NoError(t, err)
	require.Len(t, hooks, 2)
	for _, hook := range hooks {
		assert.Equal(t, "run_command", hook.Matcher)
		assert.Equal(t, map[string]int{"PreToolUse": 30, "PostToolUse": 60}[hook.Event], hook.Timeout, hook.Event)
	}
}

// RetiredGeminiHookShapes names what releases before the fix wrote for the
// same hooks, so an update can replace those entries instead of leaving a
// handler that never fires beside the corrected one.
func TestRetiredGeminiHookShapes_MapsShellHooksBackToTheirPreviousShape(t *testing.T) {
	t.Parallel()

	hooks, _, err := content.GenerateProjectHookConfigs(config.DefaultFullConfig("retired"), "gemini", true)
	require.NoError(t, err)

	assert.ElementsMatch(t, []adapter.HookConfig{
		{Event: "BeforeTool", Matcher: "Bash", Type: "command", Command: archCheckCommand, Timeout: 30},
		{Event: "AfterTool", Matcher: "Bash", Type: "command", Command: reactCheckCommand, Timeout: 60},
	}, content.RetiredGeminiHookShapes(hooks), "the edit guard kept its shape, so it has no retired one")
}
