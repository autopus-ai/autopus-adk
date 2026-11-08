package agentexec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agy's real message when a headless edit run hits a command it may not run.
const agyDenied = `jetski: no output produced — a tool required the "command" permission that headless mode cannot prompt for, so it was auto-denied. Add an allow-rule under permissions.allow in settings.json (e.g. command(<target>)). Alternatively, re-run with --dangerously-skip-permissions to auto-approve all tools.
`

// An exit 0 that did nothing because a tool was denied is not a success.
func TestRun_SilentPermissionDenialIsAnError(t *testing.T) {
	t.Parallel()
	r := fakeRunner(t, nil, func(context.Context, []string, string, string) (string, string, int, error) {
		return "", agyDenied, 0, nil
	})
	_, err := r.Run(context.Background(), Request{Target: TargetGemini, Mode: ModeEdit, Prompt: "fix it"})
	var agentErr *AgentError
	require.ErrorAs(t, err, &agentErr)
	assert.Equal(t, CodePermissionDenied, agentErr.Code)
	assert.Contains(t, err.Error(), "auto-denied")

	// Output alongside the notice means the agent did work; that run stands.
	r = fakeRunner(t, nil, func(context.Context, []string, string, string) (string, string, int, error) {
		return "Updated main.go", agyDenied, 0, nil
	})
	_, err = r.Run(context.Background(), Request{Target: TargetGemini, Mode: ModeEdit, Prompt: "fix it"})
	assert.NoError(t, err)
}
