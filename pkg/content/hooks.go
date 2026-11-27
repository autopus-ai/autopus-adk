package content

import (
	"os"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/langgate"
)

// GitHookScript는 Git 훅 스크립트이다.
type GitHookScript struct {
	// Path는 훅 파일 경로이다.
	Path string
	// Content는 스크립트 내용이다.
	Content string
}

// GenerateProjectHookConfigs generates hooks with access to feature flags.
// @AX:ANCHOR [AUTO]: preserve the project-aware hook projection contract across platform adapters.
// @AX:REASON [AUTO]: Claude, Codex, OpenCode, Antigravity, and compatibility callers depend on the same native-hook versus Git-only precedence.
func GenerateProjectHookConfigs(cfg *config.HarnessConfig, platform string, supportsHooks bool) ([]adapter.HookConfig, []GitHookScript, error) {
	if cfg == nil {
		return GenerateHookConfigs(config.HooksConf{}, platform, supportsHooks)
	}
	if supportsHooks {
		hooks, err := generateCLIHooks(cfg.Hooks, platform)
		if err != nil {
			return nil, nil, err
		}
		hooks = append(hooks, generateCC21Hooks(cfg.Features.CC21, platform)...)
		hooks = append(hooks, generateVerifyStopHook(cfg.Hooks, platform)...)
		hooks = append(hooks, generateLanguageStopHook(cfg, platform)...)
		return hooks, generateGitOnlyHooks(cfg.Hooks), nil
	}
	return nil, generateGitHooks(cfg.Hooks), nil
}

// GenerateHookConfigs returns native CLI hooks plus Git-only checks that have
// no matching CLI event. Platforms without hooks receive all Git checks.
func GenerateHookConfigs(cfg config.HooksConf, platform string, supportsHooks bool) ([]adapter.HookConfig, []GitHookScript, error) {
	if supportsHooks {
		hooks, err := generateCLIHooks(cfg, platform)
		return hooks, generateGitOnlyHooks(cfg), err
	}
	return nil, generateGitHooks(cfg), nil
}

// generateCLIHooks는 CLI 훅 설정을 생성한다.
// Event names, tool matchers, and timeout units are translated per-platform.
// Claude Code uses PreToolUse/PostToolUse with Bash, Antigravity uses the same
// event names with run_command, and legacy Gemini CLI uses BeforeTool/AfterTool
// with run_shell_command and millisecond timeouts (hooks_gemini.go).
func generateCLIHooks(cfg config.HooksConf, platform string) ([]adapter.HookConfig, error) {
	var hooks []adapter.HookConfig
	pre := translateHookEvent("PreToolUse", platform)
	post := translateHookEvent("PostToolUse", platform)
	commandMatcher := translateHookMatcher("Bash", platform)

	if cfg.PreCommitArch {
		hooks = appendUniqueHook(hooks, adapter.HookConfig{
			Event:   pre,
			Matcher: commandMatcher,
			Type:    "command",
			Command: translateHookCommand("auto check --hygiene --arch --quiet --staged --warn-only", pre, platform),
			Timeout: translateHookTimeout(30, platform),
		})
	}

	if cfg.IsEditGuardEnabled() {
		hooks = appendEditGuardHook(hooks, platform)
	}

	// One hook serves both checks: hooks.cpu_gate alone still needs it. The
	// AUTOPUS_CPU_GATE switch is read when the hook runs, so it acts through
	// this same hook, which disk_gate's default-on registers.
	if cfg.IsDiskGateEnabled() || cfg.IsCPUGateEnabled() {
		hooks = appendDiskGateHook(hooks, platform)
	}

	// Lore check: not added as PreToolUse hook — it runs via git commit-msg hook only.
	// Checking lore on every Bash call would fail because it validates the last commit,
	// not the current action. The commit-msg hook validates the message being committed.

	if cfg.ReactCIFailure {
		hooks = appendUniqueHook(hooks, adapter.HookConfig{
			Event:   post,
			Matcher: commandMatcher,
			Type:    "command",
			Command: translateHookCommand("auto react check --quiet", post, platform),
			Timeout: translateHookTimeout(60, platform),
		})
	}

	if cfg.ReactReview {
		hooks = appendUniqueHook(hooks, adapter.HookConfig{
			Event:   post,
			Matcher: commandMatcher,
			Type:    "command",
			Command: translateHookCommand("auto react check --quiet", post, platform),
			Timeout: translateHookTimeout(60, platform),
		})
	}

	return appendConditionalDispatcher(hooks, platform)
}

// stopHookPlatform reports a CLI whose Stop event takes the gate as it is:
// Claude Code, and Codex, whose Stop payload carries the same session_id,
// cwd, stop_hook_active and last_assistant_message and whose
// {"decision":"block","reason":...} sends the reason back as the next
// prompt. Antigravity's Stop speaks another dialect (camelCase fields,
// "continue"), and OpenCode has no blocking Stop event.
func stopHookPlatform(platform string) bool {
	switch platform {
	case "claude", "claude-code", "codex":
		return true
	}
	return false
}

// verifyStopCommand is the Stop hook that builds and tests the changed files.
const verifyStopCommand = "auto verify stop"

// generateVerifyStopHook registers the verification gate on Claude Code's
// Stop event. A set hooks.verify_stop_budget becomes the command's --budget,
// and the hook timeout follows it (config.VerifyStopHookTimeout), so a larger
// budget is not killed by Claude Code before it returns a verdict. Unset
// keeps the bare command and the 600s timeout it always had.
func generateVerifyStopHook(cfg config.HooksConf, platform string) []adapter.HookConfig {
	if !stopHookPlatform(platform) || !cfg.IsVerifyStopEnabled() {
		return nil
	}
	budget, err := cfg.VerifyStopBudgetDuration()
	if err != nil {
		budget = config.DefaultVerifyStopBudget
	}
	command := verifyStopCommand
	if cfg.VerifyStopBudget != "" && err == nil {
		// Explicit whenever the key is set: the gate alone may only lower a
		// budget it is not given, so the timeout and budget move together.
		command += " --budget " + budget.String()
	}
	timeout := int(config.VerifyStopHookTimeout(budget) / time.Second)
	hooks := []adapter.HookConfig{{Event: "Stop", Type: "command", Command: command, Timeout: timeout}}
	if platform == "codex" {
		// Codex's Stop payload has no background_tasks; these record its
		// running sub-agents, so the gate waits instead of judging a tree
		// one of them is still writing.
		hooks = append(hooks,
			adapter.HookConfig{Event: "SubagentStart", Type: "command", Command: verifyStopCommand + " --subagent start", Timeout: 5},
			adapter.HookConfig{Event: "SubagentStop", Type: "command", Command: verifyStopCommand + " --subagent stop", Timeout: 5})
	}
	return hooks
}

// languageStopCommand is the Stop hook that sends a reply back when it is not
// written in language.ai_responses.
const languageStopCommand = "auto verify language"

// generateLanguageStopHook registers the response-language check on Claude
// Code's Stop event, whose payload carries the final reply. It is skipped when
// the configured language is one the check cannot judge, English included.
func generateLanguageStopHook(cfg *config.HarnessConfig, platform string) []adapter.HookConfig {
	if !stopHookPlatform(platform) || !cfg.Hooks.IsLanguageStopEnabled() ||
		!langgate.Supported(cfg.Language.AIResponses) {
		return nil
	}
	hooks := []adapter.HookConfig{{Event: "Stop", Type: "command", Command: languageStopCommand, Timeout: 10}}
	if platform == "codex" {
		// The note check reads a Claude Code transcript; on Codex it would
		// cost every tool call and find nothing.
		return hooks
	}
	// The note check runs after every tool call and reminds at the first
	// note in another language, before the rest of the turn follows it.
	return append(hooks, adapter.HookConfig{Event: "PostToolUse", Type: "command", Command: languageNoteCommandLine, Timeout: 5})
}

// languageNoteCommandLine forwards the note check's output only after it
// exited 0 and always exits 0 itself, like diskGateCommandLine: a failing or
// panicking check must never turn into a tool error.
const languageNoteCommandLine = "out=$(" + languageStopCommand + ` --note) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0`

func generateCC21Hooks(cfg config.CC21FeaturesConf, platform string) []adapter.HookConfig {
	if platform != "claude" && platform != "claude-code" {
		return nil
	}
	if !cfg.Enabled || !cfg.TaskCreatedEnabled {
		return nil
	}

	mode := cfg.TaskCreatedMode
	if mode == "" {
		mode = "warn"
	}

	return []adapter.HookConfig{{
		Event:   "TaskCreated",
		Type:    "command",
		Command: "AUTOPUS_TASKCREATED_DEFAULT_MODE=" + mode + " .claude/hooks/task-created-validate.sh",
		Timeout: 5,
	}}
}

// translateHookEvent maps Claude Code event names to the platform's native
// event names. Unknown platforms pass through the Claude Code name unchanged
// (safe default — most adapters will reject unknown names and log a warning).
func translateHookEvent(claudeEvent, platform string) string {
	if platform != "gemini" && platform != "gemini-cli" {
		return claudeEvent
	}
	switch claudeEvent {
	case "PreToolUse":
		return "BeforeTool"
	case "PostToolUse":
		return "AfterTool"
	default:
		return claudeEvent
	}
}

func translateHookMatcher(claudeMatcher, platform string) string {
	if claudeMatcher != "Bash" {
		return claudeMatcher
	}
	switch {
	case platform == "antigravity-cli":
		return "run_command"
	case isGeminiHookPlatform(platform):
		return geminiShellMatcher
	}
	return claudeMatcher
}

func appendUniqueHook(hooks []adapter.HookConfig, hook adapter.HookConfig) []adapter.HookConfig {
	for _, existing := range hooks {
		if sameHookConfig(existing, hook) {
			return hooks
		}
	}
	return append(hooks, hook)
}

func sameHookConfig(a, b adapter.HookConfig) bool {
	if a.Event != b.Event ||
		a.Matcher != b.Matcher ||
		a.Type != b.Type ||
		a.Command != b.Command ||
		a.Timeout != b.Timeout {
		return false
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for k, v := range a.Env {
		if b.Env[k] != v {
			return false
		}
	}
	return true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
