package spec_test

// SPEC-REVIEWREUSE-001 S5 (REQ-RR-JDG-05): the deterministic merge keeps a
// minority critical, security, data-loss, or escape-hatch claim, and a merged
// finding carries its most severe severity and every reviewer that raised it.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/spec"
)

func providerFinding(provider, severity, category, description string) spec.ReviewFinding {
	return spec.ReviewFinding{Provider: provider, Severity: severity, Category: spec.FindingCategory(category),
		ScopeRef: "REQ-001", Description: description, Status: spec.FindingStatusOpen}
}

func TestMergeSupermajority_KeepsMinorityEscalationClaims(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		finding spec.ReviewFinding
		kept    bool
	}{
		{"critical correctness", providerFinding("gemini", "critical", "correctness", "Rollback loses committed rows"), true},
		{"minor data loss", providerFinding("gemini", "minor", "data_loss", "Purge drops the archive"), true},
		{"minor security", providerFinding("gemini", "minor", "security", "Token is logged"), true},
		{"escape hatch", func() spec.ReviewFinding {
			f := providerFinding("gemini", "major", "completeness", "Regression in retry")
			f.EscapeHatch = true
			return f
		}(), true},
		{"major completeness", providerFinding("gemini", "major", "completeness", "Error path undefined"), false},
	}
	for _, tc := range cases {
		merged := spec.MergeSupermajority([]spec.ReviewFinding{tc.finding}, 3, 0.67)
		if !tc.kept {
			assert.Empty(t, merged, tc.name)
			continue
		}
		require.Len(t, merged, 1, tc.name)
		assert.Equal(t, []string{"gemini"}, merged[0].Sources, tc.name)
		assert.Equal(t, tc.finding.Severity, merged[0].Severity, tc.name)
	}
}

func TestMergeSupermajority_MergedFindingCarriesMostSevereSeverityAndSources(t *testing.T) {
	t.Parallel()

	merged := spec.MergeSupermajority([]spec.ReviewFinding{
		providerFinding("claude", "minor", "correctness", "Retry is unbounded"),
		providerFinding("codex", "critical", "correctness", "Retry is unbounded"),
		providerFinding("claude", "minor", "correctness", "Retry is unbounded"),
	}, 3, 0.67)

	require.Len(t, merged, 1)
	assert.Equal(t, "critical", merged[0].Severity)
	assert.Equal(t, []string{"claude", "codex"}, merged[0].Sources)
	assert.Equal(t, "claude", merged[0].Provider, "the first member stays the reporting provider")
}

func TestDeduplicateFindings_UnionsSourcesAndSeverity(t *testing.T) {
	t.Parallel()

	escape := providerFinding("codex", "minor", "completeness", "Same text")
	escape.EscapeHatch = true
	deduped := spec.DeduplicateFindings([]spec.ReviewFinding{
		providerFinding("claude", "minor", "completeness", "Same text"),
		escape,
		providerFinding("gemini", "major", "completeness", "Other text"),
	})

	require.Len(t, deduped, 2)
	assert.Equal(t, "F-001", deduped[0].ID)
	assert.Equal(t, []string{"claude", "codex"}, deduped[0].Sources)
	assert.True(t, deduped[0].EscapeHatch)
	assert.Equal(t, []string{"gemini"}, deduped[1].Sources)
}
