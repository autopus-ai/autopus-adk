package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunOrchestraCleanup_LegacyTmuxPartialFailure_PersistsOnlyRetryPanes(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "tmux"},
		closeErrors:  map[string]error{"%42": errors.New("pane busy")},
	}
	useCleanupTerminal(t, term)
	session := orchestra.OrchestraSession{
		ID: "legacy-partial", Panes: map[string]string{"claude": "%41", "codex": "%42"},
	}
	data, err := json.Marshal(session)
	require.NoError(t, err)
	path := filepath.Join(os.TempDir(), "autopus-orch-session-"+session.ID+".json")
	require.NoError(t, os.WriteFile(path, data, 0o600))

	err = runOrchestraCleanup(t.Context(), session.ID)

	require.Error(t, err)
	loaded, loadErr := orchestra.LoadSession(session.ID)
	require.NoError(t, loadErr)
	assert.Equal(t, map[string]string{"codex": "%42"}, loaded.Panes)
	info, statErr := os.Stat(path)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	delete(term.closeErrors, "%42")
	require.NoError(t, runOrchestraCleanup(t.Context(), session.ID))
	assert.Equal(t, 1, countString(term.closed, "%41"))
	assert.Equal(t, 2, countString(term.closed, "%42"))
	_, statErr = os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestOrchestraCleanup_V05085LegacyCmuxFile_ExplicitWorkspaceRecovers(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	term := &cleanupTerminal{
		stubTerminal: stubTerminal{name: "cmux"}, workspaceRef: "workspace:99",
		closeErrors: map[string]error{},
	}
	useCleanupTerminal(t, term)
	session := orchestra.OrchestraSession{
		ID: "v05085-cmux", Panes: map[string]string{"claude": "surface:1414"},
	}
	data, err := json.Marshal(session)
	require.NoError(t, err)
	path := filepath.Join(os.TempDir(), "autopus-orch-session-"+session.ID+".json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	cmd := newLegacyOrchestraCleanupCmd()
	cmd.SetArgs([]string{
		"--session-id", session.ID, "--workspace-ref", "workspace:13",
	})

	require.NoError(t, cmd.Execute())

	assert.Equal(t, []string{"surface:1414"}, term.closed)
	_, statErr := os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
