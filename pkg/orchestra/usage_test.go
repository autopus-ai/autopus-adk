package orchestra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/insajin/autopus-adk/pkg/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunProvider_CodexUsageBeforeLastMessageReplacement_IsPreserved(t *testing.T) {
	original := newCommand
	defer func() { newCommand = original }()

	var capturedArgs []string
	waitCh := make(chan error, 1)
	fake := &fakeCommand{waitCh: waitCh, exitCode: 0, startFn: func(cmd *fakeCommand) error {
		lastMessagePath := argValueAfter(capturedArgs, "--output-last-message")
		require.NotEmpty(t, lastMessagePath)
		raw := "{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"raw answer must not be retained\"}}\n" +
			"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1000,\"cached_input_tokens\":400,\"output_tokens\":300,\"reasoning_output_tokens\":100}}\n"
		if _, err := io.WriteString(cmd.stdout, raw); err != nil {
			return err
		}
		if err := os.WriteFile(lastMessagePath, []byte("final answer"), 0o600); err != nil {
			return err
		}
		waitCh <- nil
		return nil
	}}
	newCommand = func(_ context.Context, _ string, args ...string) command {
		capturedArgs = append([]string{}, args...)
		return fake
	}

	response, err := runProvider(context.Background(), ProviderConfig{
		Name: "codex", Binary: "codex", Args: []string{"exec", "--json"},
	}, "secret prompt")

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "final answer", response.Output)
	require.Len(t, response.Usage, 1)
	assert.Equal(t, int64(1300), *response.Usage[0].RawTotalTokens)
	assert.True(t, response.UsageCapability.Supported)
	assert.Equal(t, "subprocess_stdout", response.UsageCapability.Source)
	payload, marshalErr := json.Marshal(response.Usage)
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(payload), "secret prompt")
	assert.NotContains(t, string(payload), "raw answer")
}

func TestAggregateOrchestraUsage_ResponsesAndRoundHistory_DeduplicatesCalls(t *testing.T) {
	c1 := actualOrchestraUsage("run-1", "call-1", "codex", 1000, 300)
	c2 := actualOrchestraUsage("run-1", "call-2", "claude", 900, 300)
	result := &OrchestraResult{
		Responses: []ProviderResponse{{Provider: "codex", Usage: []telemetry.UsageEnvelope{c1}}},
		RoundHistory: [][]ProviderResponse{{
			{Provider: "codex", Usage: []telemetry.UsageEnvelope{c1}},
			{Provider: "claude", Usage: []telemetry.UsageEnvelope{c2}},
		}},
	}

	aggregateOrchestraUsage(result)

	assert.Len(t, result.Usage, 2)
	assert.Equal(t, 2, result.UsageAggregate.UniqueModelCallCount)
	require.NotNil(t, result.UsageAggregate.RawTotalTokens)
	assert.Equal(t, int64(2500), *result.UsageAggregate.RawTotalTokens)
}

func TestAggregateOrchestraUsage_ConflictingDuplicate_BlocksPromotion(t *testing.T) {
	original := actualOrchestraUsage("run-c", "call-c", "codex", 10, 5)
	conflict := actualOrchestraUsage("run-c", "call-c", "codex", 10, 6)
	result := &OrchestraResult{Responses: []ProviderResponse{
		{Provider: "codex", Usage: []telemetry.UsageEnvelope{original, conflict}},
	}}

	aggregateOrchestraUsage(result)

	assert.Len(t, result.Usage, 2, "conflicting receipts must remain visible")
	assert.True(t, result.UsageAggregate.PromotionBlocked)
	assert.Equal(t, telemetry.UsageReasonDuplicateCallConflict, result.UsageAggregate.UnavailableReason)
}

func TestParseProviderUsage_SupervisorIdentityOverridesSpoofedStdout(t *testing.T) {
	binding := usageBinding{
		RunID: "supervisor-run", CallID: "supervisor-call", TaskID: "supervisor-task",
		Provider: "codex", Model: "trusted-model", Effort: "max",
		Phase: "review", Role: "reviewer", Attempt: 3,
	}
	raw := `{"type":"turn.completed","run_id":"spoof-run","call_id":"spoof-call",` +
		`"task_id":"spoof-task","provider":"spoof-provider","model":"spoof-model",` +
		`"effort":"low","phase":"spoof-phase","role":"spoof-role","attempt":99,` +
		`"usage":{"input_tokens":12,"output_tokens":3}}`

	usage := parseCodexUsage(raw, binding)

	require.Len(t, usage, 1)
	assert.Equal(t, binding.RunID, usage[0].RunID)
	assert.Equal(t, binding.CallID, usage[0].CallID)
	assert.Equal(t, binding.TaskID, usage[0].TaskID)
	assert.Equal(t, binding.Provider, usage[0].Provider)
	assert.Equal(t, binding.Model, usage[0].Model)
	assert.Equal(t, binding.Effort, usage[0].Effort)
	assert.Equal(t, binding.Phase, usage[0].Phase)
	assert.Equal(t, binding.Role, usage[0].Role)
	assert.Equal(t, binding.Attempt, usage[0].Attempt)
}

func TestExecuteParallel_FailedResponseUsage_IsRetained(t *testing.T) {
	usage := actualOrchestraUsage("run-f", "call-f", "codex", 100, 50)
	backend := &usageFailureBackend{response: &ProviderResponse{
		Provider: "codex", Usage: []telemetry.UsageEnvelope{usage},
	}}

	_, failed, err := executeParallel(context.Background(), backend,
		[]ProviderConfig{{Name: "codex", Binary: "codex"}}, "prompt", "", "reviewer", 2, 30)

	require.Error(t, err)
	require.Len(t, failed, 1)
	require.Len(t, failed[0].Usage, 1)
	assert.Equal(t, int64(150), *failed[0].Usage[0].RawTotalTokens)
}

func TestUsageSerialization_YieldResponseRemainsAdditive(t *testing.T) {
	usage := actualOrchestraUsage("run-s", "call-s", "codex", 100, 20)
	capability := UsageCapability{Supported: true, Source: "subprocess_stdout"}
	yield := YieldResponse{
		Provider: "codex", Output: "answer", Usage: []telemetry.UsageEnvelope{usage}, UsageCapability: capability,
	}

	payload, err := json.Marshal(yield)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"usage"`)
	assert.Contains(t, string(payload), `"usage_capability"`)
	assert.NotContains(t, string(payload), "secret prompt")
}

func TestParseCodexUsage_MalformedOrNonUsageLines_DoNotLeakPayload(t *testing.T) {
	raw := "not json with secret\n" +
		"{\"type\":\"item.completed\",\"item\":{\"text\":\"secret response\"}}\n" +
		"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":12,\"output_tokens\":3}}\n"

	usage := parseCodexUsage(raw, usageBinding{RunID: "run", CallID: "call", Provider: "codex"})

	require.Len(t, usage, 1)
	payload, err := json.Marshal(usage)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "secret")
	assert.Equal(t, int64(15), *usage[0].RawTotalTokens)
}

func TestDecorateProviderUsage_ClaudeCacheBreakdown_IsNormalized(t *testing.T) {
	raw := `{"type":"result","run_id":"run-a","call_id":"call-a","result":"secret response",` +
		`"usage":{"input_tokens":600,"cache_creation_input_tokens":100,"cache_read_input_tokens":300,"output_tokens":200}}`

	usage, capability := decorateProviderUsage(ProviderConfig{Name: "claude", Binary: "claude"}, "reviewer", 1, raw)

	require.Len(t, usage, 1)
	assert.True(t, capability.Supported)
	assert.True(t, capability.Observed)
	assert.Equal(t, int64(1000), *usage[0].InputTokensTotal)
	assert.Equal(t, int64(1200), *usage[0].RawTotalTokens)
	payload, err := json.Marshal(usage)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "secret response")
}

func TestDecorateProviderUsage_ClaudeCostFields_TotalTakesPriorityWithFallback(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want float64
	}{
		{name: "total priority", raw: `{"type":"result","cost_usd":0.02,"total_cost_usd":0.04}`, want: 0.04},
		{name: "legacy fallback", raw: `{"type":"result","cost_usd":0.02}`, want: 0.02},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usage, capability := decorateProviderUsage(ProviderConfig{Name: "claude"}, "reviewer", 1, test.raw)
			require.Len(t, usage, 1)
			assert.True(t, capability.Observed)
			assert.Equal(t, telemetry.UsageStatusCostOnly, usage[0].UsageStatus)
			require.NotNil(t, usage[0].ActualCostUSD)
			assert.Equal(t, test.want, *usage[0].ActualCostUSD)
		})
	}
}

func TestRunSubprocessPipeline_UsageAcrossRoundAndFinalResponse_IsDeduplicated(t *testing.T) {
	backend := &usagePipelineBackend{}
	cfg := SubprocessPipelineConfig{
		Backend: backend, Providers: []ProviderConfig{{Name: "codex", Binary: "codex"}},
		PromptData: PromptData{ProjectName: "test", Topic: "topic"},
		Judge:      ProviderConfig{Name: "claude", Binary: "claude"},
	}

	result, err := RunSubprocessPipeline(context.Background(), cfg)

	require.NoError(t, err)
	assert.Equal(t, 2, result.UsageAggregate.UniqueModelCallCount)
	require.NotNil(t, result.UsageAggregate.RawTotalTokens)
	assert.Equal(t, int64(40), *result.UsageAggregate.RawTotalTokens)
}

type usageFailureBackend struct{ response *ProviderResponse }

func (b *usageFailureBackend) Execute(context.Context, ProviderRequest) (*ProviderResponse, error) {
	return b.response, errors.New("provider failed after reporting usage")
}

func (b *usageFailureBackend) Name() string { return "subprocess" }

type usagePipelineBackend struct{}

func (b *usagePipelineBackend) Execute(_ context.Context, req ProviderRequest) (*ProviderResponse, error) {
	usage := actualOrchestraUsage("pipeline-run", fmt.Sprintf("%s-%s-%d", req.Provider, req.Role, req.Round), req.Provider, 15, 5)
	return &ProviderResponse{
		Provider: req.Provider, Output: defaultOutput(req.Role), Usage: []telemetry.UsageEnvelope{usage},
		UsageCapability: UsageCapability{Supported: true, Observed: true, Source: usageSourceSubprocess},
	}, nil
}

func (b *usagePipelineBackend) Name() string { return "subprocess" }

// freshExecutionPerRequest models the subprocess backend's isolated process per usage receipt.
func (b *usagePipelineBackend) freshExecutionPerRequest() bool { return true }

func actualOrchestraUsage(runID, callID, provider string, input, output int64) telemetry.UsageEnvelope {
	return telemetry.NormalizeUsage(telemetry.UsageInput{
		RunID: runID, CallID: callID, Provider: provider, Source: telemetry.UsageSourceProvider,
		InputTokensTotal: &input, OutputTokensTotal: &output,
		SourceSchema: fmt.Sprintf("%s.test", provider),
	})
}
