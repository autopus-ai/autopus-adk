package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// Without --budget the gate takes hooks.verify_stop_budget, so a runner that
// starts it bare (the omp extension) honors the project's budget; an
// explicit flag wins.
func TestVerifyStop_BudgetComesFromConfigUnlessTheFlagIsGiven(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("demo")
	cfg.Hooks.VerifyStopBudget = "15m"
	require.NoError(t, config.Save(dir, cfg))

	cmd := newVerifyStopCmd()
	assert.Equal(t, 15*time.Minute, stopBudget(cmd, dir, 8*time.Minute))

	require.NoError(t, cmd.Flags().Set("budget", "2m"))
	assert.Equal(t, 2*time.Minute, stopBudget(cmd, dir, 2*time.Minute))

	assert.Equal(t, 8*time.Minute, stopBudget(newVerifyStopCmd(), t.TempDir(), 8*time.Minute), "no autopus.yaml")
}
