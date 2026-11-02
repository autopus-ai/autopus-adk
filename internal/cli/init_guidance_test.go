package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S10: init leads with standalone use in every configured platform's syntax
// and names Autopus Desktop last, as optional.
func TestInitGuidance_StandaloneFirst(t *testing.T) {
	t.Parallel()
	steps := initNextSteps([]string{"claude-code", "codex"})
	require.Len(t, steps, 3)
	assert.Equal(t, `Next: plan new work with /auto plan "<outcome>" (claude-code) · $codex-auto-plan "<outcome>" (codex)`, steps[0])
	assert.Equal(t, `Next: fix something small without a SPEC with /auto fix "<problem>" (claude-code) · `+
		`$codex-auto-fix "<problem>" (codex)`, steps[1])
	assert.True(t, strings.HasPrefix(steps[2], "Optional: Autopus Desktop"), steps[2])
	assert.Contains(t, steps[2], "the ADK works without it")
	for _, step := range steps[:2] {
		assert.NotContains(t, step, "Desktop")
	}

	assert.Equal(t, `Next: plan new work with /auto plan "<outcome>"`, initNextSteps(nil)[0])
	assert.Equal(t, `Next: plan new work with /auto plan "<outcome>" (claude-code, antigravity-cli, opencode, omp)`,
		initNextSteps([]string{"claude-code", "antigravity-cli", "opencode", "omp"})[0])
}
