package config

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
)

// sourceFallbackTiers mirrors the `model:` line of content/agents/<agent>.md,
// the fallback tier the Claude generator passes for an agent a preset omits.
var sourceFallbackTiers = map[string]string{
	"annotator": "sonnet", "architect": "opus", "debugger": "sonnet", "deep-worker": "opus",
	"devops": "sonnet", "executor": "sonnet", "explorer": "sonnet", "frontend-specialist": "sonnet",
	"perf-engineer": "sonnet", "planner": "opus", "reviewer": "sonnet", "security-auditor": "opus",
	"spec-writer": "opus", "tester": "sonnet", "ux-validator": "sonnet", "validator": "sonnet",
}

// repositoryQuality is fixture R's quality: default ultra, balanced HB2, ultra HU1.
func repositoryQuality() QualityConf {
	return QualityConf{Default: "ultra", Presets: map[string]QualityPreset{
		"balanced": {Agents: maps.Clone(shippedBalancedHB2)},
		"ultra":    {Agents: maps.Clone(shippedUltraHU1)},
	}}
}

func withBalanced(agents map[string]string) QualityConf {
	q := DefaultFullConfig("effective").Quality
	q.Presets["balanced"] = QualityPreset{Agents: maps.Clone(agents)}
	return q
}

func TestEffectiveAgentTier_StandardBalancedReportsRungTier(t *testing.T) {
	q := DefaultFullConfig("effective").Quality
	want := map[string]string{
		"planner": "fable", "architect": "fable", "spec-writer": "fable", "security-auditor": "fable",
		"debugger": "fable", "deep-worker": "fable", "reviewer": "fable",
		"executor": "sonnet", "devops": "sonnet", "frontend-specialist": "sonnet", "perf-engineer": "sonnet",
		"tester": "sonnet", "validator": "sonnet", "ux-validator": "sonnet", "explorer": "sonnet", "annotator": "sonnet",
	}
	for _, agent := range CanonicalAgentNames() {
		assert.Equal(t, want[agent], q.EffectiveAgentTier(agent, sourceFallbackTiers[agent]), agent)
	}
}

func TestEffectiveAgentTier_WrittenTierAndFallback(t *testing.T) {
	invalidTester := repositoryQuality()
	invalidTester.Presets["ultra"].Agents["tester"] = "gpt"
	tests := []struct {
		name  string
		q     QualityConf
		agent string
		want  string
	}{
		// HB2 is runtime standard: the generator ignores the written sonnet.
		{name: "HB2 debugger takes the top rung", q: withBalanced(shippedBalancedHB2), agent: "debugger", want: "fable"},
		{name: "HB2 planner takes the top rung", q: withBalanced(shippedBalancedHB2), agent: "planner", want: "fable"},
		// HB1 is classification only: a written off-rung tier keeps its tier.
		{name: "HB1 planner keeps opus", q: withBalanced(shippedBalancedHB1), agent: "planner", want: "opus"},
		{name: "HB1 validator keeps haiku", q: withBalanced(shippedBalancedHB1), agent: "validator", want: "haiku"},
		{name: "HB1 omitted deep-worker takes its rung", q: withBalanced(shippedBalancedHB1), agent: "deep-worker", want: "fable"},
		{name: "ultra written tier", q: repositoryQuality(), agent: "planner", want: "opus"},
		{name: "ultra omitted agent uses source fallback", q: repositoryQuality(), agent: "deep-worker", want: "opus"},
		{name: "ultra omitted routine agent uses source fallback", q: repositoryQuality(), agent: "annotator", want: "sonnet"},
		{name: "invalid written tier uses source fallback", q: invalidTester, agent: "tester", want: "sonnet"},
		{name: "workflow spelling resolves the agent", q: repositoryQuality(), agent: "spec_writer", want: "opus"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			agent := NormalizeAgentName(test.agent)
			assert.Equal(t, test.want, test.q.EffectiveAgentTier(test.agent, sourceFallbackTiers[agent]))
		})
	}
}

// TestEffectiveAgentTier_IgnoresProviderOverrides pins the global view: a
// provider override decides only that platform's column, never the tier the
// composition starts from. A column view passed in keeps its own mode.
func TestEffectiveAgentTier_IgnoresProviderOverrides(t *testing.T) {
	balanced := DefaultFullConfig("override").Quality
	balanced.Providers = map[string]string{QualityProviderClaude: "ultra"}
	assert.Equal(t, "sonnet", balanced.EffectiveAgentTier("tester", "sonnet"))
	assert.Equal(t, "opus", balanced.ForProvider(QualityProviderClaude).EffectiveAgentTier("tester", "sonnet"))

	ultra := DefaultFullConfig("override").Quality
	ultra.Default = "ultra"
	ultra.Providers = map[string]string{QualityProviderCodex: "balanced"}
	assert.Equal(t, "opus", ultra.EffectiveAgentTier("tester", "sonnet"))
	assert.Equal(t, "sonnet", ultra.ForProvider(QualityProviderCodex).EffectiveAgentTier("tester", "sonnet"))
}

func TestClaudeAgentProfile_MatchesGeneratedFrontmatterValues(t *testing.T) {
	claudeUltra := DefaultFullConfig("profile").Quality
	claudeUltra.Providers = map[string]string{QualityProviderClaude: "ultra"}
	codexOnly := DefaultFullConfig("profile").Quality
	codexOnly.Presets["codex-custom"] = QualityPreset{Agents: map[string]string{"planner": "haiku"}}
	codexOnly.Providers = map[string]string{QualityProviderCodex: "codex-custom"}
	tests := []struct {
		name       string
		q          QualityConf
		agent      string
		wantModel  string
		wantEffort string
	}{
		{name: "standard top rung", q: DefaultFullConfig("profile").Quality, agent: "planner", wantModel: "claude-fable-5-1", wantEffort: "max"},
		{name: "standard implementation rung", q: DefaultFullConfig("profile").Quality, agent: "executor", wantModel: "claude-sonnet-5-5", wantEffort: "max"},
		{name: "standard routine rung", q: DefaultFullConfig("profile").Quality, agent: "validator", wantModel: "claude-sonnet-5-5", wantEffort: "high"},
		{name: "HB1 planner alias", q: withBalanced(shippedBalancedHB1), agent: "planner", wantModel: "opus", wantEffort: "high"},
		{name: "HB1 debugger alias", q: withBalanced(shippedBalancedHB1), agent: "debugger", wantModel: "sonnet", wantEffort: "medium"},
		{name: "HB1 validator haiku has no effort", q: withBalanced(shippedBalancedHB1), agent: "validator", wantModel: "haiku", wantEffort: ""},
		{name: "ultra written opus", q: repositoryQuality(), agent: "planner", wantModel: "opus", wantEffort: "max"},
		{name: "ultra fallback opus", q: repositoryQuality(), agent: "deep-worker", wantModel: "opus", wantEffort: "max"},
		{name: "ultra fallback sonnet", q: repositoryQuality(), agent: "annotator", wantModel: "sonnet", wantEffort: "high"},
		{name: "claude override wins", q: claudeUltra, agent: "tester", wantModel: "opus", wantEffort: "max"},
		{name: "codex override leaves claude alone", q: codexOnly, agent: "planner", wantModel: "claude-fable-5-1", wantEffort: "max"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, effort := test.q.ClaudeAgentProfile(test.agent, sourceFallbackTiers[test.agent])
			assert.Equal(t, test.wantModel, model)
			assert.Equal(t, test.wantEffort, effort)
		})
	}
}

func TestClaudeAgentEffort_FourTierMatrix(t *testing.T) {
	tests := []struct {
		mode, tier, want string
	}{
		{mode: "ultra", tier: "fable", want: "max"},
		{mode: "ultra", tier: "opus", want: "max"},
		{mode: "ultra", tier: "sonnet", want: "high"},
		{mode: "ultra", tier: "haiku", want: ""},
		{mode: "balanced", tier: "fable", want: "max"},
		{mode: "balanced", tier: "opus", want: "high"},
		{mode: "balanced", tier: "sonnet", want: "medium"},
		{mode: "balanced", tier: "haiku", want: ""},
		{mode: "custom", tier: "opus", want: "high"},
		{mode: "custom", tier: "sonnet", want: "medium"},
	}
	for _, test := range tests {
		assert.Equal(t, test.want, ClaudeAgentEffort(test.mode, test.tier), "%s/%s", test.mode, test.tier)
	}
}
