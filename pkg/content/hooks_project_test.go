package content_test

// Project-level hook generation: GenerateProjectHookConfigs derives the
// TaskCreated hook from the CC21 features of the whole harness config.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	// SPEC-EDITGUARD-001 guard (unset flag enables it).
	require.Len(t, hooks, 4)
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
