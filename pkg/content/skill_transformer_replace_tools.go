package content

import "strings"

// replaceAgentCalls converts Agent(subagent_type="X", task="Y") to platform syntax.
// Reuses agentMappingRe from agent_transformer_mapping.go.
func replaceAgentCalls(line string, platform string, ompNative bool) string {
	return agentMappingRe.ReplaceAllStringFunc(line, func(match string) string {
		sub := agentMappingRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name := sub[1]
		task := ""
		if len(sub) >= 3 {
			task = sub[2]
		}

		switch platform {
		case "codex":
			if task != "" {
				return `spawn_agent ` + name + ` --task "` + task + `"`
			}
			return `spawn_agent ` + name
		case "gemini", "gemini-cli", "antigravity-cli":
			if task != "" {
				return `@` + name + ` ` + task
			}
			return `@` + name
		case "opencode":
			if task != "" {
				return `task tool → subagent_type="` + name + `", prompt="` + task + `"`
			}
			return `task tool → subagent_type="` + name + `"`
		case "omp":
			return renderOMPTaskBatch([]ompLegacyDispatch{{agent: name, task: task}}, ompNative)
		default:
			return match
		}
	})
}

// replaceTodoWrite rewrites TodoWrite to the platform's own todo tool name, and
// comments the reference out on platforms that have no equivalent.
// The per-platform rewrite returns immediately so that a platform tool name is
// never used as a skip condition for another platform.
func replaceTodoWrite(line string, platform string) string {
	if strings.Contains(line, "TodoWrite") {
		switch normalizePlatform(platform) {
		case "opencode":
			return todoWriteRe.ReplaceAllString(line, "todowrite")
		case "omp":
			return todoWriteRe.ReplaceAllString(line, "todo")
		}
	}
	if strings.Contains(line, "todowrite") {
		return line
	}
	if todoWriteRe.MatchString(line) {
		return "// TodoWrite is not available on this platform"
	}
	return line
}

// ompWorkflowToolReplacer is the line-level fallback for OMP bodies that do
// not require the full semantic contract. Coordination-heavy bodies are
// normalized as a whole by NormalizeOMPSemanticReferences.
var ompWorkflowToolReplacer = strings.NewReplacer(
	"AskUserQuestion", "ask the user directly",
	"request_user_input", "ask the user directly",
	"TaskCreate", "todo append operation",
	"TaskUpdate", "todo state operation",
	"TaskList", "todo view operation",
	"TaskGet", "todo view operation",
	"TeamCreate", "task batch",
	"TeamDelete", "hub cancellation",
	"SendMessage", "hub message",
	"ToolSearch", "available tool discovery",
)

var openCodeWorkflowToolReplacer = strings.NewReplacer(
	"AskUserQuestion", "question",
	"request_user_input", "question",
	"TaskCreate", "todowrite",
	"TaskUpdate", "todowrite",
	"TaskList", "todowrite",
	"TaskGet", "todowrite",
	"TeamCreate", "task",
	"SendMessage", "task result handoff",
)

func replaceWorkflowTools(line string, platform string) string {
	switch normalizePlatform(platform) {
	case "opencode":
		return openCodeWorkflowToolReplacer.Replace(line)
	case "omp":
		return ompWorkflowToolReplacer.Replace(line)
	default:
		return line
	}
}
