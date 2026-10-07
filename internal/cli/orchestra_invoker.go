package cli

import (
	"os"
	"strings"

	"github.com/insajin/autopus-adk/pkg/detect"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// @AX:WARN: [AUTO] test-overridable package global — restore after mutation to prevent cross-test invoker leakage
// @AX:REASON: [AUTO] invoker resolution tests replace this detector while production reads it during debate judge selection
var detectOrchestraInvokingProvider = currentOrchestraInvokingProvider

// @AX:ANCHOR: [AUTO] external invocation-identity boundary combining platform, process ancestry, and runtime markers
// @AX:REASON: [AUTO] automatic judge selection depends on stable provider names and deterministic signal precedence
func currentOrchestraInvokingProvider() string {
	return orchestraInvokerProviderFromSignals(
		os.Getenv("AUTOPUS_PLATFORM"),
		detect.DetectAgentRuntime(),
		os.Getenv("CLAUDECODE") != "",
		hasCodexRuntimeMarker(
			os.Getenv("CODEX"),
			os.Getenv("CODEX_CI"),
			os.Getenv("CODEX_THREAD_ID"),
			os.Getenv("CODEX_MANAGED_BY_NPM"),
		),
	)
}

// hasCodexRuntimeMarker reports whether any environment marker that the Codex
// CLI exports to its child processes is set.
func hasCodexRuntimeMarker(codex, codexCI, codexThreadID, codexManagedByNPM string) bool {
	return codex != "" || codexCI != "" || codexThreadID != "" || codexManagedByNPM != ""
}

// @AX:NOTE: [AUTO] signal precedence is AUTOPUS_PLATFORM, then exact bounded runtime ancestry, then mutually exclusive legacy markers
func orchestraInvokerProviderFromSignals(
	platform string,
	runtime detect.AgentRuntime,
	claudeCode bool,
	codex bool,
) string {
	if platform = strings.ToLower(strings.TrimSpace(platform)); platform != "" {
		switch platform {
		case "claude", "claude-code":
			return "claude"
		case "codex":
			return "codex"
		case "antigravity", "antigravity-cli", "gemini", "gemini-cli":
			return "gemini"
		case "opencode":
			return "opencode"
		default:
			return ""
		}
	}

	switch runtime {
	case detect.AgentRuntimeClaudeCode:
		return "claude"
	case detect.AgentRuntimeCodex:
		return "codex"
	case detect.AgentRuntimeAntigravityCLI:
		return "gemini"
	case detect.AgentRuntimeOpenCode:
		return "opencode"
	}

	if claudeCode == codex {
		return ""
	}
	if claudeCode {
		return "claude"
	}
	return "codex"
}

// @AX:NOTE: [AUTO] judge precedence is explicit CLI flag, then invoking provider, then configured fallback
func resolveInvocationJudge(flagJudge, configuredJudge, invokingProvider string) string {
	if flagJudge = strings.TrimSpace(flagJudge); flagJudge != "" {
		return flagJudge
	}
	if invokingProvider = strings.TrimSpace(invokingProvider); invokingProvider != "" {
		return invokingProvider
	}
	return strings.TrimSpace(configuredJudge)
}

func invocationJudgeSelectionSource(flagJudge, invokingProvider string) string {
	if strings.TrimSpace(flagJudge) != "" {
		return orchestra.JudgeSelectionExplicit
	}
	if strings.TrimSpace(invokingProvider) != "" {
		return orchestra.JudgeSelectionInvokingProvider
	}
	return orchestra.JudgeSelectionConfiguredFallback
}
