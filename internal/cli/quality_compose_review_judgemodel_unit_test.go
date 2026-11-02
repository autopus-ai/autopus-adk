package cli

// SPEC-JUDGEMODEL-001 (REQ-JM-TUI-02, CLI-01, COMPAT-01): the planner holds a
// changed judge model against the judge's list, init's memory path carries
// it, and the line removal takes a quoted value with its comment but
// refuses a value it cannot remove safely.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewJudgeModel_PlannerAndMemoryPath(t *testing.T) {
	t.Parallel()

	cfg := reviewR3Config()
	reviewRLConfig(cfg)
	baseline := newReviewBaseline(cfg, nil)
	attested := newReviewAttestation()
	attested.CLI["claude"] = reviewClaudeModels()

	working := baseline.reviewComposition.clone()
	working.JudgeModel = reviewJudgeModel{Model: "claude-opus-9", Effort: "max"}
	_, err := planReviewComposition(baseline, working, attested, composeReviewTextEN)
	require.EqualError(t, err, "claude-opus-9 is not a choice for claude judge")

	working.JudgeModel = reviewJudgeModel{Model: "claude-haiku-4-5", Effort: "max"}
	_, err = planReviewComposition(baseline, working, attested, composeReviewTextEN)
	require.EqualError(t, err, "claude-haiku-4-5 does not take effort max for claude judge")

	working.Judge = ""
	working.JudgeModel = reviewJudgeModel{Model: "claude-opus-5-5", Effort: "max"}
	_, err = planReviewComposition(baseline, working, attested, composeReviewTextEN)
	require.EqualError(t, err, composeReviewTextEN.JudgeModelNoJudge)

	working.Judge = "claude"
	plan, err := planReviewComposition(baseline, working, attested, composeReviewTextEN)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"spec.review_gate.judge_model reviewer model → claude-opus-5-5", "spec.review_gate.judge_effort none → max",
	}, plan.Changes)
	assert.Equal(t, []string{composeReviewTextEN.JudgeModelNote}, plan.Notes)

	require.NoError(t, applyReviewCompositionMemory(cfg, baseline, working))
	assert.Equal(t, [2]string{"claude-opus-5-5", "max"}, [2]string{cfg.Spec.ReviewGate.JudgeModel, cfg.Spec.ReviewGate.JudgeEffort})
	summary := reviewSummaryLine(newReviewBaseline(cfg, nil), composeReviewTextEN)
	assert.True(t, strings.HasSuffix(summary, " · judge claude (claude-opus-5-5 · max)"), summary)
	assert.True(t, strings.HasSuffix(reviewSummaryLine(baseline, composeReviewTextEN), " · judge claude"), "no judge model, no suffix")
}

func TestReviewJudgeModel_LineRemoval(t *testing.T) {
	useQualityReviewSeams(t)

	quoted := writeReviewR3(t)
	b0 := readReviewFile(t, quoted)
	replaceReviewFile(t, quoted, reviewGateTail, reviewGateTail+"        judge_model: \"anthropic/claude-opus-5-5:max\" # pinned\n")
	out, _, err := runQualityReview(t, quoted, nil, "judge", "claude", "--model", "default")
	require.NoError(t, err, out)
	assert.Equal(t, b0, readReviewFile(t, quoted), "the quoted value and its comment go with the line")

	for name, line := range map[string]string{
		"block scalar": "        judge_model: |-\n            anthropic/claude-opus-5-5:max\n",
		"anchor":       "        judge_model: &pick anthropic/claude-opus-5-5:max\n",
	} {
		dir := writeReviewR3(t)
		replaceReviewFile(t, dir, reviewGateTail, reviewGateTail+line)
		before := readReviewFile(t, dir)
		_, _, err := runQualityReview(t, dir, nil, "judge", "claude", "--model", "default")
		require.EqualError(t, err, "The review settings in autopus.yaml cannot be edited safely; nothing was written: spec.review_gate.judge_model", name)
		assert.Equal(t, before, readReviewFile(t, dir), name)
	}
}

// An OMP judge model written with its thinking inline and the same
// judge_effort beside it: show prints both keys as written, while the menu
// summary names the level once.
func TestReviewJudgeModel_InlineThinkingShowsOnce(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewR3(t)
	replaceReviewFile(t, dir, reviewGateTail, reviewGateTail+"        judge_model: anthropic/claude-opus-5-5:max\n        judge_effort: max\n")

	out, _, err := runQualityReview(t, dir, nil, "show")
	require.NoError(t, err)
	assert.Contains(t, out, "spec.review_gate.judge_model = anthropic/claude-opus-5-5:max\nspec.review_gate.judge_effort = max\n")
	line := reviewSummaryLine(newReviewBaseline(reviewLoaderValueKeepCwd(t, dir), readReviewRaw(dir)), composeReviewTextEN)
	assert.True(t, strings.HasSuffix(line, " · judge claude (anthropic/claude-opus-5-5:max)"), line)
}
