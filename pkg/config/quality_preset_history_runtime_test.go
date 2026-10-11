package config

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Shipped balanced layouts as SPEC-MODELUX-001's Historical Preset Registry
// records them, restored from git history. They are written out here instead
// of read from the registry so the runtime pins below cannot drift with it.
var (
	shippedBalancedHB1 = map[string]string{
		"planner": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"executor": "sonnet", "tester": "sonnet", "reviewer": "sonnet", "debugger": "sonnet", "devops": "sonnet",
		"validator": "haiku", "explorer": "haiku",
	}
	shippedBalancedHB2 = map[string]string{
		"planner": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"executor": "sonnet", "tester": "sonnet", "reviewer": "sonnet", "debugger": "sonnet", "devops": "sonnet",
		"validator": "sonnet", "explorer": "sonnet",
	}
	shippedBalancedHB4 = map[string]string{
		"architect": "opus", "deep-worker": "opus", "executor": "opus",
		"planner": "opus", "security-auditor": "opus", "spec-writer": "opus",
		"annotator": "sonnet", "debugger": "sonnet", "devops": "sonnet", "explorer": "sonnet",
		"frontend-specialist": "sonnet", "perf-engineer": "sonnet", "reviewer": "sonnet",
		"tester": "sonnet", "ux-validator": "sonnet", "validator": "sonnet",
	}
	shippedBalancedHB5 = map[string]string{
		"architect": "fable", "planner": "fable", "security-auditor": "fable",
		"debugger": "opus", "deep-worker": "opus", "executor": "opus", "reviewer": "opus", "spec-writer": "opus",
		"annotator": "sonnet", "devops": "sonnet", "explorer": "sonnet", "frontend-specialist": "sonnet",
		"perf-engineer": "sonnet", "tester": "sonnet", "ux-validator": "sonnet", "validator": "sonnet",
	}
	shippedUltraHU1 = map[string]string{
		"planner": "opus", "executor": "opus", "validator": "opus", "tester": "opus",
		"reviewer": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"debugger": "opus", "explorer": "opus", "devops": "opus",
	}
	shippedUltraHU2 = map[string]string{
		"annotator": "opus", "architect": "opus", "debugger": "opus", "deep-worker": "opus",
		"devops": "opus", "executor": "opus", "explorer": "opus", "frontend-specialist": "opus",
		"perf-engineer": "opus", "planner": "opus", "reviewer": "opus", "security-auditor": "opus",
		"spec-writer": "opus", "tester": "opus", "ux-validator": "opus", "validator": "opus",
	}
)

func balancedOnly(agents map[string]string) QualityConf {
	return QualityConf{Default: "balanced", Presets: map[string]QualityPreset{
		"balanced": {Agents: maps.Clone(agents)},
	}}
}

func TestIsHistoricalBalancedDefault_OnlyRuntimeStandardRowsMatch(t *testing.T) {
	tests := []struct {
		name   string
		agents map[string]string
		want   bool
	}{
		{name: "HB2 stays runtime standard", agents: shippedBalancedHB2, want: true},
		{name: "HB5 stays runtime standard", agents: shippedBalancedHB5, want: true},
		{name: "HB1 is classification only", agents: shippedBalancedHB1, want: false},
		{name: "HB4 is classification only", agents: shippedBalancedHB4, want: false},
		{name: "HU1 is an ultra layout", agents: shippedUltraHU1, want: false},
		{name: "HU2 is an ultra layout", agents: shippedUltraHU2, want: false},
		{name: "current balanced default", agents: defaultBalancedAgentTiers(), want: false},
		{name: "missing agents", agents: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, isHistoricalBalancedDefault(maps.Clone(test.agents)))
		})
	}
}

// TestNativeBalancedAgentCandidate_ClassificationOnlyRowsKeepRuntime pins the
// pre-registry runtime answer for the HB1 and HB4 layouts: only agents whose
// written tier already equals their rung tier (or that the layout omits) take
// the standard placement; every other agent stays on the custom-tier path.
func TestNativeBalancedAgentCandidate_ClassificationOnlyRowsKeepRuntime(t *testing.T) {
	const (
		claudeTop     = "anthropic/claude-fable-5-1@max"
		claudeImpl    = "anthropic/claude-sonnet-5-5@max"
		claudeRoutine = "anthropic/claude-sonnet-5-5@high"
		codexTop      = "openai-codex/gpt-6-astra@max"
		codexRest     = "openai-codex/gpt-6-luna@max"
	)
	tests := []struct {
		name     string
		agents   map[string]string
		standard map[string][2]string // agent -> {claude, codex}; absent means custom-tier path
	}{
		{
			name:   "HB1",
			agents: shippedBalancedHB1,
			standard: map[string][2]string{
				"deep-worker": {claudeTop, codexTop},
				"devops":      {claudeImpl, codexRest}, "executor": {claudeImpl, codexRest},
				"frontend-specialist": {claudeImpl, codexRest}, "perf-engineer": {claudeImpl, codexRest},
				"tester":    {claudeImpl, codexRest},
				"annotator": {claudeRoutine, codexRest}, "ux-validator": {claudeRoutine, codexRest},
			},
		},
		{
			name:   "HB4",
			agents: shippedBalancedHB4,
			standard: map[string][2]string{
				"devops": {claudeImpl, codexRest}, "frontend-specialist": {claudeImpl, codexRest},
				"perf-engineer": {claudeImpl, codexRest}, "tester": {claudeImpl, codexRest},
				"annotator": {claudeRoutine, codexRest}, "explorer": {claudeRoutine, codexRest},
				"ux-validator": {claudeRoutine, codexRest}, "validator": {claudeRoutine, codexRest},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			q := balancedOnly(test.agents)
			for _, agent := range CanonicalAgentNames() {
				want, standard := test.standard[agent]
				claude, claudeOK := q.NativeBalancedAgentCandidate(QualityProviderClaude, agent)
				codex, codexOK := q.NativeBalancedAgentCandidate(QualityProviderCodex, agent)
				assert.Equal(t, standard, claudeOK, "claude %s", agent)
				assert.Equal(t, standard, codexOK, "codex %s", agent)
				if standard {
					assert.Equal(t, want[0], claude.Selector+"@"+claude.Thinking, agent)
					assert.Equal(t, want[1], codex.Selector+"@"+codex.Thinking, agent)
				}
			}
		})
	}
}

func TestNativeBalancedAgentCandidate_RuntimeStandardRowsTakeEveryRung(t *testing.T) {
	for name, agents := range map[string]map[string]string{"HB2": shippedBalancedHB2, "HB5": shippedBalancedHB5} {
		t.Run(name, func(t *testing.T) {
			q := balancedOnly(agents)
			for _, agent := range CanonicalAgentNames() {
				_, claudeOK := q.NativeBalancedAgentCandidate(QualityProviderClaude, agent)
				_, codexOK := q.NativeBalancedAgentCandidate(QualityProviderCodex, agent)
				assert.True(t, claudeOK, "claude %s", agent)
				assert.True(t, codexOK, "codex %s", agent)
			}
		})
	}
}
