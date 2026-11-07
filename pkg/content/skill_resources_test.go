package content_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// TestRenderSkillResources_UnknownSkillIsNotAnError keeps the helper callable
// from every emitter loop without a per-skill existence check.
func TestRenderSkillResources_UnknownSkillIsNotAnError(t *testing.T) {
	t.Parallel()

	cfg := &config.HarnessConfig{Platforms: []string{"claude"}}
	resources, err := content.RenderSkillResources("no-such-skill", "claude", cfg)
	require.NoError(t, err)
	assert.Empty(t, resources)
}

// TestRenderSkillResources_RejectsUnknownPlatform stops a typo from silently
// installing untransformed Claude text onto a foreign surface.
func TestRenderSkillResources_RejectsUnknownPlatform(t *testing.T) {
	t.Parallel()

	_, err := content.RenderSkillResources("agent-pipeline", "not-a-platform", nil)
	require.Error(t, err)
}

// TestRenderSkillResources_ResourcesAreNotSkills guards the boundary that keeps
// compaction honest: reference bodies must never be registered as catalog
// skills, or the surface they were split out of would grow right back.
func TestRenderSkillResources_ResourcesAreNotSkills(t *testing.T) {
	t.Parallel()

	catalog, err := content.LoadSkillCatalogFromFS(contentfs.FS, "skills")
	require.NoError(t, err)

	for _, skill := range catalog.List() {
		assert.NotContains(t, skill.Name, "/", "catalog skill %q looks like a nested resource path", skill.Name)
		assert.NotEqual(t, "references", skill.Name, "resource directory leaked into the skill catalog")
	}
}
