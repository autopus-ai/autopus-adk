package cli

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestResolveOrchestraTimeout_FlagOverridesProviderExecutionTimeout(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		TimeoutSeconds: 120,
		Providers: map[string]config.ProviderEntry{
			"codex": {
				Binary:     "codex",
				Subprocess: config.SubprocessProvConf{Timeout: 420},
			},
		},
	}
	providers := []orchestra.ProviderConfig{{
		Name: "codex", Binary: "codex", ExecutionTimeout: 420 * time.Second,
	}}

	resolved := resolveOrchestraTimeout(conf, 300, true, providers)

	require.Len(t, resolved.Providers, 1)
	assert.Equal(t, 300*time.Second, resolved.Providers[0].Duration)
	assert.Equal(t, "flag --timeout", resolved.Providers[0].Source)
}

func TestApplyResolvedProviderTimeouts_AlignsExecutionAndDiagnostics(t *testing.T) {
	t.Parallel()

	providers := []orchestra.ProviderConfig{{
		Name: "codex", Binary: "codex", ExecutionTimeout: 420 * time.Second,
	}}
	resolved := ResolvedOrchestraTimeout{
		Seconds: 300,
		Source:  "flag --timeout",
		Providers: []ResolvedProviderTimeout{{
			Provider: "codex", Duration: 300 * time.Second, Source: "flag --timeout",
		}},
	}

	got := applyResolvedProviderTimeouts(providers, resolved)

	require.Len(t, got, 1)
	assert.Equal(t, 300*time.Second, got[0].ExecutionTimeout)
	assert.Equal(t, 420*time.Second, providers[0].ExecutionTimeout, "input provider config must remain immutable")
}

func TestRunOrchestraCommand_AppliesExplicitTimeoutToProviderExecution(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("orchestra-timeout")
	cfg.Orchestra.Providers = map[string]config.ProviderEntry{
		"codex": {
			Binary:     "codex",
			Subprocess: config.SubprocessProvConf{Timeout: 420},
		},
	}
	cfg.Orchestra.Commands["plan"] = config.CommandEntry{
		Strategy: "consensus", Providers: []string{"codex"},
	}
	require.NoError(t, config.Save(dir, cfg))

	originalWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	require.NoError(t, os.Chdir(dir))

	originalRun := runOrchestraExecute
	t.Cleanup(func() { runOrchestraExecute = originalRun })
	var captured orchestra.OrchestraConfig
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{Merged: "ok", Summary: "done"}, nil
	}

	err = runOrchestraCommand(context.Background(), "plan", "", []string{"codex"}, 300, "", "topic", 0, 0, OrchestraFlags{
		TimeoutChanged: true,
	})
	require.NoError(t, err)
	require.Len(t, captured.Providers, 1)
	assert.Equal(t, 300, captured.TimeoutSeconds)
	assert.Equal(t, 300*time.Second, captured.Providers[0].ExecutionTimeout)
	assert.Empty(t, captured.FallbackMode, "empty policy must preserve the default subprocess fallback")
}
