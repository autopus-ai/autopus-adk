package cli

// SPEC-REVIEWPOLICY-001 review fixes F-001 and F-005: a judge PASS clears
// only what it explicitly addresses, and a free-text finding whose subject is
// a section an eligible lean SPEC may omit does not block it.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

const (
	checklistFailReviewer = `{"verdict":"REVISE","summary":"probe missing","findings":[],"checklist":[{"id":"Q-COMP-08","status":"FAIL","reason":"no probe table"}]}`
	rejectReviewer        = `{"verdict":"REJECT","summary":"redesign the store","findings":[]}`
	emptyJudgePass        = `{"verdict":"PASS","findings":[],"rationale":"nothing blocks"}`
)

func judgeDismissing(scopeRef string) string {
	return `{"verdict":"PASS","findings":[{"severity":"minor","category":"completeness","scope_ref":"` + scopeRef +
		`","location":"review","description":"dismissed","suggestion":"none","decision":"reject","reason":"not supported by the SPEC text","sources":["Reviewer A"]}],"rationale":"dismissed"}`
}

// S5 oracle (F-001): a full SPEC whose only blocker is a reviewer's checklist
// FAIL, or a reviewer REJECT, stays blocked behind a judge PASS that never
// addressed it, and clears only when the judge rejects that exact item.
func TestRunSpecReviewLoop_JudgePassClearsOnlyTheVerdictBlockersItAddresses(t *testing.T) {
	cases := map[string]struct {
		reviewer, judge string
		verdict         spec.ReviewVerdict
		policy          string
	}{
		"checklist FAIL not addressed":  {checklistFailReviewer, emptyJudgePass, spec.VerdictRevise, spec.BlockingPolicyReviewerChecklist},
		"reviewer REJECT not addressed": {rejectReviewer, emptyJudgePass, spec.VerdictReject, spec.BlockingPolicyProviderReject},
		"checklist FAIL dismissed":      {checklistFailReviewer, judgeDismissing("Q-COMP-08"), spec.VerdictPass, ""},
		"reviewer REJECT dismissed":     {rejectReviewer, judgeDismissing("verdict"), spec.VerdictPass, ""},
		"other item dismissed":          {checklistFailReviewer, judgeDismissing("Q-COMP-05"), spec.VerdictRevise, spec.BlockingPolicyReviewerChecklist},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result, _ := runJudgeMergeLoopWithResponses(t, tc.judge, nil, "claude",
				[]orchestra.ProviderResponse{{Provider: "claude", Output: tc.reviewer}})

			assert.Equal(t, tc.verdict, result.Verdict)
			if tc.policy == "" {
				assert.Empty(t, result.BlockingReasons)
				return
			}
			require.NotEmpty(t, result.BlockingReasons)
			assert.Equal(t, tc.policy, result.BlockingReasons[0].Policy)
		})
	}
}

// Without a judge the same rule holds: a checklist FAIL blocks whatever the
// reviewer voted.
func TestRunSpecReviewLoop_ChecklistFailBlocksAPassVote(t *testing.T) {
	const specID = "SPEC-CHECKLIST-VOTE-001"
	specDir := scaffoldReviewSpec(t, t.TempDir(), specID)
	output := `{"verdict":"PASS","summary":"fine","findings":[],"checklist":[{"id":"Q-CORR-01","status":"FAIL","reason":"wrong path"}]}`

	result := runSingleReviewerLoop(t, specID, specDir, output)

	assert.Equal(t, spec.VerdictRevise, result.Verdict)
	require.Len(t, result.BlockingReasons, 1)
	assert.Equal(t, spec.BlockingPolicyReviewerChecklist, result.BlockingReasons[0].Policy)
}

// F-005: on an eligible lean SPEC a finding whose subject is an allowed
// omission is advisory; the same finding on any other SPEC, or one that also
// cites a real requirement or acceptance gap, still blocks.
func TestRunSpecReviewLoop_LeanOmissionFindingDoesNotBlock(t *testing.T) {
	const omission = "Risk-First Integration Probe missing from plan.md"
	cases := map[string]struct {
		lean, withProbe bool
		severity        string
		category        string
		description     string
		verdict         spec.ReviewVerdict
	}{
		"lean omission":                 {true, false, "major", "completeness", omission, spec.VerdictPass},
		"full SPEC":                     {false, false, "major", "completeness", omission, spec.VerdictRevise},
		"lean with the section":         {true, true, "major", "completeness", omission, spec.VerdictRevise},
		"lean security category":        {true, false, "major", "security", "Self-Verify Summary missing", spec.VerdictRevise},
		"lean omission plus a real gap": {true, false, "major", "completeness", omission + "; REQ-001 has no acceptance scenario", spec.VerdictRevise},
		"lean acceptance gap":           {true, false, "major", "completeness", "acceptance.md S1 has no expected output", spec.VerdictRevise},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			const specID = "SPEC-LEANOMIT-001"
			dir := t.TempDir()
			specDir := scaffoldReviewSpec(t, dir, specID)
			if tc.lean {
				extra := ""
				if tc.withProbe {
					extra = "\n## Risk-First Integration Probe\n\n| assumption_id | status |\n|---|---|\n| A1 | not-run |\n"
				}
				specDir = writeLeanReviewSpec(t, t.TempDir(), specID, "bugfix_existing_contract", "internal/cli/spec_timing.go", extra)
			}

			result := runSingleReviewerLoop(t, specID, specDir, singleFindingReviewOutput(tc.severity, tc.category, tc.description))

			assert.Equal(t, tc.verdict, result.Verdict)
			require.Len(t, result.Findings, 1)
			if tc.verdict == spec.VerdictPass {
				assert.Equal(t, spec.FindingStatusDeferred, result.Findings[0].Status)
				assert.Empty(t, result.BlockingReasons)
			} else {
				assert.Equal(t, spec.FindingStatusOpen, result.Findings[0].Status)
			}
		})
	}
}
