package antigravity

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_AutoIdeaUsesCanonicalBrainstormDebateContract(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := NewWithRoot(root).Generate(
		context.Background(), config.DefaultFullConfig("auto-idea-contract"),
	)
	require.NoError(t, err)

	for _, rel := range []string{
		filepath.Join(".gemini", "skills", "autopus", "auto-idea", "SKILL.md"),
		filepath.Join(antigravityPluginDir, "skills", "auto-idea", "SKILL.md"),
	} {
		rendered := readGeneratedAntigravitySurface(t, root, rel)
		for _, token := range []string{
			"orchestration-contract.v1",
			`auto orchestra brainstorm "{structured idea}"`,
			"--strategy debate",
			"--providers {providers}",
			"--rounds 2",
			"--judge {invoking_provider}",
			"--no-detach",
			"--format json",
			"resolved configured debate",
			"모든 provider",
			"Round 1",
			"Round 2",
			`"preserve_dissent": true`,
			"dissent appendix",
			"fresh isolated judge session evidence",
			"--subprocess",
			"orchestra_unavailable",
		} {
			assert.Contains(t, rendered, token, "%s must contain %q", rel, token)
		}
		assert.NotContains(t, rendered, `auto orchestra run "{structured idea}"`)
		assert.NotContains(t, rendered, "--rounds standard")
	}
}
