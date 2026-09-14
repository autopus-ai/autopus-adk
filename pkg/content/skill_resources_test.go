package content_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// TestRenderSkillResources_PipelineResourcesAreReadable proves the compacted
// agent-pipeline skill can still reach its split-out bodies: every reference
// the skill links to comes back as a real, non-empty file keyed by the same
// relative path the skill body uses.
func TestRenderSkillResources_PipelineResourcesAreReadable(t *testing.T) {
	t.Parallel()

	cfg := &config.HarnessConfig{Platforms: []string{"claude"}}
	resources, err := content.RenderSkillResources("agent-pipeline", "claude", cfg)
	require.NoError(t, err)
	require.NotEmpty(t, resources, "agent-pipeline must ship at least one reference body")

	for rel, data := range resources {
		assert.True(t, strings.HasPrefix(rel, "references/"),
			"resource key %q must be relative to the skill directory", rel)
		assert.True(t, strings.HasSuffix(rel, ".md"), "resource key %q must be a markdown file", rel)
		assert.NotContains(t, rel, "..", "resource key %q must not escape the skill directory", rel)
		assert.NotContains(t, rel, "\\", "resource key %q must use slash separators", rel)
		assert.Equal(t, 2, len(strings.Split(rel, "/")),
			"resource key %q must be exactly references/<file>.md", rel)
		assert.NotEmpty(t, strings.TrimSpace(string(data)), "resource %q must not be empty", rel)
	}
}

// TestRenderSkillResources_TransformsForNativePlatform confirms resources go
// through the same platform rewriter as the skill body, so a Codex install
// never reads Claude-only paths out of a reference file.
func TestRenderSkillResources_TransformsForNativePlatform(t *testing.T) {
	t.Parallel()

	cfg := &config.HarnessConfig{Platforms: []string{"codex"}}
	resources, err := content.RenderSkillResources("agent-pipeline", "codex", cfg)
	require.NoError(t, err)
	require.NotEmpty(t, resources)

	for rel, data := range resources {
		assert.NotContains(t, string(data), ".claude/",
			"resource %q still carries a Claude-native path after codex transformation", rel)
	}
}

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
