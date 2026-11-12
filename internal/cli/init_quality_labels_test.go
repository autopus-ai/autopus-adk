package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The quality step names what each preset sets: the tier of every generated
// agent, nothing it does not set.
func TestInitQualityLabels_NameEveryAgentTier(t *testing.T) {
	t.Parallel()
	labels := initQualityLabels(config.DefaultFullConfig("demo").Quality)
	assert.Equal(t, map[string]string{
		"ultra":    "Ultra — reviewer, qa-explorer: fable · frontend-specialist, ux-validator: opus",
		"balanced": "Balanced — reviewer, qa-explorer: opus · frontend-specialist, ux-validator: sonnet",
	}, labels)
}

// An edited preset is shown as edited: the line reads the config, not a
// fixed description.
func TestInitQualityLabels_FollowAnEditedPreset(t *testing.T) {
	t.Parallel()
	quality := config.DefaultFullConfig("demo").Quality
	quality.Presets["balanced"].Agents["ux-validator"] = "haiku"
	assert.Equal(t,
		"Balanced — reviewer, qa-explorer: opus · frontend-specialist: sonnet · ux-validator: haiku",
		initQualityLabels(quality)["balanced"])
}
