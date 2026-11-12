package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShippedPresetDescriptionsNameEveryAgentTier(t *testing.T) {
	t.Parallel()
	presets := DefaultFullConfig("demo").Quality.Presets
	// Each description must match the tiers the preset gives its agents.
	assert.Equal(t, "리뷰·QA 탐색은 Fable, 프론트엔드·UX 검증은 Opus.", presets["ultra"].Description)
	assert.Equal(t, map[string]string{
		"reviewer": "fable", "qa-explorer": "fable", "frontend-specialist": "opus", "ux-validator": "opus",
	}, presets["ultra"].Agents)
	assert.Equal(t, "리뷰·QA 탐색은 Opus, 프론트엔드·UX 검증은 Sonnet.", presets["balanced"].Description)
	assert.Equal(t, map[string]string{
		"reviewer": "opus", "qa-explorer": "opus", "frontend-specialist": "sonnet", "ux-validator": "sonnet",
	}, presets["balanced"].Agents)
}
