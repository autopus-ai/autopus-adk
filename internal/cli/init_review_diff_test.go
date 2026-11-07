package cli

// SPEC-TUIUX-001 T9 (Init Review Contract, diff rule): the review's changed
// settings are the YAML leaves base and current differ in, grouped by parent.

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestInitReviewChanges_LeavesGroupedByParent(t *testing.T) {
	base := config.DefaultFullConfig("p")
	current, err := cloneHarnessConfig(base)
	require.NoError(t, err)
	none, err := initReviewChanges(base, current, "(없음)")
	require.NoError(t, err)
	assert.Empty(t, none, "an unchanged config has no lines")

	current.UsageProfile = config.ProfileFullstack
	current.Spec.ReviewGate.Enabled = !base.Spec.ReviewGate.Enabled
	current.Spec.ReviewGate.Providers = []string{"claude", "codex"}
	current.Quality.Presets["claude-custom"] = config.QualityPreset{
		Description: "line\x1b[31m", Agents: map[string]string{"reviewer": "opus", "ux-validator": "opus"},
	}
	ultra := current.Quality.Presets["ultra"]
	ultra.Agents = maps.Clone(ultra.Agents)
	delete(ultra.Agents, "reviewer")
	current.Quality.Presets["ultra"] = ultra

	lines, err := initReviewChanges(base, current, "(없음)")
	require.NoError(t, err)

	assert.Equal(t, []string{
		"  quality.presets.claude-custom: description=line\\x1b[31m",
		"  quality.presets.claude-custom.agents: reviewer=opus, ux-validator=opus",
		"  quality.presets.ultra.agents: reviewer=(없음)",
		"  spec.review_gate: enabled=" + map[bool]string{true: "true", false: "false"}[current.Spec.ReviewGate.Enabled] +
			", providers=[claude, codex]",
		"  usage_profile: fullstack",
	}, lines)
}
