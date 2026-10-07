package adapter

// SPEC-PANERM-001 T11: the group S declaration that update retraction and the
// doctor report share (REQ-13).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaleCompletionHookScripts_ListsTheClosedGroupSSetPerPlatform(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{
		".claude/hooks/autopus/hook-claude-sessionstart.sh", ".claude/hooks/autopus/hook-claude-stop.sh",
		".claude/hooks/autopus/hook-codex-sessionstart.sh", ".claude/hooks/autopus/hook-codex-stop.sh",
		".claude/hooks/autopus/hook-gemini-afteragent.sh", ".claude/hooks/autopus/hook-gemini-sessionstart.sh",
		".claude/hooks/autopus/hook-gemini-stop.sh", ".claude/hooks/autopus/hook-opencode-complete.ts",
	}, StaleCompletionHookScripts("claude-code"))
	assert.Equal(t, []string{".codex/hooks/autopus/hook-codex-sessionstart.sh", ".codex/hooks/autopus/hook-codex-stop.sh"},
		StaleCompletionHookScripts("codex"))
	assert.Equal(t, []string{".gemini/hooks/autopus/hook-gemini-afteragent.sh", ".gemini/hooks/autopus/hook-gemini-stop.sh"},
		StaleCompletionHookScripts("antigravity-cli"))
	assert.Empty(t, StaleCompletionHookScripts("opencode"), "OpenCode owns plugin entries, not scripts")
	assert.Len(t, AllStaleCompletionHookScripts(), 12)
	assert.True(t, AllStaleCompletionHookScripts()[0] < AllStaleCompletionHookScripts()[11], "sorted")
}

func TestIsStaleCompletionHookCommand_MatchesCommandsEndingInAGroupSScript(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		platform, command string
		want              bool
	}{
		{"claude-code", `"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-claude-stop.sh`, true},
		{"claude-code", `$HOME/.claude/hooks/autopus/hook-claude-sessionstart.sh`, true},
		{"claude-code", ` .claude/hooks/autopus/hook-claude-stop.sh `, true},
		{"claude-code", `bash .claude/hooks/autopus/hook-claude-stop.sh`, true},
		{"claude-code", `.claude/hooks/autopus/hook-claude-stop.sh --verbose`, false},
		{"claude-code", `/x/my.claude/hooks/autopus/hook-claude-stop.sh`, false},
		{"claude-code", `.claude/hooks/autopus/react-review.sh`, false},
		{"claude-code", `./scripts/notify.sh`, false},
		{"codex", `"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/.codex/hooks/autopus/hook-codex-stop.sh"`, true},
		{"codex", `"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-claude-stop.sh`, false},
		{"antigravity-cli", `"${GEMINI_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"/.gemini/hooks/autopus/hook-gemini-afteragent.sh`, true},
		{"antigravity-cli", `"$(cd .. && pwd)/.gemini/hooks/autopus/hook-gemini-stop.sh"`, true},
		{"opencode", `.claude/hooks/autopus/hook-opencode-complete.ts`, false},
	} {
		assert.Equal(t, tc.want, IsStaleCompletionHookCommand(tc.platform, tc.command), "%s %q", tc.platform, tc.command)
	}
}

func TestRetractHookHandlers_RemovesOwnedHandlersAndKeepsUserOnes(t *testing.T) {
	t.Parallel()
	user := map[string]any{"matcher": "user", "hooks": []any{map[string]any{"type": "command", "command": "./user.sh", "timeout": float64(10)}}}
	mixed := map[string]any{"matcher": "mixed", "hooks": []any{
		map[string]any{"type": "command", "command": "owned-stop"},
		"not-a-handler",
		map[string]any{"type": "command", "command": "./notify.sh", "timeout": float64(10)},
	}}
	odd := map[string]any{"matcher": "odd", "hooks": "owned-stop"}
	hooks := map[string]any{
		"Stop":         []any{user, map[string]any{"hooks": []any{map[string]any{"command": "owned-stop"}}}, mixed, "raw"},
		"SessionStart": []any{map[string]any{"hooks": []map[string]any{{"command": "owned-start"}}}},
		"Notification": map[string]any{"command": "owned-stop"},
		"PostToolUse":  []any{odd},
	}
	owned := func(command string) bool { return command == "owned-stop" || command == "owned-start" }

	assert.Equal(t, 3, RetractHookHandlers(hooks, owned))
	assert.Equal(t, []any{user, map[string]any{"matcher": "mixed", "hooks": []any{
		"not-a-handler", map[string]any{"type": "command", "command": "./notify.sh", "timeout": float64(10)},
	}}, "raw"}, hooks["Stop"], "the mixed entry keeps its matcher and position with only the user handler")
	assert.NotContains(t, hooks, "SessionStart", "an event with no entry left is dropped")
	assert.Equal(t, map[string]any{"command": "owned-stop"}, hooks["Notification"], "unknown shapes stay")
	assert.Equal(t, []any{odd}, hooks["PostToolUse"])
	assert.Equal(t, 0, RetractHookHandlers(hooks, owned), "a second pass removes nothing")
}

func writeStaleFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
}

func removePaths(t *testing.T, removes []TransactionRemove) []string {
	t.Helper()
	paths := make([]string, 0, len(removes))
	for _, remove := range removes {
		assert.False(t, remove.Recursive, "a script remove never recurses")
		paths = append(paths, remove.Path)
	}
	return paths
}

func TestStaleCompletionScriptRemoves_PlansPresentUnreferencedScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scripts := StaleCompletionHookScripts("claude-code")
	writeStaleFile(t, root, scripts[1], "#!/bin/sh\n")
	writeStaleFile(t, root, scripts[0], "#!/bin/sh\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.FromSlash(scripts[2])), 0o755))
	writeStaleFile(t, root, "elsewhere.sh", "#!/bin/sh\n")
	require.NoError(t, os.Symlink(filepath.Join(root, "elsewhere.sh"), filepath.Join(root, filepath.FromSlash(scripts[3]))))

	got := removePaths(t, StaleCompletionScriptRemoves(root, scripts, nil))
	assert.Equal(t, []string{scripts[0], scripts[1]}, got, "only regular files reachable without a symlink")
}

func TestStaleCompletionScriptRemoves_KeepsScriptsTheFinalStateStillNames(t *testing.T) {
	t.Parallel()
	const stop, start = ".claude/hooks/autopus/hook-claude-stop.sh", ".claude/hooks/autopus/hook-claude-sessionstart.sh"
	const ts, codexStop = ".claude/hooks/autopus/hook-opencode-complete.ts", ".claude/hooks/autopus/hook-codex-stop.sh"
	root := t.TempDir()
	for _, rel := range []string{stop, start, ts, codexStop} {
		writeStaleFile(t, root, rel, "x\n")
	}
	writeStaleFile(t, root, ".claude/settings.json", `{"hooks":{"Stop":[{"hooks":[{"command":"`+stop+`"}]}]}}`)
	writeStaleFile(t, root, "opencode.json", `{"plugin":["file://`+root+`\/.claude\/hooks\/autopus\/hook-opencode-complete.ts"]}`)
	writeStaleFile(t, root, ".codex/hooks.json", `{"hooks":{"Stop":[{"hooks":[{"command":".codex/hooks/autopus/hook-codex-stop.sh"}]}]}}`)
	writeStaleFile(t, root, ".gemini/settings.json", "{ not json: hook-claude-sessionstart.sh")

	planned := []TransactionWrite{{Path: ".claude/settings.json", Content: []byte(`{"hooks":{}}`)}}
	scripts := []string{stop, start, ts, codexStop}
	assert.Equal(t, []string{stop, codexStop}, removePaths(t, StaleCompletionScriptRemoves(root, scripts, planned)),
		"the planned settings write drops stop; opencode.json names the .ts; undecodable text naming a file keeps it")
	assert.Equal(t, []string{codexStop}, removePaths(t, StaleCompletionScriptRemoves(root, scripts, nil)),
		"without the planned write the on-disk settings still name stop")
}

func TestStaleCompletionScriptRemoves_UnreadableSettingsKeepEveryScript(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStaleFile(t, root, ".codex/hooks/autopus/hook-codex-stop.sh", "x\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "opencode.json"), 0o755))
	assert.Empty(t, StaleCompletionScriptRemoves(root, StaleCompletionHookScripts("codex"), nil))
}
