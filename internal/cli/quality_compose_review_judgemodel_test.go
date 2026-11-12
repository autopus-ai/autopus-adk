package cli

// SPEC-JUDGEMODEL-001 S7, S8, S9 (REQ-JM-TUI-01..03): the review tab picks
// the judge model of the kept judge through the judge select's last option,
// asks a new judge's model at once, clears it through 리뷰어 model과 같음,
// writes only the key lines, and leaves a placeholder judge model alone.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The kept judge's model: option 4, a Claude tab model and its effort.
func TestReviewTab_JudgeModelForTheKeptJudge(t *testing.T) {
	dir := writeReviewRL(t)
	replaceReviewFile(t, dir, "    review_gate:\n", "    # who reviews SPECs\n    review_gate:\n")
	b0 := readReviewFile(t, dir)

	// participation Enter ×3, claude and codex models Enter, judge model
	// option, claude-opus-5-5, high, check Enter, 적용, 유지.
	run := runReviewCLIForms(t, dir, []string{"claude"}, reviewLines(reviewOpen,
		[]string{"\n", "\n", "\n", "\n", "\n", "4\n", "3\n", "3\n", "\n", "2\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "judge \n1. 그대로 — claude\n2. codex\n3. gemini\n4. claude — judge model 고르기\nEnter")
	assert.Contains(t, run.out, "claude judge model \n1. 그대로 — 리뷰어 model과 같음\n2. claude-fable-5-1\n3. claude-opus-5-5\n"+
		"4. claude-sonnet-5-5\n5. claude-haiku-5-5\nEnter")
	assert.Contains(t, run.out, "claude judge effort \n1. low\n2. medium\n3. high\n4. xhigh\n5. max\nEnter")
	assert.Contains(t, run.out, "바뀌는 설정:\n  spec.review_gate.judge_model 리뷰어 model → claude-opus-5-5\n"+
		"  spec.review_gate.judge_effort 없음 → high\n"+composeReviewTextKO.JudgeModelNote+"\n")
	assert.Contains(t, run.out, "judge (claude-opus-5-5 · high) *")
	assert.Equal(t, strings.Replace(b0, reviewGateTail, reviewGateTail+"        judge_model: claude-opus-5-5\n        judge_effort: high\n", 1),
		readReviewFile(t, dir), "only the two key lines are added; the comment stays")
	assert.Contains(t, run.out[strings.Index(run.out, "구성을 적용했습니다."):], "· judge claude (claude-opus-5-5 · high)\n",
		"the redrawn summary names the judge model")

	loaded := reviewLoaderValueKeepCwd(t, dir)
	judge, err := assembleSpecReviewJudge(t.Context(), loaded, nil, "claude", 0)
	require.NoError(t, err)
	assert.True(t, reviewArgsHasRun(judge.Args, "--model", "claude-opus-5-5", "--effort", "high"), "%v", judge.Args)
	assert.Equal(t, reviewRLClaudeArgs, "            "+reviewArgsLine(t, readReviewFile(t, dir), "claude"), "the reviewer's args stay")
}

// A new judge asks its model at once; 리뷰어 model과 같음 removes the keys.
func TestReviewTab_NewJudgeAsksItsModelAndClears(t *testing.T) {
	dir := writeReviewR3(t)
	b0 := readReviewFile(t, dir)
	models := []string{"\n", "\n", "\n", "\n", "\n", "\n"}

	// judge codex, openai-codex/gpt-6-luna, thinking Enter (high), check Enter, 적용, 유지.
	run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, models, []string{"2\n", "3\n", "\n", "\n", "2\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "codex judge model \n1. 그대로 — 리뷰어 model과 같음\n2. openai-codex/gpt-6-astra\n3. openai-codex/gpt-6-luna\nEnter")
	assert.Contains(t, run.out, "codex judge thinking (openai-codex/gpt-6-luna) \n1. high\n2. low\n3. medium\nEnter")
	assert.Contains(t, run.out, "  spec.review_gate.judge claude → codex\n  spec.review_gate.judge_model 리뷰어 model → openai-codex/gpt-6-luna\n")
	b1 := strings.Replace(strings.Replace(b0, "        judge: claude\n", "        judge: codex\n", 1),
		reviewGateTail, reviewGateTail+"        judge_model: openai-codex/gpt-6-luna\n        judge_effort: high\n", 1)
	assert.Equal(t, b1, readReviewFile(t, dir))
	loaded := reviewLoaderValueKeepCwd(t, dir)
	judge, err := assembleSpecReviewJudge(t.Context(), loaded, nil, "codex", 0)
	require.NoError(t, err)
	assert.Equal(t, "openai-codex/gpt-6-luna:high", judge.Model)

	// The kept judge's model option, 리뷰어 model과 같음, check Enter, 적용, 유지.
	run = runReviewForms(t, dir, nil, reviewLines(reviewOpen, models, []string{"4\n", "2\n", "\n", "2\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "judge \n1. 그대로 — codex (openai-codex/gpt-6-luna:high)\n2. claude\n3. gemini\n4. codex — judge model 고르기\nEnter")
	assert.Contains(t, run.out, "codex judge model \n1. 그대로 — openai-codex/gpt-6-luna:high\n2. 리뷰어 model과 같음\n"+
		"3. openai-codex/gpt-6-astra\n4. openai-codex/gpt-6-luna\nEnter")
	assert.Contains(t, run.out, "  spec.review_gate.judge_model openai-codex/gpt-6-luna → 리뷰어 model\n  spec.review_gate.judge_effort high → 없음\n")
	assert.NotContains(t, run.out, composeReviewTextKO.JudgeModelNote, "the note follows only a judge model that is set")
	assert.Equal(t, strings.Replace(b0, "        judge: claude\n", "        judge: codex\n", 1), readReviewFile(t, dir))
}

// Cancel, keeping the judge, and the judge model's keep option write nothing.
func TestReviewTab_JudgeModelKeepAndCancelWriteNothing(t *testing.T) {
	dir := writeReviewR3(t)
	sum := reviewFileSum(t, dir)
	models := []string{"\n", "\n", "\n", "\n", "\n", "\n"}

	for name, answers := range map[string][]string{
		"cancel":         {"4\n", "3\n", "\n", "\n", "\n", "\n"},
		"keep the judge": {"\n", "\n", "\n"},
		"keep its model": {"4\n", "\n", "\n", "\n"},
	} {
		run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, models, answers)...)
		require.NoError(t, run.err, "%s\n%s", name, run.out)
		assert.Equal(t, sum, reviewFileSum(t, dir), name)
	}
}

// A judge model written as a placeholder: the refusal line replaces the
// judge select, and the file stays byte for byte.
func TestReviewTab_PlaceholderJudgeModelIsLeft(t *testing.T) {
	t.Setenv("JUDGE_MODEL", "anthropic/claude-opus-5-5")
	dir := writeReviewR3(t)
	replaceReviewFile(t, dir, reviewGateTail, reviewGateTail+"        judge_model: ${JUDGE_MODEL}\n")
	sum := reviewFileSum(t, dir)

	run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, []string{"\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "spec.review_gate의 judge_model 값이 환경 변수라 이 탭에서 바꾸지 않습니다: ${JUDGE_MODEL}\n")
	assert.NotContains(t, run.out, "judge \n1.")
	assert.NotContains(t, run.out, "claude-opus-5-5 ", "the expanded value never shows")
	assert.Equal(t, sum, reviewFileSum(t, dir))
}
