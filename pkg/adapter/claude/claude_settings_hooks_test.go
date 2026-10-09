package claude

// SPEC-EDITGUARD-001 REQ-EG-12 and REQ-EG-19 oracles for the settings writer:
// the guard command line is owned through its anchored prefix, and retraction
// removes Autopus handlers one at a time, so a user handler that shares an
// entry with one survives with its matcher.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

// guardCommandLine is the canonical command line of spec.md's Decision Output
// Contract.
const guardCommandLine = `out=$(auto guard edit --platform claude-code) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0`

func guardHookConfig() adapter.HookConfig {
	return adapter.HookConfig{Event: "PreToolUse", Matcher: "Edit|Write|MultiEdit", Type: "command",
		Command: guardCommandLine, Timeout: 5}
}

// guardHandlerJSON is the guard handler as settings.json stores it.
func guardHandlerJSON(t *testing.T) string {
	t.Helper()

	encoded, err := json.Marshal(guardCommandLine)
	require.NoError(t, err)
	return `{"type":"command","command":` + string(encoded) + `,"timeout":5}`
}

// TestIsManagedClaudeHookCommand_GuardPrefixIsAnchored: the guard is owned by
// `out=$(auto guard edit ` at the start of the command, whatever flags a later
// release adds, and a command that only contains or resembles it is not.
func TestIsManagedClaudeHookCommand_GuardPrefixIsAnchored(t *testing.T) {
	t.Parallel()

	for _, command := range []string{
		guardCommandLine,
		"  " + guardCommandLine,
		`out=$(auto guard edit --platform claude-code --from-a-newer-release) && [ -n "$out" ]; exit 0`,
	} {
		assert.True(t, isManagedClaudeHookCommand(command), "owned: %q", command)
	}
	for _, command := range []string{
		"auto guard edit --platform claude-code",
		"./audit.sh 'out=$(auto guard edit --platform claude-code)'",
		"out=$(auto guard editor --platform claude-code); exit 0",
		"out=$(auto guard edits) && exit 0",
	} {
		assert.False(t, isManagedClaudeHookCommand(command), "not owned: %q", command)
	}
}

// TestRetractManagedHookEntries_RemovesManagedHandlersOneAtATime fixes the
// whole result: a managed handler leaves its entry, an entry is dropped only
// when that left it empty, and every value this writer does not own (other
// handlers, other entry keys, shapes it does not understand, an entry that was
// empty before) stays as it was.
func TestRetractManagedHookEntries_RemovesManagedHandlersOneAtATime(t *testing.T) {
	t.Parallel()

	guard := guardHandlerJSON(t)
	var hooks map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
	  "PreToolUse": [
	    {"matcher": "Edit", "hooks": [{"type": "command", "command": "./my-hook.sh"}]},
	    {"matcher": "Edit|Write|MultiEdit", "note": "kept", "hooks": [
	      {"type": "command", "command": "./mixed-hook.sh"}, `+guard+`]},
	    {"matcher": "Edit|Write|MultiEdit", "hooks": [`+guard+`]},
	    {"matcher": "Bash", "hooks": [
	      {"type": "command", "command": "auto check --hygiene --arch --quiet --staged --warn-only", "timeout": 30},
	      {"type": "command", "command": "auto rules fire --event PreToolUse", "timeout": 10}]},
	    {"matcher": "Bash", "hooks": [`+guard+`, "not-a-handler", {"type": "command"},
	      {"type": "command", "command": 42}]},
	    {"matcher": "Write", "hooks": []},
	    {"matcher": "Write", "hooks": "not-a-list"},
	    "not-an-entry"
	  ],
	  "Stop": [{"hooks": [{"type": "command",
	    "command": "\"${CLAUDE_PROJECT_DIR:-.}\"/.claude/hooks/autopus/hook-claude-stop.sh", "timeout": 300}]}],
	  "Notification": "user-value"
	}`), &hooks))

	retractManagedHookEntries(hooks)

	assert.JSONEq(t, `{
	  "PreToolUse": [
	    {"matcher": "Edit", "hooks": [{"type": "command", "command": "./my-hook.sh"}]},
	    {"matcher": "Edit|Write|MultiEdit", "note": "kept", "hooks": [
	      {"type": "command", "command": "./mixed-hook.sh"}]},
	    {"matcher": "Bash", "hooks": ["not-a-handler", {"type": "command"}, {"type": "command", "command": 42}]},
	    {"matcher": "Write", "hooks": []},
	    {"matcher": "Write", "hooks": "not-a-list"},
	    "not-an-entry"
	  ],
	  "Notification": "user-value"
	}`, canonical(t, hooks))
}

// TestPrepareSettingsMapping_GuardEntryIsIdempotentBesideAMixedUserEntry runs
// the writer twice over the S11 seed: both passes write the same bytes, with
// the guard alone in its own entry after the user's entries.
func TestPrepareSettingsMapping_GuardEntryIsIdempotentBesideAMixedUserEntry(t *testing.T) {
	t.Parallel()

	seed := `{"hooks":{"PreToolUse":[` +
		`{"matcher":"Edit","hooks":[{"type":"command","command":"./my-hook.sh"}]},` +
		`{"matcher":"Edit|Write|MultiEdit","hooks":[{"type":"command","command":"./mixed-hook.sh"},` +
		guardHandlerJSON(t) + `]}]}}`
	hooks := []adapter.HookConfig{guardHookConfig()}

	first, settings := mapSettings(t, seed, hooks)
	second, _ := mapSettings(t, first, hooks)

	assert.Equal(t, first, second, "regeneration must write identical bytes")
	assert.JSONEq(t, `[
	  {"matcher": "Edit", "hooks": [{"type": "command", "command": "./my-hook.sh"}]},
	  {"matcher": "Edit|Write|MultiEdit", "hooks": [{"type": "command", "command": "./mixed-hook.sh"}]},
	  {"matcher": "Edit|Write|MultiEdit", "hooks": [`+guardHandlerJSON(t)+`]}
	]`, canonical(t, hooksOf(t, settings)["PreToolUse"]))

	_, retracted := mapSettings(t, first, nil)
	assert.JSONEq(t, `[
	  {"matcher": "Edit", "hooks": [{"type": "command", "command": "./my-hook.sh"}]},
	  {"matcher": "Edit|Write|MultiEdit", "hooks": [{"type": "command", "command": "./mixed-hook.sh"}]}
	]`, canonical(t, hooksOf(t, retracted)["PreToolUse"]), "flag off keeps both user handlers")
}

// TestIsManagedClaudeHookCommand_CompoundUserCommandsAreNotOwned: a handler
// that starts with a generated script or invocation and then chains its own
// shell work, or passes arguments generation never emitted, is the user's.
// Retracting it would delete that work and, once no settings file names the
// script any more, the script with it.
func TestIsManagedClaudeHookCommand_CompoundUserCommandsAreNotOwned(t *testing.T) {
	t.Parallel()

	for _, command := range []string{
		".claude/hooks/autopus/hook-claude-stop.sh && ./audit-log.sh",
		`"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-gemini-afteragent.sh | tee log`,
		`"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-claude-sessionstart.sh; ./notify.sh`,
		".claude/hooks/autopus/hook-claude-stop.sh --verbose",
		".claude/hooks/autopus/my-own-hook.sh",
		`"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/my-own-hook.sh`,
		"auto react check --quiet && ./notify.sh",
		"auto check --hygiene --arch --quiet --staged --warn-only || ./fallback.sh",
		"auto rules fire --event PreToolUse; ./mine.sh",
		"auto rules sticky --event UserPromptSubmit | ./filter.sh",
		"AUTOPUS_TASKCREATED_DEFAULT_MODE=warn .claude/hooks/task-created-validate.sh && ./audit.sh",
		"AUTOPUS_TASKCREATED_DEFAULT_MODE=warn ./scripts/mine.sh",
		".claude/hooks/task-created-validate.sh > /tmp/task.log",
	} {
		assert.False(t, isManagedClaudeHookCommand(command), "user command: %q", command)
	}
}

// TestIsManagedClaudeHookCommand_GeneratedShapesStayOwned is the other side:
// every command line generation has written, the retired group S launchers
// included, is still retracted.
func TestIsManagedClaudeHookCommand_GeneratedShapesStayOwned(t *testing.T) {
	t.Parallel()

	for _, command := range []string{
		".claude/hooks/autopus/hook-claude-stop.sh",
		`"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-claude-sessionstart.sh`,
		`"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-gemini-afteragent.sh`,
		"auto check --hygiene --arch --quiet --staged --warn-only",
		"auto react check --quiet",
		"auto rules fire --event PreToolUse",
		"auto rules sticky --event UserPromptSubmit",
		"AUTOPUS_TASKCREATED_DEFAULT_MODE=warn .claude/hooks/task-created-validate.sh",
		"AUTOPUS_TASKCREATED_DEFAULT_MODE=enforce .claude/hooks/task-created-validate.sh",
		".claude/hooks/task-created-validate.sh",
		guardCommandLine,
	} {
		assert.True(t, isManagedClaudeHookCommand(command), "generated command: %q", command)
	}
}
