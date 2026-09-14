package content

import (
	"regexp"
	"strings"
)

type ompLegacyDispatch struct {
	agent    string
	task     string
	isolated bool
}

var (
	ompFencedCodeRe           = regexp.MustCompile("(?s)```[^\\n]*\\n.*?\\n```")
	ompInlineDispatchCodeRe   = regexp.MustCompile("`(?:Agent|task|spawn_agent)\\s*\\([^`\\n]*\\)`")
	ompCodexNativeSkillPathRe = regexp.MustCompile(`\.omp/skills/codex-([a-z0-9-]+)/SKILL\.md`)
	ompLegacyCallRe           = regexp.MustCompile(`(?s)\b(?:Agent|task)\s*\((.*?)\)`)
	ompLegacyRoleRe           = regexp.MustCompile(`(?i)\b(?:subagent_type|agent)\s*[:=]\s*(?:"([^"]+)"|'([^']+)')`)
	ompLegacyTaskRe           = regexp.MustCompile(`(?s)\b(?:task|prompt)\s*[:=]\s*(?:"""(.*?)"""|"([^"]*)"|'([^']*)')`)
	ompSpawnStartRe           = regexp.MustCompile(`(?m)^\s*spawn_agent\s+([A-Za-z0-9_-]+)[^\n]*`)
	ompSpawnTaskRe            = regexp.MustCompile(`(?s)--task\s+(?:"([^"]*)"|'([^']*)'|([^\s\\]+))`)
	ompIsolationRe            = regexp.MustCompile(`(?i)\b(?:isolation|isolated)\s*[:=]\s*(?:"worktree"|'worktree'|true)`)
	ompLegacySpawnCallRe      = regexp.MustCompile(`(?s)\bspawn_agent\s*\((.*?)\)`)
	ompLegacyNamedCallRe      = regexp.MustCompile(`(?s)\b(TodoWrite|TaskCreate|TaskUpdate|TaskList|TaskGet|TeamCreate|TeamDelete|SendMessage|ToolSearch|AskUserQuestion|request_user_input|send_input|wait_agent|close_agent)\s*\((.*?)\)`)
	ompLegacyTaskToolRe       = regexp.MustCompile(`task tool\s*→\s*(?:subagent_type|agent)="([^"]+)"(?:,\s*prompt="([^"]*)")?`)
)

var ompLegacyCoordinationTokens = []string{
	"Agent(",
	"Agent (",
	"task(",
	"task (",
	"task tool",
	"spawn_agent",
	"subagent_type",
	"prompt =",
	"prompt=",
	"multi_agent",
	"send_input",
	"wait_agent",
	"close_agent",
	"TodoWrite",
	"TaskCreate",
	"TaskUpdate",
	"TaskList",
	"TaskGet",
	"TeamCreate",
	"TeamDelete",
	"SendMessage",
	"ToolSearch",
	"auto pipeline worktree",
}

// NormalizeOMPSemanticReferences translates legacy coordination examples into
// native OMP tool calls without adding unrelated workflow instructions.
func NormalizeOMPSemanticReferences(body string) string {
	body = NormalizeOMPResourcePaths(body)
	body = normalizeOMPLegacyDispatchBlocks(body)
	body = ompInlineDispatchCodeRe.ReplaceAllStringFunc(body, normalizeOMPInlineDispatchCode)
	body = strings.NewReplacer(
		"Agent() calls", "task batch items",
		"Agent() call", "task batch item",
		"Agent()", "task batch",
		"Agent(...) calls", "task batches",
		"Agent(...) call", "task batch",
		"Agent(...)", "task batch",
		"task(...) calls", "task batches",
		"task(...) call", "task batch",
		"task(...)", "task batch",
		"spawn_agent(...) calls", "task batches",
		"spawn_agent(...) call", "task batch",
		"spawn_agent(...)", "task batch",
	).Replace(body)
	body = ompLegacySpawnCallRe.ReplaceAllStringFunc(body, func(string) string {
		return renderOMPTaskBatch([]ompLegacyDispatch{{}})
	})
	body = ompLegacyNamedCallRe.ReplaceAllStringFunc(body, normalizeOMPNamedLegacyCall)
	body = ompLegacyTaskToolRe.ReplaceAllStringFunc(body, func(call string) string {
		match := ompLegacyTaskToolRe.FindStringSubmatch(call)
		dispatch := ompLegacyDispatch{}
		if len(match) > 1 {
			dispatch.agent = match[1]
		}
		if len(match) > 2 {
			dispatch.task = match[2]
		}
		return renderOMPTaskBatch([]ompLegacyDispatch{dispatch})
	})
	body = normalizeOMPInlineLegacyCalls(body)
	body = ompIsolationRe.ReplaceAllString(body, `conditional per-item isolation`)
	body = strings.NewReplacer(
		"auto pipeline worktree", `task item with conditional isolation after schema inspection`,
		"spawn_agent(...)", "task batch",
		"spawn_agent", "task batch",
		"multi_agent", "task batch",
		"task tool →", "task batch:",
		"task tool", "task",
		"subagent_type", "agent",
		"send_input", "hub message",
		"prompt =", "task =",
		"prompt=", "task=",
		"wait_agent", "hub job wait",
		"close_agent", "hub cancellation",
		"TodoWrite", "todo operation",
		"TaskCreate", "todo append operation",
		"TaskUpdate", "todo state operation",
		"TaskList", "todo view operation",
		"TaskGet", "todo view operation",
		"TeamCreate", "task batch",
		"TeamDelete", "hub cancellation",
		"SendMessage", "hub message",
		"ToolSearch", "available tool discovery",
		"AskUserQuestion", "ask the user directly",
		"request_user_input", "ask the user directly",
	).Replace(body)
	return body
}

// NormalizeOMPResourcePaths completes the native OMP resource cutover after
// the shared first-stage platform rewrite.
// @AX:ANCHOR [AUTO]: keep OMP resource-path cutover centralized in this normalization boundary.
// @AX:REASON [AUTO]: agent rendering and two skill transformation paths depend on identical native resource references.
func NormalizeOMPResourcePaths(body string) string {
	body = strings.NewReplacer(
		".agents/skills/", ".omp/skills/",
		".agents/commands/", ".omp/commands/",
	).Replace(body)
	return ompCodexNativeSkillPathRe.ReplaceAllString(body, ".omp/skills/$1/SKILL.md")
}

func hasOMPLegacyCoordination(body string) bool {
	if ompLegacyCallRe.MatchString(body) || ompLegacyNamedCallRe.MatchString(body) ||
		ompLegacySpawnCallRe.MatchString(body) || ompLegacyTaskToolRe.MatchString(body) {
		return true
	}
	for _, token := range ompLegacyCoordinationTokens {
		if strings.Contains(body, token) {
			return true
		}
	}
	return ompIsolationRe.MatchString(body)
}

func normalizeOMPLegacyDispatchBlocks(body string) string {
	return ompFencedCodeRe.ReplaceAllStringFunc(body, func(block string) string {
		if !hasOMPLegacyDispatch(block) {
			return block
		}
		return renderOMPTaskBatch(parseOMPLegacyCalls(block))
	})
}

func hasOMPLegacyDispatch(body string) bool {
	return ompLegacyCallRe.MatchString(body) || strings.Contains(body, "spawn_agent")
}

func normalizeOMPInlineDispatchCode(call string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(call, "`"), "`")
	dispatch := ompLegacyDispatch{}
	if match := ompLegacyCallRe.FindStringSubmatch(inner); len(match) > 1 {
		dispatch.agent = firstOMPFieldValue(ompLegacyRoleRe.FindStringSubmatch(match[1]))
		dispatch.isolated = ompIsolationRe.MatchString(match[1])
	} else if match := ompLegacySpawnCallRe.FindStringSubmatch(inner); len(match) > 1 {
		dispatch.agent = firstOMPFieldValue(ompLegacyRoleRe.FindStringSubmatch(match[1]))
		dispatch.isolated = ompIsolationRe.MatchString(match[1])
	}
	result := "`task` batch"
	if agent := ompNativeDispatchAgent(dispatch.agent); agent != "" {
		result += " selecting agent `" + agent + "`"
	}
	if dispatch.isolated {
		result += " with per-item isolation"
	}
	return result
}

func normalizeOMPNamedLegacyCall(call string) string {
	match := ompLegacyNamedCallRe.FindStringSubmatch(call)
	if len(match) < 2 {
		return call
	}
	switch match[1] {
	case "TodoWrite", "TaskCreate":
		return `todo with {"i":"Updating parent-owned progress","op":"append","phase":"Implementation","items":["<task>"]}`
	case "TaskUpdate":
		return `todo with one top-level state operation such as {"i":"Updating parent-owned progress","op":"done","task":"<exact task content>"}`
	case "TaskList", "TaskGet":
		return `todo with {"i":"Inspecting parent-owned progress","op":"view"}`
	case "TeamCreate":
		return renderOMPTaskBatch([]ompLegacyDispatch{{}})
	case "TeamDelete", "close_agent":
		return `hub with {"i":"Cancelling abandoned work","op":"cancel","ids":["<job id>"]}`
	case "SendMessage", "send_input":
		return `hub with {"i":"Following up with an existing worker","op":"send","to":"<same agent id>","message":"<follow-up>"}`
	case "wait_agent":
		return `hub with {"i":"Waiting for blocked work","op":"wait","ids":["<job id>"]}`
	case "ToolSearch":
		return "available tool discovery"
	case "AskUserQuestion", "request_user_input":
		return "ask the user directly"
	default:
		return call
	}
}

func normalizeOMPInlineLegacyCalls(body string) string {
	matches := ompLegacyCallRe.FindAllStringIndex(body, -1)
	if len(matches) == 0 {
		return body
	}

	var b strings.Builder
	cursor := 0
	for i := 0; i < len(matches); {
		start, end := matches[i][0], matches[i][1]
		dispatches := parseOMPLegacyCalls(body[start:end])
		j := i + 1
		for j < len(matches) && strings.TrimSpace(body[end:matches[j][0]]) == "" {
			dispatches = append(dispatches, parseOMPLegacyCalls(body[matches[j][0]:matches[j][1]])...)
			end = matches[j][1]
			j++
		}
		b.WriteString(body[cursor:start])
		b.WriteString(renderOMPTaskBatch(dispatches))
		if j > i+1 {
			b.WriteString("\n")
		}
		cursor = end
		i = j
	}
	b.WriteString(body[cursor:])
	return b.String()
}
