package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Scope expansion of SPEC-REA-001 Gate 1: only the standard placement of an
// analysis-rung agent (reverse-engineer, qa-explorer) falls back to the Codex
// session model, and only when a read catalog lacks the rung's model or effort.
func TestCodexReaAnalysisFallback_OnlyTheStandardAnalysisPlacement(t *testing.T) {
	t.Parallel()
	balanced := DefaultFullConfig("rea-codex").Quality
	lacking := []CodexResolutionReason{
		CodexResolutionModelUnavailable, CodexResolutionEffortUnavailable, CodexResolutionRuntimeDefault,
	}
	for _, reason := range lacking {
		answer := CodexProfileResolution{Fallback: true, Reason: reason}
		assert.True(t, balanced.CodexReaAnalysisFallback("reverse-engineer", answer), reason)
		assert.True(t, balanced.CodexReaAnalysisFallback("reverse_engineer", answer), reason)
		assert.True(t, balanced.CodexReaAnalysisFallback("qa-explorer", answer), reason)
		for _, agent := range []string{"planner", "executor", "explorer", "reviewer", "my-agent"} {
			assert.False(t, balanced.CodexReaAnalysisFallback(agent, answer), "%s %s", agent, reason)
		}
	}
	for _, answer := range []CodexProfileResolution{
		{Reason: CodexResolutionSupported},
		{Fallback: true, Reason: CodexResolutionCatalogUnknown},
		{Reason: CodexResolutionCatalogUnknown},
	} {
		assert.False(t, balanced.CodexReaAnalysisFallback("reverse-engineer", answer), answer.Reason)
	}

	missing := CodexProfileResolution{Fallback: true, Reason: CodexResolutionModelUnavailable}
	custom := DefaultFullConfig("rea-codex-custom").Quality
	custom.Presets["balanced"].Agents["reverse-engineer"] = "sonnet"
	assert.False(t, custom.CodexReaAnalysisFallback("reverse-engineer", missing), "a custom tier keeps its path")
	custom.Presets["balanced"].Agents["qa-explorer"] = "sonnet"
	assert.False(t, custom.CodexReaAnalysisFallback("qa-explorer", missing), "a custom tier keeps its path")

	ultra := DefaultFullConfig("rea-codex-ultra").Quality
	ultra.Default = "ultra"
	assert.False(t, ultra.CodexReaAnalysisFallback("reverse-engineer", missing), "ultra keeps its ladder")
}

func TestCodexSessionModelResolution_WritesNoModelOrEffort(t *testing.T) {
	t.Parallel()
	requested := CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}
	assert.Equal(t, CodexProfileResolution{
		Requested: requested, Fallback: true, Reason: CodexResolutionSessionModel,
	}, CodexSessionModelResolution(requested))
	assert.Equal(t, CodexResolutionReason("session_model"), CodexResolutionSessionModel)
}
