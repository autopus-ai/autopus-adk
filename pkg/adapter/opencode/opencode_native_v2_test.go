package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/stretchr/testify/require"
)

// This opt-in fixture uses the real installed host and a deterministic local
// protocol stub. It verifies native hook dispatch, not model quality/delegation.
func TestNativeOpenCodeV2HookDispatch(t *testing.T) {
	if os.Getenv("AUTOPUS_OPENCODE_V2_LIVE") != "1" {
		t.Skip("set AUTOPUS_OPENCODE_V2_LIVE=1 for installed OpenCode native hook integration")
	}
	if runtime.GOOS == "windows" {
		t.Skip("native fixture requires POSIX shell/process-group cleanup")
	}
	binary, err := exec.LookPath("opencode")
	require.NoError(t, err)
	python, err := exec.LookPath("python3")
	require.NoError(t, err)
	root := t.TempDir()
	plugin, err := renderHookPluginV2([]adapter.HookConfig{
		{Event: "PreToolUse", Command: "printf 'before\\n' >> lifecycle.log; test ! -e reject-before", Timeout: 3},
		{Event: "PostToolUse", Command: "test -f shell-ran && printf 'after\\n' >> lifecycle.log", Timeout: 3},
	})
	require.NoError(t, err)
	pluginDir := filepath.Join(root, ".opencode", "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "autopus-hooks.js"), []byte(plugin), 0600))
	logDir := os.Getenv("AUTOPUS_OPENCODE_V2_LOG_DIR")
	if logDir == "" {
		logDir = filepath.Join(os.TempDir(), "autopus-opencode-v2-research", fmt.Sprintf("native-run-%d", time.Now().UnixNano()))
	}
	require.NoError(t, os.MkdirAll(logDir, 0700))
	runner, err := filepath.Abs("testdata/native_v2_probe.py")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, runner, root, binary, logDir)
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 12 * time.Second
	output, err := command.CombinedOutput()
	t.Logf("native fixture log directory: %s", logDir)
	require.NoError(t, err, "%s", output)
	require.Contains(t, string(output), "NATIVE_V2_HOOKS_PASS")
}
