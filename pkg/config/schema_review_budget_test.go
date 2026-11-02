package config

// SPEC-REVIEWPOLICY-001 S9 (REQ-RP-BUD-01): the review budget keys are
// optional, validated with the dotted key in every error, and absent from a
// saved default configuration.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadReviewBudgetConfig(t *testing.T, budget string) (*HarnessConfig, error) {
	t.Helper()
	dir := t.TempDir()
	content := "mode: full\nproject_name: test\nplatforms:\n  - claude-code\nspec:\n" +
		"  id_format: \"SPEC-{DOMAIN}-{NUMBER}\"\n  review_gate:\n    enabled: true\n" + budget
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(content), 0o644))
	return Load(dir)
}

func TestReviewBudgetConf_LoadsAndValidates(t *testing.T) {
	t.Parallel()

	cfg, err := loadReviewBudgetConfig(t, "    judge: claude\n")
	require.NoError(t, err)
	assert.Equal(t, ReviewBudgetConf{}, cfg.Spec.ReviewGate.Budget, "absent section means no limit")

	cfg, err = loadReviewBudgetConfig(t, "    budget:\n      max_calls: 12\n      max_minutes: 30\n      max_tokens: 400000\n      max_cost_usd: 2.5\n")
	require.NoError(t, err)
	assert.Equal(t, ReviewBudgetConf{MaxCalls: 12, MaxMinutes: 30, MaxTokens: 400000, MaxCostUSD: 2.5}, cfg.Spec.ReviewGate.Budget)

	cases := map[string]string{
		"    budget:\n      max_calls: -1\n":      "spec.review_gate.budget.max_calls must be >= 0",
		"    budget:\n      max_minutes: -1\n":    "spec.review_gate.budget.max_minutes must be >= 0",
		"    budget:\n      max_tokens: -5\n":     "spec.review_gate.budget.max_tokens must be >= 0",
		"    budget:\n      max_cost_usd: -0.5\n": "spec.review_gate.budget.max_cost_usd must be >= 0",
		"    budget:\n      max_cost_usd: .inf\n": "spec.review_gate.budget.max_cost_usd must be finite",
		"    budget:\n      max_cost_usd: .nan\n": "spec.review_gate.budget.max_cost_usd must be finite",
		"    budget:\n      max_call: 3\n":        "max_call",
	}
	for section, want := range cases {
		_, err := loadReviewBudgetConfig(t, section)
		require.Error(t, err, section)
		assert.Contains(t, err.Error(), want, section)
	}
}

func TestReviewBudgetConf_DefaultConfigWritesNoBudgetKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, Save(dir, DefaultFullConfig("budget-default")))
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(data), "budget:"), "a default config must not write the budget section")

	cfg := DefaultFullConfig("budget-round-trip")
	cfg.Spec.ReviewGate.Budget = ReviewBudgetConf{MaxCalls: 6}
	require.NoError(t, Save(dir, cfg))
	reloaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, ReviewBudgetConf{MaxCalls: 6}, reloaded.Spec.ReviewGate.Budget)
}
