package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestUnknownRuntimePreservesExistingV2Plugin(t *testing.T) {
	for _, operation := range []string{"generate", "hooks"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, ".opencode", "plugins", "autopus-hooks.js")
			original, err := renderHookPluginV2(nil)
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
			require.NoError(t, os.WriteFile(file, []byte(original), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"plugin":["./.opencode/plugins/autopus-hooks.js"]}`), 0600))
			a := NewWithRoot(dir, WithCLIVersion(""))
			if operation == "generate" {
				_, err = a.Generate(context.Background(), config.DefaultFullConfig("test"))
			} else {
				err = a.InstallHooks(context.Background(), nil, nil)
			}
			require.ErrorContains(t, err, "cannot replace existing V2 plugin")
			body, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, original, string(body))
		})
	}
}

func TestInstallHooksValidatesConfigBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, ".opencode", "plugins", "autopus-hooks.js")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
	require.NoError(t, os.WriteFile(file, []byte("preserved"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"plugins":{}}`), 0600))
	a := NewWithRoot(dir, WithCLIVersion("2.0.10"))
	err := a.InstallHooks(context.Background(), []adapter.HookConfig{{Event: "PreToolUse", Command: "true", Timeout: 1}}, nil)
	require.Error(t, err)
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, "preserved", string(body))
}
