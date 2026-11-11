package content

import "github.com/insajin/autopus-adk/pkg/adapter"

// diskGateTimeoutSeconds bounds the disk gate. It reads one config and makes
// one statfs; a timed-out pre-tool command does not block, so a hung gate
// stays fail-open.
const diskGateTimeoutSeconds = 5

// DiskGateCommand is the bare disk gate invocation.
const DiskGateCommand = "auto disk gate"

// diskGateCommandLine forwards the gate's decision only after it exited 0
// and always exits 0 itself, for the reason editGuardCommandLine gives: exit
// 2 blocks, and it is also how an unrecovered Go panic exits.
const diskGateCommandLine = "out=$(" + DiskGateCommand + `) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0`

// appendDiskGateHook registers the disk gate on Claude Code's PreToolUse
// Bash calls, which fire for subagent tool calls too. Other platforms get no
// entry: their hook payloads and deny contracts differ, and OMP has no
// pre-tool hook at all.
func appendDiskGateHook(hooks []adapter.HookConfig, platform string) []adapter.HookConfig {
	if platform != "claude" && platform != "claude-code" {
		return hooks
	}
	return appendUniqueHook(hooks, adapter.HookConfig{
		Event:   "PreToolUse",
		Matcher: "Bash",
		Type:    "command",
		Command: diskGateCommandLine,
		Timeout: diskGateTimeoutSeconds,
	})
}
