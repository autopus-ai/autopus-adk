package content_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// hooks.verify_stop_budget becomes the Stop hook's --budget, and the hook
// timeout grows with it so Claude Code does not kill the gate first. The
// default keeps the bare command and the 600s timeout.
func TestGenerateProjectHookConfigs_VerifyStopBudget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		budget, command string
		timeout         int
	}{
		{"", "auto verify stop", 600},
		{"8m", "auto verify stop", 600},
		{"5m", "auto verify stop --budget 5m0s", 600},
		{"15m", "auto verify stop --budget 15m0s", 990},
	} {
		cfg := config.DefaultFullConfig("demo")
		cfg.Hooks = config.HooksConf{VerifyStopBudget: tc.budget}

		hooks, _, err := content.GenerateProjectHookConfigs(cfg, "claude-code", true)
		require.NoError(t, err)
		stop := findHook(hooks, "Stop")
		require.NotNil(t, stop, tc.budget)
		assert.Equal(t, tc.command, stop.Command, tc.budget)
		assert.Equal(t, tc.timeout, stop.Timeout, tc.budget)
	}
}

func TestHooksConf_VerifyStopBudgetIsValidated(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"soon", "30s", "45m"} {
		cfg := config.DefaultFullConfig("demo")
		cfg.Hooks.VerifyStopBudget = bad
		assert.Error(t, cfg.Validate(), bad)
	}
	cfg := config.DefaultFullConfig("demo")
	cfg.Hooks.VerifyStopBudget = "20m"
	assert.NoError(t, cfg.Validate())
}
