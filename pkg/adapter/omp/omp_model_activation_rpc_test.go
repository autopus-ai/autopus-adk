package omp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/require"
)

// modelSelectorRPCFakeRunner answers a get_state probe for the concrete
// selector the session was started with, the way a live OMP session reports
// the model it settled on.
type modelSelectorRPCFakeRunner struct {
	selectors map[string]string
	rpcCalls  atomic.Int64
}

func (runner *modelSelectorRPCFakeRunner) Run(
	context.Context,
	string,
	...string,
) ([]byte, error) {
	return nil, fmt.Errorf("config get must not read the override map when RPC is available")
}

func (runner *modelSelectorRPCFakeRunner) RunWithInput(
	_ context.Context,
	executable string,
	input []byte,
	args ...string,
) ([]byte, error) {
	if executable != cliBinary || string(input) != `{"id":"autopus-model-state","type":"get_state"}`+"\n" {
		return nil, fmt.Errorf("unexpected RPC request")
	}
	requested := ""
	for index, arg := range args {
		if arg == "--model" && index+1 < len(args) {
			requested = args[index+1]
		}
	}
	if !ompFakeRPCSelectorConfigured(runner.selectors, requested) {
		return nil, fmt.Errorf("unconfigured selector %q", requested)
	}
	modelSelector, thinking, err := splitOMPProjectedSelector(requested)
	if err != nil {
		return nil, err
	}
	provider, model, ok := parseOMPRoutingSelector(modelSelector)
	if !ok {
		return nil, fmt.Errorf("invalid selector")
	}
	runner.rpcCalls.Add(1)
	frame := map[string]any{
		"id": "autopus-model-state", "type": "response", "command": "get_state", "success": true,
		"data": map[string]any{
			"model":         map[string]any{"provider": provider, "id": model},
			"thinkingLevel": thinking,
		},
	}
	encoded, err := json.Marshal(frame)
	return append(encoded, '\n'), err
}

func ompFakeRPCSelectorConfigured(selectors map[string]string, requested string) bool {
	for _, selector := range selectors {
		if selector == requested {
			return true
		}
	}
	return false
}

// The readback must address bundled agents, never an autopus_* role alias:
// after the cutover Autopus owns no OMP model role to resolve through.
func TestReadOMPModelExpectedValues_ResolvesEveryNativeAgentSelectorOverRPC(t *testing.T) {
	t.Parallel()
	overrides := map[string]string{
		"reviewer":          "anthropic/claude-opus-5:max",
		"task":              "openai-codex/gpt-5.6-sol:max",
		"security-reviewer": "openai-codex/gpt-5.6-terra:high",
	}
	runner := &modelSelectorRPCFakeRunner{selectors: overrides}

	readback, err := ReadOMPModelExpectedValues(
		context.Background(), runner, "/tmp/config.yml",
		map[string]any{config.OMPNativeAgentModelOverridesKey: overrides},
	)

	require.NoError(t, err)
	require.Equal(t, int64(len(overrides)), runner.rpcCalls.Load())
	var decoded map[string]map[string]string
	require.NoError(t, json.Unmarshal(readback, &decoded))
	require.Equal(t, overrides, decoded[config.OMPNativeAgentModelOverridesKey])
}

// The readback spawns one omp process per bundled agent, so the argv shape is
// a trust boundary: only the provider-free rpc session form may reach the pin.
func TestSafeOMPModelSelectorRPCArgs_AcceptsOnlyProviderFreeSelectorSessions(t *testing.T) {
	t.Parallel()
	good := []string{"--config", "/tmp/routing.yml", "--model", "anthropic/claude-opus-5:xhigh",
		"--mode", "rpc", "--no-tools", "--no-skills", "--no-extensions"}
	require.True(t, SafeOMPModelSelectorRPCArgs(good))

	mutate := func(index int, value string) []string {
		bad := append([]string(nil), good...)
		bad[index] = value
		return bad
	}
	for name, args := range map[string][]string{
		"relative config":    mutate(1, "routing.yml"),
		"newline in config":  mutate(1, "/tmp/a\nb.yml"),
		"role alias":         mutate(3, "@autopus_executor"),
		"bare model id":      mutate(3, "claude-opus-5"),
		"selector no effort": mutate(3, "anthropic/claude-opus-5"),
		"unsupported effort": mutate(3, "anthropic/claude-opus-5:turbo"),
		"injection attempt":  mutate(3, "anthropic/claude-opus-5:xhigh --yolo"),
		"unsafe model":       mutate(3, "anthropic/claude;rm:xhigh"),
		"non-rpc mode":       mutate(5, "cli"),
		"tools enabled":      mutate(6, "--tools"),
		"missing flag":       good[:8],
		"extra flag":         append(append([]string(nil), good...), "--yolo"),
		"empty":              {},
	} {
		require.False(t, SafeOMPModelSelectorRPCArgs(args), name)
	}
}

// concurrencyProbeRunner records how many sessions overlap so the worker
// bound is observable, and can corrupt one agent's answer.
type concurrencyProbeRunner struct {
	modelSelectorRPCFakeRunner
	inFlight atomic.Int64
	peak     atomic.Int64
	corrupt  string
}

func (runner *concurrencyProbeRunner) RunWithInput(
	ctx context.Context, executable string, input []byte, args ...string,
) ([]byte, error) {
	current := runner.inFlight.Add(1)
	defer runner.inFlight.Add(-1)
	for {
		peak := runner.peak.Load()
		if current <= peak || runner.peak.CompareAndSwap(peak, current) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	output, err := runner.modelSelectorRPCFakeRunner.RunWithInput(ctx, executable, input, args...)
	if err == nil && runner.corrupt != "" &&
		strings.Contains(strings.Join(args, " "), runner.corrupt) {
		output = []byte(strings.Replace(string(output), `"thinkingLevel":"xhigh"`, `"thinkingLevel":"low"`, 1))
	}
	return output, err
}

// One distinct selector per bundled agent, so the readback cannot pass by
// resolving a single shared selector.
func nativeAgentSelectors() map[string]string {
	models := []string{"claude-opus-5", "claude-fable-5-1", "claude-sonnet-5", "claude-haiku-5", "claude-astra-5"}
	selectors := make(map[string]string, len(models))
	for index, agent := range config.OMPNativeAgentNames() {
		selectors[agent] = "anthropic/" + models[index] + ":xhigh"
	}
	return selectors
}

func TestReadOMPModelAgentSelectorsViaRPC_BoundsInFlightSessionsToWorkerCount(t *testing.T) {
	t.Parallel()
	selectors := nativeAgentSelectors()
	runner := &concurrencyProbeRunner{
		modelSelectorRPCFakeRunner: modelSelectorRPCFakeRunner{selectors: selectors},
	}

	resolved, err := readOMPModelAgentSelectorsViaRPC(
		context.Background(), runner, "/tmp/config.yml", selectors,
	)

	require.NoError(t, err)
	require.Equal(t, selectors, resolved)
	require.Equal(t, int64(len(selectors)), runner.rpcCalls.Load())
	require.LessOrEqual(t, runner.peak.Load(), int64(ompModelSelectorRPCWorkers))
	require.Greater(t, runner.peak.Load(), int64(1), "readback must actually overlap sessions")
}

func TestReadOMPModelAgentSelectorsViaRPC_FailsWholeReadbackOnOneMismatch(t *testing.T) {
	t.Parallel()
	selectors := nativeAgentSelectors()
	runner := &concurrencyProbeRunner{
		modelSelectorRPCFakeRunner: modelSelectorRPCFakeRunner{selectors: selectors},
		corrupt:                    selectors["sonic"],
	}

	resolved, err := readOMPModelAgentSelectorsViaRPC(
		context.Background(), runner, "/tmp/config.yml", selectors,
	)

	require.Nil(t, resolved)
	require.ErrorContains(t, err, "activation agent readback mismatch: sonic")
}
