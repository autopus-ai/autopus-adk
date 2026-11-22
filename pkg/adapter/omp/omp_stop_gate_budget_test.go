package omp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// With hooks.verify_stop_budget set, the extension passes --budget to the
// gate and its kill timer outlasts it, so the two always move together; the
// default leaves the file unchanged.
func TestPrepareStopGateMappings_BudgetAndTimeoutFollowTheConfig(t *testing.T) {
	t.Parallel()
	for budget, want := range map[string][]string{
		"":    {ompStopGateTimeoutLine, ompStopGateSpawnArgs},
		"15m": {"const TIMEOUT_MS = 990 * 1000", `["--budget", "15m0s"]`},
		"5m":  {ompStopGateTimeoutLine, `["--budget", "5m0s"]`},
	} {
		cfg := config.DefaultFullConfig("demo")
		cfg.Hooks.VerifyStopBudget = budget
		files, err := prepareStopGateMappings(cfg)
		require.NoError(t, err)
		require.Len(t, files, 1)
		for _, w := range want {
			assert.Contains(t, string(files[0].Content), w, budget)
		}
	}
}
