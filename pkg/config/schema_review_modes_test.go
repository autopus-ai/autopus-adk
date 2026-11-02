package config

// SPEC-REVIEWREUSE-001 S1 (REQ-RR-JDG-01): the judge mode is optional,
// validated with the dotted key and the allowed values, and absent from a
// saved default configuration.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewGateModes_LoadAndValidate(t *testing.T) {
	t.Parallel()

	cfg, err := loadReviewBudgetConfig(t, "    judge: claude\n")
	require.NoError(t, err)
	assert.Empty(t, cfg.Spec.ReviewGate.JudgeMode, "omitted means always")

	for _, mode := range []string{"always", "on_disagreement", "on_escalation"} {
		cfg, err := loadReviewBudgetConfig(t, "    judge_mode: "+mode+"\n")
		require.NoError(t, err, mode)
		assert.Equal(t, mode, cfg.Spec.ReviewGate.JudgeMode)
	}

	_, err = loadReviewBudgetConfig(t, "    judge_mode: sometimes\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `spec.review_gate.judge_mode must be one of always, on_disagreement, on_escalation, got "sometimes"`)
}

func TestReviewGateModes_DefaultConfigWritesNoJudgeMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, Save(dir, DefaultFullConfig("modes-default")))
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(data), "judge_mode"))
}
