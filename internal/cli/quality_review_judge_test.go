package cli

// SPEC-JUDGEMODEL-001 S5, S6 (REQ-JM-CLI-01..04, COMPAT-01): `auto quality
// review judge --model` sets the judge model with the tab's lists and line
// writer, default and a judge change clear it by removing the key lines,
// refusals write nothing, and show lists the keys as written.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	reviewGateTail      = "        doc_context_max_lines: 200\n"
	reviewJudgeModelOMP = "        judge_model: anthropic/claude-opus-5-5\n        judge_effort: max\n"
)

// writeReviewR3Commented is R3 with a comment line above the review gate and
// an inline comment on the judge line, which every write must keep.
func writeReviewR3Commented(t *testing.T) string {
	t.Helper()
	dir := writeReviewR3(t)
	replaceReviewFile(t, dir, "    review_gate:\n", "    # who reviews SPECs\n    review_gate:\n")
	replaceReviewFile(t, dir, "        judge: claude\n", "        judge: claude # arbiter\n")
	return dir
}

func TestQualityReviewJudge_SetsAndClearsTheJudgeModel(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewR3Commented(t)
	b0 := readReviewFile(t, dir)
	changes := "Settings that change:\n" +
		"  spec.review_gate.judge_model reviewer model → anthropic/claude-opus-5-5\n" +
		"  spec.review_gate.judge_effort none → max\n" + composeReviewTextEN.JudgeModelNote + "\n"

	out, _, err := runQualityReview(t, dir, nil, "judge", "claude", "--model", "anthropic/claude-opus-5-5", "--effort", "max", "--dry-run")
	require.NoError(t, err, out)
	assert.Equal(t, changes+"--dry-run: nothing written.\n", out)
	assert.Equal(t, b0, readReviewFile(t, dir))

	out, _, err = runQualityReview(t, dir, nil, "judge", "claude", "--model", "anthropic/claude-opus-5-5:max")
	require.NoError(t, err, out)
	assert.Equal(t, changes+"Composition applied.\n", out)
	b1 := strings.Replace(b0, reviewGateTail, reviewGateTail+reviewJudgeModelOMP, 1)
	assert.Equal(t, b1, readReviewFile(t, dir), "only the two key lines are added")
	loaded := reviewLoaderValue(t, dir)
	assert.Equal(t, [2]string{"anthropic/claude-opus-5-5", "max"}, [2]string{loaded.Spec.ReviewGate.JudgeModel, loaded.Spec.ReviewGate.JudgeEffort})

	out, _, err = runQualityReview(t, dir, nil, "judge", "claude")
	require.NoError(t, err)
	assert.Equal(t, "No changes; nothing written.\n", out, "keeping the judge keeps its model")

	out, _, err = runQualityReview(t, dir, nil, "judge", "claude", "--model", "default")
	require.NoError(t, err, out)
	assert.Equal(t, "Settings that change:\n"+
		"  spec.review_gate.judge_model anthropic/claude-opus-5-5 → reviewer model\n"+
		"  spec.review_gate.judge_effort max → none\nComposition applied.\n", out)
	assert.Equal(t, b0, readReviewFile(t, dir), "clearing removes exactly the key lines")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(b1), 0o644))
	out, _, err = runQualityReview(t, dir, nil, "judge", "codex")
	require.NoError(t, err, out)
	assert.Contains(t, out, "  spec.review_gate.judge claude → codex\n  spec.review_gate.judge_model anthropic/claude-opus-5-5 → reviewer model\n")
	assert.Equal(t, strings.Replace(b0, "judge: claude # arbiter", "judge: codex # arbiter", 1), readReviewFile(t, dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(b1), 0o644))
	out, _, err = runQualityReview(t, dir, nil, "judge", "none")
	require.NoError(t, err, out)
	assert.Contains(t, out, "  spec.review_gate.judge claude → no judge\n  spec.review_gate.judge_model anthropic/claude-opus-5-5 → none\n")
	assert.Equal(t, strings.Replace(b0, "judge: claude # arbiter", `judge: "" # arbiter`, 1), readReviewFile(t, dir))
	assert.Empty(t, reviewLoaderValue(t, dir).Spec.ReviewGate.JudgeModel)
}

// A CLI judge takes a Claude tab model; without --effort the effort follows
// the tab's default, the reviewer's when the model takes it.
func TestQualityReviewJudge_CLIEffortDefault(t *testing.T) {
	useQualityReviewSeams(t, "claude")
	dir := writeReviewRL(t)
	b0 := readReviewFile(t, dir)

	out, _, err := runQualityReview(t, dir, nil, "judge", "claude", "--model", "claude-opus-5-5")
	require.NoError(t, err, out)
	assert.Contains(t, out, "  spec.review_gate.judge_model reviewer model → claude-opus-5-5\n  spec.review_gate.judge_effort none → max\n")
	assert.Equal(t, strings.Replace(b0, reviewGateTail, reviewGateTail+"        judge_model: claude-opus-5-5\n        judge_effort: max\n", 1),
		readReviewFile(t, dir))
	assert.Equal(t, reviewRLClaudeArgs, "            "+reviewArgsLine(t, readReviewFile(t, dir), "claude"), "the reviewer's args stay")

	out, _, err = runQualityReview(t, dir, nil, "judge", "claude", "--model", "claude-haiku-5-5")
	require.NoError(t, err, out)
	assert.Contains(t, out, "  spec.review_gate.judge_model claude-opus-5-5 → claude-haiku-5-5\n")
	assert.NotContains(t, out, "judge_effort max →")
	assert.Contains(t, readReviewFile(t, dir), "        judge_model: claude-haiku-5-5\n        judge_effort: max\n")
}

func TestQualityReviewJudge_Refusals(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewR3(t)
	b0 := readReviewFile(t, dir)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"claude", "--effort", "max"}, "--effort needs --model"},
		{[]string{"none", "--model", "x"}, "judge none takes no judge model"},
		{[]string{"claude", "--model", "default", "--effort", "max"}, "--model default takes no --effort"},
		{[]string{"claude", "--model", "anthropic/claude-opus-9"}, "The installed OMP catalog does not attest claude judge model anthropic/claude-opus-9 " +
			"(accepted: anthropic/claude-fable-5-1 (high, low, max, medium), anthropic/claude-opus-5-5 (high, max))"},
		{[]string{"claude", "--model", "anthropic/claude-opus-5-5", "--effort", "low"}, "anthropic/claude-opus-5-5 reports no thinking level low for claude judge (accepted: high, max)"},
	} {
		_, _, err := runQualityReview(t, dir, nil, append([]string{"judge"}, tc.args...)...)
		require.EqualError(t, err, tc.want, "%v", tc.args)
	}
	assert.Equal(t, b0, readReviewFile(t, dir))

	t.Setenv("JUDGE_MODEL", "anthropic/claude-opus-5-5")
	placeholder := writeReviewR3(t)
	replaceReviewFile(t, placeholder, reviewGateTail, reviewGateTail+"        judge_model: ${JUDGE_MODEL}\n")
	before := readReviewFile(t, placeholder)
	for _, args := range [][]string{{"judge", "claude", "--model", "x"}, {"judge", "codex"}} {
		_, _, err := runQualityReview(t, placeholder, nil, args...)
		require.EqualError(t, err, "judge_model of spec.review_gate comes from an environment variable; this tab leaves it: ${JUDGE_MODEL}", "%v", args)
	}
	assert.Equal(t, before, readReviewFile(t, placeholder))
}

func TestQualityReviewJudge_ShowListsTheJudgeModel(t *testing.T) {
	useQualityReviewSeams(t)
	dir := writeReviewR3(t)
	plain, _, err := runQualityReview(t, dir, nil, "show")
	require.NoError(t, err)
	plainJSON, _, err := runQualityReview(t, dir, nil, "show", "--json")
	require.NoError(t, err)
	assert.NotContains(t, plain+plainJSON, "judge_model")
	assert.NotContains(t, plainJSON, "judge_effort")

	replaceReviewFile(t, dir, reviewGateTail, reviewGateTail+reviewJudgeModelOMP)
	out, _, err := runQualityReview(t, dir, nil)
	require.NoError(t, err)
	assert.Contains(t, out, "spec.review_gate.judge = claude\n"+
		"spec.review_gate.judge_model = anthropic/claude-opus-5-5\nspec.review_gate.judge_effort = max\nprovider ")
	assert.Equal(t, strings.Replace(plain, "spec.review_gate.judge = claude\n", "spec.review_gate.judge = claude\n"+
		"spec.review_gate.judge_model = anthropic/claude-opus-5-5\nspec.review_gate.judge_effort = max\n", 1), out)

	out, _, err = runQualityReview(t, dir, nil, "show", "--json")
	require.NoError(t, err)
	var envelope struct {
		Data qualityReviewSettings `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope), out)
	assert.Equal(t, [2]string{"anthropic/claude-opus-5-5", "max"}, [2]string{envelope.Data.JudgeModel, envelope.Data.JudgeEffort})
	assert.Contains(t, out, `"judge_model": "anthropic/claude-opus-5-5"`)
}
