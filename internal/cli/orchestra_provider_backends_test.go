package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestOMPProviderBackends_ConfiguresProviderAndJudgeRoutes(t *testing.T) {
	t.Parallel()

	providerConfig := orchestra.OrchestraConfig{
		Providers:  []orchestra.ProviderConfig{{Name: "reviewer", Backend: config.ProviderBackendOMP}},
		WorkingDir: t.TempDir(),
	}
	providerBackends := ompProviderBackends(providerConfig)
	require.Contains(t, providerBackends, config.ProviderBackendOMP)
	assert.NotNil(t, providerBackends[config.ProviderBackendOMP])
	assert.Equal(t, config.ProviderBackendOMP, providerBackends[config.ProviderBackendOMP].Name())

	judge := orchestra.ProviderConfig{Name: "judge", Backend: config.ProviderBackendOMP}
	judgeBackends := ompProviderBackends(orchestra.OrchestraConfig{JudgeConfig: &judge})
	require.Contains(t, judgeBackends, config.ProviderBackendOMP)
	assert.NotNil(t, judgeBackends[config.ProviderBackendOMP])

	assert.Nil(t, ompProviderBackends(orchestra.OrchestraConfig{}))
}

func TestRunOrchestraCommand_AssemblesOMPProviderBackend(t *testing.T) {
	projectDir := t.TempDir()
	harness := config.DefaultFullConfig("route-direct-test")
	harness.Orchestra.Judge = ""
	harness.Orchestra.Providers = map[string]config.ProviderEntry{
		"claude": {
			Backend: config.ProviderBackendOMP,
			Model:   "anthropic/claude-fable-5-1:max",
			Tools:   []string{"read"},
			Binary:  config.ProviderBackendOMP,
		},
	}
	harness.Orchestra.Commands["secure"] = config.CommandEntry{
		Strategy: "consensus", Providers: []string{"claude"},
	}
	require.NoError(t, config.Save(projectDir, harness))
	t.Chdir(projectDir)
	t.Setenv("HOME", t.TempDir())

	originalRun := runOrchestraExecute
	t.Cleanup(func() { runOrchestraExecute = originalRun })
	var captured orchestra.OrchestraConfig
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{Merged: "routed", Summary: "done"}, nil
	}

	err := runOrchestraCommand(
		context.Background(), "secure", "consensus", []string{"claude"},
		30, "", "topic", 0, 0, OrchestraFlags{NoPersist: true},
	)

	require.NoError(t, err)
	require.Contains(t, captured.ProviderBackends, config.ProviderBackendOMP)
	assert.NotNil(t, captured.ProviderBackends[config.ProviderBackendOMP])
}

func TestExecuteOrchestraRunStrategy_AssemblesOMPProviderBackend(t *testing.T) {
	originalRun := runOrchestraExecute
	t.Cleanup(func() { runOrchestraExecute = originalRun })
	var captured orchestra.OrchestraConfig
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{Strategy: cfg.Strategy}, nil
	}
	cfg := orchestra.OrchestraConfig{
		Providers: []orchestra.ProviderConfig{{Name: "reviewer", Backend: config.ProviderBackendOMP}},
		Strategy:  orchestra.StrategyConsensus,
	}

	_, err := executeOrchestraRunStrategy(
		context.Background(), orchestra.StrategyConsensus, cfg, orchestra.SubprocessPipelineConfig{},
	)

	require.NoError(t, err)
	require.Contains(t, captured.ProviderBackends, config.ProviderBackendOMP)
	assert.NotNil(t, captured.ProviderBackends[config.ProviderBackendOMP])
}
