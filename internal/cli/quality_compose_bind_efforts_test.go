package cli

// SPEC-MODELUX-003 T10 (REQ-MUX3-20; Efforts Preservation Contract,
// acceptance S20): every path that copies, rebuilds, previews, or refreshes a
// quality preset carries its efforts, and a built-in never takes efforts the
// operator did not put there.

import (
	"maps"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeEffortsCustomP is the second S20 file's quality: quality.default
// custom, custom holding D's balanced tiers with executor at xhigh.
func composeEffortsCustomP(q *config.QualityConf) {
	q.Presets["custom"] = config.QualityPreset{
		Description: composeCustomDescription,
		Agents:      maps.Clone(composeBindCurrent("balanced").Agents),
		Efforts:     map[string]string{"executor": "xhigh"},
	}
	q.Default = "custom"
}

// The direct calls of S20: with efforts on the binding preset P, a result
// equal to another built-in's default binds custom with P's efforts whether
// that built-in is current, historical, or absent, and the built-in's class
// stays as it was.
func TestComposeBindTarget_EffortsOnPBindCustomForEveryBuiltinClass(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, builtin string
		mutate        func(*config.QualityConf)
		class         config.BuiltinQualityPresetClass
	}{
		{"current", "balanced", func(*config.QualityConf) {}, config.BuiltinQualityPresetCurrent},
		{"historical", "balanced", func(q *config.QualityConf) {
			q.Presets["balanced"] = config.QualityPreset{Agents: maps.Clone(composeTableHB1)}
		}, config.BuiltinQualityPresetHistorical},
		{"absent", "ultra", func(q *config.QualityConf) { delete(q.Presets, "ultra") }, config.BuiltinQualityPresetAbsent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, cfg := composeBindSaved(t, func(q *config.QualityConf) {
				composeEffortsCustomP(q)
				tc.mutate(q)
			})
			edited := qualityComposition{tiers: maps.Clone(composeBindCurrent(tc.builtin).Agents)}

			name, write := composeBindTarget(cfg.Quality, edited)

			assert.Equal(t, "custom", name)
			require.NotNil(t, write)
			assert.Equal(t, composePresetRewrite, write.Kind)
			assert.Equal(t, map[string]string{"executor": "xhigh"}, write.Preset.Efforts)
			assert.Equal(t, edited.tiers, write.Preset.Agents)
			assert.Zero(t, write.Carried, "the efforts stay under the name they came from")
			status, _ := config.ClassifyBuiltinQualityPreset(cfg.Quality, tc.builtin)
			assert.Equal(t, tc.class, status.Class)
		})
	}
}

// Without efforts on P the built-in branches are taken as before.
func TestComposeBindTarget_NoEffortsOnPKeepsTheBuiltinBranches(t *testing.T) {
	t.Parallel()
	_, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		composeEffortsCustomP(q)
		custom := q.Presets["custom"]
		custom.Efforts = nil
		q.Presets["custom"] = custom
		ultra := q.Presets["ultra"]
		ultra.Efforts = map[string]string{"planner": "low"}
		q.Presets["ultra"] = ultra
	})

	name, write := composeBindTarget(cfg.Quality, qualityComposition{tiers: maps.Clone(composeBindCurrent("balanced").Agents)})

	assert.Equal(t, "balanced", name)
	assert.Nil(t, write)
}

// P itself historical: the result equal to P's current default refreshes P,
// whose efforts stay, and binds nothing else.
func TestComposeBindTarget_HistoricalPRefreshKeepsItsEfforts(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["balanced"] = config.QualityPreset{Agents: maps.Clone(composeTableHB1), Efforts: map[string]string{"planner": "high"}}
	})
	start := composeBindStart(t, cfg)
	edited := qualityComposition{tiers: maps.Clone(composeBindCurrent("balanced").Agents)}

	plan := planComposeBinding(cfg, start, edited)

	assert.Equal(t, []string{"balanced"}, composeBindPresetNames(plan))
	assert.Equal(t, composePresetRefresh, plan.Presets[0].Kind)
	assert.Empty(t, plan.Default)
	assert.Equal(t, map[string]string{"planner": "high"}, plan.Candidate.Quality.Presets["balanced"].Efforts)
	written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
	assert.Equal(t, composeBindCurrent("balanced").Agents, written.Presets["balanced"].Agents)
	assert.Equal(t, map[string]string{"planner": "high"}, written.Presets["balanced"].Efforts)
	assert.NotContains(t, written.Presets, "custom")
}

// A custom made from a balanced binding with efforts carries them to the new
// name and says so in the preview.
func TestPlanComposeBinding_CustomCarriesPEffortsWithTheCarryLine(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		balanced := q.Presets["balanced"]
		balanced.Efforts = map[string]string{"planner": "high"}
		q.Presets["balanced"] = balanced
	})
	start := composeBindStart(t, cfg)
	edited := composeBindEdited(t, start, nil)
	require.NoError(t, edited.SetGroupTier(composeRoleGroups[1], "haiku"))

	plan := planComposeBinding(cfg, start, edited)

	require.Equal(t, []string{"custom"}, composeBindPresetNames(plan))
	assert.Equal(t, 1, plan.Presets[0].Carried)
	lines := composeBindDescribe(t, dir, cfg, plan, "ko")
	assert.Equal(t, []string{
		"quality.presets.custom 새로 씀 (16개 에이전트)",
		"quality.presets.custom.efforts 옮겨 씀 (1개 에이전트)",
		"quality.default balanced → custom",
	}, lines[:3])
	assert.Contains(t, composeBindDescribe(t, dir, cfg, plan, "en"), "quality.presets.custom.efforts carried (1 agents)")
	written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
	assert.Equal(t, map[string]string{"planner": "high"}, written.Presets["custom"].Efforts)
	assert.Equal(t, map[string]string{"planner": "high"}, written.Presets["balanced"].Efforts, "the source keeps its own")
}

// `u` refreshes the tiers and description of a historical built-in and
// keeps its efforts; the candidate copies every preset's efforts.
func TestPlanComposeRefresh_KeepsTheRefreshedPresetsEfforts(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["balanced"] = config.QualityPreset{Agents: maps.Clone(composeTableHB1), Efforts: map[string]string{"planner": "high"}}
		q.Presets["other"] = config.QualityPreset{Agents: map[string]string{"planner": "opus"}, Efforts: map[string]string{"planner": "low"}}
	})

	plan, ok := planComposeRefresh(cfg)

	require.True(t, ok)
	assert.Equal(t, map[string]string{"planner": "high"}, plan.Candidate.Quality.Presets["balanced"].Efforts)
	assert.Equal(t, map[string]string{"planner": "low"}, plan.Candidate.Quality.Presets["other"].Efforts)
	written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
	assert.Equal(t, composeBindCurrent("balanced").Agents, written.Presets["balanced"].Agents)
	assert.Equal(t, composeBindCurrent("balanced").Description, written.Presets["balanced"].Description)
	assert.Equal(t, map[string]string{"planner": "high"}, written.Presets["balanced"].Efforts)
	assert.Equal(t, map[string]string{"planner": "low"}, written.Presets["other"].Efforts)
}

// The raw writer round-trips an empty efforts mapping a hand-edited file
// holds: an unrelated write neither refuses nor keeps it.
func TestSaveQualityBlock_EmptyEffortsMappingDoesNotBlockAWrite(t *testing.T) {
	t.Parallel()
	_, saved := composeBindSaved(t, nil)
	text := readBlockFixture(t, composeBindWrittenDir(t, saved))
	require.Contains(t, text, "                validator: sonnet\n        ultra:\n")
	dir := writeBlockFixture(t, strings.Replace(text,
		"                validator: sonnet\n        ultra:\n",
		"                validator: sonnet\n            efforts: {}\n        ultra:\n", 1))
	cfg := composeBindLoad(t, dir)
	agents := maps.Clone(composeBindCurrent("balanced").Agents)

	require.NoError(t, saveQualityToolPresetEfforts(dir, cfg, composeToolNamed(t, "codex"), "codex-custom", agents, nil))

	written := rawQualityOf(t, readBlockFixture(t, dir))
	assert.Equal(t, "codex-custom", written.Providers["codex"])
	assert.Empty(t, written.Presets["balanced"].Efforts)
}

// composeBindWrittenDir saves cfg into a fresh project and returns it.
func composeBindWrittenDir(t *testing.T, cfg *config.HarnessConfig) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, cfg))
	return dir
}
