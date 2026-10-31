package templates_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

// The lean profile only shortens planning when the plan surface routes a
// low-risk new SPEC to it and the spec-writer knows the reduced floor. A
// platform that misses either keeps writing the full set, or writes a lean set
// without the frontmatter `auto spec validate` and `auto spec review` read.
func TestLeanProfileSurfaceParity(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("lean-project")
	tokens := []string{"profile: lean", "change_class: <class>"}

	paths := map[string]string{
		"spec-writer-content":      filepath.Join(root, "..", "content", "agents", "spec-writer.md"),
		"codex-spec-writer-agent":  filepath.Join(root, "codex", "agents", "spec-writer.toml.tmpl"),
		"gemini-spec-writer-agent": filepath.Join(root, "gemini", "agents", "spec-writer.md.tmpl"),
		"claude-workflows":         filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"),
		"codex-plan-skill":         filepath.Join(root, "codex", "skills", "auto-plan.md.tmpl"),
		"codex-plan-prompt":        filepath.Join(root, "codex", "prompts", "auto-plan.md.tmpl"),
		"gemini-plan-skill":        filepath.Join(root, "gemini", "skills", "auto-plan", "SKILL.md.tmpl"),
	}
	for name, path := range paths {
		name, path := name, path
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			text, err := semanticContractSurface(e, path, cfg)
			require.NoError(t, err)
			for _, token := range tokens {
				assert.Contains(t, text, token, "%s should contain %q", path, token)
			}
		})
	}
}
