package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// S10: init leads with standalone use in every configured platform's syntax,
// names Autopus Desktop last and optional, and says for each step where it
// runs, its provider, and what it calls and writes.
func TestInitGuidance_StandaloneFirst(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("p")
	cfg.Platforms = []string{"claude-code", "codex"}
	cfg.Spec.ReviewGate.Enabled, cfg.Spec.ReviewGate.Providers = true, []string{"claude", "codex"}

	steps := initNextSteps(cfg, dir)
	require.Len(t, steps, 3)
	assert.Equal(t, `Next: plan new work as a SPEC before implementing it — /auto plan "<outcome>" (claude-code) · `+
		`$codex-auto-plan "<outcome>" (codex)`, steps[0].headline)
	assert.Equal(t, "in "+dir+" · runs in agent_session · provider: the agent session's model · "+
		"calls: model calls of the agent session and, for the SPEC review, claude, codex · "+
		"writes: .autopus/specs (a new SPEC)", steps[0].facts)
	assert.Equal(t, `Next: fix something small without a SPEC: a reproduction test, a focused patch, and its `+
		`verification — /auto fix "<problem>" (claude-code) · $codex-auto-fix "<problem>" (codex)`, steps[1].headline)
	assert.Equal(t, "in "+dir+" · runs in agent_session · provider: the agent session's model · "+
		"calls: model calls of the agent session · writes: the working tree", steps[1].facts)
	assert.Equal(t, "Optional: connect this machine to the Autopus server through Autopus Desktop; the ADK works "+
		"without it — auto connect", steps[2].headline)
	assert.Equal(t, "optional · in "+dir+" · runs in terminal · provider: none · calls: Autopus server sign-in, "+
		"workspace selection, and OpenAI OAuth · writes: the local Autopus connection config and credentials", steps[2].facts)
	for _, step := range steps[:2] {
		assert.NotContains(t, step.headline+step.facts, "Desktop")
	}

	cfg.Platforms = nil
	assert.True(t, strings.HasSuffix(initNextSteps(cfg, dir)[0].headline, `— /auto plan "<outcome>"`))
}
