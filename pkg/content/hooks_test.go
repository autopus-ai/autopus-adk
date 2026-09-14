// Package content_test는 훅 설정 생성 패키지의 테스트이다.
package content_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

func TestGenerateHookConfigs_WithHooks(t *testing.T) {
	t.Parallel()

	cfg := config.HooksConf{
		PreCommitArch:  true,
		PreCommitLore:  true,
		ReactCIFailure: false,
		ReactReview:    false,
	}

	hooks, gitHooks, err := content.GenerateHookConfigs(cfg, "claude", true)
	require.NoError(t, err)
	// CLI hooks carry the arch check; lore remains a commit-msg hook because
	// Claude has no equivalent event.
	assert.NotEmpty(t, hooks)
	for _, h := range hooks {
		assert.NotContains(t, h.Command, "--lore", "lore should not be a CLI hook")
	}
	// findHook takes the first PreToolUse entry: the arch check is registered
	// before the SPEC-CONDRULE-001 dispatcher that shares its Bash matcher.
	archHook := findHook(hooks, "PreToolUse")
	require.NotNil(t, archHook, "expected a PreToolUse arch hook")
	assert.Contains(t, archHook.Command, "--hygiene")
	assert.Contains(t, archHook.Command, "--arch")
	assert.Contains(t, archHook.Command, "--staged")
	assert.Contains(t, archHook.Command, "--warn-only")
	require.Len(t, gitHooks, 1)
	assert.Equal(t, ".git/hooks/commit-msg", gitHooks[0].Path)
}

func TestGenerateHookConfigs_WithoutHooks(t *testing.T) {
	t.Parallel()

	cfg := config.HooksConf{
		PreCommitArch: true,
		PreCommitLore: true,
	}

	hooks, gitHooks, err := content.GenerateHookConfigs(cfg, "codex", false)
	require.NoError(t, err)
	// CLI hooks not supported — git hook scripts returned.
	assert.Empty(t, hooks)
	assert.NotEmpty(t, gitHooks)
	// pre-commit (hygiene + arch --staged) + commit-msg (lore --message) both present.
	var paths []string
	for _, g := range gitHooks {
		paths = append(paths, g.Path)
	}
	assert.Contains(t, paths, ".git/hooks/pre-commit")
	assert.Contains(t, paths, ".git/hooks/commit-msg")
	for _, g := range gitHooks {
		if g.Path == ".git/hooks/commit-msg" {
			assert.Contains(t, g.Content, "\"$AUTO_BIN\" check --lore --quiet --message")
			assert.Contains(t, g.Content, "\"$AUTO_BIN\" lore validate \"$1\"")
		}
	}
}

func TestGenerateHookConfigs_AllDisabled(t *testing.T) {
	t.Parallel()

	cfg := config.HooksConf{
		PreCommitArch:  false,
		PreCommitLore:  false,
		ReactCIFailure: false,
		ReactReview:    false,
	}

	hooks, gitHooks, err := content.GenerateHookConfigs(cfg, "claude", true)
	require.NoError(t, err)
	// All HooksConf fields disabled — the unconditional hooks remain: the
	// completion Stop hook, the SessionStart ready hook (SPEC-ORCH-022), the
	// SPEC-CONDRULE-001 dispatcher, and the SPEC-STICKYRULE-001 UserPromptSubmit
	// entry, which is gated by the compiled sticky set rather than by HooksConf.
	require.Len(t, hooks, 4,
		"completion, session-start ready, dispatcher, and sticky hooks are unconditional")
	events := map[string]string{}
	for _, h := range hooks {
		events[h.Event] = h.Command
	}
	assert.Equal(t, `"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-claude-stop.sh`, events["Stop"])
	assert.Equal(t, `"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-claude-sessionstart.sh`, events["SessionStart"])
	assert.Equal(t, "auto rules fire --event PreToolUse", events["PreToolUse"])
	assert.Empty(t, gitHooks)
}

// TestGenerateHookConfigs_GeminiTranslatesEventNames verifies that the legacy
// Gemini CLI surface receives BeforeTool/AfterTool instead of Claude Code's
// PreToolUse/PostToolUse, which that CLI rejected as
// "Invalid hook event name".
func TestGenerateHookConfigs_GeminiTranslatesEventNames(t *testing.T) {
	t.Parallel()

	cfg := config.HooksConf{
		PreCommitArch:  true,
		ReactCIFailure: true,
	}

	hooks, _, err := content.GenerateHookConfigs(cfg, "gemini", true)
	require.NoError(t, err)

	events := eventNames(hooks)
	assert.Contains(t, events, "BeforeTool", "PreToolUse must be translated to BeforeTool for gemini")
	assert.Contains(t, events, "AfterTool", "PostToolUse must be translated to AfterTool for gemini")
	assert.NotContains(t, events, "PreToolUse", "gemini hooks must not use Claude Code event names")
	assert.NotContains(t, events, "PostToolUse", "gemini hooks must not use Claude Code event names")
}

func TestGenerateHookConfigs_AntigravityKeepsOfficialEventNames(t *testing.T) {
	t.Parallel()

	cfg := config.HooksConf{
		PreCommitArch:  true,
		ReactCIFailure: true,
	}

	hooks, _, err := content.GenerateHookConfigs(cfg, "antigravity-cli", true)
	require.NoError(t, err)

	events := eventNames(hooks)
	assert.Contains(t, events, "PreToolUse")
	assert.Contains(t, events, "PostToolUse")
	assert.NotContains(t, events, "BeforeTool")
	assert.NotContains(t, events, "AfterTool")
	// Stop is the completion hook in the Antigravity lifecycle.
	assert.Contains(t, events, "Stop")

	// Tool-use hooks must be wrapped for Antigravity JSON stdout protocol;
	// the completion Stop hook is a plain command (not tool-use).
	for _, h := range hooks {
		if h.Event == "Stop" {
			// Completion hook: plain command, no run_command matcher.
			continue
		}
		assert.Equal(t, "run_command", h.Matcher, "Antigravity tool-use hooks must match official tool names")
		assert.Contains(t, h.Command, "sh -c", "Antigravity tool-use hooks must wrap commands for JSON stdout")
		assert.Contains(t, h.Command, ">&2", "Antigravity tool-use hooks should keep command output off stdout")
		if h.Event == "PreToolUse" {
			assert.Contains(t, h.Command, `decision`, "PreToolUse must emit a decision JSON object")
		}
		if h.Event == "PostToolUse" {
			assert.Contains(t, h.Command, `{}`, "PostToolUse must emit an empty JSON object")
		}
	}
}

// TestGenerateHookConfigs_ClaudeKeepsEventNames verifies claude-code still
// receives PreToolUse/PostToolUse (its native event names).
func TestGenerateHookConfigs_ClaudeKeepsEventNames(t *testing.T) {
	t.Parallel()

	cfg := config.HooksConf{
		PreCommitArch:  true,
		ReactCIFailure: true,
	}

	hooks, _, err := content.GenerateHookConfigs(cfg, "claude", true)
	require.NoError(t, err)

	events := eventNames(hooks)
	assert.Contains(t, events, "PreToolUse")
	assert.Contains(t, events, "PostToolUse")
	assert.NotContains(t, events, "BeforeTool")
	assert.NotContains(t, events, "AfterTool")
}

func TestGenerateHookConfigs_DeduplicatesReactHooks(t *testing.T) {
	t.Parallel()

	cfg := config.HooksConf{
		ReactCIFailure: true,
		ReactReview:    true,
	}

	hooks, _, err := content.GenerateHookConfigs(cfg, "claude", true)
	require.NoError(t, err)
	// ReactCIFailure and ReactReview both enabled — dedup keeps only one PostToolUse react hook,
	// plus the unconditional completion Stop hook, the SessionStart ready hook
	// (SPEC-ORCH-022), the SPEC-CONDRULE-001 dispatcher, and the SPEC-STICKYRULE-001 entry.
	require.Len(t, hooks, 5,
		"expected one deduped react hook plus the Stop, SessionStart, dispatcher, and sticky hooks")
	reactHook := findHook(hooks, "PostToolUse")
	require.NotNil(t, reactHook, "expected a PostToolUse react hook")
	assert.Equal(t, "auto react check --quiet", reactHook.Command)
}

func TestGenerateProjectHookConfigs_ClaudeTaskCreatedEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("demo")
	cfg.Hooks = config.HooksConf{}
	cfg.Features.CC21 = config.CC21FeaturesConf{
		Enabled:                 true,
		EffortEnabled:           true,
		MonitorEnabled:          true,
		TaskCreatedEnabled:      true,
		InitialPromptEnabled:    true,
		TaskCreatedMode:         "warn",
		MonitorPatternTimeoutMS: 30000,
	}

	hooks, gitHooks, err := content.GenerateProjectHookConfigs(cfg, "claude-code", true)
	require.NoError(t, err)
	// Expect: completion Stop hook + SessionStart ready hook (SPEC-ORCH-022) +
	// TaskCreated hook + SPEC-CONDRULE-001 dispatcher + SPEC-STICKYRULE-001 entry.
	require.Len(t, hooks, 5)
	assert.Empty(t, gitHooks)
	taskCreatedHook := findHook(hooks, "TaskCreated")
	require.NotNil(t, taskCreatedHook, "expected a TaskCreated hook")
	assert.Equal(t, "AUTOPUS_TASKCREATED_DEFAULT_MODE=warn .claude/hooks/task-created-validate.sh", taskCreatedHook.Command)
	assert.Empty(t, taskCreatedHook.Env)
}

func TestGenerateProjectHookConfigs_TaskCreatedDisabledOutsideClaude(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("demo")
	cfg.Hooks = config.HooksConf{}
	cfg.Features.CC21 = config.CC21FeaturesConf{
		Enabled:            true,
		TaskCreatedEnabled: true,
		TaskCreatedMode:    "enforce",
	}

	hooks, gitHooks, err := content.GenerateProjectHookConfigs(cfg, "codex", true)
	require.NoError(t, err)
	// TaskCreated is disabled outside claude; Codex still receives the completion
	// and readiness hooks required by pane IPC.
	require.Len(t, hooks, 2, "completion Stop and SessionStart hooks expected for codex")
	assert.ElementsMatch(t, []string{"Stop", "SessionStart"}, eventNames(hooks))
	assert.Empty(t, gitHooks)
}

func TestGitHookScript_Content(t *testing.T) {
	t.Parallel()

	cfg := config.HooksConf{
		PreCommitArch: true,
	}

	_, gitHooks, err := content.GenerateHookConfigs(cfg, "gemini", false)
	require.NoError(t, err)
	require.NotEmpty(t, gitHooks)

	// Script uses --staged to only check staged files.
	assert.Contains(t, gitHooks[0].Content, "\"$AUTO_BIN\" check --hygiene --arch --quiet --staged")
}

// TestGenerateHookConfigs_ConditionalDispatcher is the pkg/content half of S10
// (REQ-CONDRULE-COMPILE-03): the hook-fired rules sharing the PreToolUse/Bash
// pair collapse into exactly one `auto rules fire` entry on Claude Code, and a
// platform that cannot consume hookSpecificOutput.additionalContext gets none.
func TestGenerateHookConfigs_ConditionalDispatcher(t *testing.T) {
	t.Parallel()

	want := map[string]int{"claude": 1, "claude-code": 1, "gemini": 0,
		"gemini-cli": 0, "antigravity-cli": 0, "codex": 0, "opencode": 0}
	// Every option on: the dispatcher must not merge with, duplicate, or displace
	// the arch and react entries that share its Bash matcher.
	cfg := config.HooksConf{PreCommitArch: true, PreCommitLore: true, ReactCIFailure: true, ReactReview: true}

	for platform, count := range want {
		hooks, _, err := content.GenerateHookConfigs(cfg, platform, true)
		require.NoError(t, err)
		project, _, err := content.GenerateProjectHookConfigs(config.DefaultFullConfig("demo"), platform, true)
		require.NoError(t, err)

		for _, set := range [][]adapter.HookConfig{hooks, project} {
			var fired []adapter.HookConfig
			for _, h := range set {
				if strings.Contains(h.Command, "auto rules fire") {
					fired = append(fired, h)
				}
			}
			require.Len(t, fired, count, platform)
			for _, h := range fired {
				assert.Equal(t, adapter.HookConfig{Event: "PreToolUse", Matcher: "Bash",
					Type: "command", Command: "auto rules fire --event PreToolUse",
					Timeout: h.Timeout}, h, platform)
				assert.Positive(t, h.Timeout, `Claude Code rejects "timeout": 0`)
			}
		}
	}
}
