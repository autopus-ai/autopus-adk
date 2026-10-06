package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/terminal"
)

// Read-only commands must stay on the subprocess backend even on a pane-capable
// terminal without --subprocess: the pane launch path adds a permission bypass
// flag and auto-approves tool prompts.
func TestOrchestraBrainstorm_ReadOnlyForcesSubprocessOnPaneTerminal(t *testing.T) {
	fixture := newBrainstormFixture(t, true)

	origDetector := runOrchestraTerminalDetector
	t.Cleanup(func() { runOrchestraTerminalDetector = origDetector })
	runOrchestraTerminalDetector = func() terminal.Terminal { return stubTerminal{name: "cmux"} }

	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		fixture.captured = &cfg
		return &orchestra.OrchestraResult{}, nil
	}

	_ = runOrchestraCommand(context.Background(), "brainstorm", "debate", []string{"codex", "gemini"},
		30, "claude", "brainstorm topic", 1, 0,
		OrchestraFlags{OutputFormat: orchestraOutputJSON, NoDetach: true, TimeoutChanged: true})

	cfg := fixture.captured
	require.NotNil(t, cfg)
	assert.True(t, cfg.ReadOnly)
	assert.True(t, cfg.SubprocessMode, "read-only brainstorm must not select the pane backend")
}
