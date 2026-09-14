package content

import (
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
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
// Event names and tool matchers are translated per-platform. Claude Code uses
// PreToolUse/PostToolUse with Bash, Antigravity uses the same event names with
// run_command, and legacy Gemini CLI used BeforeTool/AfterTool.
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
			Timeout: 30,
		})
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
			Timeout: 60,
		})
	}

	if cfg.ReactReview {
		hooks = appendUniqueHook(hooks, adapter.HookConfig{
			Event:   post,
			Matcher: commandMatcher,
			Type:    "command",
			Command: translateHookCommand("auto react check --quiet", post, platform),
			Timeout: 60,
		})
	}

	hooks, err := appendConditionalDispatcher(hooks, platform)
	if err != nil {
		return nil, err
	}
	hooks = append(hooks, generateCompletionHooks(platform)...)
	return hooks, nil
}

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
	if claudeMatcher == "Bash" && platform == "antigravity-cli" {
		return "run_command"
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

// DetectPermissions는 프로젝트 루트를 분석하여 기본 권한을 생성한다.
func DetectPermissions(projectRoot string, extra config.PermissionsConf) *adapter.PermissionSet {
	allow := []string{
		// Common: always included
		"Bash(auto *)",
		"Bash(auto:*)",
		"Bash(git *)",
		"Bash(git:*)",
		"Bash(make:*)",
		"Bash(ls:*)",
		"Bash(cat:*)",
		"Bash(find:*)",
		"Bash(grep:*)",
		"Bash(wc:*)",
		"Bash(sort:*)",
		"Bash(mkdir:*)",
		"Bash(echo:*)",
		"Bash(gh:*)",
		"mcp__sequential-thinking__sequentialthinking",
		"WebSearch",

		// Pipeline: current Claude orchestration tools.
		"Agent",
		"AskUserQuestion",
		"TaskCreate",
		"TaskGet",
		"TaskList",
		"TaskUpdate",
		"SendMessage",
		"Workflow",
		"ToolSearch",
	}

	// Go project detection
	if fileExists(filepath.Join(projectRoot, "go.mod")) {
		allow = append(allow,
			"Bash(go build:*)", "Bash(go test:*)", "Bash(go vet:*)",
			"Bash(go run:*)", "Bash(go mod:*)", "Bash(go tool:*)",
			"Bash(go get:*)", "Bash(go install:*)", "Bash(go version:*)",
			"Bash(go env:*)", "Bash(go clean:*)",
			"Bash(golangci-lint:*)", "Bash(gofmt:*)",
		)
	}

	// Node project detection
	if fileExists(filepath.Join(projectRoot, "package.json")) {
		allow = append(allow,
			"Bash(npm *)", "Bash(npx *)", "Bash(node *)",
			"Bash(pnpm *)", "Bash(yarn *)",
		)
	}

	// Merge extra permissions from autopus.yaml
	allow = append(allow, extra.ExtraAllow...)

	var deny []string
	deny = append(deny, extra.ExtraDeny...)

	return &adapter.PermissionSet{
		Allow: allow,
		Deny:  deny,
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
