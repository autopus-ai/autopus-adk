package omp

// SPEC-MODELUX-003 S9 through the adapter: in autopus mode the activation
// reads every one of the 17 routed agents back in its own RPC session, and
// one disagreeing session fails the whole activation with nothing left
// behind.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ompAutopusRPCRunner is the integration fake plus the get_state RPC session
// the readback prefers. A session reports the selector it was started with,
// except a corrupt one, which reports another thinking level.
type ompAutopusRPCRunner struct {
	*modelIntegrationFakeRunner
	mu       sync.Mutex
	sessions []string
	corrupt  string
}

func (r *ompAutopusRPCRunner) RunWithInput(_ context.Context, executable string, input []byte, args ...string) ([]byte, error) {
	if executable != cliBinary || string(input) != `{"id":"autopus-model-state","type":"get_state"}`+"\n" ||
		!SafeOMPModelSelectorRPCArgs(args) {
		return nil, fmt.Errorf("unexpected RPC request %v", args)
	}
	requested := args[3]
	r.mu.Lock()
	r.sessions = append(r.sessions, requested)
	r.mu.Unlock()
	selector, thinking, err := splitOMPProjectedSelector(requested)
	if err != nil {
		return nil, err
	}
	if requested == r.corrupt {
		thinking = "low"
	}
	provider, model, ok := parseOMPRoutingSelector(selector)
	if !ok {
		return nil, fmt.Errorf("invalid selector %q", selector)
	}
	encoded, err := json.Marshal(map[string]any{
		"id": "autopus-model-state", "type": "response", "command": "get_state", "success": true,
		"data": map[string]any{"model": map[string]any{"provider": provider, "id": model}, "thinkingLevel": thinking},
	})
	return append(encoded, '\n'), err
}

func (r *ompAutopusRPCRunner) configGets() []string {
	var keys []string
	for _, call := range r.calls {
		if key := integrationConfigGetKey(call); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func ompProjectOverrides(t *testing.T, p *claimProject) map[string]string {
	t.Helper()
	data, ok := p.read(configFile)
	require.True(t, ok)
	value, err := integrationYAMLPath([]byte(data), config.OMPNativeAgentModelOverridesKey)
	require.NoError(t, err)
	overrides := map[string]string{}
	for agent, selector := range value.(map[string]any) {
		overrides[agent] = selector.(string)
	}
	return overrides
}

func TestOMPModelActivation_S9_AutopusReadsEveryAgentBackOverRPC(t *testing.T) {
	p := newClaimProject(t)
	cfg := integrationHarnessConfig(config.RoleModelConfigModeProjectManaged)
	require.NoError(t, config.Save(p.root, cfg))
	runner := &ompAutopusRPCRunner{modelIntegrationFakeRunner: newModelIntegrationRunner()}

	_, err := NewWithRoot(p.root).WithModelIntegrationRunner(runner).Generate(context.Background(), cfg)

	require.NoError(t, err)
	overrides := ompProjectOverrides(t, p)
	agents := make([]string, 0, len(overrides))
	selectors := make([]string, 0, len(overrides))
	for agent, selector := range overrides {
		agents, selectors = append(agents, agent), append(selectors, selector)
	}
	sort.Strings(agents)
	want := config.OMPAutopusAgentNames()
	sort.Strings(want)
	assert.Equal(t, want, agents, "the 16 canonical agents and task, no bundled scout, security-reviewer, or sonic")
	assert.ElementsMatch(t, selectors, runner.sessions, "one RPC session per routed agent, on its override")
	reads := 0
	for _, key := range runner.configGets() {
		if key == config.OMPNativeAgentModelOverridesKey {
			reads++
		}
	}
	assert.Equal(t, 1, reads, "the override record is read back once with config get")
}

func TestOMPModelActivation_S9_OneDisagreeingSessionFailsTheActivation(t *testing.T) {
	p := newClaimProject(t)
	p.mustRun("generate", configForOMP())
	before := claimSnapshot(t, p)
	cfg := integrationHarnessConfig(config.RoleModelConfigModeProjectManaged)
	require.NoError(t, config.Save(p.root, cfg))
	runner := &ompAutopusRPCRunner{modelIntegrationFakeRunner: newModelIntegrationRunner(), corrupt: "anthropic/alpha-reasoner:xhigh"}

	_, err := NewWithRoot(p.root).WithModelIntegrationRunner(runner).Generate(context.Background(), cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "activation agent readback mismatch")
	assert.Equal(t, before, claimSnapshot(t, p), ".omp, the overlay, and the ledger are as before")
	assert.True(t, strings.Contains(strings.Join(runner.sessions, ","), runner.corrupt))
}
