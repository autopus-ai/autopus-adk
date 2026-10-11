package cli

// SPEC-MODELUX-001 REQ-MUX-11/12, acceptance S9: the `u` refresh plan, its
// per-preset preview lines, and edits made on top of an unconfirmed refresh.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestPlanComposeRefresh_HistoricalPresets_TakeTheCurrentDefaults(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindWritten(t, composeBindRepo)

	plan, ok := planComposeRefresh(cfg)

	require.True(t, ok)
	assert.Equal(t, composeWriterBlock, plan.Writer)
	assert.Empty(t, plan.Default)
	assert.Equal(t, []string{"ultra", "balanced"}, composeBindPresetNames(plan))
	after := composeBindPersist(t, dir, cfg, plan)
	written := rawQualityOf(t, after)
	assert.Equal(t, composeBindCurrent("ultra"), written.Presets["ultra"])
	assert.Equal(t, composeBindCurrent("balanced"), written.Presets["balanced"])
	assert.Equal(t, "ultra", written.Default)
	assert.Equal(t, "quality", written.SupervisorModelPolicy)
	assert.True(t, strings.HasPrefix(after, blockRepoPrefix))
	assert.True(t, strings.HasSuffix(after, blockRepoSuffix))
}

func TestPlanComposeRefresh_CustomizedPreset_IsNeverRefreshed(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindWritten(t, strings.Replace(composeBindRepo,
		"                devops: opus\n", "                devops: sonnet\n", 1))
	original := rawQualityOf(t, readBlockFixture(t, dir)).Presets["ultra"]

	plan, ok := planComposeRefresh(cfg)

	require.True(t, ok)
	assert.Equal(t, []string{"balanced"}, composeBindPresetNames(plan))
	written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
	assert.Equal(t, original, written.Presets["ultra"])
	assert.Equal(t, composeBindCurrent("balanced"), written.Presets["balanced"])
}

func TestPlanComposeRefresh_NoHistoricalPreset_OffersNothing(t *testing.T) {
	t.Parallel()
	_, d := composeBindSaved(t, nil)
	_, absent := composeBindSaved(t, func(q *config.QualityConf) { delete(q.Presets, "ultra") })

	for _, cfg := range []*config.HarnessConfig{d, absent} {
		_, ok := planComposeRefresh(cfg)
		assert.False(t, ok)
	}
}

func TestDescribeComposeRefresh_R_ShowsPerPresetLines(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindWritten(t, composeBindRepo)
	plan, ok := planComposeRefresh(cfg)
	require.True(t, ok)

	assert.Equal(t, []string{
		"quality.presets.ultra 최신 기본값으로 갱신: architect, debugger, planner, reviewer, security-auditor, " +
			"spec-writer opus→fable · 추가 annotator→opus, deep-worker→fable, frontend-specialist→opus, " +
			"perf-engineer→opus, ux-validator→opus",
		"quality.presets.balanced 최신 기본값으로 갱신: 모델 변화 없음, YAML만 정리",
		"quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)",
	}, composeBindDescribe(t, dir, cfg, plan, "ko"))
}

func TestDescribeComposeRefresh_BoundHB1_ShowsEveryTransition(t *testing.T) {
	t.Parallel()
	// Fixture H: D with balanced HB1, which the runtime does not keep on the
	// standard placement, so the refresh moves models.
	hb1 := map[string]string{
		"planner": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"executor": "sonnet", "tester": "sonnet", "reviewer": "sonnet", "debugger": "sonnet",
		"devops": "sonnet", "validator": "haiku", "explorer": "haiku",
	}
	dir, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["balanced"] = config.QualityPreset{Agents: hb1}
	})
	plan, ok := planComposeRefresh(cfg)
	require.True(t, ok)

	lines := composeBindDescribe(t, dir, cfg, plan, "en")

	require.Len(t, lines, 2)
	assert.Equal(t, "quality.presets.balanced refreshed to the current default: architect, planner, "+
		"security-auditor, spec-writer opus→fable · debugger, reviewer sonnet→fable · explorer, "+
		"validator haiku→sonnet · added annotator→sonnet, deep-worker→fable, frontend-specialist→sonnet, "+
		"perf-engineer→sonnet, ux-validator→sonnet", lines[0])
}

func TestDescribeComposeRefresh_BoundRuntimeStandardBalanced_MovesNoModel(t *testing.T) {
	t.Parallel()
	// Fixture B2: balanced HB2 is the binding, yet the runtime already keeps
	// it on the standard placement.
	hb2 := composeBindHB2(t)
	dir, cfg := composeBindSaved(t, func(q *config.QualityConf) {
		q.Presets["balanced"] = config.QualityPreset{Agents: hb2}
	})
	plan, ok := planComposeRefresh(cfg)
	require.True(t, ok)

	assert.Equal(t, []string{
		"quality.presets.balanced 최신 기본값으로 갱신: 모델 변화 없음, YAML만 정리",
		"quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)",
	}, composeBindDescribe(t, dir, cfg, plan, "ko"))
}

func TestDescribeComposeRefresh_UltraFeedsOnlyTheBuiltinOMPProfile(t *testing.T) {
	t.Parallel()
	hu1 := rawQualityOf(t, blockRepoQuality).Presets["ultra"]
	for name, tt := range map[string]struct {
		profile string
		want    string
	}{
		"builtin ultra OMP profile": {profile: "ultra", want: "spec-writer opus→fable"},
		"no OMP profile":            {profile: "", want: "모델 변화 없음, YAML만 정리"},
	} {
		cfg := config.DefaultFullConfig("p")
		cfg.Platforms = []string{"claude-code", "omp"}
		cfg.Quality.Presets["ultra"] = hu1
		cfg.RoleModelPolicy.Profile = tt.profile
		plan, ok := planComposeRefresh(cfg)
		require.True(t, ok, name)

		// No source bytes: an in-memory config (init) has no block to rewrite.
		lines := describeComposeBinding(cfg, plan, driftGenerationSnapshot{}, nil, "ko")

		require.Len(t, lines, 1, name)
		assert.Contains(t, lines[0], tt.want, name)
	}
}

func TestComposeAgentChangeText_RemovedAgent_IsNamed(t *testing.T) {
	t.Parallel()

	got := composeAgentChangeText(
		map[string]string{"planner": "opus", "annotator": "sonnet"},
		map[string]string{"planner": "fable"},
		composeBindTextFor("ko"),
	)

	assert.Equal(t, "planner opus→fable · 제거 annotator", got)
}

func TestPlanComposeBindingAfter_EditsOnTopOfRefresh_KeepTheRefreshInTheSameWrite(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindWritten(t, composeBindRepo)
	refresh, ok := planComposeRefresh(cfg)
	require.True(t, ok)
	// `u`, then `e`: the edit menu continues from the refreshed candidate.
	start := composeBindStart(t, refresh.Candidate)
	edited := composeBindEdited(t, start, map[string]string{"planner": "sonnet"})

	plan := planComposeBindingAfter(cfg, refresh, start, edited)

	assert.Equal(t, composeWriterBlock, plan.Writer)
	assert.Equal(t, "custom", plan.Default)
	assert.Equal(t, []string{"ultra", "balanced", "custom"}, composeBindPresetNames(plan))
	assert.True(t, newQualityComposition(plan.Candidate.Quality, composeGroupsSources(t)).Equal(edited))
	assert.Equal(t, []string{
		"quality.presets.ultra 최신 기본값으로 갱신: 모델 변화 없음, YAML만 정리",
		"quality.presets.balanced 최신 기본값으로 갱신: 모델 변화 없음, YAML만 정리",
		"quality.presets.custom 새로 씀 (16개 에이전트)",
		"quality.default ultra → custom",
		"Codex 기본 세션: 6-astra · ultra → 6-astra · xhigh",
		"custom은 ultra effort 사다리를 받지 않음",
		"quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)",
	}, composeBindDescribe(t, dir, cfg, plan, "ko"))

	written := rawQualityOf(t, composeBindPersist(t, dir, cfg, plan))
	assert.Equal(t, composeBindCurrent("ultra"), written.Presets["ultra"])
	assert.Equal(t, composeBindCurrent("balanced"), written.Presets["balanced"])
	assert.Equal(t, edited.Agents(), written.Presets["custom"].Agents)
	assert.Equal(t, "custom", written.Default)
}

func TestPlanComposeBindingAfter_NoEditOnTopOfRefresh_IsTheRefresh(t *testing.T) {
	t.Parallel()
	_, cfg := composeBindWritten(t, composeBindRepo)
	refresh, ok := planComposeRefresh(cfg)
	require.True(t, ok)
	start := composeBindStart(t, refresh.Candidate)

	plan := planComposeBindingAfter(cfg, refresh, start, start.Clone())

	assert.Equal(t, refresh.Writer, plan.Writer)
	assert.Equal(t, refresh.Default, plan.Default)
	assert.Equal(t, refresh.Presets, plan.Presets)
	assert.Equal(t, refresh.Candidate.Quality, plan.Candidate.Quality)
}
