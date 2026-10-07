package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestraCollect_PersistedWorkspace_ReadsOriginalPaneContext(t *testing.T) {
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "cmux"}, workspaceRef: "workspace:99",
		readOutput: "provider response", closeErrors: map[string]error{},
	}
	useCleanupTerminal(t, term)
	session := orchestra.OrchestraSession{
		ID: "collect-context-" + orchestra.NewSessionID(), TerminalKind: "cmux",
		WorkspaceRef: "workspace:13", Panes: map[string]string{"claude": "surface:1414"},
		Providers: []orchestra.SessionProviderConfig{{Name: "claude", Binary: "claude"}},
		CreatedAt: time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))
	t.Cleanup(func() { _ = orchestra.RemoveSession(session.ID) })
	cmd := newLegacyOrchestraCollectCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{session.ID})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "workspace:13", term.workspaceRef)
	assert.Equal(t, []terminal.PaneID{"surface:1414"}, term.readPanes)
	assert.Contains(t, stdout.String(), "provider response")
}

func TestOrchestraInject_PersistedWorkspace_TargetsOriginalPaneContext(t *testing.T) {
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "cmux"}, workspaceRef: "workspace:99",
		closeErrors: map[string]error{},
	}
	useCleanupTerminal(t, term)
	originalDelay := orchestraInjectSubmitDelay
	orchestraInjectSubmitDelay = 0
	t.Cleanup(func() { orchestraInjectSubmitDelay = originalDelay })
	session := orchestra.OrchestraSession{
		ID: "inject-context-" + orchestra.NewSessionID(), TerminalKind: "cmux",
		WorkspaceRef: "workspace:13", Panes: map[string]string{"claude": "surface:1414"},
		CreatedAt: time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))
	t.Cleanup(func() { _ = orchestra.RemoveSession(session.ID) })
	cmd := newLegacyOrchestraInjectCmd()
	cmd.SetArgs([]string{"--session-id", session.ID, "--provider", "claude", "follow up"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "workspace:13", term.workspaceRef)
	assert.Equal(t, []string{"follow up"}, term.longTexts)
	assert.Equal(t, []string{"\n"}, term.commands)
}

func TestOrchestraCollect_LegacyCmuxSession_DoesNotGuessCurrentWorkspace(t *testing.T) {
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "cmux"}, workspaceRef: "workspace:99",
		closeErrors: map[string]error{},
	}
	useCleanupTerminal(t, term)
	session := orchestra.OrchestraSession{
		ID:        "collect-legacy-" + orchestra.NewSessionID(),
		Panes:     map[string]string{"claude": "surface:1414"},
		Providers: []orchestra.SessionProviderConfig{{Name: "claude"}}, CreatedAt: time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))
	t.Cleanup(func() { _ = orchestra.RemoveSession(session.ID) })
	cmd := newLegacyOrchestraCollectCmd()
	cmd.SetArgs([]string{session.ID})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "legacy session")
	assert.Empty(t, term.readPanes)
}

func TestOrchestraCollect_LegacyCmuxSession_ExplicitWorkspaceSucceeds(t *testing.T) {
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "cmux"}, workspaceRef: "workspace:99",
		readOutput: "legacy response", closeErrors: map[string]error{},
	}
	useCleanupTerminal(t, term)
	session := orchestra.OrchestraSession{
		ID:        "collect-legacy-override-" + orchestra.NewSessionID(),
		Panes:     map[string]string{"claude": "surface:1414"},
		Providers: []orchestra.SessionProviderConfig{{Name: "claude"}}, CreatedAt: time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))
	t.Cleanup(func() { _ = orchestra.RemoveSession(session.ID) })
	cmd := newLegacyOrchestraCollectCmd()
	cmd.SetArgs([]string{"--workspace-ref", "workspace:13", session.ID})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "workspace:13", term.workspaceRef)
	assert.Equal(t, []terminal.PaneID{"surface:1414"}, term.readPanes)
}

func TestOrchestraInject_LegacyCmuxSession_ExplicitWorkspaceSucceeds(t *testing.T) {
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "cmux"}, workspaceRef: "workspace:99",
		closeErrors: map[string]error{},
	}
	useCleanupTerminal(t, term)
	originalDelay := orchestraInjectSubmitDelay
	orchestraInjectSubmitDelay = 0
	t.Cleanup(func() { orchestraInjectSubmitDelay = originalDelay })
	session := orchestra.OrchestraSession{
		ID:    "inject-legacy-override-" + orchestra.NewSessionID(),
		Panes: map[string]string{"claude": "surface:1414"}, CreatedAt: time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))
	t.Cleanup(func() { _ = orchestra.RemoveSession(session.ID) })
	cmd := newLegacyOrchestraInjectCmd()
	cmd.SetArgs([]string{
		"--session-id", session.ID, "--provider", "claude",
		"--workspace-ref", "workspace:13", "follow up",
	})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "workspace:13", term.workspaceRef)
	assert.Equal(t, []string{"follow up"}, term.longTexts)
}

func TestOrchestraCleanup_LegacyCmuxSession_ExplicitWorkspaceSucceeds(t *testing.T) {
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "cmux"}, workspaceRef: "workspace:99",
		closeErrors: map[string]error{},
	}
	useCleanupTerminal(t, term)
	session := orchestra.OrchestraSession{
		ID:    "cleanup-legacy-override-" + orchestra.NewSessionID(),
		Panes: map[string]string{"claude": "surface:1414"}, CreatedAt: time.Now(),
	}
	require.NoError(t, orchestra.SaveSession(session))
	cmd := newLegacyOrchestraCleanupCmd()
	cmd.SetArgs([]string{
		"--session-id", session.ID, "--workspace-ref", "workspace:13",
	})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "workspace:13", term.workspaceRef)
	assert.Equal(t, []string{"surface:1414"}, term.closed)
	_, err := orchestra.LoadSession(session.ID)
	assert.Error(t, err)
}

func TestNewOrchestraInjectCmd_ExposesLegacyWorkspaceFlag(t *testing.T) {
	cmd := newLegacyOrchestraInjectCmd()

	assert.NotNil(t, cmd.Flags().Lookup("workspace-ref"))
}
