package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// runWorkerCmd executes a worker subcommand with captured output.
func runWorkerCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func writeWorkerFile(t *testing.T, home, rel, content string) string {
	t.Helper()
	path := filepath.Join(home, ".config", "autopus", rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// Guards the logs failure path: a missing log file must be reported with its path
// rather than printing an empty result.
func TestWorkerLogsCmd_MissingFileReportsPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := runWorkerCmd(t, newWorkerLogsCmd())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log file not found")
	assert.Contains(t, err.Error(), filepath.Join(home, ".config", "autopus", "logs"))
}

// Guards the --task filter: only matching lines are emitted, and without the flag
// every line is emitted.
func TestWorkerLogsCmd_TaskFilterSelectsMatchingLines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeWorkerFile(t, home, filepath.Join("logs", "autopus-worker.out.log"),
		"task-a started\ntask-b started\ntask-a finished\n")

	filtered, err := runWorkerCmd(t, newWorkerLogsCmd(), "--task", "task-a")
	require.NoError(t, err)
	assert.Contains(t, filtered, "task-a started")
	assert.Contains(t, filtered, "task-a finished")
	assert.NotContains(t, filtered, "task-b")

	all, err := runWorkerCmd(t, newWorkerLogsCmd())
	require.NoError(t, err)
	assert.Contains(t, all, "task-b started")
}

// Guards the history empty-state: absence of the history file is a normal result,
// not an error.
func TestWorkerHistoryCmd_EmptyStateAndContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out, err := runWorkerCmd(t, newWorkerHistoryCmd())
	require.NoError(t, err)
	assert.Contains(t, out, "No task history found.")

	writeWorkerFile(t, home, "task-history.log", "task-1 ok\n")
	out, err = runWorkerCmd(t, newWorkerHistoryCmd())
	require.NoError(t, err)
	assert.Equal(t, "task-1 ok\n", out)
}

// Guards the cost empty-state and pass-through of recorded cost data.
func TestWorkerCostCmd_EmptyStateAndContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out, err := runWorkerCmd(t, newWorkerCostCmd())
	require.NoError(t, err)
	assert.Contains(t, out, "No cost data found.")

	writeWorkerFile(t, home, "cost.log", "usd=1.25\n")
	out, err = runWorkerCmd(t, newWorkerCostCmd())
	require.NoError(t, err)
	assert.Equal(t, "usd=1.25\n", out)
}

// Guards the status human-readable path: daemon/platform/PID lines must be present
// and the unconfigured machine must report "not running".
func TestWorkerStatusCmd_HumanOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	out, err := runWorkerCmd(t, newWorkerStatusCmd())
	require.NoError(t, err)
	assert.Contains(t, out, "Daemon installed: false")
	assert.Contains(t, out, "Platform: ")
	assert.Contains(t, out, "PID: not running")
}

// Guards format validation: an unsupported --format must fail before any status
// collection or output.
func TestWorkerStatusCmd_RejectsUnsupportedFormat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	out, err := runWorkerCmd(t, newWorkerStatusCmd(), "--format", "yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.NotContains(t, out, "Daemon installed")
}

// Guards the JSON contract: an unconfigured worker must produce warn status with
// the configuration/auth/daemon warning codes.
func TestWorkerStatusCmd_JSONWarnsWhenUnconfigured(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())

	out, err := runWorkerCmd(t, newWorkerStatusCmd(), "--json")
	require.NoError(t, err)

	var env struct {
		Status   string `json:"status"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	assert.Equal(t, "warn", env.Status)

	codes := make([]string, 0, len(env.Warnings))
	for _, w := range env.Warnings {
		codes = append(codes, w.Code)
	}
	assert.Contains(t, codes, "worker_not_configured")
	assert.Contains(t, codes, "worker_auth_invalid")
	assert.Contains(t, codes, "worker_daemon_stopped")
}

// Guards the --format json alias: it must yield the same envelope as --json.
func TestWorkerStatusCmd_FormatJSONAliasProducesEnvelope(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())

	out, err := runWorkerCmd(t, newWorkerStatusCmd(), "--format", "json")
	require.NoError(t, err)

	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	assert.Equal(t, "warn", env["status"])
	assert.NotContains(t, out, "Daemon installed")
}

// Guards the registration surface: legacy local-host commands stay visible with the
// legacy notice, while compatibility shims are hidden from help.
func TestAddWorkerSubcommands_VisibilityAndLegacyNotice(t *testing.T) {
	parent := &cobra.Command{Use: "worker"}
	addWorkerSubcommands(parent)

	byName := map[string]*cobra.Command{}
	for _, c := range parent.Commands() {
		byName[c.Name()] = c
	}

	for _, name := range []string{"start", "stop", "status", "logs", "restart", "history", "cost", "setup"} {
		cmd, ok := byName[name]
		require.True(t, ok, "missing worker subcommand %q", name)
		assert.False(t, cmd.Hidden, "legacy command %q must stay visible", name)
		assert.Contains(t, cmd.Long, legacyLocalHostWorkerNotice, "command %q must carry the legacy notice", name)
	}

	for _, name := range []string{"sidecar", "session", "ensure"} {
		cmd, ok := byName[name]
		require.True(t, ok, "missing worker subcommand %q", name)
		assert.True(t, cmd.Hidden, "compatibility shim %q must be hidden", name)
	}
}

// Guards the status Long text ordering: the legacy notice is prepended, preserving
// the command's own description.
func TestMarkLegacyLocalHostWorker_PrependsWithoutDroppingLong(t *testing.T) {
	cmd := newWorkerStatusCmd()
	original := cmd.Long
	require.NotEmpty(t, original)

	markLegacyLocalHostWorker(cmd)
	assert.True(t, len(cmd.Long) > len(original))
	assert.Contains(t, cmd.Long, original)
	assert.Equal(t, legacyLocalHostWorkerNotice, cmd.Long[:len(legacyLocalHostWorkerNotice)])
}

// Guards setup flag wiring: non-interactive flags must exist with the documented
// default backend so CI invocations keep working.
func TestNewWorkerSetupCmd_FlagDefaults(t *testing.T) {
	cmd := newWorkerSetupCmd()

	backend, err := cmd.Flags().GetString("backend")
	require.NoError(t, err)
	assert.Equal(t, "https://api.autopus.co", backend)

	for _, name := range []string{"token", "workspace", "api-key"} {
		val, err := cmd.Flags().GetString(name)
		require.NoError(t, err, "flag %q must be registered", name)
		assert.Empty(t, val, "flag %q must default to empty (interactive)", name)
	}
}

// Guards the start command flag surface: --daemon must default off so a bare
// `worker start` runs in the foreground.
func TestNewWorkerStartCmd_DaemonFlagDefaultsOff(t *testing.T) {
	cmd := newWorkerStartCmd()
	val, err := cmd.Flags().GetBool("daemon")
	require.NoError(t, err)
	assert.False(t, val)
}
