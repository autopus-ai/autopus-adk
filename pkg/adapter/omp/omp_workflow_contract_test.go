package omp

import "regexp"

var ompIntentionalPlatformRootGlobRe = regexp.MustCompile(
	`\.(codex|claude|gemini|opencode)/\*\*([^/A-Za-z0-9_-]|$)`,
)

func stripOMPIntentionalPlatformRootGlobs(body string) string {
	return ompIntentionalPlatformRootGlobRe.ReplaceAllString(body, "${2}")
}

func ompWorkflowForbiddenTokens() []string {
	return []string{
		".codex/", ".claude/", ".opencode/", ".gemini/",
		"@auto ", "@auto-",
		"Agent(", "subagent_type", "prompt =", "prompt=", "task tool", "task(...)",
		"spawn_agent", "multi_agent", "send_input", "wait_agent", "close_agent",
		"TodoWrite", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
		"TeamCreate", "TeamDelete", "SendMessage", "ToolSearch",
		"AskUserQuestion", "request_user_input",
		`isolation: "worktree"`, `isolation = "worktree"`, "auto pipeline worktree",
		"send_message", "followup_task", "interrupt_agent", "list_agents",
		"get_goal", "create_goal", "update_goal",
		".omp/skills/agent-teams",
	}
}
