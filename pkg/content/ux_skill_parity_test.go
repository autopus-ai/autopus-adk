package content_test

import (
	"testing"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUXSkillIntelligencePassTransformsForAllSupportedPlatforms(t *testing.T) {
	t.Parallel()

	transformer, err := content.NewSkillTransformerFromFS(contentfs.FS, "skills")
	require.NoError(t, err)

	platforms := []string{"claude", "codex", "gemini", "opencode"}
	for _, platform := range platforms {
		platform := platform
		t.Run(platform, func(t *testing.T) {
			t.Parallel()

			skills, _, err := transformer.TransformForPlatform(platform)
			require.NoError(t, err)

			frontendSkill := findTransformedSkill(t, skills, "frontend-skill")
			assert.Contains(t, frontendSkill.Content, "## UX Intelligence Pass")
			assert.Contains(t, frontendSkill.Content, "Design Discovery Matrix")
			assert.Contains(t, frontendSkill.Content, "Pre-delivery checklist")

			polishSkill := findTransformedSkill(t, skills, "make-interfaces-feel-better")
			assert.Contains(t, polishSkill.Content, "## Detail Pass")
			assert.Contains(t, polishSkill.Content, "Nested rounded surfaces")
			assert.Contains(t, polishSkill.Content, "Do not use `transition: all`")
			assert.Contains(t, polishSkill.Content, "384562064fcdd99778fcbafd8729626fe6aab02f")

			verifySkill := findTransformedSkill(t, skills, "frontend-verify")
			assert.Contains(t, verifySkill.Content, "Phase 0.4: 디자인 소스 팩 수집")
			assert.Contains(t, verifySkill.Content, "Phase 0.6: UX 인텔리전스 기준 합성")
			assert.Contains(t, verifySkill.Content, "## UX Intelligence")
			assert.Contains(t, verifySkill.Content, "auto design pack --format markdown")
			assert.Contains(t, verifySkill.Content, "auto design figma fetch --format markdown")
			assert.Contains(t, verifySkill.Content, "--visual-gate")
			assert.Contains(t, verifySkill.Content, "--strict-visual-gate")
		})
	}
}

func TestMakeInterfacesFeelBetterCatalogMetadata(t *testing.T) {
	t.Parallel()

	catalog, err := content.LoadSkillCatalogFromFS(contentfs.FS, "skills")
	require.NoError(t, err)

	skill, ok := catalog.Get("make-interfaces-feel-better")
	require.True(t, ok)
	assert.Equal(t, "methodology", skill.Category)
	assert.NotContains(t, skill.Bundles, "core",
		"UI polish is opt-in guidance; claiming the core bundle would put it on every default surface")
	assert.Contains(t, skill.Bundles, "frontend")
	assert.Contains(t, skill.CompileTargets, "codex")
	assert.Contains(t, skill.CompileTargets, "opencode")
	assert.Equal(t, "shared", skill.Visibility)
}

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
