package content

import (
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/editguard"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// editGuardTimeoutSeconds bounds the guard hook (REQ-EG-12). A timed-out
// pre-tool command does not block, so a hung guard stays fail-open.
const editGuardTimeoutSeconds = 5

// geminiEditGuardTimeoutMillis is the same bound for Gemini CLI, which reads a
// command hook timeout in milliseconds: in the T11 probe a timeout of 5 killed
// the hook before it read stdin, and 5000 let a 1 second hook deny.
const geminiEditGuardTimeoutMillis = editGuardTimeoutSeconds * 1000

// Native matchers of the lanes that do not take the canonical file-editing
// matcher rulecond.MatcherEdit.
const (
	// codexEditGuardMatcher keeps Codex shell calls, which reach PreToolUse as
	// tool Bash, away from the guard; only apply_patch names its targets.
	codexEditGuardMatcher = "apply_patch"
	// geminiEditGuardMatcher is anchored because Gemini CLI tests a matcher as
	// an unanchored regular expression against every tool name, MCP tools
	// included.
	geminiEditGuardMatcher = "^(write_file|replace)$"
)

// appendEditGuardHook registers `auto guard edit` (SPEC-EDITGUARD-001) on the
// lanes the enforcement matrix marks enforced. Antigravity, OMP, and unknown
// platforms get no entry (REQ-EG-12 to REQ-EG-14).
func appendEditGuardHook(hooks []adapter.HookConfig, platform string) []adapter.HookConfig {
	lane, ok := editguard.LaneFor(platform)
	if !ok || lane.State != editguard.Enforced {
		return hooks
	}
	hook := adapter.HookConfig{
		Event:   "PreToolUse",
		Matcher: rulecond.MatcherEdit,
		Type:    "command",
		Command: editGuardCommandLine(lane.Platform),
		Timeout: editGuardTimeoutSeconds,
	}
	switch lane.Platform {
	case editguard.PlatformCodex:
		hook.Matcher = codexEditGuardMatcher
	case editguard.PlatformOpenCode:
		// The plugin spawns the guard without a shell, applies the clean-exit
		// rule itself, and maps the canonical matcher to its native tools per
		// plugin API version.
		hook.Command = EditGuardCommand(lane.Platform)
	case editguard.PlatformGemini:
		hook.Event = translateHookEvent(hook.Event, platform)
		hook.Matcher = geminiEditGuardMatcher
		hook.Timeout = geminiEditGuardTimeoutMillis
	}
	return appendUniqueHook(hooks, hook)
}

// EditGuardCommand is the bare `auto guard edit` invocation of a lane. Hooks a
// host runs through a shell wrap it in editGuardCommandLine; the OpenCode
// plugin splits it into argv and spawns it directly.
func EditGuardCommand(platform string) string {
	return "auto guard edit --platform " + platform
}

// editGuardCommandLine is the registered command line of REQ-EG-11. It buffers
// the guard's decision and forwards it only after the guard exited 0, and it
// always exits 0 itself: a host blocks on exit 2, which is also how the Go
// runtime reports an unrecovered panic, so forwarding any other exit would turn
// a guard fault, or a crash after printing a deny, into a blocked edit.
func editGuardCommandLine(platform string) string {
	return "out=$(" + EditGuardCommand(platform) + `) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0`
}
