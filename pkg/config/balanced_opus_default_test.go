package config

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shippedBalancedHB7 is the seventeen-agent balanced default v0.50.126 through
// v0.50.133 wrote: the seven-role core on Fable. It is written out here
// instead of derived from the current defaults so the pins below cannot drift.
var shippedBalancedHB7 = map[string]string{
	"architect": "fable", "debugger": "fable", "deep-worker": "fable", "planner": "fable",
	"reviewer": "fable", "security-auditor": "fable", "spec-writer": "fable",
	"reverse-engineer": "opus",
	"annotator":        "sonnet", "devops": "sonnet", "executor": "sonnet", "explorer": "sonnet",
	"frontend-specialist": "sonnet", "perf-engineer": "sonnet", "tester": "sonnet",
	"ux-validator": "sonnet", "validator": "sonnet",
}

var balancedTopRungAgents = []string{
	"architect", "debugger", "deep-worker", "planner", "reviewer", "security-auditor", "spec-writer",
}

// The balanced top rung runs Opus 5.5 at max on Claude; Fable is Ultra's
// alone. The Codex top rung keeps Astra at max.
func TestBalancedDefault_TopRungIsOpusMaxOnClaudeAndAstraOnCodex(t *testing.T) {
	t.Parallel()
	q := DefaultFullConfig("opus").Quality
	for _, agent := range balancedTopRungAgents {
		assert.Equal(t, "opus", q.AgentTier(QualityProviderClaude, agent, "sonnet"), agent)
		model, effort := q.ClaudeAgentProfile(agent, "sonnet")
		assert.Equal(t, "claude-opus-5-5", model, agent)
		assert.Equal(t, "max", effort, agent)
		codex := q.CodexAgentProfile(agent, "sonnet", CodexEffortMedium)
		assert.Equal(t, CodexProfile{Model: "gpt-6-astra", Effort: "max"}, codex, agent)
	}
	for agent, tier := range q.Presets["balanced"].Agents {
		assert.NotEqual(t, "fable", tier, "balanced must not place %s on fable", agent)
	}
	ultra := q
	ultra.Default = "ultra"
	assert.Equal(t, "fable", ultra.AgentTier(QualityProviderClaude, "architect", "sonnet"), "ultra keeps its Fable core")
}

// An untouched v0.50.126-v0.50.133 balanced layout is a historical shipped
// default: it follows the new standard placement without a YAML rewrite.
func TestNativeBalancedAgentCandidate_PreviousFableDefaultFollowsOpus(t *testing.T) {
	t.Parallel()
	q := balancedOnly(shippedBalancedHB7)

	status, ok := ClassifyBuiltinQualityPreset(q, "balanced")
	require.True(t, ok)
	assert.Equal(t, BuiltinQualityPresetHistorical, status.Class)
	assert.Equal(t, "HB7", status.Row)
	assert.True(t, status.RuntimeStandard)

	for _, agent := range balancedTopRungAgents {
		model, effort := q.ClaudeAgentProfile(agent, "sonnet")
		assert.Equal(t, "claude-opus-5-5", model, agent)
		assert.Equal(t, "max", effort, agent)
		assert.Equal(t, "opus", q.EffectiveAgentTier(agent, "sonnet"), agent)
	}
	assert.Equal(t, maps.Clone(shippedBalancedHB7), q.Presets["balanced"].Agents, "the stored layout is not rewritten")
}

// The v0.50.116-v0.50.125 sixteen-agent layout follows the same way.
func TestNativeBalancedAgentCandidate_SixteenAgentFableDefaultFollowsOpus(t *testing.T) {
	t.Parallel()
	q := balancedOnly(shippedBalancedHB6)
	model, effort := q.ClaudeAgentProfile("reviewer", "sonnet")
	assert.Equal(t, "claude-opus-5-5", model)
	assert.Equal(t, "max", effort)
}

// A Fable the user set on purpose is not a shipped layout: it stays Fable,
// and its siblings keep the standard placement.
func TestNativeBalancedAgentCandidate_ExplicitFableStaysFable(t *testing.T) {
	t.Parallel()
	agents := defaultBalancedAgentTiers()
	agents["reviewer"] = "fable"
	q := balancedOnly(agents)

	_, standard := q.NativeBalancedAgentCandidate(QualityProviderClaude, "reviewer")
	assert.False(t, standard)
	model, effort := q.ClaudeAgentProfile("reviewer", "sonnet")
	assert.Equal(t, "fable", model)
	assert.Equal(t, "max", effort)
	assert.Equal(t, "fable", q.EffectiveAgentTier("reviewer", "sonnet"))
	assert.Equal(t, "claude-fable-5-1", q.ClaudeAgentModel("reviewer", "sonnet"))

	model, effort = q.ClaudeAgentProfile("architect", "sonnet")
	assert.Equal(t, "claude-opus-5-5", model)
	assert.Equal(t, "max", effort)
}

// The shipped claude review default moved to Opus; the Fable argv it replaced
// is a former default that is reported and never rewritten.
func TestClaudeProviderFormerDefault_ReportsThePreviousFableDefault(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--print", "--model", "claude-fable-5-1", "--effort", "max"},
		{"--print", "--model", "claude-fable-5-1", "--effort", "max", "--output-format", "json"},
		{"-p", "--model", "claude-fable-5-1", "--effort", "max"},
	} {
		entry := ProviderEntry{Binary: "claude", Args: args}
		model, ok := ClaudeProviderFormerDefault(entry)
		assert.True(t, ok, "%v", args)
		assert.Equal(t, "claude-fable-5-1", model, "%v", args)
	}
	_, ok := ClaudeProviderFormerDefault(ProviderEntry{Binary: "claude",
		Args: []string{"--print", "--model", "claude-fable-5-1", "--effort", "high"}})
	assert.False(t, ok, "an effort autopus never shipped is a user choice")
}

func TestMigrateOrchestraConfig_KeepsPreviousFableClaudeDefault(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--print", "--model", "claude-fable-5-1", "--effort", "max"},
		{"--print", "--model", "claude-fable-5-1", "--effort", "max", "--output-format", "json"},
	} {
		cfg := claudeMigrationConfig(ProviderEntry{Binary: "claude", Args: append([]string(nil), args...)})
		_, err := MigrateOrchestraConfig(cfg)
		require.NoError(t, err)
		want := []string{"--print", "--model", "claude-fable-5-1", "--effort", "max", "--output-format", "json"}
		assert.Equal(t, want, cfg.Orchestra.Providers["claude"].Args)

		require.NoError(t, EnsureOrchestraProvider(cfg, "claude"))
		assert.Equal(t, want, cfg.Orchestra.Providers["claude"].Args)
	}
}

// customTopRungQuality binds a custom preset copied from the balanced default,
// so every agent sits off the standard placement.
func customTopRungQuality(efforts map[string]string) QualityConf {
	q := DefaultFullConfig("custom-top").Quality
	// The custom preset spells out every role, as a hand-written copy of the
	// seventeen-agent layout does.
	q.Presets["custom"] = QualityPreset{Agents: maps.Clone(shippedBalancedHB8), Efforts: efforts}
	q.Default = "custom"
	return q
}

// Off the standard placement, a top-rung agent on the top tier keeps the
// balanced top-rung model and its max thinking on both tools.
func TestTopRungOpus_OffTheStandardPlacementKeepsTheRungCandidate(t *testing.T) {
	t.Parallel()
	q := customTopRungQuality(nil)
	for _, agent := range balancedTopRungAgents {
		_, standard := q.NativeBalancedAgentCandidate(QualityProviderClaude, agent)
		require.False(t, standard, agent)
		model, effort := q.ClaudeAgentProfile(agent, "sonnet")
		assert.Equal(t, [2]string{"claude-opus-5-5", "max"}, [2]string{model, effort}, agent)
		assert.Equal(t, "claude-opus-5-5", q.ClaudeAgentModel(agent, "sonnet"), agent)
		assert.Equal(t, CodexProfile{Model: "gpt-6-astra", Effort: "max"},
			q.CodexAgentProfile(agent, "sonnet", CodexEffortMedium), agent)
	}
}

// An explicit effort keeps the rung model and applies the chosen level.
func TestTopRungOpus_ExplicitEffortKeepsTheRungModel(t *testing.T) {
	t.Parallel()
	q := customTopRungQuality(map[string]string{"planner": "high"})
	model, effort := q.ClaudeAgentProfile("planner", "opus")
	assert.Equal(t, [2]string{"claude-opus-5-5", "high"}, [2]string{model, effort})
	assert.Equal(t, CodexProfile{Model: "gpt-6-astra", Effort: "high"}, q.CodexAgentProfile("planner", "opus", "max"))

	balanced := DefaultFullConfig("balanced-effort").Quality
	balanced.Presets["balanced"] = QualityPreset{Agents: defaultBalancedAgentTiers(), Efforts: map[string]string{"planner": "high"}}
	model, effort = balanced.ClaudeAgentProfile("planner", "opus")
	assert.Equal(t, [2]string{"claude-opus-5-5", "high"}, [2]string{model, effort})
	assert.Equal(t, CodexProfile{Model: "gpt-6-astra", Effort: "high"}, balanced.CodexAgentProfile("planner", "opus", "max"))
}

// Analysis-rung and other agents on opus, and Ultra, keep the tier ladder.
func TestTopRungOpus_OtherRungsAndUltraKeepTheLadder(t *testing.T) {
	t.Parallel()
	q := customTopRungQuality(nil)
	q.Presets["custom"].Agents["executor"] = "opus"
	for _, agent := range []string{"reverse-engineer", "executor"} {
		model, effort := q.ClaudeAgentProfile(agent, "sonnet")
		assert.Equal(t, [2]string{"opus", "high"}, [2]string{model, effort}, agent)
		assert.Equal(t, CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"},
			q.CodexAgentProfile(agent, "sonnet", CodexEffortMedium), agent)
	}

	ultra := DefaultFullConfig("ultra-top").Quality
	ultra.Default = "ultra"
	ultra.Presets["ultra"].Agents["planner"] = "opus"
	model, effort := ultra.ClaudeAgentProfile("planner", "sonnet")
	assert.Equal(t, [2]string{"opus", "max"}, [2]string{model, effort})
	assert.Equal(t, CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"}, ultra.CodexAgentProfile("planner", "sonnet", CodexEffortMedium))
}
