package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Read-only commands must stay on the subprocess backend even in a
// pane-capable context without --subprocess: the pane launch path added a
// permission bypass flag and auto-approved tool prompts. SPEC-PANERM-001
// retired that path, so the context changes nothing.
func TestOrchestraBrainstorm_ReadOnlyForcesSubprocessOnPaneTerminal(t *testing.T) {
	fixture := newBrainstormFixture(t, true)
	tmuxLog := usePaneCapableContext(t)

	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		fixture.captured = &cfg
		return &orchestra.OrchestraResult{}, nil
	}

	_ = runOrchestraCommand(context.Background(), "brainstorm", "debate", []string{"codex", "gemini"},
		30, "claude", "brainstorm topic", 1, 0,
		OrchestraFlags{OutputFormat: orchestraOutputJSON, TimeoutChanged: true})

	cfg := fixture.captured
	require.NotNil(t, cfg)
	assert.True(t, cfg.ReadOnly)
	assert.Equal(t, "subprocess", selectRoutedBackend(*cfg).Name(), "read-only brainstorm must not select the pane backend")
	assert.NoFileExists(t, tmuxLog, "read-only brainstorm must not call the terminal")
}
