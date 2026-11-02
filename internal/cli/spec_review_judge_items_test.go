package cli

// SPEC-REVIEWREUSE-001 S3 and S4 (REQ-RR-JDG-03, REQ-RR-JDG-04): an item is
// contested when some usable reviews hold it and others do not, compared item
// by item (the finding's group key, the checklist id of a FAIL, the REJECT
// vote), never by whether each reviewer holds some item. on_disagreement
// compares blocking and escalation items; on_escalation only escalation
// items.

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

type judgeItemRow struct {
	name          string
	claude, codex spec.ReviewResult
	run           bool
	reason        string
}

func assertJudgeDecisions(t *testing.T, mode string, rows []judgeItemRow) {
	t.Helper()
	for _, row := range rows {
		run, reason := decideSpecReviewJudge(mode, []spec.ReviewResult{row.claude, row.codex})
		assert.Equal(t, row.run, run, row.name)
		assert.Equal(t, row.reason, reason, row.name)
	}
}

var (
	itemMajorX            = judgeItemFinding("major", spec.FindingCategoryCompleteness, "REQ-001", "X")
	itemMajorY            = judgeItemFinding("major", spec.FindingCategoryCorrectness, "REQ-002", "Y")
	itemMajorCorrectnessX = judgeItemFinding("major", spec.FindingCategoryCorrectness, "REQ-001", "X")
	itemCriticalX         = judgeItemFinding("critical", spec.FindingCategoryCorrectness, "REQ-001", "X")
	itemCriticalY         = judgeItemFinding("critical", spec.FindingCategoryCorrectness, "REQ-002", "Y")
	itemDataLossZ         = judgeItemFinding("minor", spec.FindingCategoryDataLoss, "REQ-003", "Z")
)

// S3: on_disagreement compares blocking items, ordinary ones included.
func TestDecideSpecReviewJudge_OnDisagreementComparesBlockingItems(t *testing.T) {
	t.Parallel()

	assertJudgeDecisions(t, config.ReviewJudgeModeOnDisagreement, []judgeItemRow{
		{"different blocking findings", judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemMajorX}),
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemMajorY}), true, ""},
		{"different checklist failures", judgeItemReview(spec.VerdictRevise, nil, "Q-CORR-01"),
			judgeItemReview(spec.VerdictRevise, nil, "Q-SEC-01"), true, ""},
		{"the same checklist failure", judgeItemReview(spec.VerdictRevise, nil, "Q-CORR-01"),
			judgeItemReview(spec.VerdictRevise, nil, "Q-CORR-01"), false, specReviewJudgeSkipConsensusBlock},
		{"the same finding", judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemMajorX}),
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemMajorX}), false, specReviewJudgeSkipConsensusBlock},
	})
}

// S4: on_escalation compares escalation items only; an ordinary blocking
// finding or a checklist FAIL is never one, and consensus_block is not a
// reason it records.
func TestDecideSpecReviewJudge_OnEscalationComparesOnlyEscalationItems(t *testing.T) {
	t.Parallel()

	assertJudgeDecisions(t, config.ReviewJudgeModeOnEscalation, []judgeItemRow{
		{"different blocking findings", judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemMajorX}),
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemMajorY}), false, specReviewJudgeSkipNoEscalation},
		{"different checklist failures", judgeItemReview(spec.VerdictRevise, nil, "Q-CORR-01"),
			judgeItemReview(spec.VerdictRevise, nil, "Q-SEC-01"), false, specReviewJudgeSkipNoEscalation},
		{"the same critical claim", judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemCriticalX}),
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemCriticalX}), false, specReviewJudgeSkipUnanimousEscalation},
		{"the same REJECT vote", judgeItemReview(spec.VerdictReject, nil),
			judgeItemReview(spec.VerdictReject, nil), false, specReviewJudgeSkipUnanimousEscalation},
		{"a REJECT vote against a data-loss finding", judgeItemReview(spec.VerdictReject, nil),
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemDataLossZ}), true, ""},
		{"different critical claims", judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemCriticalX}),
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemCriticalY}), true, ""},
		{"one claim raised major and critical", judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemMajorCorrectnessX}),
			judgeItemReview(spec.VerdictRevise, []spec.ReviewFinding{itemCriticalX}), true, ""},
	})
}
