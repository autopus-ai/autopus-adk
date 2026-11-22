package omp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The extension's kill timer outlasts hooks.verify_stop_budget the way the
// Claude Code hook timeout does; the default leaves the file unchanged.
func TestPrepareStopGateMappings_TimeoutFollowsTheVerifyStopBudget(t *testing.T) {
	t.Parallel()
	for budget, want := range map[string]string{
		"":    ompStopGateTimeoutLine,
		"15m": "const TIMEOUT_MS = 990 * 1000",
	} {
		cfg := config.DefaultFullConfig("demo")
		cfg.Hooks.VerifyStopBudget = budget
		files, err := prepareStopGateMappings(cfg)
		require.NoError(t, err)
		require.Len(t, files, 1)
		assert.Contains(t, string(files[0].Content), want, budget)
	}
}
