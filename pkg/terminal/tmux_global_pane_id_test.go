package terminal

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTmuxOutputMock replaces execCommand with a recorder whose command emits
// output. SplitPane tests use it to model tmux's -P/-F global pane ID response.
func newTmuxOutputMock(output string) (restore func(), captured *capturedCmd) {
	orig := execCommand
	origCtx := execCommandContext
	cap := &capturedCmd{}
	buildCmd := func(name string, args ...string) *exec.Cmd {
		cap.name = name
		cap.args = args
		return exec.Command("printf", "%s", output)
	}
	execCommand = buildCmd
	execCommandContext = func(_ context.Context, name string, args ...string) *exec.Cmd {
		return buildCmd(name, args...)
	}
	return func() {
		execCommand = orig
		execCommandContext = origCtx
	}, cap
}

// TestTmuxAdapter_SplitPane_ZeroValueReturnsGlobalPaneID verifies that the
// adapter created by DetectTerminal can split the current tmux pane without an
// empty session target and returns a globally addressable pane ID.
func TestTmuxAdapter_SplitPane_ZeroValueReturnsGlobalPaneID(t *testing.T) {
	// Given: tmux reports the new pane's global ID.
	restore, captured := newTmuxOutputMock("%42\n")
	defer restore()

	// When: a zero-value adapter splits the active pane.
	paneID, err := (&TmuxAdapter{}).SplitPane(context.Background(), Horizontal)

	// Then: SplitPane requests and returns #{pane_id}, and never emits `-t ""`.
	require.NoError(t, err)
	assert.Equal(t, PaneID("%42"), paneID)
	assert.Contains(t, captured.args, "-P", "split-window must print the created pane ID")
	assert.Contains(t, captured.args, "-F", "split-window must select an explicit output format")
	assert.Contains(t, captured.args, "#{pane_id}", "split-window must return tmux's global pane target")
	for i, arg := range captured.args {
		if arg != "-t" {
			continue
		}
		require.Less(t, i+1, len(captured.args), "-t must have a target")
		assert.NotEmpty(t, captured.args[i+1], "zero-value adapter must not send an empty -t target")
	}
}

// TestTmuxAdapter_SendCommand_GlobalPaneIDUsesDirectTarget verifies that the
// global ID returned by SplitPane is accepted unchanged by follow-up commands.
func TestTmuxAdapter_SendCommand_GlobalPaneIDUsesDirectTarget(t *testing.T) {
	restore, captured := newTmuxMock()
	defer restore()

	err := (&TmuxAdapter{}).SendCommand(context.Background(), PaneID("%42"), "go test ./...")

	require.NoError(t, err)
	assert.Equal(t, "tmux", captured.name)
	assert.Equal(t, []string{"send-keys", "-t", "%42", "go test ./...", "Enter"}, captured.args)
}

// TestTmuxAdapter_Close_GlobalPaneIDKillsPane verifies cleanup closes only the
// pane created for the provider rather than treating its global ID as a session.
func TestTmuxAdapter_Close_GlobalPaneIDKillsPane(t *testing.T) {
	restore, captured := newTmuxMock()
	defer restore()

	err := (&TmuxAdapter{}).Close(context.Background(), "%42")

	require.NoError(t, err)
	assert.Equal(t, "tmux", captured.name)
	assert.Equal(t, []string{"kill-pane", "-t", "%42"}, captured.args)
}
