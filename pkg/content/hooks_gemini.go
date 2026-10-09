package content

import "github.com/insajin/autopus-adk/pkg/adapter"

// Legacy Gemini CLI hook dialect (.gemini/settings.json). Gemini CLI 0.52.0
// reads a command hook timeout in milliseconds (DEFAULT_HOOK_TIMEOUT 6e4) and
// tests a matcher as an unanchored regular expression against the tool name,
// MCP tools included; its shell tool is run_shell_command. Evidence:
// .autopus/specs/SPEC-EDITGUARD-001/evidence/t11-probes.txt.
const (
	// geminiShellMatcher selects the shell tool alone. Anchored for the same
	// reason as geminiEditGuardMatcher.
	geminiShellMatcher = "^run_shell_command$"
	// retiredGeminiShellMatcher is the Claude matcher earlier releases copied
	// into the Gemini entries. No Gemini tool is named Bash, so those entries
	// never ran.
	retiredGeminiShellMatcher = "Bash"
	millisPerSecond           = 1000
)

func isGeminiHookPlatform(platform string) bool {
	return platform == "gemini" || platform == "gemini-cli"
}

// translateHookTimeout converts a timeout given in seconds, the unit every
// other hook host reads, into the unit platform reads.
func translateHookTimeout(seconds int, platform string) int {
	if isGeminiHookPlatform(platform) {
		return seconds * millisPerSecond
	}
	return seconds
}

// RetiredGeminiHookShapes returns, for the current Gemini CLI hooks, the
// shell-tool entries that releases before the dialect fix wrote for the same
// commands: matcher "Bash" and the timeout in seconds. Settings writers subtract
// them with the current shapes, so an update replaces such an entry instead of
// leaving a handler that never fires beside the corrected one. Hooks that kept
// their shape (the edit guard) have no retired form.
func RetiredGeminiHookShapes(hooks []adapter.HookConfig) []adapter.HookConfig {
	var retired []adapter.HookConfig
	for _, hook := range hooks {
		if hook.Matcher != geminiShellMatcher {
			continue
		}
		previous := hook
		previous.Matcher = retiredGeminiShellMatcher
		previous.Timeout = hook.Timeout / millisPerSecond
		retired = append(retired, previous)
	}
	return retired
}
