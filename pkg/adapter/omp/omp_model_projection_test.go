package omp

import (
	"math/rand/v2"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileOMPModelProjection_BindsOneSelectorPerServedAgent(t *testing.T) {
	t.Parallel()

	projection, err := CompileOMPModelProjection(ompProjectionFixture(t))
	require.NoError(t, err)

	const (
		coder    = "openai/beta-coder:high"
		reasoner = "anthropic/alpha-reasoner:xhigh"
		dissent  = "anthropic/alpha-reasoner:high"
	)
	assert.Equal(t, []OMPAgentModelProjection{
		{Agent: "explorer", Role: "autopus_explorer", Capability: "fast_validation",
			Thinking: "high", EffectiveSelector: coder},
		{Agent: "reviewer", Role: "autopus_reviewer", Capability: "independent_dissent",
			Thinking: "high", EffectiveSelector: dissent},
		{Agent: "security-auditor", Role: "autopus_security_auditor", Capability: "independent_dissent",
			Thinking: "high", EffectiveSelector: dissent},
		{Agent: "validator", Role: "autopus_validator", Capability: "deterministic_transform",
			Thinking: "high", EffectiveSelector: coder},
		{Agent: "task", Role: "autopus_planner", Capability: "deep_reasoning",
			Thinking: "xhigh", EffectiveSelector: reasoner},
	}, projection.Agents)

	assert.Equal(t, []OMPFallbackChainProjection{
		{Selector: reasoner, Candidates: []string{"openai/beta-coder:high", "anthropic/omega-reasoner:high"}},
	}, projection.FallbackChains)
}

func TestCompileOMPModelProjection_UnresolvedAgentsInheritRuntimeDefaults(t *testing.T) {
	t.Parallel()

	input := ompProjectionFixture(t)
	input.Agents = ompProjectionWithoutAgents(input.Agents, "reviewer", "security-auditor")
	projection, err := CompileOMPModelProjection(input)
	require.NoError(t, err)

	require.Len(t, projection.Agents, 3)
	for _, agent := range projection.Agents {
		assert.NotEqual(t, "reviewer", agent.Agent)
		assert.NotEqual(t, "security-auditor", agent.Agent)
	}
	overlay, err := OMPModelOverlayFromProjection(projection)
	require.NoError(t, err)
	assert.NotContains(t, overlay.AgentModelOverrides, "reviewer")
	assert.NotContains(t, overlay.AgentModelOverrides, "security-auditor")
}

func TestCompileOMPModelProjection_AcceptsNativeThinkingLevels(t *testing.T) {
	t.Parallel()

	for _, thinking := range []string{"off", "none", "minimal", "low", "medium", "high", "xhigh", "max", "auto"} {
		input := ompProjectionFixture(t)
		input.Agents[0].Thinking = thinking
		projection, err := CompileOMPModelProjection(input)
		require.NoError(t, err, thinking)
		require.Equal(t, "explorer", projection.Agents[0].Agent)
		assert.Equal(t, "openai/beta-coder:"+thinking, projection.Agents[0].EffectiveSelector)
		assert.Equal(t, thinking, projection.Agents[0].Thinking)
	}
}

func TestOMPModelOverlayFromProjection_ReturnsDetachedOverrides(t *testing.T) {
	t.Parallel()

	projection, err := CompileOMPModelProjection(ompProjectionFixture(t))
	require.NoError(t, err)
	overlay, err := OMPModelOverlayFromProjection(projection)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"explorer":         "openai/beta-coder:high",
		"reviewer":         "anthropic/alpha-reasoner:high",
		"security-auditor": "anthropic/alpha-reasoner:high",
		"task":             "anthropic/alpha-reasoner:xhigh",
		"validator":        "openai/beta-coder:high",
	}, overlay.AgentModelOverrides)
	assert.Equal(t, []string{"openai/beta-coder:high", "anthropic/omega-reasoner:high"},
		overlay.FallbackChains["anthropic/alpha-reasoner:xhigh"])

	overlay.AgentModelOverrides["task"] = "mutated"
	overlay.FallbackChains["anthropic/alpha-reasoner:xhigh"][0] = "mutated"
	assert.Equal(t, "anthropic/alpha-reasoner:xhigh", projection.Agents[4].EffectiveSelector)
	assert.Equal(t, "openai/beta-coder:high", projection.FallbackChains[0].Candidates[0])
}

// An entry may only name the agent its own role row governs: another role.s
// agent, a bundled agent OMP no longer serves, or an unknown name binds no
// model the operator chose for it.
func TestCompileOMPModelProjection_RejectsAnAgentItsRoleDoesNotGovern(t *testing.T) {
	t.Parallel()

	for _, agent := range []string{"planner", "executor", "security-auditor", "scout", "security-reviewer", "sonic", "future-agent"} {
		input := ompProjectionFixture(t)
		input.Agents[0].Agent = agent
		_, err := CompileOMPModelProjection(input)
		require.Error(t, err, agent)
		assert.Contains(t, err.Error(), "agent_role_unmapped", agent)
	}
}

func TestCompileOMPModelProjection_RejectsSecondEntryForOneAgent(t *testing.T) {
	t.Parallel()

	input := ompProjectionFixture(t)
	duplicate := input.Agents[3]
	input.Agents = append(input.Agents, duplicate)

	_, err := CompileOMPModelProjection(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_duplicate")
}

func TestCompileOMPModelProjection_RejectsRoleThatCollapsesElsewhere(t *testing.T) {
	t.Parallel()

	input := ompProjectionFixture(t)
	// autopus_reviewer governs the reviewer agent, never explorer.
	input.Agents[0].Role = config.OMPAgentRoleName("reviewer")
	_, err := CompileOMPModelProjection(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_role_unmapped")
}

func TestCompileOMPModelProjection_RejectsCapabilityDisagreeingWithRole(t *testing.T) {
	t.Parallel()

	input := ompProjectionFixture(t)
	input.Agents[3].Capability = config.CapabilityCodingToolUse
	_, err := CompileOMPModelProjection(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role_capability_mismatch")
}

func TestCompileOMPModelProjection_IsDeterministicAcrossInputOrder(t *testing.T) {
	t.Parallel()

	want, err := CompileOMPModelProjection(ompProjectionFixture(t))
	require.NoError(t, err)

	for seed := uint64(0); seed < 100; seed++ {
		shuffled := ompProjectionFixture(t)
		rng := rand.New(rand.NewPCG(seed, 0)) // #nosec G404 -- deterministic test permutation only.
		rng.Shuffle(len(shuffled.Agents), func(i, j int) {
			shuffled.Agents[i], shuffled.Agents[j] = shuffled.Agents[j], shuffled.Agents[i]
		})
		got, compileErr := CompileOMPModelProjection(shuffled)
		require.NoError(t, compileErr)
		assert.Equal(t, want, got, "seed=%d", seed)
	}
}

func TestCompileOMPModelProjection_SecurityRejectsRawTierAndInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		selector string
		thinking string
	}{
		{name: "raw opus", selector: "opus", thinking: "high"},
		{name: "raw sonnet", selector: "sonnet", thinking: "high"},
		{name: "raw haiku", selector: "haiku", thinking: "low"},
		{name: "selector newline", selector: "openai/beta\nmodel: evil", thinking: "high"},
		{name: "thinking newline", selector: "openai/beta-coder", thinking: "high\ntools: [bash]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := ompProjectionFixture(t)
			input.Agents[0].Selector = tt.selector
			input.Agents[0].Thinking = tt.thinking
			_, err := CompileOMPModelProjection(input)
			require.Error(t, err)
		})
	}
}

// The emitted value is a concrete selector, so a projection whose recorded
// thinking level disagrees with its selector suffix must not reach OMP config.
func TestOMPModelOverlayFromProjection_RejectsThinkingSuffixMismatch(t *testing.T) {
	t.Parallel()

	projection, err := CompileOMPModelProjection(ompProjectionFixture(t))
	require.NoError(t, err)
	projection.Agents[0].EffectiveSelector = "openai/beta-coder:max"
	_, err = OMPModelOverlayFromProjection(projection)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_projection_mismatch")
}

func TestOMPModelOverlayFromProjection_RejectsSelectorWithoutThinking(t *testing.T) {
	t.Parallel()

	projection, err := CompileOMPModelProjection(ompProjectionFixture(t))
	require.NoError(t, err)
	projection.Agents[0].EffectiveSelector = "openai/beta-coder"
	_, err = OMPModelOverlayFromProjection(projection)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "selector_invalid")
}

// ompProjectionFixture resolves five served agents through their own ADK
// role capability.
func ompProjectionFixture(t *testing.T) OMPModelProjectionInput {
	t.Helper()
	byCapability := map[string]OMPProjectionAgent{
		config.CapabilityDeepReasoning: {Selector: "anthropic/alpha-reasoner", Thinking: "xhigh", Fallbacks: []OMPProjectionCandidate{
			{Selector: "openai/beta-coder", Thinking: "high"},
			{Selector: "anthropic/omega-reasoner", Thinking: "high"},
		}},
		config.CapabilityFastValidation:         {Selector: "openai/beta-coder", Thinking: "high"},
		config.CapabilityIndependentDissent:     {Selector: "anthropic/alpha-reasoner", Thinking: "high"},
		config.CapabilityDeterministicTransform: {Selector: "openai/beta-coder", Thinking: "high"},
	}
	// The agents that represented the five bundled agents before v0.50.130,
	// in their old fixture order: task stays at index 3.
	names := []string{"explorer", "reviewer", "security-auditor", "task", "validator"}
	agents := make([]OMPProjectionAgent, 0, len(names))
	for _, name := range names {
		resolved, err := config.ResolveOMPPolicyAgent(name)
		require.NoError(t, err)
		entry := byCapability[resolved.Capability]
		entry.Agent, entry.Role, entry.Capability = name, resolved.Role, resolved.Capability
		entry.Fallbacks = append([]OMPProjectionCandidate(nil), entry.Fallbacks...)
		agents = append(agents, entry)
	}
	return OMPModelProjectionInput{Agents: agents}
}

func ompProjectionWithoutAgents(agents []OMPProjectionAgent, names ...string) []OMPProjectionAgent {
	excluded := make(map[string]struct{}, len(names))
	for _, name := range names {
		excluded[name] = struct{}{}
	}
	kept := make([]OMPProjectionAgent, 0, len(agents))
	for _, agent := range agents {
		if _, skip := excluded[agent.Agent]; !skip {
			kept = append(kept, agent)
		}
	}
	return kept
}
