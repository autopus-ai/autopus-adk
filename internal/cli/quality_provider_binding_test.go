package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestClaudeProviderQualityFeedsRouteTeamBinding(t *testing.T) {
	t.Parallel()

	quality := config.DefaultFullConfig("claude-binding").Quality
	quality.Default = "ultra"
	quality.Providers = map[string]string{
		config.QualityProviderClaude: "balanced",
		config.QualityProviderCodex:  "ultra",
	}

	persisted := resolveTeamQualityBinding(
		quality.EffectiveMode(config.QualityProviderClaude),
		"",
	)
	assert.Equal(t, "claude-fable-5-1", persisted.Phases["planning"].Model)
	assert.Equal(t, "max", persisted.Phases["planning"].Effort)
	assert.Equal(t, "claude-sonnet-5-5", persisted.Phases["implementation"].Model)
	assert.Equal(t, "max", persisted.Phases["implementation"].Effort)

	explicitGlobal := quality.WithGlobalOverride("ultra")
	overrideBinding := resolveTeamQualityBinding(
		explicitGlobal.EffectiveMode(config.QualityProviderClaude),
		"",
	)
	assert.Len(t, overrideBinding.Phases, 6)
	assert.Equal(t, "claude-fable-5-1", overrideBinding.Phases["planning"].Model)
	assert.Equal(t, "max", overrideBinding.Phases["planning"].Effort)
	assert.Equal(t, "claude-opus-5-5", overrideBinding.Phases["implementation"].Model)
	assert.Equal(t, "max", overrideBinding.Phases["implementation"].Effort)
}
