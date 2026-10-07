package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// fakeClaudeCommand returns a *exec.Cmd that echoes the given output string.
// It replaces the real `claude --version` invocation during tests.
func fakeClaudeCommand(output string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", output)
	}
}

// failClaudeCommand returns a *exec.Cmd that exits non-zero (simulates missing binary).
func failClaudeCommand() func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	}
}

// withEnv sets environment variables for the duration of a test, then restores them.
func withEnv(t *testing.T, pairs map[string]string) {
	t.Helper()
	originals := make(map[string]string, len(pairs))
	for k, v := range pairs {
		originals[k] = os.Getenv(k)
		require.NoError(t, os.Setenv(k, v))
	}
	t.Cleanup(func() {
		for k, orig := range originals {
			if orig == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, orig)
			}
		}
	})
}

// withoutEnv unsets environment variables for the duration of a test.
func withoutEnv(t *testing.T, keys ...string) {
	t.Helper()
	originals := make(map[string]string, len(keys))
	for _, k := range keys {
		originals[k] = os.Getenv(k)
		require.NoError(t, os.Unsetenv(k))
	}
	t.Cleanup(func() {
		for k, orig := range originals {
			if orig != "" {
				_ = os.Setenv(k, orig)
			}
		}
	})
}

// patchExecCommand overrides execCommandFunc for the duration of a test.
func patchExecCommand(t *testing.T, fn func(ctx context.Context, name string, args ...string) *exec.Cmd) {
	t.Helper()
	orig := execCommandFunc
	execCommandFunc = fn
	t.Cleanup(func() { execCommandFunc = orig })
}

func enabledCC21Features() config.CC21FeaturesConf {
	return config.CC21FeaturesConf{
		Enabled:              true,
		EffortEnabled:        true,
		MonitorEnabled:       true,
		TaskCreatedEnabled:   true,
		InitialPromptEnabled: true,
		TaskCreatedMode:      "warn",
	}
}

func withCC21Config(t *testing.T, cc21 config.CC21FeaturesConf) {
	t.Helper()

	root := t.TempDir()
	cfg := config.DefaultFullConfig("demo")
	cfg.Features.CC21 = cc21
	require.NoError(t, config.Save(root, cfg))

	nested := filepath.Join(root, "nested", "workspace")
	require.NoError(t, os.MkdirAll(nested, 0755))
	t.Chdir(nested)
}
