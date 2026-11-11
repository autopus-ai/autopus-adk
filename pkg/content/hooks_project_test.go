package content_test

// Project-level hook generation: GenerateProjectHookConfigs derives the
// TaskCreated hook from the CC21 features of the whole harness config.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

func TestGenerateProjectHookConfigs_ClaudeTaskCreatedEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("demo")
	cfg.Hooks = config.HooksConf{}
	cfg.Features.CC21 = config.CC21FeaturesConf{
		Enabled:              true,
		EffortEnabled:        true,
		MonitorEnabled:       true,
		TaskCreatedEnabled:   true,
		InitialPromptEnabled: true,
		TaskCreatedMode:      "warn",
	}

	hooks, gitHooks, err := content.GenerateProjectHookConfigs(cfg, "claude-code", true)
	require.NoError(t, err)
	// Expect: TaskCreated hook + SPEC-CONDRULE-001 dispatcher + SPEC-STICKYRULE-001 entry +
	// SPEC-EDITGUARD-001 guard and disk gate (unset flags enable them) + the verification Stop hook.
	require.Len(t, hooks, 6)
	stop := findHook(hooks, "Stop")
	require.NotNil(t, stop, "expected the verification Stop hook")
	assert.Equal(t, "auto verify stop", stop.Command)
	assert.Empty(t, gitHooks)
	taskCreatedHook := findHook(hooks, "TaskCreated")
	require.NotNil(t, taskCreatedHook, "expected a TaskCreated hook")
	assert.Equal(t, "AUTOPUS_TASKCREATED_DEFAULT_MODE=warn .claude/hooks/task-created-validate.sh", taskCreatedHook.Command)
	assert.Empty(t, taskCreatedHook.Env)
}

func TestGenerateProjectHookConfigs_TaskCreatedDisabledOutsideClaude(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("demo")
	// The edit guard, on by default, is a Codex PreToolUse entry of its own
	// (hooks_edit_guard_test.go); off here, only TaskCreated is under test.
	cfg.Hooks = config.HooksConf{EditGuard: new(false)}
	cfg.Features.CC21 = config.CC21FeaturesConf{
		Enabled:            true,
		TaskCreatedEnabled: true,
		TaskCreatedMode:    "enforce",
	}

	hooks, gitHooks, err := content.GenerateProjectHookConfigs(cfg, "codex", true)
	require.NoError(t, err)
	// TaskCreated is disabled outside claude, and the retired completion and
	// readiness hooks (SPEC-PANERM-001) leave Codex with no native hook here.
	assert.Empty(t, hooks, "no hook expected for codex: %v", eventNames(hooks))
	assert.Empty(t, gitHooks)
}

func TestGenerateProjectHookConfigs_VerifyStopIsClaudeOnlyAndSwitchable(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("demo")
	hooks, _, err := content.GenerateProjectHookConfigs(cfg, "claude-code", true)
	require.NoError(t, err)
	stop := findHook(hooks, "Stop")
	require.NotNil(t, stop, "on by default")
	assert.Equal(t, 600, stop.Timeout)

	for _, platform := range []string{"codex", "opencode", "antigravity-cli"} {
		other, _, err := content.GenerateProjectHookConfigs(cfg, platform, true)
		require.NoError(t, err)
		assert.Nil(t, findHook(other, "Stop"), platform)
	}

	off := false
	cfg.Hooks.VerifyStop = &off
	hooks, _, err = content.GenerateProjectHookConfigs(cfg, "claude-code", true)
	require.NoError(t, err)
	assert.Nil(t, findHook(hooks, "Stop"), "hooks.verify_stop: false removes it")
}

func TestGenerateProjectHookConfigs_LanguageStopFollowsAIResponses(t *testing.T) {
	t.Parallel()

	languageStop := func(hooks []adapter.HookConfig) *adapter.HookConfig {
		for i := range hooks {
			if hooks[i].Event == "Stop" && hooks[i].Command == "auto verify language" {
				return &hooks[i]
			}
		}
		return nil
	}
	cfg := config.DefaultFullConfig("demo")
	cfg.Language.AIResponses = "ko"
	hooks, _, err := content.GenerateProjectHookConfigs(cfg, "claude-code", true)
	require.NoError(t, err)
	got := languageStop(hooks)
	require.NotNil(t, got, "ko responses register the check")
	assert.Equal(t, 10, got.Timeout)
	assert.NotNil(t, findHook(hooks, "Stop"), "the verification Stop hook stays")

	codex, _, err := content.GenerateProjectHookConfigs(cfg, "codex", true)
	require.NoError(t, err)
	assert.Nil(t, languageStop(codex), "the payload it reads is Claude Code's")

	cfg.Language.AIResponses = "en"
	hooks, _, err = content.GenerateProjectHookConfigs(cfg, "claude-code", true)
	require.NoError(t, err)
	assert.Nil(t, languageStop(hooks), "nothing to check for English")

	cfg.Language.AIResponses = "ko"
	off := false
	cfg.Hooks.LanguageStop = &off
	hooks, _, err = content.GenerateProjectHookConfigs(cfg, "claude-code", true)
	require.NoError(t, err)
	assert.Nil(t, languageStop(hooks), "hooks.language_stop: false removes it")
}
