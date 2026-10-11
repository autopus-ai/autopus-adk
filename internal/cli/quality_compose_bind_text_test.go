package cli

// SPEC-MODELUX-001 REQ-MUX-05/06, acceptance S6: the preview lines of a
// binding plan — every autopus.yaml key the write changes and every effect
// outside the table.

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
)

func composeBindSnapshotA() driftGenerationSnapshot {
	return driftGenerationSnapshot{codexCatalog: []byte(composeCatalogA), codexCatalogFixed: true}
}

func composeBindDescribe(t *testing.T, dir string, cfg *config.HarnessConfig, plan composeBindingPlan, lang string) []string {
	t.Helper()
	return describeComposeBinding(cfg, plan, composeBindSnapshotA(), []byte(readBlockFixture(t, dir)), lang)
}

func TestDescribeComposeBinding_S6_ListsTheFourChangeLines(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindSaved(t, nil)
	start := composeBindStart(t, cfg)
	plan := planComposeBinding(cfg, start, composeBindS6(t, start))

	assert.Equal(t, []string{
		"quality.presets.custom 새로 씀 (16개 에이전트)",
		"quality.default balanced → custom",
		"custom은 balanced 표준 배치를 받지 않음",
		"quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)",
	}, composeBindDescribe(t, dir, cfg, plan, "ko"))
	assert.Equal(t, []string{
		"quality.presets.custom written new (16 agents)",
		"quality.default balanced → custom",
		"custom does not get the balanced standard placement",
		"the quality block is rewritten: its 0 comment lines and layout are not kept (values are)",
	}, composeBindDescribe(t, dir, cfg, plan, "en"))
}

func TestDescribeComposeBinding_UltraToCustom_ShowsSupervisorAndEffortLadder(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindWritten(t, composeBindRepo)
	start := composeBindStart(t, cfg)
	plan := planComposeBinding(cfg, start, composeBindEdited(t, start, map[string]string{"planner": "sonnet"}))

	assert.Equal(t, []string{
		"quality.presets.custom 새로 씀 (16개 에이전트)",
		"quality.default ultra → custom",
		"Codex 기본 세션: 6-astra · ultra → 6-astra · xhigh",
		"custom은 ultra effort 사다리를 받지 않음",
		"quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)",
	}, composeBindDescribe(t, dir, cfg, plan, "ko"))
}

func TestDescribeComposeBinding_PresetKinds_NameTheirKeys(t *testing.T) {
	t.Parallel()
	hb2 := composeBindHB2(t)
	tests := []struct {
		name   string
		mutate func(*config.QualityConf)
		edits  map[string]string
		want   []string
	}{
		{
			name: "line writer back to the current balanced",
			mutate: func(q *config.QualityConf) {
				q.Presets["custom"] = config.QualityPreset{Agents: map[string]string{"planner": "haiku"}}
				q.Default = "custom"
			},
			edits: composeBindCurrent("balanced").Agents,
			want:  []string{"quality.default custom → balanced"},
		},
		{
			name: "historical balanced refreshed and bound",
			mutate: func(q *config.QualityConf) {
				q.Presets["balanced"] = config.QualityPreset{Agents: maps.Clone(hb2)}
				q.Default = "ultra"
			},
			edits: composeBindCurrent("balanced").Agents,
			want: []string{
				"quality.presets.balanced 최신 기본값으로 갱신 (16개 에이전트)",
				"quality.default ultra → balanced",
				"quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)",
			},
		},
		{
			name: "custom that is not the binding is overwritten",
			mutate: func(q *config.QualityConf) {
				q.Presets["custom"] = config.QualityPreset{Agents: map[string]string{"planner": "haiku"}}
			},
			edits: map[string]string{"planner": "opus"},
			want: []string{
				"quality.presets.custom 덮어씀 (16개 에이전트)",
				"quality.default balanced → custom",
				"custom은 balanced 표준 배치를 받지 않음",
				"quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)",
			},
		},
		{
			name: "bound custom is rewritten",
			mutate: func(q *config.QualityConf) {
				q.Presets["custom"] = config.QualityPreset{Agents: map[string]string{"planner": "haiku"}}
				q.Default = "custom"
			},
			edits: map[string]string{"planner": "opus"},
			want: []string{
				"quality.presets.custom 고쳐 씀 (16개 에이전트)",
				"quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, cfg := composeBindSaved(t, tt.mutate)
			start := composeBindStart(t, cfg)
			plan := planComposeBinding(cfg, start, composeBindEdited(t, start, tt.edits))

			assert.Equal(t, tt.want, composeBindDescribe(t, dir, cfg, plan, "ko"))
		})
	}
}

func TestDescribeComposeBinding_NoWrite_HasNoLines(t *testing.T) {
	t.Parallel()
	dir, cfg := composeBindSaved(t, nil)
	start := composeBindStart(t, cfg)

	assert.Empty(t, composeBindDescribe(t, dir, cfg, planComposeBinding(cfg, start, start.Clone()), "ko"))
}

func TestDescribeComposeBinding_ControlCharactersInADefaultName_AreEscaped(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("p")
	cfg.Quality.Presets["fast\x1b[31m"] = config.QualityPreset{Agents: map[string]string{"planner": "haiku"}}
	cfg.Quality.Default = "fast\x1b[31m"
	start := newQualityComposition(cfg.Quality, composeGroupsSources(t))
	plan := planComposeBinding(cfg, start, composeBindEdited(t, start, map[string]string{"planner": "opus"}))

	lines := describeComposeBinding(cfg, plan, driftGenerationSnapshot{}, nil, "en")

	assert.Contains(t, lines, `quality.default fast\x1b[31m → custom`)
	for _, line := range lines {
		assert.NotContains(t, line, "\x1b")
	}
}

func TestComposeBindProfileText_MissingEffort_ShowsDash(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "5.6-sol · -", composeBindProfileText(config.CodexProfile{Model: "gpt-5.6-sol"}))
	assert.Equal(t, "6-astra · ultra", composeBindProfileText(config.CodexProfile{Model: "gpt-6-astra", Effort: "ultra"}))
}

func TestQualityBlockCommentLines_CountsOnlyCommentsInsideTheBlock(t *testing.T) {
	t.Parallel()
	const fixture = `# head of file
project_name: demo
mode: full
platforms: [claude-code]
# before quality
quality:   # managed by auto
  # head comment inside
  default: balanced # trailing
# column-one note inside
  presets:
    balanced:
      description: "b # not a comment"
# outside note
workflow:
  coverage_threshold: 85 # outer trailing
`
	for want, data := range map[int][]string{
		4: {fixture},
		1: {blockHandPrefix + blockHandQuality + blockHandSuffix, blockColumnPrefix + blockColumnQuality + blockColumnSuffix},
		0: {blockAbsentPrefix, "quality: ["},
	} {
		for _, text := range data {
			count, ok := qualityBlockCommentLines([]byte(text))
			assert.True(t, ok, text)
			assert.Equal(t, want, count, text)
		}
	}
	// A line break the split at \n does not see leaves the block unlocated.
	_, ok := qualityBlockCommentLines([]byte("# a\rb\n" + fixture))
	assert.False(t, ok)
}
