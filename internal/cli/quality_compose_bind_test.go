package cli

// SPEC-MODELUX-001 REQ-MUX-07/12, acceptance S7: the Binding Contract
// decision table over heterogeneous fixtures, written through the quality
// writers, and the shared fixture helpers of the binding tests.

import (
	"maps"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const composeBindRepo = blockRepoPrefix + blockRepoQuality + blockRepoSuffix

// composeBindSaved saves a fixture-D variant with config.Save and loads it the
// way `auto quality` does.
func composeBindSaved(t *testing.T, mutate func(*config.QualityConf)) (string, *config.HarnessConfig) {
	t.Helper()
	cfg := config.DefaultFullConfig("p")
	cfg.Platforms = []string{"claude-code", "codex"}
	if mutate != nil {
		mutate(&cfg.Quality)
	}
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, cfg))
	return dir, composeBindLoad(t, dir)
}

// composeBindWritten writes fixture bytes verbatim and loads them.
func composeBindWritten(t *testing.T, content string) (string, *config.HarnessConfig) {
	t.Helper()
	dir := writeBlockFixture(t, content)
	return dir, composeBindLoad(t, dir)
}

func composeBindLoad(t *testing.T, dir string) *config.HarnessConfig {
	t.Helper()
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	return cfg
}

func composeBindStart(t *testing.T, cfg *config.HarnessConfig) qualityComposition {
	t.Helper()
	return newQualityComposition(cfg.Quality, composeGroupsSources(t))
}

// composeBindEdited applies agents to a copy of start as advanced edits.
func composeBindEdited(t *testing.T, start qualityComposition, agents map[string]string) qualityComposition {
	t.Helper()
	edited := start.Clone()
	for agent, tier := range agents {
		_, _, err := edited.ApplyAssignment(agent + "=" + tier)
		require.NoError(t, err)
	}
	return edited
}

func composeBindCurrent(name string) config.QualityPreset {
	return config.DefaultFullConfig("").Quality.Presets[name]
}

func composeBindHB2(t *testing.T) map[string]string {
	t.Helper()
	return rawQualityOf(t, blockRepoQuality).Presets["balanced"].Agents
}

// composeBindS6 is acceptance S6's group edit on D: 구현 = sonnet, 탐색·보조 = haiku.
func composeBindS6(t *testing.T, start qualityComposition) qualityComposition {
	t.Helper()
	edited := start.Clone()
	require.NoError(t, edited.SetGroupTier(composeRoleGroups[1], "sonnet"))
	require.NoError(t, edited.SetGroupTier(composeRoleGroups[3], "haiku"))
	return edited
}

func composeBindPersist(t *testing.T, dir string, cfg *config.HarnessConfig, plan composeBindingPlan) string {
	t.Helper()
	require.NoError(t, persistComposeBinding(dir, cfg, plan))
	return readBlockFixture(t, dir)
}

func composeBindPresetNames(plan composeBindingPlan) []string {
	names := make([]string, 0, len(plan.Presets))
	for _, write := range plan.Presets {
		names = append(names, write.Name)
	}
	return names
}

func TestPlanComposeBinding_EditedEqualsStart_WritesNothing(t *testing.T) {
	t.Parallel()
	_, d := composeBindSaved(t, nil)
	_, b2 := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["balanced"] = config.QualityPreset{Agents: composeBindHB2(t)}
	})
	repoDir, repo := composeBindWritten(t, composeBindRepo)

	for name, cfg := range map[string]*config.HarnessConfig{"D": d, "B2": b2, "R": repo} {
		start := composeBindStart(t, cfg)
		// g and Enter four times: every group kept.
		plan := planComposeBinding(cfg, start, start.Clone())
		assert.Equal(t, composeWriterNone, plan.Writer, name)
		assert.Empty(t, plan.Presets, name)
		assert.Empty(t, plan.Default, name)
	}

	start := composeBindStart(t, repo)
	before := readBlockFixture(t, repoDir)
	plan := planComposeBinding(repo, start, start.Clone())
	assert.Equal(t, before, composeBindPersist(t, repoDir, repo, plan))
}

func TestPlanComposeBinding_CurrentBalancedFromCustom_WritesOnlyTheDefaultLine(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["custom"] = config.QualityPreset{Description: "mine", Agents: map[string]string{"planner": "haiku"}}
		q.Default = "custom"
	})
	start := composeBindStart(t, cfg)
	before := readBlockFixture(t, dir)

	plan := planComposeBinding(cfg, start, composeBindEdited(t, start, composeBindCurrent("balanced").Agents))

	assert.Equal(t, composeWriterLine, plan.Writer)
	assert.Equal(t, "balanced", plan.Default)
	assert.Empty(t, plan.Presets)
	after := composeBindPersist(t, dir, cfg, plan)
	beforeLines, afterLines := strings.Split(before, "\n"), strings.Split(after, "\n")
	require.Len(t, afterLines, len(beforeLines))
	var changed []string
	for i := range beforeLines {
		if beforeLines[i] != afterLines[i] {
			changed = append(changed, strings.TrimSpace(afterLines[i]))
		}
	}
	assert.Equal(t, []string{"default: balanced"}, changed)
	assert.Equal(t, "balanced", cfg.Quality.Default, "the loaded config follows the write")
}

func TestPlanComposeBinding_CurrentBalancedOverHistoricalOrAbsent_RefreshesAndBinds(t *testing.T) {
	t.Parallel()
	hb2 := composeBindHB2(t)
	tests := map[string]struct {
		mutate   func(*config.QualityConf)
		wantKind composePresetWriteKind
	}{
		"historical HB2": {
			mutate: func(q *config.QualityConf) {
				q.Presets["balanced"] = config.QualityPreset{Description: "old", Agents: maps.Clone(hb2)}
				q.Default = "ultra"
			},
			wantKind: composePresetRefresh,
		},
		"absent": {
			mutate: func(q *config.QualityConf) {
				delete(q.Presets, "balanced")
				q.Default = "ultra"
			},
			wantKind: composePresetNew,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir, cfg := composeBindSaved(t, tt.mutate)
			start := composeBindStart(t, cfg)

			plan := planComposeBinding(cfg, start, composeBindEdited(t, start, composeBindCurrent("balanced").Agents))

			assert.Equal(t, composeWriterBlock, plan.Writer)
			assert.Equal(t, "balanced", plan.Default)
			require.Equal(t, []string{"balanced"}, composeBindPresetNames(plan))
			assert.Equal(t, tt.wantKind, plan.Presets[0].Kind)
			written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
			assert.Equal(t, composeBindCurrent("balanced"), written.Presets["balanced"])
			assert.Equal(t, "balanced", written.Default)
			assert.Equal(t, composeBindCurrent("ultra"), written.Presets["ultra"])
			_, err := config.Load(dir)
			assert.NoError(t, err, "the written file passes Validate")
		})
	}
}

func TestPlanComposeBinding_CurrentDefaultOverCustomizedBuiltin_PersistsAsCustom(t *testing.T) {
	t.Parallel()
	// S7: balanced customized as the current default plus tester: fable.
	dir, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["balanced"].Agents["tester"] = "fable"
	})
	original := rawQualityOf(t, readBlockFixture(t, dir)).Presets["balanced"]
	start := composeBindStart(t, cfg)
	require.Equal(t, "fable", start.Tier("tester"))

	plan := planComposeBinding(cfg, start, composeBindEdited(t, start, map[string]string{"tester": "sonnet"}))

	assert.Equal(t, composeWriterBlock, plan.Writer)
	assert.Equal(t, "custom", plan.Default)
	require.Equal(t, []string{"custom"}, composeBindPresetNames(plan))
	written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
	assert.Equal(t, config.QualityPreset{Description: "역할 그룹 구성", Agents: composeBindCurrent("balanced").Agents},
		written.Presets["custom"])
	assert.Equal(t, original, written.Presets["balanced"], "a customized preset keeps its values")
	assert.Equal(t, "custom", written.Default)
}

func TestPlanComposeBinding_CurrentUltraOverCustomizedUltra_PersistsAsCustom(t *testing.T) {
	t.Parallel()
	// Fixture C: R with ultra devops: sonnet.
	dir, cfg := composeBindWritten(t, strings.Replace(composeBindRepo,
		"                devops: opus\n", "                devops: sonnet\n", 1))
	original := rawQualityOf(t, readBlockFixture(t, dir)).Presets["ultra"]
	start := composeBindStart(t, cfg)

	plan := planComposeBinding(cfg, start, composeBindEdited(t, start, composeBindCurrent("ultra").Agents))

	assert.Equal(t, "custom", plan.Default)
	require.Equal(t, []string{"custom"}, composeBindPresetNames(plan))
	written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
	assert.Equal(t, original, written.Presets["ultra"])
	assert.Equal(t, composeBindCurrent("ultra").Agents, written.Presets["custom"].Agents)
}

func TestPlanComposeBinding_CurrentUltraOverHistoricalBoundUltra_RefreshesOnlyUltra(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindWritten(t, composeBindRepo)
	start := composeBindStart(t, cfg)

	plan := planComposeBinding(cfg, start, composeBindEdited(t, start, composeBindCurrent("ultra").Agents))

	assert.Equal(t, composeWriterBlock, plan.Writer)
	assert.Empty(t, plan.Default, "quality.default already names ultra")
	require.Equal(t, []string{"ultra"}, composeBindPresetNames(plan))
	after := composeBindPersist(t, dir, cfg, plan)
	written := rawQualityOf(t, after)
	assert.Equal(t, composeBindCurrent("ultra"), written.Presets["ultra"])
	assert.Equal(t, rawQualityOf(t, composeBindRepo).Presets["balanced"], written.Presets["balanced"])
	assert.Equal(t, "ultra", written.Default)
	assert.True(t, strings.HasPrefix(after, blockRepoPrefix))
	assert.True(t, strings.HasSuffix(after, blockRepoSuffix))
}
