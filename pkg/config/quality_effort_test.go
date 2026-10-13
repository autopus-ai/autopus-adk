package config

// SPEC-MODELUX-003 T2 (REQ-MUX3-01, REQ-MUX3-02; acceptance S1, S2): the
// optional quality.presets.<name>.efforts mapping, its load validation, and
// the explicit effort every native projection reads.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// effortFixture is fixture D (the saved code defaults with Claude Code and
// Codex) whose Claude column follows claude-custom: every agent on sonnet
// with the given efforts.
func effortFixture(efforts map[string]string) *HarnessConfig {
	cfg := DefaultFullConfig("p")
	cfg.Platforms = []string{"claude-code", "codex"}
	cfg.Quality.SupervisorModelPolicy = SupervisorModelPolicyInherit
	sonnet := make(map[string]string, len(canonicalAgentNames))
	for _, agent := range canonicalAgentNames {
		sonnet[agent] = "sonnet"
	}
	cfg.Quality.Presets["claude-custom"] = QualityPreset{Description: "claude", Agents: sonnet, Efforts: efforts}
	cfg.Quality.Providers = map[string]string{QualityProviderClaude: "claude-custom"}
	return cfg
}

// writeEffortFixture writes cfg as YAML without validating it, the way a
// hand-edited file reaches the loader.
func writeEffortFixture(t *testing.T, cfg *HarnessConfig) string {
	t.Helper()
	dir := t.TempDir()
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), data, 0o644))
	return dir
}

// S1: a valid mapping loads as written.
func TestQualityPresetEfforts_LoadReadsTheMapping(t *testing.T) {
	t.Parallel()
	dir := writeEffortFixture(t, effortFixture(map[string]string{"planner": "xhigh"}))

	cfg, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"planner": "xhigh"}, cfg.Quality.Presets["claude-custom"].Efforts)
}

// S1: a value outside the five levels and a key that is not a canonical
// agent name fail the load with the key path and the value.
func TestQualityPresetEfforts_LoadRejectsAnInvalidValueOrAgent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		efforts map[string]string
		want    string
	}{
		{"misspelled level", map[string]string{"planner": "hihg"}, `quality.presets.claude-custom.efforts.planner: effort_invalid "hihg"`},
		{"codex ultra is not a level", map[string]string{"planner": "ultra"}, `quality.presets.claude-custom.efforts.planner: effort_invalid "ultra"`},
		{"case differs", map[string]string{"planner": "High"}, `quality.presets.claude-custom.efforts.planner: effort_invalid "High"`},
		{"empty level", map[string]string{"planner": ""}, `quality.presets.claude-custom.efforts.planner: effort_invalid ""`},
		{"unknown agent", map[string]string{"not-an-agent": "high"}, `quality.presets.claude-custom.efforts.not-an-agent: agent_invalid "high"`},
		{"workflow spelling is not canonical", map[string]string{"spec_writer": "high"}, `quality.presets.claude-custom.efforts.spec_writer: agent_invalid "high"`},
		{"unprintable key is quoted", map[string]string{"plan\x1bner": "high"}, `quality.presets.claude-custom.efforts."plan\x1bner": agent_invalid "high"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := writeEffortFixture(t, effortFixture(tc.efforts))

			_, err := Load(dir)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// S1: several invalid entries report the same first one on every load.
func TestQualityPresetEfforts_ValidationOrderIsDeterministic(t *testing.T) {
	t.Parallel()
	cfg := effortFixture(map[string]string{"validator": "bad", "architect": "worse", "planner": "high"})
	for range 20 {
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), `quality.presets.claude-custom.efforts.architect: effort_invalid "worse"`)
	}
}

// INV-MUX3-01: a preset without efforts saves without the key, so a file
// that never set it stays byte-identical to what v0.50.125 writes.
func TestQualityPresetEfforts_SaveOmitsAnAbsentMapping(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := DefaultFullConfig("p")
	cfg.Quality.Presets["custom"] = QualityPreset{Agents: map[string]string{"planner": "opus"}, Efforts: map[string]string{}}

	require.NoError(t, Save(dir, cfg))

	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "efforts")
}

// The written mapping round-trips through Save and Load.
func TestQualityPresetEfforts_SaveAndLoadRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	efforts := map[string]string{"planner": "low", "executor": "max"}

	require.NoError(t, Save(dir, effortFixture(efforts)))
	loaded, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, efforts, loaded.Quality.Presets["claude-custom"].Efforts)
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), "efforts:"))
}

func TestQualityEfforts_IsTheFiveLevelsWeakestFirst(t *testing.T) {
	t.Parallel()
	levels := QualityEfforts()
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, levels)
	levels[0] = "changed"
	assert.Equal(t, "low", QualityEfforts()[0], "a detached copy")
	for _, level := range []string{"low", "medium", "high", "xhigh", "max"} {
		assert.True(t, IsQualityEffort(level), level)
	}
	for _, level := range []string{"ultra", "High", " high", ""} {
		assert.False(t, IsQualityEffort(level), level)
	}
}

// AgentEffort reads the preset AgentTier reads: the provider override, then
// the global default, then balanced when the default names no preset.
func TestAgentEffort_ReadsThePresetAgentTierReads(t *testing.T) {
	t.Parallel()
	q := DefaultFullConfig("p").Quality
	q.Presets["balanced"] = withEfforts(q.Presets["balanced"], map[string]string{"planner": "high"})
	q.Presets["codex-custom"] = QualityPreset{Agents: map[string]string{"planner": "opus"}, Efforts: map[string]string{"spec-writer": "low"}}
	q.Providers = map[string]string{QualityProviderCodex: "codex-custom"}

	effort, ok := q.AgentEffort(QualityProviderClaude, "planner")
	assert.True(t, ok)
	assert.Equal(t, "high", effort, "Claude follows the global binding")
	_, ok = q.AgentEffort(QualityProviderCodex, "planner")
	assert.False(t, ok, "Codex follows its override, which leaves planner automatic")
	effort, ok = q.AgentEffort(QualityProviderCodex, "spec_writer")
	assert.True(t, ok, "workflow spelling resolves the agent")
	assert.Equal(t, "low", effort)
	effort, ok = q.AgentEffort("", "planner")
	assert.True(t, ok, "the global view")
	assert.Equal(t, "high", effort)

	unbound := q
	unbound.Default = "missing"
	unbound.Providers = nil
	effort, ok = unbound.AgentEffort(QualityProviderClaude, "planner")
	assert.True(t, ok, "an unknown default falls back to balanced")
	assert.Equal(t, "high", effort)
	_, ok = QualityConf{}.AgentEffort(QualityProviderClaude, "planner")
	assert.False(t, ok, "no preset, no explicit effort")
}

func withEfforts(preset QualityPreset, efforts map[string]string) QualityPreset {
	preset.Efforts = efforts
	return preset
}

// REQ-MUX3-02: an explicit effort takes the agent off the standard balanced
// placement, as an explicit different tier does; siblings stay on it.
func TestNativeBalancedAgentCandidate_ExplicitEffortLeavesTheStandardPlacement(t *testing.T) {
	t.Parallel()
	q := DefaultFullConfig("p").Quality
	q.Presets["balanced"] = withEfforts(q.Presets["balanced"], map[string]string{"planner": "max"})

	for _, provider := range []string{QualityProviderClaude, QualityProviderCodex} {
		_, standard := q.NativeBalancedAgentCandidate(provider, "planner")
		assert.False(t, standard, "%s planner", provider)
		_, standard = q.NativeBalancedAgentCandidate(provider, "architect")
		assert.True(t, standard, "%s architect", provider)
	}
	assert.Equal(t, "fable", q.EffectiveAgentTier("planner", "opus"), "the written tier")
}

// S2 (B1): balanced with planner at high projects the tier alias and the
// effort for Claude, the ladder model at high for Codex, and leaves
// architect on the standard placement.
func TestClaudeAndCodexAgentProfile_B1PlannerAtHigh(t *testing.T) {
	t.Parallel()
	q := DefaultFullConfig("p").Quality
	q.Presets["balanced"] = withEfforts(q.Presets["balanced"], map[string]string{"planner": "high"})

	model, effort := q.ClaudeAgentProfile("planner", "opus")
	assert.Equal(t, []string{"fable", "high"}, []string{model, effort})
	model, effort = q.ClaudeAgentProfile("architect", "opus")
	assert.Equal(t, []string{ClaudeFableModel, "max"}, []string{model, effort})
	assert.Equal(t, CodexProfile{Model: CodexAstraModel, Effort: "high"}, q.CodexAgentProfile("planner", "opus", "max"))
	assert.Equal(t, CodexProfile{Model: CodexAstraModel, Effort: "max"}, q.CodexAgentProfile("architect", "opus", "max"))
}

// S2 (B2): claude-custom with executor on opus at xhigh and annotator on
// haiku at high. Claude writes no effort for haiku; Codex, which follows
// balanced, is untouched by claude-custom.
func TestClaudeAndCodexAgentProfile_B2HaikuTakesNoClaudeEffort(t *testing.T) {
	t.Parallel()
	q := DefaultFullConfig("p").Quality
	custom := withComposeD(q.Presets["balanced"].Agents)
	custom["executor"], custom["annotator"] = "opus", "haiku"
	q.Presets["claude-custom"] = QualityPreset{Agents: custom, Efforts: map[string]string{"executor": "xhigh", "annotator": "high"}}
	q.Providers = map[string]string{QualityProviderClaude: "claude-custom"}

	model, effort := q.ClaudeAgentProfile("executor", "sonnet")
	assert.Equal(t, []string{"opus", "xhigh"}, []string{model, effort})
	model, effort = q.ClaudeAgentProfile("annotator", "sonnet")
	assert.Equal(t, []string{"haiku", ""}, []string{model, effort})
	model, effort = q.ClaudeAgentProfile("planner", "opus")
	assert.Equal(t, []string{"fable", "max"}, []string{model, effort}, "an agent without an effort keeps the automatic one")
	assert.Equal(t, CodexProfile{Model: CodexLunaModel, Effort: "max"}, q.CodexAgentProfile("executor", "sonnet", "medium"))
}

func withComposeD(agents map[string]string) map[string]string {
	copied := make(map[string]string, len(agents))
	for agent, tier := range agents {
		copied[agent] = tier
	}
	return copied
}

// The explicit effort replaces the Codex ladder's effort slot in every
// mode and tier, the haiku slot included; the model stays the ladder's.
func TestCodexAgentProfile_ExplicitEffortReplacesTheLadderEffort(t *testing.T) {
	t.Parallel()
	custom := QualityConf{Default: "custom", Presets: map[string]QualityPreset{"custom": {
		Agents:  map[string]string{"planner": "fable", "executor": "opus", "tester": "sonnet", "annotator": "haiku"},
		Efforts: map[string]string{"planner": "low", "executor": "max", "tester": "medium", "annotator": "xhigh"},
	}}}
	ultra := DefaultFullConfig("p").Quality
	ultra.Default = "ultra"
	ultra.Presets["ultra"] = withEfforts(ultra.Presets["ultra"], map[string]string{"executor": "low"})

	assert.Equal(t, CodexProfile{Model: CodexAstraModel, Effort: "low"}, custom.CodexAgentProfile("planner", "opus", "max"))
	assert.Equal(t, CodexProfile{Model: CodexSolModel, Effort: "max"}, custom.CodexAgentProfile("executor", "sonnet", "medium"))
	assert.Equal(t, CodexProfile{Model: CodexLunaModel, Effort: "medium"}, custom.CodexAgentProfile("tester", "sonnet", "medium"))
	assert.Equal(t, CodexProfile{Model: CodexLunaModel, Effort: "xhigh"}, custom.CodexAgentProfile("annotator", "sonnet", "medium"))
	assert.Equal(t, CodexProfile{Model: CodexSolModel, Effort: "low"}, ultra.CodexAgentProfile("executor", "sonnet", "medium"))
	assert.Equal(t, "xhigh", custom.CodexAgentEffort("annotator", "sonnet", "medium"))
	assert.Equal(t, CodexLunaModel, custom.CodexAgentModel("annotator", "sonnet"))
}

// INV-MUX3-01: without the key every projection is the automatic one.
func TestAgentProfiles_WithoutEffortsStayAutomatic(t *testing.T) {
	t.Parallel()
	q := DefaultFullConfig("p").Quality
	for _, agent := range CanonicalAgentNames() {
		_, ok := q.AgentEffort(QualityProviderClaude, agent)
		assert.False(t, ok, agent)
		_, standard := q.NativeBalancedAgentCandidate(QualityProviderClaude, agent)
		assert.True(t, standard, agent)
	}
	model, effort := q.ClaudeAgentProfile("executor", "sonnet")
	assert.Equal(t, []string{ClaudeSonnetModel, "max"}, []string{model, effort})
}
