package cli

// SPEC-REVIEWREUSE-001 S3 and S4 (REQ-RR-JDG-03, REQ-RR-JDG-04): an item is
// contested when some usable reviews hold it and others do not, compared item
// by item (the finding's group key, the checklist id of a FAIL, the REJECT
// vote), never by whether each reviewer holds some item.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

func judgeItemReview(verdict spec.ReviewVerdict, findings []spec.ReviewFinding, failedChecklist ...string) spec.ReviewResult {
	review := spec.ReviewResult{Verdict: verdict, Findings: findings}
	for _, id := range failedChecklist {
		review.ChecklistOutcomes = append(review.ChecklistOutcomes, spec.ChecklistOutcome{ID: id, Status: spec.ChecklistStatusFail, Reason: "gap"})
	}
	return review
}

func judgeItemFinding(severity string, category spec.FindingCategory, scope, description string) spec.ReviewFinding {
	return spec.ReviewFinding{Severity: severity, Category: category, ScopeRef: scope, Description: description, Status: spec.FindingStatusOpen}
}

func TestDecideSpecReviewJudge_ContestedIsDecidedPerItem(t *testing.T) {
	t.Parallel()

	majorX := judgeItemFinding("major", spec.FindingCategoryCompleteness, "REQ-001", "X")
	majorY := judgeItemFinding("major", spec.FindingCategoryCorrectness, "REQ-002", "Y")
	criticalX := judgeItemFinding("critical", spec.FindingCategoryCorrectness, "REQ-001", "X")
	criticalY := judgeItemFinding("critical", spec.FindingCategoryCorrectness, "REQ-002", "Y")
	dataLossZ := judgeItemFinding("minor", spec.FindingCategoryDataLoss, "REQ-003", "Z")
	majorCorrectnessX := judgeItemFinding("major", spec.FindingCategoryCorrectness, "REQ-001", "X")
	cases := []struct {
		name          string
		mode          string
		claude, codex spec.ReviewResult
		run           bool
		reason        string
	}{
		{"different escalations are both contested", config.ReviewJudgeModeOnEscalation,
			judgeItemReview(spec.VerdictReject, nil), judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{dataLossZ}), true, ""},
		{"different critical claims", config.ReviewJudgeModeOnEscalation,
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{criticalX}), judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{criticalY}), true, ""},
		{"the same REJECT vote", config.ReviewJudgeModeOnEscalation,
			judgeItemReview(spec.VerdictReject, nil), judgeItemReview(spec.VerdictReject, nil), false, specReviewJudgeSkipUnanimousEscalation},
		{"the same critical claim", config.ReviewJudgeModeOnEscalation,
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{criticalX}), judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{criticalX}), false, specReviewJudgeSkipUnanimousEscalation},
		{"no escalation", config.ReviewJudgeModeOnEscalation,
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{majorX}), judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{majorY}), false, specReviewJudgeSkipNoEscalation},
		{"different blocking findings", config.ReviewJudgeModeOnDisagreement,
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{majorX}), judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{majorY}), true, ""},
		{"different checklist failures", config.ReviewJudgeModeOnDisagreement,
			judgeItemReview(spec.VerdictRevise, nil, "Q-CORR-01"), judgeItemReview(spec.VerdictRevise, nil, "Q-SEC-01"), true, ""},
		{"the same checklist failure", config.ReviewJudgeModeOnDisagreement,
			judgeItemReview(spec.VerdictRevise, nil, "Q-CORR-01"), judgeItemReview(spec.VerdictRevise, nil, "Q-CORR-01"), false, specReviewJudgeSkipConsensusBlock},
		{"the same finding", config.ReviewJudgeModeOnDisagreement,
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{majorX}), judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{majorX}), false, specReviewJudgeSkipConsensusBlock},
		{"one claim, two severities", config.ReviewJudgeModeOnDisagreement,
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{majorCorrectnessX}), judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{criticalX}), true, ""},
	}
	for _, tc := range cases {
		run, reason := decideSpecReviewJudge(tc.mode, []spec.ReviewResult{tc.claude, tc.codex})
		assert.Equal(t, tc.run, run, tc.name)
		assert.Equal(t, tc.reason, reason, tc.name)
	}
}
