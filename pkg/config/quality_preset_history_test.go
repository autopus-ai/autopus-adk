package config

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestHistoricalQualityPresets_RowsMatchShippedLayouts fixes every registry row
// against SPEC-MODELUX-001's Historical Preset Registry table: preset, commit,
// runtime-standard mark, and the exact shipped agent map.
func TestHistoricalQualityPresets_RowsMatchShippedLayouts(t *testing.T) {
	want := map[string]struct {
		preset          string
		commit          string
		runtimeStandard bool
		agents          map[string]string
	}{
		"HB1": {preset: "balanced", commit: "c95806e2", agents: shippedBalancedHB1},
		"HB2": {preset: "balanced", commit: "c21f7316", runtimeStandard: true, agents: shippedBalancedHB2},
		"HB4": {preset: "balanced", commit: "aaeae52d", agents: shippedBalancedHB4},
		"HB5": {preset: "balanced", commit: "001830fd", runtimeStandard: true, agents: shippedBalancedHB5},
		"HU1": {preset: "ultra", commit: "c95806e2", agents: shippedUltraHU1},
		"HU2": {preset: "ultra", commit: "aaeae52d", agents: shippedUltraHU2},
	}
	require.Len(t, historicalQualityPresets, len(want))
	for _, row := range historicalQualityPresets {
		expected, ok := want[row.id]
		require.True(t, ok, "unexpected registry row %q", row.id)
		assert.Equal(t, expected.preset, row.preset, row.id)
		assert.Equal(t, expected.commit, row.commit, row.id)
		assert.Equal(t, expected.runtimeStandard, row.runtimeStandard, row.id)
		assert.Equal(t, expected.agents, row.agents, row.id)
		assert.NotEmpty(t, row.shipped, row.id)
	}
}

// TestHistoricalQualityPresets_RowInvariants pins the four REQ-MUX-10 row rules:
// canonical names only, at most 16 entries, valid tiers, and never the current
// default, so classification can never confuse a row with an up-to-date file.
func TestHistoricalQualityPresets_RowInvariants(t *testing.T) {
	canonical := CanonicalAgentNames()
	for _, row := range historicalQualityPresets {
		t.Run(row.id, func(t *testing.T) {
			assert.LessOrEqual(t, len(row.agents), 16)
			for agent, tier := range row.agents {
				assert.Contains(t, canonical, agent)
				normalized, valid := NormalizeQualityTier(tier)
				assert.True(t, valid && normalized == tier, "%s=%q", agent, tier)
			}
			current := DefaultFullConfig("history").Quality.Presets[row.preset].Agents
			require.NotEmpty(t, current, "row %s must name a built-in preset", row.id)
			assert.False(t, maps.Equal(current, row.agents), "row %s equals the current default", row.id)
		})
	}
}

func TestClassifyBuiltinQualityPreset_EveryRegistryRowIsHistorical(t *testing.T) {
	for _, row := range historicalQualityPresets {
		q := QualityConf{Presets: map[string]QualityPreset{row.preset: {Agents: maps.Clone(row.agents)}}}
		status, ok := ClassifyBuiltinQualityPreset(q, row.preset)
		require.True(t, ok, row.id)
		assert.Equal(t, BuiltinQualityPresetHistorical, status.Class, row.id)
		assert.Equal(t, row.id, status.Row, row.id)
		assert.Equal(t, row.runtimeStandard, status.RuntimeStandard, row.id)
	}
}

func TestClassifyBuiltinQualityPreset_CurrentDefaultsAreCurrent(t *testing.T) {
	q := DefaultFullConfig("current").Quality
	for _, name := range []string{"balanced", "ultra"} {
		status, ok := ClassifyBuiltinQualityPreset(q, name)
		require.True(t, ok, name)
		assert.Equal(t, BuiltinQualityPresetStatus{Class: BuiltinQualityPresetCurrent}, status, name)
	}
}

func TestClassifyBuiltinQualityPreset_ExactMatchIgnoringDescription(t *testing.T) {
	ultraWithout := func(agent string) map[string]string {
		agents := maps.Clone(shippedUltraHU1)
		delete(agents, agent)
		return agents
	}
	currentWith := func(name, agent, tier string) map[string]string {
		agents := maps.Clone(DefaultFullConfig("custom").Quality.Presets[name].Agents)
		if tier == "" {
			delete(agents, agent)
		} else {
			agents[agent] = tier
		}
		return agents
	}
	executorPromotion := maps.Clone(shippedBalancedHB2) // 0cb6296b never shipped alone
	executorPromotion["executor"] = "opus"
	tests := []struct {
		name        string
		preset      string
		value       QualityPreset
		wantClass   BuiltinQualityPresetClass
		wantRow     string
		wantStd     bool
		wantDiffers int
	}{
		{name: "description-only edit of HB2", preset: "balanced",
			value:     QualityPreset{Description: "my words", Agents: maps.Clone(shippedBalancedHB2)},
			wantClass: BuiltinQualityPresetHistorical, wantRow: "HB2", wantStd: true, wantDiffers: 11},
		{name: "HU1 without devops", preset: "ultra", value: QualityPreset{Agents: ultraWithout("devops")},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 12},
		{name: "HU1 with an invalid tester tier", preset: "ultra",
			value:     QualityPreset{Agents: func() map[string]string { a := maps.Clone(shippedUltraHU1); a["tester"] = "gpt"; return a }()},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 12},
		{name: "current balanced missing one agent", preset: "balanced",
			value:     QualityPreset{Agents: currentWith("balanced", "annotator", "")},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 1},
		{name: "current balanced with one moved agent", preset: "balanced",
			value:     QualityPreset{Agents: currentWith("balanced", "tester", "fable")},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 1},
		{name: "current ultra with a non-canonical spelling", preset: "ultra",
			value:     QualityPreset{Agents: currentWith("ultra", "tester", "Opus")},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 1},
		{name: "current ultra with an extra agent", preset: "ultra",
			value:     QualityPreset{Agents: currentWith("ultra", "my-agent", "opus")},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 0},
		{name: "balanced written with an ultra row", preset: "balanced",
			value:     QualityPreset{Agents: maps.Clone(shippedUltraHU2)},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 16},
		{name: "ultra written with a balanced row", preset: "ultra",
			value:     QualityPreset{Agents: maps.Clone(shippedBalancedHB4)},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 15},
		{name: "unshipped executor promotion", preset: "balanced", value: QualityPreset{Agents: executorPromotion},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 12},
		{name: "preset without agents", preset: "balanced", value: QualityPreset{Description: "empty"},
			wantClass: BuiltinQualityPresetCustomized, wantDiffers: 16},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			q := QualityConf{Default: "ultra", Presets: map[string]QualityPreset{test.preset: test.value}}
			status, ok := ClassifyBuiltinQualityPreset(q, test.preset)
			require.True(t, ok)
			assert.Equal(t, test.wantClass, status.Class)
			assert.Equal(t, test.wantRow, status.Row)
			assert.Equal(t, test.wantStd, status.RuntimeStandard)
			assert.Equal(t, test.wantDiffers, status.Differs)
		})
	}
}

func TestClassifyBuiltinQualityPreset_AbsentAndNonBuiltinNames(t *testing.T) {
	q := QualityConf{Default: "custom", Presets: map[string]QualityPreset{
		"custom": {Agents: map[string]string{"planner": "fable"}},
	}}
	status, ok := ClassifyBuiltinQualityPreset(q, "balanced")
	require.True(t, ok)
	assert.Equal(t, BuiltinQualityPresetAbsent, status.Class)
	assert.Equal(t, len(CanonicalAgentNames()), status.Differs)

	status, ok = ClassifyBuiltinQualityPreset(QualityConf{}, "ultra")
	require.True(t, ok)
	assert.Equal(t, BuiltinQualityPresetAbsent, status.Class)

	for _, name := range []string{"custom", "Balanced", " ultra", ""} {
		_, ok := ClassifyBuiltinQualityPreset(q, name)
		assert.False(t, ok, "%q is not a built-in preset name", name)
	}
}

// TestClassifyBuiltinQualityPreset_DecodedRepositoryFileIsHistorical decodes
// the quality block this repository's autopus.yaml carried when the SPEC was
// written (fixture R): balanced is HB2 and ultra is HU1.
func TestClassifyBuiltinQualityPreset_DecodedRepositoryFileIsHistorical(t *testing.T) {
	const fixture = `
default: ultra
supervisor_model_policy: quality
presets:
    balanced:
        description: 핵심 분석은 Opus, 기본 작업은 Sonnet. Haiku 미사용.
        agents:
            architect: opus
            debugger: sonnet
            devops: sonnet
            executor: sonnet
            explorer: sonnet
            planner: opus
            reviewer: sonnet
            security-auditor: opus
            spec-writer: opus
            tester: sonnet
            validator: sonnet
    ultra:
        description: 모든 에이전트를 Opus로 실행. 최고 품질.
        agents:
            architect: opus
            debugger: opus
            devops: opus
            executor: opus
            explorer: opus
            planner: opus
            reviewer: opus
            security-auditor: opus
            spec-writer: opus
            tester: opus
            validator: opus
`
	var q QualityConf
	require.NoError(t, yaml.Unmarshal([]byte(fixture), &q))
	got := map[string]BuiltinQualityPresetStatus{}
	for _, name := range []string{"balanced", "ultra"} {
		status, ok := ClassifyBuiltinQualityPreset(q, name)
		require.True(t, ok, name)
		got[name] = status
	}
	assert.Equal(t, BuiltinQualityPresetStatus{
		Class: BuiltinQualityPresetHistorical, Row: "HB2", RuntimeStandard: true, Differs: 11,
	}, got["balanced"])
	assert.Equal(t, BuiltinQualityPresetStatus{
		Class: BuiltinQualityPresetHistorical, Row: "HU1", Differs: 11,
	}, got["ultra"])
}
