package codex

import (
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

const codexPipelineNativeGuidance = `
## Codex Execution

Use native spawn_agent, send_message, followup_task, wait_agent(),
interrupt_agent, and list_agents when delegation is justified. Read their
current schemas (including task_name and message) before invoking them.
Workers share the same cwd and filesystem: parallel work requires disjoint
write ownership, not an assumption that conversation forks isolate files.
Keep the configured model, effort, sandbox, and user permissions.
`

// normalizeCodexExtendedSkill replaces a shared skill body with the Codex
// native rewrite. cfg supplies the project's own settings so the installed
// skill states the configured worker ceiling instead of a literal default.
func normalizeCodexExtendedSkill(name, body string, cfg *config.HarnessConfig) string {
	switch name {
	case "agent-teams":
		return strings.TrimSpace(codexAgentTeamsSkillBody(cfg.CodexAgentConcurrency())) + "\n"
	case "agent-pipeline":
		return strings.TrimSpace(body) + "\n\n" + strings.TrimSpace(codexPipelineNativeGuidance) + "\n"
	case "worktree-isolation":
		return strings.TrimSpace(codexWorktreeIsolationSkillBody()) + "\n"
	case "subagent-dev":
		return strings.TrimSpace(codexSubagentDevSkillBody()) + "\n"
	case "prd":
		return strings.TrimSpace(rewriteCodexPRDSkillBody(body)) + "\n"
	default:
		return body
	}
}

func rewriteCodexPRDSkillBody(body string) string {
	body = strings.ReplaceAll(
		body,
		"PRD 작성 전에 6개 핵심 질문으로 컨텍스트를 수집합니다. 사용자 입력이 불충분할 경우 AskUserQuestion으로 확인:",
		"PRD 작성 전에 6개 핵심 질문으로 컨텍스트를 수집합니다. 사용자 입력이 불충분하면 active tool list에 있는 Codex `request_user_input`을 반드시 사용하고, 없을 때만 메인 세션이 짧은 plain-text 질문으로 직접 확인합니다:",
	)
	body = strings.ReplaceAll(body, "AskUserQuestion", "Codex `request_user_input` when available, otherwise a short plain-text question")
	return body
}
