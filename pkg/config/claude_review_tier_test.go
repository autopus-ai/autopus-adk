package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClaudeProviderModel_ReadsTheCLIModelFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		entry  ProviderEntry
		want   string
		wantOK bool
	}{
		{name: "shipped default", entry: DefaultClaudeProviderEntry(), want: ClaudeFableModel, wantOK: true},
		{name: "former default", entry: ProviderEntry{Binary: "claude", Args: []string{"-p", "--model", "opus", "--effort", "max"}}, want: "opus", wantOK: true},
		{name: "equals spelling", entry: ProviderEntry{Binary: "claude", Args: []string{"--print", "--model=sonnet"}}, want: "sonnet", wantOK: true},
		{name: "model-less print", entry: ProviderEntry{Binary: "claude", Args: []string{"--print"}}},
		{name: "dangling flag", entry: ProviderEntry{Binary: "claude", Args: []string{"--print", "--model"}}},
		{name: "backend routed", entry: ProviderEntry{Backend: ProviderBackendOMP, Model: "anthropic/" + ClaudeFableModel}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ClaudeProviderModel(tt.entry)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClaudeModelTier_FoldsAliasesAndSlugs(t *testing.T) {
	t.Parallel()

	for model, want := range map[string]string{
		"opus": "opus", "Opus": "opus", "opus[1m]": "opus", "claude-opus-4-8": "opus", ClaudeOpusModel: "opus",
		"fable": "fable", ClaudeFableModel: "fable", ClaudeSonnetModel: "sonnet", "haiku": "haiku",
	} {
		got, ok := ClaudeModelTier(model)
		assert.True(t, ok, model)
		assert.Equal(t, want, got, model)
	}
	for _, model := range []string{"", "gpt-6", "claude-mystery-1", "${CLAUDE_MODEL}", "opusx"} {
		_, ok := ClaudeModelTier(model)
		assert.False(t, ok, model)
	}
}

func TestQualityConf_ClaudeTopTier_FollowsTheEffectiveClaudePreset(t *testing.T) {
	t.Parallel()

	allOpus := map[string]string{}
	for _, name := range CanonicalAgentNames() {
		allOpus[name] = "opus"
	}
	defaults := DefaultFullConfig("p").Quality

	assert.Equal(t, "fable", defaults.ClaudeTopTier(), "shipped balanced puts its top rung on fable")

	custom := defaults
	custom.Presets = map[string]QualityPreset{"balanced": defaults.Presets["balanced"], "claude-custom": {Agents: allOpus}}
	custom.Providers = map[string]string{QualityProviderClaude: "claude-custom"}
	assert.Equal(t, "opus", custom.ClaudeTopTier())

	ultra := defaults
	ultra.Default = "ultra"
	assert.Equal(t, "fable", ultra.ClaudeTopTier())
}
