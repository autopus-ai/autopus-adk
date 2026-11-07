package content_test

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/stretchr/testify/assert"
)

// TestReplacePlatformReferencesOMP_S12_EmittedBodies sweeps the real rule and
// skill sources so no Claude-native path, stage-1-only skill path, or doubled
// rule namespace reaches an omp surface.
func TestReplacePlatformReferencesOMP_S12_EmittedBodies(t *testing.T) {
	t.Parallel()

	bodies := ompContentBodies(t, "rules")
	for name, body := range ompContentBodies(t, "skills") {
		bodies[name] = body
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			out := content.ReplacePlatformReferences(body, "omp")

			for _, token := range ompLegacyCoordinationTokens {
				assert.NotContains(t, out, token, "%s retained legacy coordination token %q", name, token)
			}
			sweepOut := stripOMPTestRootGlobInventory(out)
			for _, token := range ompForeignSurfaceTokens {
				assert.NotContains(t, sweepOut, token, "%s retained foreign surface token %q", name, token)
			}
			assert.Empty(t, ompFlatSkillPathRe.FindAllString(out, -1),
				"%s must reference .agents/skills/<name>/SKILL.md", name)
			assert.Empty(t, ompDoubledRuleNamespaceRe.FindAllString(out, -1),
				"%s must reference .omp/rules/autopus-<name>.md", name)
			assert.NotContains(t, out, `isolation: "worktree"`)
			assert.NotContains(t, out, `isolation = "worktree"`)
		})
	}
}
