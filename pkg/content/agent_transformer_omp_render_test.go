package content_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/content"
)

// ompAgentSource returns a shipped agent source or, for a role the
// verification harness retired, its frozen last-shipped copy under
// testdata/retired-agents. The renderers still serve any agent source, so the
// planning and implementation roles keep exercising their tool mappings.
func ompAgentSource(t *testing.T, name string) content.AgentSource {
	t.Helper()
	for _, root := range []struct {
		fsys fs.FS
		dir  string
	}{{contentfs.FS, "agents"}, {os.DirFS("testdata"), "retired-agents"}} {
		sources, err := content.LoadAgentSourcesFromFS(root.fsys, root.dir)
		require.NoError(t, err)
		for _, source := range sources {
			if source.Meta.Name == name {
				return source
			}
		}
	}
	t.Fatalf("agent source %s missing", name)
	return content.AgentSource{}
}

func ompDefinitionFrontmatter(t *testing.T, rendered string) map[string]any {
	t.Helper()
	parts := strings.SplitN(rendered, "---\n", 3)
	require.Len(t, parts, 3, "definition must open with a frontmatter block")
	require.Empty(t, parts[0])
	fields := map[string]any{}
	require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &fields))
	return fields
}

// OMP Agent File Contract: the canonical name and description, the portable
// tool map plus the role's native tools, sorted, and no model or spawns.
func TestRenderOMPAgentDefinition_FrontmatterFollowsTheContract(t *testing.T) {
	t.Parallel()
	planner := ompAgentSource(t, "planner")
	rendered, err := content.RenderOMPAgentDefinition(planner, content.OMPAgentDefinitionOptions{ThinkingLevel: "max"})
	require.NoError(t, err)
	fields := ompDefinitionFrontmatter(t, rendered)
	assert.Equal(t, "planner", fields["name"])
	assert.Equal(t, planner.Meta.Description, fields["description"])
	assert.Equal(t, "max", fields["thinkingLevel"])
	assert.Equal(t, []any{
		"bash", "glob", "grep", "mcp__sequential-thinking__sequentialthinking", "read", "web_search",
	}, fields["tools"], "WebSearch and WebFetch fold onto one web_search")
	for _, absent := range []string{"model", "spawns", "output", "effort", "permissionMode", "skills"} {
		assert.NotContains(t, fields, absent)
	}

	executor := ompAgentSource(t, "executor")
	rendered, err = content.RenderOMPAgentDefinition(executor, content.OMPAgentDefinitionOptions{ThinkingLevel: "medium"})
	require.NoError(t, err)
	fields = ompDefinitionFrontmatter(t, rendered)
	assert.Equal(t, "medium", fields["thinkingLevel"])
	assert.Equal(t, []any{"bash", "edit", "glob", "grep", "lsp", "read", "write"}, fields["tools"],
		"TodoWrite is dropped and the implementation role gains lsp")

	frontend := ompAgentSource(t, "frontend-specialist")
	rendered, err = content.RenderOMPAgentDefinition(frontend, content.OMPAgentDefinitionOptions{})
	require.NoError(t, err)
	assert.Equal(t, []any{"bash", "browser", "edit", "eval", "glob", "grep", "inspect_image", "lsp", "read", "write"},
		ompDefinitionFrontmatter(t, rendered)["tools"],
		"omp/17.2.7 browser tools plus the eval prelude OMP 18 drives the browser from")
}

// An agent with a route gets its thinking from the override, so its file must
// not declare one that could compete with it.
func TestRenderOMPAgentDefinition_OmitsThinkingWithoutALevel(t *testing.T) {
	t.Parallel()
	rendered, err := content.RenderOMPAgentDefinition(ompAgentSource(t, "planner"), content.OMPAgentDefinitionOptions{})
	require.NoError(t, err)
	assert.NotContains(t, ompDefinitionFrontmatter(t, rendered), "thinkingLevel")
	assert.NotContains(t, rendered, "thinkingLevel")
}

// The reviewer keeps the bundled reviewer's verdict schema; nobody else
// declares an output.
func TestRenderOMPAgentDefinition_OnlyTheReviewerDeclaresTheVerdictSchema(t *testing.T) {
	t.Parallel()
	rendered, err := content.RenderOMPAgentDefinition(ompAgentSource(t, "reviewer"), content.OMPAgentDefinitionOptions{})
	require.NoError(t, err)
	output, ok := ompDefinitionFrontmatter(t, rendered)["output"].(map[string]any)
	require.True(t, ok, "reviewer must declare an output mapping")
	properties := output["properties"].(map[string]any)
	assert.Equal(t, []any{"correct", "incorrect"}, properties["overall_correctness"].(map[string]any)["enum"])
	assert.Equal(t, "string", properties["explanation"].(map[string]any)["type"])
	assert.Equal(t, "number", properties["confidence"].(map[string]any)["type"])
	findings := output["optionalProperties"].(map[string]any)["findings"].(map[string]any)
	elements := findings["elements"].(map[string]any)["properties"].(map[string]any)
	for _, field := range []string{"title", "body", "priority", "confidence", "file_path", "line_start", "line_end"} {
		assert.Contains(t, elements, field)
	}

	rendered, err = content.RenderOMPAgentDefinition(ompAgentSource(t, "validator"), content.OMPAgentDefinitionOptions{})
	require.NoError(t, err)
	assert.NotContains(t, ompDefinitionFrontmatter(t, rendered), "output")
}

// The schema alone does not make the reviewer report a verdict: on omp 18.12.3
// the task finalizes from the agent's text, and the canonical body asks for
// TRUST 5 only (S23). The body therefore names the three verdict fields, and
// only the reviewer's body does.
func TestRenderOMPAgentDefinition_TheReviewerBodyAsksForTheVerdictFields(t *testing.T) {
	t.Parallel()
	rendered, err := content.RenderOMPAgentDefinition(ompAgentSource(t, "reviewer"), content.OMPAgentDefinitionOptions{})
	require.NoError(t, err)
	_, body, found := strings.Cut(strings.TrimPrefix(rendered, "---\n"), "\n---\n")
	require.True(t, found)
	for _, field := range []string{"`overall_correctness`", "`explanation`", "`confidence`", "`yield`"} {
		assert.Contains(t, body, field)
	}

	rendered, err = content.RenderOMPAgentDefinition(ompAgentSource(t, "validator"), content.OMPAgentDefinitionOptions{})
	require.NoError(t, err)
	assert.NotContains(t, rendered, "overall_correctness")
}

// Scalars cannot close the frontmatter early, and an unknown thinking level is
// refused instead of being written for OMP to reject at load.
func TestRenderOMPAgentDefinition_EncodesScalarsAndRefusesUnknownThinking(t *testing.T) {
	t.Parallel()
	source := ompAgentSource(t, "planner")
	source.Meta.Description = "plans: the work\ntools: [write]"
	rendered, err := content.RenderOMPAgentDefinition(source, content.OMPAgentDefinitionOptions{})
	require.NoError(t, err)
	fields := ompDefinitionFrontmatter(t, rendered)
	assert.Equal(t, source.Meta.Description, fields["description"])
	assert.NotContains(t, fields["tools"], "write")

	_, err = content.RenderOMPAgentDefinition(source, content.OMPAgentDefinitionOptions{ThinkingLevel: "turbo"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "turbo")
}

// Security review I1: thinkingLevel goes through the YAML scalar encoder like
// every other frontmatter value, so a level a YAML 1.1 reader takes for a
// boolean (`off`) stays a string, and ordinary levels keep their bytes.
func TestRenderOMPAgentDefinition_EncodesThinkingLevelAsAScalar(t *testing.T) {
	t.Parallel()
	planner := ompAgentSource(t, "planner")
	for level, line := range map[string]string{"off": `thinkingLevel: "off"`, "max": "thinkingLevel: max"} {
		rendered, err := content.RenderOMPAgentDefinition(planner, content.OMPAgentDefinitionOptions{ThinkingLevel: level})

		require.NoError(t, err)
		assert.Contains(t, rendered, "\n"+line+"\n")
		assert.Equal(t, level, ompDefinitionFrontmatter(t, rendered)["thinkingLevel"])
	}
}
