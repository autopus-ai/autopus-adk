package orchestra

import "strings"

// ProviderSandboxMode reports the sandbox mode a provider runs under, judged
// from the argv it executes. A permission or sandbox bypass anywhere in args
// yields unrestricted even over a policy stamp, so evidence claims read-only
// only while no bypass reached the executed argv. Otherwise the policy stamp
// wins, and an unstamped provider is classified from its native argv.
func ProviderSandboxMode(provider ProviderConfig, args []string) string {
	if argvBypassesSandbox(args) {
		return SandboxModeUnrestricted
	}
	if mode := strings.TrimSpace(provider.SandboxMode); mode != "" {
		return mode
	}
	return inferSandboxModeFromArgv(args)
}

// argvBypassesSandbox reports a flag, or a flag and value pair, that disables
// permission checks or the sandbox. Items match exactly, so prompt text that
// merely mentions a bypass is not mistaken for one.
func argvBypassesSandbox(args []string) bool {
	for index, arg := range args {
		switch arg {
		case "--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox", "--yolo", "-y",
			"--permission-mode=bypassPermissions", "--sandbox=danger-full-access", "-s=danger-full-access":
			return true
		}
		if index+1 >= len(args) {
			continue
		}
		next := args[index+1]
		if (arg == "--permission-mode" && next == "bypassPermissions") ||
			((arg == "--sandbox" || arg == "-s") && next == "danger-full-access") {
			return true
		}
	}
	return false
}

// inferSandboxModeFromArgv classifies an unstamped provider: codex declares its
// sandbox explicitly and a claude plan permission mode is read-only. Without
// any restriction the orchestrator imposes nothing, which is recorded as
// unrestricted rather than guessed.
func inferSandboxModeFromArgv(args []string) string {
	for index, arg := range args {
		switch {
		case arg == "--sandbox" && index+1 < len(args) && !strings.HasPrefix(args[index+1], "-"):
			return args[index+1]
		case strings.HasPrefix(arg, "--sandbox=") && len(arg) > len("--sandbox="):
			return strings.TrimPrefix(arg, "--sandbox=")
		case arg == "--permission-mode" && index+1 < len(args) && args[index+1] == "plan",
			arg == "--permission-mode=plan":
			return SandboxModeReadOnly
		}
	}
	return SandboxModeUnrestricted
}
