package templates_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexQualityGuidanceDocumentsLoadedAgentBoundary(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	adaptivePaths := []string{
		filepath.Join(root, "..", "content", "skills", "adaptive-quality.md"),
		filepath.Join(root, "codex", "skills", "adaptive-quality.md.tmpl"),
		filepath.Join(root, "gemini", "skills", "adaptive-quality", "SKILL.md.tmpl"),
	}
	for _, path := range adaptivePaths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "auto quality <mode> --apply")
		assert.Contains(t, content, "auto quality supervisor inherit --apply")
		assert.Contains(t, content, "User Codex runtime default")
		assert.Contains(t, content, "quality-managed supervisor")
		assert.Contains(t, content, "user-owned root model or effort assignments remain preserved and take precedence")
		assert.Contains(t, content, "new Codex session")
		assert.Contains(t, content, "cannot hot-swap agents already loaded")
		assert.Contains(t, content, "| Ultra | Fable-tier worker | Astra + `max` |")
		assert.Contains(t, content, "| Ultra | Opus-tier worker | Sol + `xhigh` |")
		assert.NotContains(t, content, "| Ultra | managed worker | Sol + `max` |")
		assert.NotContains(t, content, "Sol/`max` for `planner`, `architect`, and `security-auditor`")
	}

	// The Codex provider/model tier table used to be restated inside the shared
	// pipeline body. It is platform-specific projection, so it now lives only on
	// the Codex quality surface checked above; the shared pipeline must not
	// carry a provider tier mapping at all.
	pipelinePaths := []string{
		filepath.Join(root, "..", "content", "skills", "agent-pipeline.md"),
		filepath.Join(root, "gemini", "skills", "agent-pipeline", "SKILL.md.tmpl"),
	}
	for _, path := range pipelinePaths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		content := string(data)
		for _, providerTier := range []string{
			"Astra", "Sol", "Fable-tier", "Opus-tier",
			"`planner`, `architect`, and `security-auditor` use Sol/`max`",
			"the depth-0 supervisor and orchestra use Sol/`ultra`",
		} {
			assert.NotContains(t, content, providerTier,
				"%s must not pin a provider tier; quality projection is platform-owned", path)
		}
	}
}
