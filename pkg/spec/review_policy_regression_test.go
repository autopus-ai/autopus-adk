package spec_test

// SPEC-REVIEWPOLICY-001 regressions reproduced on origin/main 109c7ea4. Each
// test names the defect it pins; all of them failed before the policy landed.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// S2 (REQ-RP-POL-02, REQ-RP-POL-08): a lean declaration the profile rules
// refuse used to be validated against the lean floor only, so a high-risk
// SPEC skipped the full section checks it must face.
func TestValidateSpecSet_IneligibleLeanFacesFullSectionChecks(t *testing.T) {
	t.Parallel()

	cases := map[string]map[string]string{
		"high-risk class":        leanSpecFiles("feature"),
		"security surface":       leanSpecFilesWithSurface("test_only", "internal/auth/session_test.go, internal/auth/session.go"),
		"missing surface":        leanSpecFilesWithSurface("docs_only", ""),
		"missing change class":   leanSpecFilesWithSurface("", "docs/guide/timing.md"),
		"unknown change class":   leanSpecFiles("tiny"),
		"surface across domains": leanSpecFilesWithSurface("bugfix_existing_contract", "internal/cli/a.go, pkg/spec/b.go"),
	}
	for name, files := range cases {
		files := files
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			specDir, doc := writeAuthoringPreflightSpec(t, files)
			require.False(t, doc.LeanEligible())

			errs := spec.ValidateSpecSet(specDir, doc)
			assertValidationError(t, errs, "spec.md", "lean profile")
			assertValidationError(t, errs, "research.md", "Self-Verify Summary")
			assertValidationError(t, errs, "plan.md", "Risk-First Integration Probe")
			assertValidationError(t, errs, "research.md", "Semantic Invariant Inventory")
		})
	}
}

// The eligible lean floor keeps passing: the fix must not drag full checks
// into a SPEC the profile rules accept.
func TestValidateSpecSet_EligibleLeanStillPassesWithItsFloor(t *testing.T) {
	t.Parallel()

	specDir, doc := writeAuthoringPreflightSpec(t, leanSpecFiles("bugfix_existing_contract"))
	require.True(t, doc.LeanEligible())
	for _, e := range spec.ValidateSpecSet(specDir, doc) {
		assert.NotEqual(t, "error", e.Level, "unexpected error: %s", e.Message)
	}
}

// S3 (REQ-RP-POL-03): the default reviewer prompt told reviewers that two open
// major findings were a PASS while the runtime blocks every open major.
func TestBuildReviewPrompt_DefaultRulesNeverPassAnOpenMajor(t *testing.T) {
	t.Parallel()

	for _, mode := range []spec.ReviewMode{spec.ReviewModeDiscover, spec.ReviewModeVerify} {
		opts := spec.ReviewPromptOptions{Mode: mode}
		if mode == spec.ReviewModeVerify {
			opts.PriorFindings = []spec.ReviewFinding{{ID: "F-001", Severity: "major", Description: "gap"}}
		}
		prompt := spec.BuildReviewPrompt(&spec.SpecDocument{ID: "SPEC-RULES-001", Title: "Rules"}, "", opts)
		assert.NotContains(t, prompt, "major <= 2", "mode %s", mode)
		assert.NotContains(t, prompt, "REJECT: critical > 0 OR security > 0", "mode %s", mode)
	}
}

// S3 (REQ-RP-POL-05): the structured parser collapsed contract and data_loss
// into completeness, so a suggestion-severity data-loss finding from a JSON
// reviewer stopped blocking while the text parser kept blocking it.
func TestParseVerdict_StructuredImpactCategoriesStayBlocking(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		category string
		want     spec.FindingCategory
		policy   string
	}{
		"data_loss":    {"data_loss", "data_loss", spec.BlockingPolicyOpenCorrectness},
		"data-loss":    {"data-loss", "data_loss", spec.BlockingPolicyOpenCorrectness},
		"data loss":    {"Data Loss", "data_loss", spec.BlockingPolicyOpenCorrectness},
		"contract":     {"contract", "contract", spec.BlockingPolicyOpenCorrectness},
		"completeness": {"completeness", spec.FindingCategoryCompleteness, ""},
	}
	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			output := `{"verdict":"REVISE","summary":"s","findings":[{"severity":"suggestion","category":"` + tc.category +
				`","scope_ref":"spec.md","location":"spec.md:1","description":"rows can be dropped on retry","suggestion":"keep them"}]}`
			result := spec.ParseVerdict("SPEC-CAT-001", output, "codex", 0, nil)
			require.Len(t, result.Findings, 1)
			assert.Equal(t, tc.want, result.Findings[0].Category)
			policy, blocks := spec.FindingBlockingPolicy(result.Findings[0])
			assert.Equal(t, tc.policy, policy)
			assert.Equal(t, tc.policy != "", blocks)
		})
	}
}

// The same normalization applies to judge-accepted findings.
func TestJudgedFindingsToReview_KeepsImpactCategories(t *testing.T) {
	t.Parallel()

	out := spec.JudgedFindingsToReview("SPEC-CAT-002", 0, judgeOutputWithCategory("data-loss"), map[string]string{"Reviewer A": "codex"}, nil)
	require.Len(t, out, 1)
	assert.Equal(t, spec.FindingCategory("data_loss"), out[0].Category)
}

func judgeOutputWithCategory(category string) *orchestra.ReviewJudgeOutput {
	return &orchestra.ReviewJudgeOutput{Verdict: "REVISE", Rationale: "r", Findings: []orchestra.JudgedFinding{{
		Severity: "suggestion", Category: category, ScopeRef: "spec.md", Location: "spec.md:1",
		Description: "rows can be dropped on retry", Decision: "accept", Sources: []string{"Reviewer A"},
	}}}
}
