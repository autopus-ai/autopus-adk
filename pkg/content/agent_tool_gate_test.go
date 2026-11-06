package content_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/content"
)

func findTransformedSkill(t *testing.T, skills []content.TransformedSkill, name string) content.TransformedSkill {
	t.Helper()

	for _, skill := range skills {
		if skill.Name == name {
			return skill
		}
	}
	require.Failf(t, "missing transformed skill", "skill %q was not generated", name)
	return content.TransformedSkill{}
}

// The other agents keep their MCP tools exactly as before: only the REA
// server is platform-gated.
func TestREAToolGate_LeavesOtherMCPToolsUntouched(t *testing.T) {
	t.Parallel()
	source := content.AgentSource{Meta: content.AgentSourceMeta{
		Name: "mcp-fixture", Tools: "Read, mcp__context7, mcp__reader, mcp__rea__inspect",
	}}
	rendered, err := content.RenderOMPAgentDefinition(source, content.OMPAgentDefinitionOptions{})
	require.NoError(t, err)
	assert.Equal(t, []any{"mcp__context7", "mcp__reader", "read"}, ompDefinitionFrontmatter(t, rendered)["tools"])
	openCode := content.TransformAgentForOpenCode(source)
	assert.Contains(t, openCode, `"mcp__reader": allow`)
	assert.NotContains(t, openCode, "mcp__rea__inspect")
	frontmatter, _ := decodeGeminiAgent(t, content.TransformAgentForGemini(source))
	assert.Equal(t, []string{"read_file"}, frontmatter.Tools)
	assert.True(t, frontmatter.InheritMCP, "the REA grant opts into the session's MCP servers")
	other := content.AgentSource{Meta: content.AgentSourceMeta{Name: "mcp-other", Tools: "Read, mcp__context7, mcp__reader"}}
	frontmatter, _ = decodeGeminiAgent(t, content.TransformAgentForGemini(other))
	assert.False(t, frontmatter.InheritMCP, "mcp__reader is not the REA server")
}
