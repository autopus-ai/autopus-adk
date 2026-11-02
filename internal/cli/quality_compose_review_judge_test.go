package cli

// SPEC-REVMODEL-001 T5 (REQ-RVM-04, 05; acceptance S3, S9, S18): the judge
// select, the count check right after participation, the after-error
// select, judge 없음, and a baseline that already has fewer than two
// providers.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const reviewAfterError = "멀티프로바이더 리뷰 \n1. 다시 고르기\n2. 나가기\nEnter"

// writeReviewR3Judge writes R3 with its judge line replaced by line: RN for
// "", RE for `judge: ""`.
func writeReviewR3Judge(t *testing.T, line string) string {
	t.Helper()
	dir := writeReviewR3(t)
	text := readReviewFile(t, dir)
	require.Contains(t, text, "        judge: claude\n")
	text = strings.Replace(text, "        judge: claude\n", line, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(text), 0o644))
	return dir
}

// S3(b): the judge off note, then only the participating providers.
func TestReviewTab_S3bTurnedOffJudgeIsChosenAgain(t *testing.T) {
	dir := writeReviewR3(t)
	before := readReviewFile(t, dir)

	// A new judge asks its judge model next; Enter keeps the reviewer model
	// (SPEC-JUDGEMODEL-001 REQ-JM-TUI-01).
	run := runReviewForms(t, dir, nil, reviewLines(reviewOpen,
		[]string{"2\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "2\n", "\n"})...)

	require.NoError(t, run.err, run.out)
	assert.Contains(t, run.out, "judge claude가 꺼져 judge를 다시 고릅니다\njudge \n1. codex\n2. gemini\nEnter")
	loaded := reviewLoaderValue(t, dir)
	assert.Equal(t, "codex", loaded.Spec.ReviewGate.Judge)
	assert.Equal(t, []string{"codex", "gemini"}, loaded.Spec.ReviewGate.Providers)
	assert.Equal(t, "claude", loaded.Orchestra.Judge)
	assert.Len(t, reviewFileDiff(t, before, readReviewFile(t, dir)), 2)
}

// S3(c)·(d): fewer than two providers stops the pass before any model or
// judge select; 다시 고르기 asks again from the baseline, 나가기 leaves.
func TestReviewTab_S3CountCheckRightAfterParticipation(t *testing.T) {
	cases := []struct {
		name   string
		lines  []string
		error  string
		after  string
		passes int
	}{
		{"one provider, choose again", []string{"\n", "2\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n"},
			"리뷰에는 provider가 두 개 이상 필요합니다 (지금 1개)", "변경 없음\n" + reviewCheckR3, 2},
		{"no provider, leave", []string{"2\n", "2\n", "2\n", "2\n", "\n"},
			"리뷰에는 provider가 두 개 이상 필요합니다 (지금 0개)", "구성을 바꿀까요?", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeReviewR3(t)
			sum := reviewFileSum(t, dir)

			run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, tc.lines)...)

			require.NoError(t, run.err, run.out)
			first := run.out[:strings.Index(run.out, tc.error)]
			assert.NotContains(t, first, "claude model")
			assert.NotContains(t, first, "judge \n")
			assert.Contains(t, run.out, "gemini 리뷰 참여 \n"+reviewKeepOn+"Enter a number between 1 and 2: \n"+tc.error+"\n"+reviewAfterError)
			rest := run.out[strings.Index(run.out, reviewAfterError):]
			assert.Contains(t, rest, tc.after)
			assert.Equal(t, tc.passes, strings.Count(run.out, "codex 리뷰 참여 \n"+reviewKeepOn))
			assert.Equal(t, sum, reviewFileSum(t, dir))
		})
	}
}

// S9(a)·(b)·(c): judge 없음 stays unless a judge is chosen; a missing key
// is added once, and judge: "" changes its value only.
func TestReviewTab_S9NoJudgeAndJudgeKeyInsertion(t *testing.T) {
	t.Run("RN keeps judge 없음", func(t *testing.T) {
		dir := writeReviewR3Judge(t, "")
		before := readReviewFile(t, dir)
		run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, []string{"\n", "\n", "2\n", "\n", "\n", "\n", "\n", "2\n", "\n"})...)
		require.NoError(t, run.err, run.out)
		assert.Contains(t, run.out, "judge \n1. 그대로 — judge 없음\n2. claude\n3. codex\nEnter")
		after := readReviewFile(t, dir)
		assert.NotContains(t, after, "        judge:")
		assert.Len(t, reviewFileDiff(t, before, after), 1)
	})
	// Choosing a judge asks its judge model next; Enter keeps the reviewer
	// model (SPEC-JUDGEMODEL-001 REQ-JM-TUI-01).
	t.Run("RN adds the judge key", func(t *testing.T) {
		dir := writeReviewR3Judge(t, "")
		before := readReviewFile(t, dir)
		run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, []string{"\n", "\n", "\n", "\n", "\n", "\n", "3\n", "\n", "\n", "2\n", "\n"})...)
		require.NoError(t, run.err, run.out)
		assert.Contains(t, run.out, "  spec.review_gate.judge judge 없음 → codex\n")
		assert.Equal(t, strings.Replace(before, "        doc_context_max_lines: 200\n",
			"        doc_context_max_lines: 200\n        judge: codex\n", 1), readReviewFile(t, dir))
		assert.Equal(t, "codex", reviewLoaderValue(t, dir).Spec.ReviewGate.Judge)
	})
	t.Run("RE changes the value", func(t *testing.T) {
		dir := writeReviewR3Judge(t, "        judge: \"\"\n")
		before := readReviewFile(t, dir)
		run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, []string{"\n", "\n", "\n", "\n", "\n", "\n", "2\n", "\n", "\n", "2\n", "\n"})...)
		require.NoError(t, run.err, run.out)
		assert.Equal(t, [][2]string{{"        judge: \"\"", "        judge: claude"}}, reviewFileDiff(t, before, readReviewFile(t, dir)))
	})
}

// S18: a baseline with one provider warns before the first select, can
// leave without writing, and can turn a second provider on.
func TestReviewTab_S18BaselineBelowTwoProviders(t *testing.T) {
	rb := func(cfg *config.HarnessConfig) { cfg.Spec.ReviewGate.Providers = []string{"claude"} }
	warn := "지금 리뷰 provider가 1개뿐입니다. 두 개 이상 켜야 적용할 수 있습니다\nclaude 리뷰 참여 \n"

	t.Run("leave", func(t *testing.T) {
		dir := writeReviewR3(t, rb)
		sum := reviewFileSum(t, dir)
		run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, []string{"\n", "\n", "\n", "2\n", "\n"})...)
		require.NoError(t, run.err, run.out)
		assert.Contains(t, run.out, warn)
		assert.Contains(t, run.out, "codex 리뷰 참여 \n"+reviewKeepOff)
		assert.Contains(t, run.out, "리뷰에는 provider가 두 개 이상 필요합니다 (지금 1개)\n"+reviewAfterError)
		assert.Equal(t, sum, reviewFileSum(t, dir))
	})
	t.Run("turn codex on", func(t *testing.T) {
		dir := writeReviewR3(t, rb)
		run := runReviewForms(t, dir, nil, reviewLines(reviewOpen, []string{"\n", "2\n", "\n", "\n", "\n", "\n", "\n", "2\n", "\n"})...)
		require.NoError(t, run.err, run.out)
		assert.Contains(t, run.out, "  spec.review_gate.providers [claude] → [claude, codex]\n")
		loaded := reviewLoaderValue(t, dir)
		assert.Equal(t, []string{"claude", "codex"}, loaded.Spec.ReviewGate.Providers)
		assert.Equal(t, "claude", loaded.Spec.ReviewGate.Judge)
	})
}
