package content_test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformAgentForGemini_ProjectsKnownNativeToolsAndOmitsUnsupported(t *testing.T) {
	t.Parallel()

	source := content.AgentSource{
		Meta: content.AgentSourceMeta{
			Name:        "tool-projection",
			Description: "Gemini tool projection fixture",
			Tools:       "Read, Write, Edit, Grep, Glob, Bash, TodoWrite, Agent, MysteryExtension",
			Skills:      []string{"tdd", "verification"},
		},
		Body: "## Contract\n\nCodex guidance remains advisory.\n\nBody stays byte-stable.",
	}

	rendered := content.TransformAgentForGemini(source)
	frontmatter, body := decodeGeminiAgent(t, rendered)

	assert.Equal(t, []string{
		"read_file",
		"write_file",
		"replace",
		"grep_search",
		"glob",
		"run_shell_command",
	}, frontmatter.Tools)
	assert.Equal(t, source.Meta.Skills, frontmatter.Skills)
	assert.Equal(t, strings.TrimSpace(source.Body), strings.TrimSpace(body))
	assert.NotContains(t, rendered, "TodoWrite")
	assert.NotContains(t, rendered, "MysteryExtension")
	assert.NotContains(t, rendered, "\n  - Agent\n")
	assert.NotContains(t, rendered, "Codex native enforcement")
}

// Shipped prompt content is injected verbatim into every provider session, so a
// credential or a developer's absolute home path baked into it leaks on every
// run. The narrative "Context Evolution Examples" section this check used to be
// scoped to no longer exists; the leak sweep is the part that defended
// behaviour, so it now covers every shipped skill body and resource.
func TestShippedSkillContent_CarriesNoCredentialsOrHostPaths(t *testing.T) {
	t.Parallel()

	forbidden := []string{"sk-proj-", "AKIA", "/Users/", "/home/", "C:\\"}
	for _, dir := range []string{"skills", "skills/references/agent-pipeline"} {
		entries, err := contentfs.FS.ReadDir(dir)
		require.NoError(t, err)
		require.NotEmpty(t, entries)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := dir + "/" + entry.Name()
			t.Run(path, func(t *testing.T) {
				t.Parallel()
				data, readErr := contentfs.FS.ReadFile(path)
				require.NoError(t, readErr)
				for _, token := range forbidden {
					assert.NotContains(t, string(data), token,
						"%s leaks sensitive data into every session", path)
				}
			})
		}
	}
}

type geminiAgentFrontmatter struct {
	Name   string   `yaml:"name"`
	Skills []string `yaml:"skills"`
	Tools  []string `yaml:"tools"`
}

func decodeGeminiAgent(t *testing.T, rendered string) (geminiAgentFrontmatter, string) {
	t.Helper()
	parts := strings.SplitN(rendered, "---", 3)
	require.Len(t, parts, 3)
	var frontmatter geminiAgentFrontmatter
	require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &frontmatter))
	return frontmatter, parts[2]
}
