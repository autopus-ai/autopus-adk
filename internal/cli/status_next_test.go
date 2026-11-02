package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStatusProbe makes status see exactly these AI CLIs, Desktop helper, and
// home directory. It replaces a package variable, so callers do not run in
// parallel.
func stubStatusProbe(t *testing.T, installed []string, helperInstalled bool, home string) {
	t.Helper()
	previous := newStatusProbe
	newStatusProbe = func() statusProbe {
		return statusProbe{
			lookPath: func(name string) (string, error) {
				for _, candidate := range installed {
					if candidate == name {
						return "/stub/bin/" + name, nil
					}
				}
				return "", exec.ErrNotFound
			},
			desktopHelper: func() (string, error) {
				if helperInstalled {
					return "/stub/autopus-desktop-runtime", nil
				}
				return "", errRuntimeHelperNotFound
			},
			homeDir: func() (string, error) { return home, nil },
		}
	}
	t.Cleanup(func() { newStatusProbe = previous })
}

// statusActions indexes the JSON next actions by id.
func statusActions(t *testing.T, data map[string]any) map[string]map[string]any {
	t.Helper()
	byID := map[string]map[string]any{}
	actions, _ := data["next_actions"].([]any)
	for _, raw := range actions {
		action, _ := raw.(map[string]any)
		byID[action["id"].(string)] = action
	}
	return byID
}

// S7: Desktop is optional. Without it status still names valid commands; an
// installed but unconnected Desktop adds only an optional connect action.
func TestStatusNext_DesktopIsOptional(t *testing.T) {
	home := t.TempDir()
	f := newStatusFixture(t, "claude-code")
	f.commit("baseline")

	stubStatusProbe(t, []string{"claude"}, false, home)
	text := f.runStatus()
	assert.Contains(t, text, "Desktop: not installed (optional; the ADK works standalone)")
	assert.NotContains(t, text, "auto connect")
	assert.Contains(t, text, `/auto fix "<problem>" (claude-code)`)
	data := f.statusJSON()
	assert.Equal(t, "not_installed", jsonPath(data, "environment", "desktop", "state"))
	assert.NotContains(t, statusActions(t, data), "desktop_connect")

	stubStatusProbe(t, []string{"claude"}, true, home)
	text = f.runStatus()
	assert.Contains(t, text, "Desktop: installed, not connected (optional; the ADK works standalone)")
	assert.Contains(t, text, "auto connect\n")
	assert.Contains(t, text, "optional · runs in terminal · provider: none · calls: Autopus server sign-in")
	connect := statusActions(t, f.statusJSON())["desktop_connect"]
	require.NotNil(t, connect)
	assert.Equal(t, "auto connect", connect["command"])
	assert.Equal(t, true, connect["optional"])
	assert.Equal(t, false, connect["auto_run"])
	assert.Contains(t, connect["external_calls"], "Autopus server")

	workerConfig := filepath.Join(home, ".config", "autopus", "worker.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(workerConfig), 0o700))
	require.NoError(t, os.WriteFile(workerConfig, []byte("server: fixture\n"), 0o600))
	text = f.runStatus()
	assert.Contains(t, text, "Desktop: connected (worker config found)")
	assert.NotContains(t, text, "auto connect")
}

// S8: with several AI CLIs installed, status names the platform `auto
// pipeline run` would pick, why, and how it relates to the configured ones.
func TestStatusNext_SeveralProvidersInstalled(t *testing.T) {
	f := newStatusFixture(t, "codex")
	specDir := f.writeSpec("SPEC-RUN-004", "approved")
	f.commit("baseline")
	// A run on another commit cannot resume, so status offers a new run.
	f.saveCheckpoint("SPEC-RUN-004", stoppedRunCheckpoint("SPEC-RUN-004", specDir, "0000000"), false)

	stubStatusProbe(t, []string{"claude", "codex", "agy"}, false, t.TempDir())
	text := f.runStatus()
	assert.Contains(t, text, "AI CLIs on PATH: claude, codex, agy")
	assert.Contains(t, text, "Configured platforms: codex")
	assert.Contains(t, text, "auto pipeline run platform: claude — first installed of claude → codex → agy")
	assert.Contains(t, text, "note: claude is not a configured platform here; pass --platform codex to use codex")
	data := f.statusJSON()
	platform := jsonPath(data, "environment", "pipeline_platform")
	assert.Equal(t, "claude", jsonPath(platform, "platform"))
	assert.Equal(t, false, jsonPath(platform, "configured"))
	rerun := statusActions(t, data)["rerun"]
	require.NotNil(t, rerun)
	assert.Equal(t, "auto pipeline run SPEC-RUN-004", rerun["command"])
	assert.Equal(t, "claude", rerun["provider"])
	assert.Equal(t, "model calls through the claude CLI", rerun["external_calls"])

	stubStatusProbe(t, nil, false, t.TempDir())
	text = f.runStatus()
	assert.Contains(t, text, "AI CLIs on PATH: none")
	assert.Contains(t, text, "auto pipeline run platform: claude — no AI CLI installed; falls back to claude")
	assert.Contains(t, text, "note: none of claude, codex, agy is installed, so auto pipeline run cannot start without --platform")
	actions, _ := f.statusJSON()["next_actions"].([]any)
	require.NotEmpty(t, actions)
	assert.Equal(t, "doctor", jsonPath(actions[0], "id"))
	assert.Equal(t, "auto doctor", jsonPath(actions[0], "command"))
}
