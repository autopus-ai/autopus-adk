package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func budgetProject(t *testing.T, budget string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("demo")
	cfg.Hooks.VerifyStopBudget = budget
	require.NoError(t, config.Save(dir, cfg))
	return dir
}

// F2: without --budget the gate may only lower the budget from
// hooks.verify_stop_budget. Raising it there needs the --budget the
// generators write, because the runner's kill timer must move with it; a
// bare gate under a 600s hook timeout would be killed with no verdict.
func TestVerifyStop_FlaglessBudgetOnlyLowersFromConfig(t *testing.T) {
	assert.Equal(t, 8*time.Minute, stopBudget(newVerifyStopCmd(), budgetProject(t, "15m"), 8*time.Minute),
		"a raise without --budget is ignored")
	assert.Equal(t, 5*time.Minute, stopBudget(newVerifyStopCmd(), budgetProject(t, "5m"), 8*time.Minute))

	cmd := newVerifyStopCmd()
	require.NoError(t, cmd.Flags().Set("budget", "15m"))
	assert.Equal(t, 15*time.Minute, stopBudget(cmd, budgetProject(t, "2m"), 15*time.Minute), "an explicit flag wins")

	assert.Equal(t, 8*time.Minute, stopBudget(newVerifyStopCmd(), t.TempDir(), 8*time.Minute), "no autopus.yaml")
}
