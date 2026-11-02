package config

// SPEC-JUDGEMODEL-001 S1 (REQ-JM-CFG-01, CFG-02): the judge model keys are
// optional, validated with the dotted key in every error, kept by a save,
// and absent from a saved default configuration.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewGateJudgeModel_LoadsValidatesAndRoundTrips(t *testing.T) {
	t.Parallel()

	cfg, err := loadReviewBudgetConfig(t, "    judge: claude\n    judge_model: claude-opus-5-5\n    judge_effort: high\n")
	require.NoError(t, err)
	assert.Equal(t, "claude-opus-5-5", cfg.Spec.ReviewGate.JudgeModel)
	assert.Equal(t, "high", cfg.Spec.ReviewGate.JudgeEffort)

	dir := t.TempDir()
	require.NoError(t, Save(dir, cfg))
	saved, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, [3]string{"claude", "claude-opus-5-5", "high"},
		[3]string{saved.Spec.ReviewGate.Judge, saved.Spec.ReviewGate.JudgeModel, saved.Spec.ReviewGate.JudgeEffort}, "a save keeps both keys")
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "judge_model: claude-opus-5-5\n")
	assert.Contains(t, string(data), "judge_effort: high\n")

	cfg, err = loadReviewBudgetConfig(t, "    judge: claude\n    judge_model: anthropic/claude-opus-5-5:max\n")
	require.NoError(t, err, "an OMP selector carries its thinking without judge_effort")
	assert.Equal(t, "anthropic/claude-opus-5-5:max", cfg.Spec.ReviewGate.JudgeModel)

	cases := map[string]string{
		"    judge_model: claude-opus-5-5\n":                                      "spec.review_gate.judge_model needs spec.review_gate.judge",
		"    judge: \"\"\n    judge_effort: high\n":                               "spec.review_gate.judge_effort needs spec.review_gate.judge",
		"    judge: claude\n    judge_effort: high\n":                             "spec.review_gate.judge_effort needs spec.review_gate.judge_model",
		"    judge: claude\n    judge_model: -x\n":                                "spec.review_gate.judge_model must not start with \"-\" or hold whitespace or control characters",
		"    judge: claude\n    judge_model: \"a b\"\n":                           "spec.review_gate.judge_model must not start with \"-\" or hold whitespace or control characters",
		"    judge: claude\n    judge_model: \"a\\tb\"\n":                         "spec.review_gate.judge_model must not start with",
		"    judge: claude\n    judge_model: m\n    judge_effort: \"x\\u0007\"\n": "spec.review_gate.judge_effort must not start with",
		"    judge: claude\n    judge_mode1: x\n":                                 "judge_mode1",
	}
	for section, want := range cases {
		_, err := loadReviewBudgetConfig(t, section)
		require.Error(t, err, section)
		assert.Contains(t, err.Error(), want, section)
	}
}

func TestReviewGateJudgeModel_DefaultConfigWritesNoKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, Save(dir, DefaultFullConfig("judge-model-default")))
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(data), "judge_model"), "a default config must not write judge_model")
	assert.False(t, strings.Contains(string(data), "judge_effort"), "a default config must not write judge_effort")
}

// B3: a decoder without the fields refuses the key, which is why the keys
// are written only when a judge model is set (REQ-JM-COMPAT-01).
func TestReviewGateJudgeModel_UnknownToAStrictDecoderWithoutTheField(t *testing.T) {
	t.Parallel()

	var older struct {
		Judge string `yaml:"judge"`
	}
	_, err := decodeStrict([]byte("judge: claude\njudge_model: claude-opus-5-5\n"), &older, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "judge_model")
}
