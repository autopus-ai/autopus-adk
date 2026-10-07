package content

import (
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/editguard"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// editGuardTimeoutSeconds bounds the guard hook (REQ-EG-12). A timed-out
// pre-tool command does not block, so a hung guard stays fail-open.
const editGuardTimeoutSeconds = 5

// appendEditGuardHook registers `auto guard edit` (SPEC-EDITGUARD-001) on the
// lanes whose deny contract has been probed and whose wiring lives in this
// generator: Claude Code (A1) and OpenCode (A2). Every other platform gets no
// entry here.
func appendEditGuardHook(hooks []adapter.HookConfig, platform string) []adapter.HookConfig {
	hook := adapter.HookConfig{
		Event:   "PreToolUse",
		Matcher: rulecond.MatcherEdit,
		Type:    "command",
		Timeout: editGuardTimeoutSeconds,
	}
	switch platform {
	case "claude", "claude-code":
		hook.Command = editGuardCommandLine(editguard.PlatformClaudeCode)
	case "opencode":
		// The plugin spawns the guard without a shell, applies the clean-exit
		// rule itself, and maps the canonical matcher to its native tools per
		// plugin API version.
		hook.Command = EditGuardCommand(editguard.PlatformOpenCode)
	default:
		return hooks
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
