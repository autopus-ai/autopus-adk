package content_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/content"
)

func TestReplaceToolReferences_AgentToSpawnAgent(t *testing.T) {
	t.Parallel()

	// S3: Agent() → spawn_agent mapping
	body := `Use Agent(subagent_type="executor", task="implement feature") for delegation.`

	codex := content.ReplaceToolReferences(body, "codex")
	assert.Contains(t, codex, `spawn_agent executor --task "implement feature"`)
	assert.NotContains(t, codex, "Agent(subagent_type=")

	gemini := content.ReplaceToolReferences(body, "gemini")
	assert.Contains(t, gemini, "@executor implement feature")
	assert.NotContains(t, gemini, "Agent(subagent_type=")
}

func TestReplaceToolReferences_MCPToWebSearch(t *testing.T) {
	t.Parallel()

	// With arguments: detailed replacement via mcpResolveRe
	body := `Call mcp__context7__resolve-library-id(cobra) to find docs.`

	codex := content.ReplaceToolReferences(body, "codex")
	assert.Contains(t, codex, "Context7 MCP first")
	assert.Contains(t, codex, `WebSearch "cobra docs"`)
	assert.NotContains(t, codex, "mcp__context7__")

	gemini := content.ReplaceToolReferences(body, "gemini")
	assert.Contains(t, gemini, "Context7 MCP first")
	assert.Contains(t, gemini, `WebSearch "cobra docs"`)

	// Bare mcp__ reference without arguments: generic replacement
	bare := `Call mcp__context7 for help.`
	result := content.ReplaceToolReferences(bare, "codex")
	assert.Contains(t, result, "WebSearch")
	assert.NotContains(t, result, "mcp__")
}

func TestReplaceToolReferences_PathMapping(t *testing.T) {
	t.Parallel()

	// S5: .claude/ → .codex/.gemini/ path mapping
	body := `Check .claude/agents/autopus/ for definitions.`

	codex := content.ReplaceToolReferences(body, "codex")
	assert.Contains(t, codex, ".codex/agents/autopus/")
	assert.NotContains(t, codex, ".claude/")

	gemini := content.ReplaceToolReferences(body, "gemini")
	assert.Contains(t, gemini, ".gemini/agents/autopus/")
	assert.NotContains(t, gemini, ".claude/")
}

func TestReplaceToolReferences_TodoWriteReplaced(t *testing.T) {
	t.Parallel()

	body := `Use TodoWrite to track progress.`

	codex := content.ReplaceToolReferences(body, "codex")
	assert.Contains(t, codex, "// TodoWrite is not available on this platform")

	gemini := content.ReplaceToolReferences(body, "gemini")
	assert.Contains(t, gemini, "// TodoWrite is not available on this platform")
}

func TestReplaceToolReferences_WorktreeIsolation(t *testing.T) {
	t.Parallel()

	body := `Set isolation: "worktree" for parallel execution.`

	codex := content.ReplaceToolReferences(body, "codex")
	assert.Contains(t, codex, "auto pipeline worktree")
	assert.NotContains(t, codex, `isolation: "worktree"`)
}

func TestReplaceToolReferences_ClaudePassthrough(t *testing.T) {
	t.Parallel()

	body := `Use Agent(subagent_type="tester", task="run") and .claude/rules/.`

	result := content.ReplaceToolReferences(body, "claude")
	assert.Equal(t, body, result, "claude platform should pass through unchanged")

	result2 := content.ReplaceToolReferences(body, "claude-code")
	assert.Equal(t, body, result2, "claude-code platform should pass through unchanged")
}

func TestMapModel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "gpt-6-astra", content.MapModel("fable", "codex"))
	assert.Equal(t, "gpt-6-sol", content.MapModel("opus", "codex"))
	assert.Equal(t, "gpt-5.6-terra", content.MapModel("sonnet", "codex"))
	assert.Equal(t, "gpt-6-luna", content.MapModel("haiku", "codex"))
	assert.Equal(t, "gemini-3.1-pro", content.MapModel("fable", "gemini"))
	assert.Equal(t, "gemini-3.1-pro", content.MapModel("opus", "gemini"))
	assert.Equal(t, "gemini-3.1-pro", content.MapModel("sonnet", "gemini"))
	assert.Equal(t, "gemini-3.8-flash", content.MapModel("haiku", "gemini"))

	// Unknown model returns as-is
	assert.Equal(t, "unknown-model", content.MapModel("unknown-model", "codex"))
	// Unknown platform returns as-is
	assert.Equal(t, "sonnet", content.MapModel("sonnet", "unknown-platform"))
}

func TestMapEffort(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "max", content.MapEffort("max", "codex"))
	assert.Equal(t, "xhigh", content.MapEffort("xhigh", "codex"))
	assert.Equal(t, "high", content.MapEffort("high", "codex"))
	assert.Equal(t, "medium", content.MapEffort("medium", "codex"))
	assert.Equal(t, "low", content.MapEffort("low", "codex"))
	assert.Equal(t, "medium", content.MapEffort("", "codex"))

	// Unknown effort/platform returns as-is.
	assert.Equal(t, "extreme", content.MapEffort("extreme", "codex"))
	assert.Equal(t, "max", content.MapEffort("max", "unknown-platform"))
}
