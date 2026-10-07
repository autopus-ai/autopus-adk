package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cleanupTerminal struct {
	stubTerminal
	workspaceRef string
	closed       []string
	closeErrors  map[string]error
	readOutput   string
	readPanes    []terminal.PaneID
	longTexts    []string
	commands     []string
}

func (t *cleanupTerminal) Close(_ context.Context, ref string) error {
	t.closed = append(t.closed, ref)
	return t.closeErrors[ref]
}

func (t *cleanupTerminal) WorkspaceRef() (string, error) {
	return t.workspaceRef, nil
}

func (t *cleanupTerminal) WithWorkspaceRef(ref string) (terminal.Terminal, error) {
	t.workspaceRef = ref
	return t, nil
}

func (t *cleanupTerminal) ReadScreen(_ context.Context, pane terminal.PaneID, _ terminal.ReadScreenOpts) (string, error) {
	t.readPanes = append(t.readPanes, pane)
	return t.readOutput, nil
}

func (t *cleanupTerminal) SendLongText(_ context.Context, _ terminal.PaneID, text string) error {
	t.longTexts = append(t.longTexts, text)
	return nil
}

func (t *cleanupTerminal) SendCommand(_ context.Context, _ terminal.PaneID, command string) error {
	t.commands = append(t.commands, command)
	return nil
}

func useCleanupTerminal(t *testing.T, term terminal.Terminal) {
	t.Helper()
	original := orchestraSessionTerminalDetector
	orchestraSessionTerminalDetector = func() terminal.Terminal { return term }
	t.Cleanup(func() { orchestraSessionTerminalDetector = original })
}

func TestNewOrchestraCleanupCmd_Flags(t *testing.T) {
	t.Parallel()

	cmd := newLegacyOrchestraCleanupCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "cleanup", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("session-id"), "session-id flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("workspace-ref"), "legacy cmux recovery flag must exist")
}

func TestNewOrchestraCleanupCmd_RequiresSessionID(t *testing.T) {
	t.Parallel()

	cmd := newLegacyOrchestraCleanupCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err, "should fail without --session-id")
}

func TestRunOrchestraCleanup_MissingSession(t *testing.T) {
	t.Parallel()

	// Cleanup of a non-existent session should not error (idempotent).
	err := runOrchestraCleanup(t.Context(), "nonexistent-session-cleanup-test")
	assert.NoError(t, err)
}

func TestRunOrchestraCleanup_RemovesSessionFile(t *testing.T) {
	t.Parallel()

	// Create a session file
	session := orchestra.OrchestraSession{
		ID:        "test-cleanup-" + orchestra.NewSessionID(),
		Panes:     map[string]string{},
		CreatedAt: time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))

	// Cleanup should succeed (pane kill will fail since no real terminal, but that's fine)
	err := runOrchestraCleanup(t.Context(), session.ID)
	assert.NoError(t, err)

	// Session file should be removed
	_, loadErr := orchestra.LoadSession(session.ID)
	assert.Error(t, loadErr, "session should be removed after cleanup")
}

func TestRunOrchestraCleanup_RemovesLegacySessionFile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	session := orchestra.OrchestraSession{
		ID:        "legacy-cleanup",
		Panes:     map[string]string{},
		CreatedAt: time.Now(),
	}
	data, err := json.Marshal(session)
	require.NoError(t, err)
	path := filepath.Join(os.TempDir(), "autopus-orch-session-"+session.ID+".json")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	require.NoError(t, runOrchestraCleanup(t.Context(), session.ID))

	_, err = os.Lstat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunOrchestraCleanup_AllPanesClosed_RemovesSessionAndIsIdempotent(t *testing.T) {
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "cmux"},
		workspaceRef: "workspace:99",
		closeErrors:  map[string]error{},
	}
	useCleanupTerminal(t, term)
	session := orchestra.OrchestraSession{
		ID:           "cleanup-success-" + orchestra.NewSessionID(),
		TerminalKind: "cmux",
		WorkspaceRef: "workspace:13",
		Panes: map[string]string{
			"claude": "surface:1414",
			"codex":  "surface:1415",
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))

	require.NoError(t, runOrchestraCleanup(t.Context(), session.ID))
	assert.ElementsMatch(t, []string{"surface:1414", "surface:1415"}, term.closed)
	assert.Equal(t, "workspace:13", term.workspaceRef)
	_, err := orchestra.LoadSession(session.ID)
	assert.Error(t, err)

	assert.NoError(t, runOrchestraCleanup(t.Context(), session.ID),
		"a repeated cleanup after durable removal must be a no-op")
}

func TestRunOrchestraCleanup_PartialCloseFailure_ReturnsErrorAndKeepsSession(t *testing.T) {
	closeFailure := errors.New("surface busy")
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "cmux"},
		workspaceRef: "workspace:99",
		closeErrors:  map[string]error{"surface:1415": closeFailure},
	}
	useCleanupTerminal(t, term)
	session := orchestra.OrchestraSession{
		ID:           "cleanup-partial-" + orchestra.NewSessionID(),
		TerminalKind: "cmux",
		WorkspaceRef: "workspace:13",
		Panes: map[string]string{
			"claude": "surface:1414",
			"codex":  "surface:1415",
		},
		CreatedAt: time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))
	t.Cleanup(func() { _ = orchestra.RemoveSession(session.ID) })

	err := runOrchestraCleanup(t.Context(), session.ID)

	require.Error(t, err)
	assert.ErrorContains(t, err, "1/2 panes")
	loaded, loadErr := orchestra.LoadSession(session.ID)
	require.NoError(t, loadErr)
	assert.Equal(t, map[string]string{"codex": "surface:1415"}, loaded.Panes,
		"only failed panes should remain in the durable retry handle")

	delete(term.closeErrors, "surface:1415")
	require.NoError(t, runOrchestraCleanup(t.Context(), session.ID))
	assert.Equal(t, 1, countString(term.closed, "surface:1414"),
		"a successful pane must not be targeted again during retry")
	assert.Equal(t, 2, countString(term.closed, "surface:1415"))
	_, loadErr = orchestra.LoadSession(session.ID)
	assert.Error(t, loadErr)
}

func countString(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func TestRunOrchestraCleanup_PlainBackend_ReturnsErrorAndKeepsSession(t *testing.T) {
	useCleanupTerminal(t, &terminal.PlainAdapter{})
	session := orchestra.OrchestraSession{
		ID:           "cleanup-plain-" + orchestra.NewSessionID(),
		TerminalKind: "plain",
		Panes:        map[string]string{"claude": "surface:1414"},
		CreatedAt:    time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))
	t.Cleanup(func() { _ = orchestra.RemoveSession(session.ID) })

	err := runOrchestraCleanup(t.Context(), session.ID)

	require.Error(t, err)
	assert.ErrorContains(t, err, "plain")
	_, loadErr := orchestra.LoadSession(session.ID)
	assert.NoError(t, loadErr, "a no-op backend must not consume the salvage handle")
}
