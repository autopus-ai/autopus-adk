package content_test

import (
	"encoding/json"
	"strings"
	"testing"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// The on-demand receipt schema remains usable without embedding tool tutorials.
func TestReplacePlatformReferencesOMP_CoordinationResourceCarriesReceiptSchema(t *testing.T) {
	t.Parallel()
	raw, err := contentfs.FS.ReadFile("skills/references/agent-pipeline/coordination.md")
	require.NoError(t, err)
	out := content.ReplacePlatformReferences(string(raw), "omp")
	_, rest, found := strings.Cut(out, "```json\n")
	require.True(t, found)
	block, _, closed := strings.Cut(rest, "\n```")
	require.True(t, closed)
	var schema struct {
		Type                 string   `json:"type"`
		AdditionalProperties *bool    `json:"additionalProperties"`
		Required             []string `json:"required"`
		Properties           map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal([]byte(block), &schema))
	require.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.AdditionalProperties)
	assert.False(t, *schema.AdditionalProperties)
	assert.ElementsMatch(t, []string{"owned_paths", "changed_files", "verification", "blockers", "next_required_step"}, schema.Required)
	require.Len(t, schema.Properties, len(schema.Required))
	for _, field := range schema.Required {
		kind := "array"
		if field == "next_required_step" {
			kind = "string"
		}
		assert.Equal(t, kind, schema.Properties[field].Type)
	}
}
