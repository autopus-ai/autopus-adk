package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orchestraRunTestCmd builds the command carrier the pipeline needs for output and
// context without printing to the real process streams.
func orchestraRunTestCmd(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

func orchestraRunJSONCmd(ctx context.Context, out *bytes.Buffer) *cobra.Command {
	root := &cobra.Command{Use: "auto"}
	orch := &cobra.Command{Use: "orchestra"}
	cmd := &cobra.Command{Use: "run"}
	orch.AddCommand(cmd)
	root.AddCommand(orch)
	cmd.SetContext(ctx)
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	return cmd
}

// stubOrchestraRunExecution replaces the run seams so the CLI boundary can be
// exercised without spawning providers. The supplied result is finalized exactly
// as the real engine finalizes it, so receipt fields are populated.
func stubOrchestraRunExecution(t *testing.T, result *orchestra.OrchestraResult) *orchestra.OrchestraConfig {
	t.Helper()
	originalLoad := orchestraRunLoadConfig
	originalBuild := orchestraRunBuildProviders
	originalBackend := orchestraRunBackendFactory
	originalRun := runOrchestraExecute
	t.Cleanup(func() {
		orchestraRunLoadConfig = originalLoad
		orchestraRunBuildProviders = originalBuild
		orchestraRunBackendFactory = originalBackend
		runOrchestraExecute = originalRun
	})
	orchestraRunLoadConfig = func(globalFlags) (*config.HarnessConfig, error) {
		return &config.HarnessConfig{Orchestra: config.OrchestraConf{
			Providers: map[string]config.ProviderEntry{
				"claude": {Binary: "claude"},
				"codex":  {Binary: "codex"},
			},
		}}, nil
	}
	orchestraRunBuildProviders = buildProviderConfigsForRuntime
	orchestraRunBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend {
		return noopExecutionBackend{}
	}
	captured := &orchestra.OrchestraConfig{}
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		*captured = cfg
		result.Strategy = cfg.Strategy
		return orchestra.FinalizeOrchestrationResult(result, cfg), nil
	}
	return captured
}

func TestOrchestraRunJSON_EmitsRunReceiptOnSuccess(t *testing.T) {
	captured := stubOrchestraRunExecution(t, &orchestra.OrchestraResult{
		Strategy: orchestra.StrategyConsensus,
		Merged:   "merged body",
		Summary:  "summary",
		Responses: []orchestra.ProviderResponse{
			{Provider: "claude", Output: "1. shared claim", ExitCode: 0},
			{Provider: "codex", Output: "1. shared claim", ExitCode: 0},
		},
	})
	var out bytes.Buffer

	err := runSubprocessPipeline(orchestraRunJSONCmd(context.Background(), &out), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "consensus",
		Providers:        []string{"claude", "codex"},
		RoundsPreset:     "fast",
		Timeout:          30,
		TimeoutChanged:   true,
		Judge:            "",
		DryRun:           false,
		JSONMode:         true,
		RequireAgreement: 0,
	})

	require.NoError(t, err)
	require.NotNil(t, captured)
	payload := decodeJSONMap(t, out.Bytes())
	assertCommonJSONEnvelope(t, payload, "auto orchestra run")
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok, "receipt must be emitted as the envelope data")
	assert.Equal(t, orchestra.OrchestrationReceiptSchema, data["schema"])
	assert.NotEmpty(t, data["terminal_state"])
	assert.NotContains(t, out.String(), "merged body",
		"json mode must not also print the rendered markdown")
}

func TestOrchestraRunJSON_EmitsReceiptWhenGateBlocks(t *testing.T) {
	stubOrchestraRunExecution(t, &orchestra.OrchestraResult{
		Strategy:      orchestra.StrategyConsensus,
		Merged:        "merged body",
		TerminalState: orchestra.TerminalBlocked,
		Responses: []orchestra.ProviderResponse{
			{Provider: "claude", Output: "1. claim", ExitCode: 0},
		},
	})
	var out bytes.Buffer

	err := runSubprocessPipeline(orchestraRunJSONCmd(context.Background(), &out), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "consensus",
		Providers:        []string{"claude"},
		RoundsPreset:     "fast",
		Timeout:          30,
		TimeoutChanged:   true,
		Judge:            "",
		DryRun:           false,
		JSONMode:         true,
		RequireAgreement: 0,
	})

	require.Error(t, err, "a blocked gate must still fail the command")
	payload := decodeJSONMap(t, out.Bytes())
	assert.Equal(t, string(jsonStatusError), payload["status"])
	assert.Equal(t, "orchestra_run_blocked", payload["error"].(map[string]any)["code"])
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok, "blocked runs must still carry the receipt as evidence")
	assert.Equal(t, orchestra.TerminalBlocked, data["terminal_state"])
}

func TestOrchestraRunJSON_DegradedRunSurfacesAsWarnStatus(t *testing.T) {
	stubOrchestraRunExecution(t, &orchestra.OrchestraResult{
		Strategy: orchestra.StrategyConsensus,
		Responses: []orchestra.ProviderResponse{
			{Provider: "claude", Output: "1. shared claim", ExitCode: 0},
			{Provider: "codex", Output: "1. shared claim", ExitCode: 0},
		},
		// One failed peer out of three still meets quorum, so the run completes
		// while the engine records degradation. That is the branchable case.
		FailedProviders: []orchestra.FailedProvider{{Name: "gemini", FailureClass: "timeout"}},
	})
	var out bytes.Buffer

	err := runSubprocessPipeline(orchestraRunJSONCmd(context.Background(), &out), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "consensus",
		Providers:        []string{"claude", "codex", "gemini"},
		RoundsPreset:     "fast",
		Timeout:          30,
		TimeoutChanged:   true,
		Judge:            "",
		DryRun:           false,
		JSONMode:         true,
		RequireAgreement: 0,
	})

	require.NoError(t, err)
	payload := decodeJSONMap(t, out.Bytes())
	assert.Equal(t, string(jsonStatusWarn), payload["status"],
		"degradation must be branchable without parsing markdown")
	data := payload["data"].(map[string]any)
	assert.NotEmpty(t, data["degraded_reasons"])
}

func TestConsensusMetrics_AgreementRatioIsDefinedForEveryOutcome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		responses []orchestra.ProviderResponse
		want      float64
	}{
		{
			name: "full agreement",
			responses: []orchestra.ProviderResponse{
				{Provider: "a", Output: "1. same claim"},
				{Provider: "b", Output: "1. same claim"},
			},
			want: 1,
		},
		{
			name: "total dissent",
			responses: []orchestra.ProviderResponse{
				{Provider: "a", Output: "1. claim one"},
				{Provider: "b", Output: "1. claim two"},
			},
			want: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := &orchestra.OrchestraResult{
				Strategy: orchestra.StrategyConsensus, Responses: test.responses,
			}
			finalized := orchestra.FinalizeOrchestrationResult(result, orchestra.OrchestraConfig{
				Strategy: orchestra.StrategyConsensus,
			})
			require.NotNil(t, finalized.ConsensusMetrics)
			assert.InDelta(t, test.want, finalized.ConsensusMetrics.AgreementRatio, 1e-9)

			encoded, err := json.Marshal(finalized.ConsensusMetrics)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"agreement_ratio"`,
				"callers gate on the ratio, so it must always serialize")
		})
	}
}
