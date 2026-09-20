package opencode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/stretchr/testify/require"
)

// These optional Node/POSIX contracts do not replace the opt-in native test.
// AUTOPUS_OPENCODE_V2_LIVE requires its stronger OpenCode V2, Python, and
// loopback-server prerequisites; a skipped fixture is not host verification.
func TestOpenCodeV2GeneratedPluginRuntime(t *testing.T) {
	for _, scenario := range []string{"normal", "before-failure", "timeout", "output-limit", "missing-session", "setup-failure", "dispose-failure"} {
		t.Run(scenario, func(t *testing.T) {
			hooks := []adapter.HookConfig{{Event: "PreToolUse", Command: "printf before >> hook.log", Timeout: 2}, {Event: "PostToolUse", Command: "printf after >> hook.log", Timeout: 2}}
			switch scenario {
			case "before-failure":
				hooks[0].Command = "printf secret-credential >&2; exit 7"
			case "timeout":
				hooks[0].Command = "sh -c 'sleep 2; printf leaked > escaped.log' & wait"
				hooks[0].Timeout = 1
			case "output-limit":
				hooks[0].Command = "yes secret-credential"
			}
			body, err := renderHookPluginV2(hooks)
			require.NoError(t, err)
			require.NotContains(t, body, `from "@opencode/plugin"`)
			runPluginContract(t, body, scenario)
		})
	}
}

func TestOpenCodeV1PluginRuntimeRemainsCompatible(t *testing.T) {
	body, err := renderHookPlugin([]adapter.HookConfig{{Event: "PreToolUse", Command: "printf before >> hook.log", Timeout: 2}, {Event: "PostToolUse", Command: "printf after >> hook.log", Timeout: 2}})
	require.NoError(t, err)
	runPluginContract(t, body, "v1")
}

func TestOpenCodeV2PluginMappingUsesNativeDefinition(t *testing.T) {
	a := NewWithRoot(t.TempDir(), WithCLIVersion("2.0.10"))
	mapping, err := a.prepareHookPluginMapping(nil)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(".opencode", "plugins", "autopus-hooks.js"), mapping.TargetPath)
	require.Contains(t, string(mapping.Content), `// Autopus OpenCode V2 native plugin`)
	require.Contains(t, string(mapping.Content), `id: "autopus.hooks"`)
	require.NotContains(t, string(mapping.Content), `"tool.execute.before":`)
}

func runPluginContract(t *testing.T, body, scenario string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("generated plugin runtime fixture requires POSIX sh and process groups; skipped is not runtime verification")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("generated plugin runtime fixture requires Node; skipped is not runtime verification")
	}
	for _, command := range []string{"sh", "yes", "sleep"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("generated plugin runtime fixture requires POSIX command %s; skipped is not runtime verification", command)
		}
	}
	root := t.TempDir()
	pluginPath := filepath.Join(root, "plugin.mjs")
	require.NoError(t, os.WriteFile(pluginPath, []byte(body), 0600))
	fixture, err := os.ReadFile("testdata/plugin_v2_contract.mjs")
	require.NoError(t, err)
	runner := filepath.Join(root, "contract.mjs")
	require.NoError(t, os.WriteFile(runner, fixture, 0600))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, runner, pluginPath, scenario, root)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	require.Contains(t, string(output), "CONTRACT_PASS")
}
