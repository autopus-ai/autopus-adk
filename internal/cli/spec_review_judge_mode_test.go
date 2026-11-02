package cli

// SPEC-REVIEWREUSE-001 S1 to S4 and S6 (REQ-RR-JDG-01 to REQ-RR-JDG-04,
// REQ-RR-JDG-06): the judge mode is validated and printed, `always` keeps the
// one-call round, a conditional mode calls the judge only for a disagreement
// or a contested escalation, and a judge that cannot rule never clears a
// safety claim.

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

const (
	passNoFinding     = `{"verdict":"PASS","summary":"ok","findings":[]}`
	rejectNoFinding   = `{"verdict":"REJECT","summary":"redesign","findings":[]}`
	judgeRejectsFirst = `{"verdict":"PASS","findings":[{"severity":"major","category":"completeness","scope_ref":"REQ-001","location":"spec.md","description":"X","suggestion":"none","decision":"reject","reason":"not supported","sources":["Reviewer A"]}],"rationale":"ok"}`
)

func reviewerWith(verdict, severity, category, scope, description string) string {
	return `{"verdict":"` + verdict + `","summary":"s","findings":[{"severity":"` + severity + `","category":"` + category +
		`","scope_ref":"` + scope + `","location":"spec.md:1","description":"` + description + `","suggestion":"fix"}]}`
}

func judgeModeFixture(t *testing.T, mode string, budget config.ReviewBudgetConf) (*readOnlyReviewFixture, *scriptedBudgetBackend) {
	t.Helper()
	return budgetFixture(t, budget, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Judge = "claude"
		cfg.Spec.ReviewGate.JudgeMode = mode
	})
}

func (b *scriptedBudgetBackend) countRole(role string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	count := 0
	for _, request := range b.requests {
		if request.Role == role {
			count++
		}
	}
	return count
}

func TestSpecReviewJudgeMode_FlagIsValidatedAndPrinted(t *testing.T) {
	fixture, backend := judgeModeFixture(t, "", config.ReviewBudgetConf{})

	err := runSpecReviewCommand(t, fixture, "--judge-mode", "maybe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `--judge-mode "maybe": expected one of always, on_disagreement, on_escalation`)
	assert.Zero(t, backend.count())

	stderr := captureSpecReviewStderr(t, func() {
		_ = captureStdout(t, func() {
			require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0,
				specReviewOptions{judgeMode: "on_disagreement", judgeModeSet: true}))
		})
	})
	assert.Contains(t, stderr, "  judge: claude (on_disagreement: 판정이 갈리거나 차단·escalation 항목이 일부 리뷰어에만 있을 때) — spec.review_gate.judge\n")
	assert.Contains(t, stderr, "  호출: 라운드당 예상 2 + judge 0-1, 상한 5 (JSON 재요청 포함) · 이번 실행 상한 5 (fallback 0개 포함)\n")
}

// S2: the default keeps the judge call of every round.
func TestSpecReviewJudgeMode_AlwaysKeepsTheJudgeCall(t *testing.T) {
	fixture, backend := judgeModeFixture(t, "", config.ReviewBudgetConf{})

	stderr := captureSpecReviewStderr(t, func() {
		_ = captureStdout(t, func() {
			require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))
		})
	})

	assert.Equal(t, 1, backend.countRole(specReviewRoleJudge))
	assert.Equal(t, 3, backend.count())
	assert.Contains(t, stderr, "  judge: claude (always) — spec.review_gate.judge\n")
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	assert.Equal(t, "PASS", receipt.Verdict)
	require.NotNil(t, receipt.Judge)
	assert.Equal(t, "always", receipt.Judge.Mode)
	assert.Equal(t, "ok", receipt.Judge.Status)
}

type judgeModeRow struct {
	name          string
	claude, codex scriptedReply
	judgeCalls    int
	status        string
	reason        string
	verdict       string
}

func runJudgeModeTable(t *testing.T, mode string, rows []judgeModeRow) {
	t.Helper()
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			fixture, backend := judgeModeFixture(t, mode, config.ReviewBudgetConf{})
			backend.replies["claude/reviewer"] = []scriptedReply{row.claude}
			backend.replies["codex/reviewer"] = []scriptedReply{row.codex}
			backend.replies["claude/judge"] = []scriptedReply{{output: judgeRejectsFirst}}

			stderr := captureSpecReviewStderr(t, func() {
				_ = captureStdout(t, func() {
					require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))
				})
			})

			assert.Equal(t, row.judgeCalls, backend.countRole(specReviewRoleJudge))
			receipt := readSpecReviewReceipt(t, fixture.specDir)
			assert.Equal(t, row.verdict, receipt.Verdict)
			require.NotNil(t, receipt.Judge)
			assert.Equal(t, mode, receipt.Judge.Mode)
			assert.Equal(t, row.status, receipt.Judge.Status)
			if row.reason != "" {
				assert.Equal(t, row.reason, receipt.Judge.Reason)
				assert.Contains(t, stderr, "SPEC 리뷰 judge 생략: "+row.reason+" ("+mode+")")
			}
		})
	}
}

// S3: on_disagreement.
func TestSpecReviewJudgeMode_OnDisagreementCallsTheJudgeOnlyForADisagreement(t *testing.T) {
	majorX := reviewerWith("REVISE", "major", "completeness", "REQ-001", "X")
	runJudgeModeTable(t, "on_disagreement", []judgeModeRow{
		{"consensus pass", scriptedReply{output: passNoFinding}, scriptedReply{output: passNoFinding}, 0, "skipped", "consensus_pass", "PASS"},
		{"consensus block", scriptedReply{output: majorX}, scriptedReply{output: majorX}, 0, "skipped", "consensus_block", "REVISE"},
		{"verdicts differ", scriptedReply{output: passNoFinding}, scriptedReply{output: majorX}, 1, "ok", "", "PASS"},
		{"different blocking findings", scriptedReply{output: majorX},
			scriptedReply{output: reviewerWith("REVISE", "major", "correctness", "REQ-002", "Y")}, 1, "ok", "", "PASS"},
		{"contested escalation", scriptedReply{output: passNoFinding},
			scriptedReply{output: reviewerWith("PASS", "critical", "security", "REQ-002", "Y")}, 1, "ok", "", "PASS"},
		{"single reviewer", scriptedReply{output: reviewerWith("REVISE", "major", "correctness", "REQ-001", "X")},
			scriptedReply{err: errors.New("exit status 1")}, 0, "skipped", "single_reviewer", "REVISE"},
	})
}

// S4: on_escalation.
func TestSpecReviewJudgeMode_OnEscalationCallsTheJudgeOnlyForAContestedEscalation(t *testing.T) {
	runJudgeModeTable(t, "on_escalation", []judgeModeRow{
		{"consensus pass", scriptedReply{output: passNoFinding}, scriptedReply{output: passNoFinding}, 0, "skipped", "no_escalation", "PASS"},
		{"ordinary disagreement", scriptedReply{output: passNoFinding},
			scriptedReply{output: reviewerWith("REVISE", "major", "completeness", "REQ-001", "X")}, 0, "skipped", "no_escalation", "REVISE"},
		{"contested critical", scriptedReply{output: passNoFinding},
			scriptedReply{output: reviewerWith("REVISE", "critical", "correctness", "REQ-001", "X")}, 1, "ok", "", "PASS"},
		{"different escalations", scriptedReply{output: rejectNoFinding},
			scriptedReply{output: reviewerWith("REVISE", "minor", "data_loss", "REQ-003", "Z")}, 1, "ok", "", "REJECT"},
		{"unanimous escalation", scriptedReply{output: rejectNoFinding}, scriptedReply{output: rejectNoFinding},
			0, "skipped", "unanimous_escalation", "REJECT"},
	})
}

// S6: a judge that fails, times out, or is refused by the budget leaves the
// contested security claim blocking.
func TestSpecReviewJudgeMode_AJudgeThatCannotRuleNeverClearsASafetyClaim(t *testing.T) {
	security := reviewerWith("PASS", "critical", "security", "REQ-002", "Token logged")
	cases := map[string]struct {
		judge  scriptedReply
		budget config.ReviewBudgetConf
		status string
	}{
		"judge exits 1":   {scriptedReply{err: errors.New("exit status 1")}, config.ReviewBudgetConf{}, "failed"},
		"judge times out": {scriptedReply{err: context.DeadlineExceeded}, config.ReviewBudgetConf{}, "failed"},
		"budget refuses":  {scriptedReply{output: judgeRejectsFirst}, config.ReviewBudgetConf{MaxCalls: 2}, "failed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture, backend := judgeModeFixture(t, "on_disagreement", tc.budget)
			backend.replies["claude/reviewer"] = []scriptedReply{{output: passNoFinding}}
			backend.replies["codex/reviewer"] = []scriptedReply{{output: security}}
			backend.replies["claude/judge"] = []scriptedReply{tc.judge}

			runBudgetReview(t, fixture, specReviewOptions{})

			receipt := readSpecReviewReceipt(t, fixture.specDir)
			assert.Equal(t, "REVISE", receipt.Verdict)
			assert.Contains(t, blockingPolicies(receipt.BlockingReasons), spec.BlockingPolicyOpenSecurity)
			require.NotNil(t, receipt.Judge)
			assert.Equal(t, tc.status, receipt.Judge.Status)
			if tc.budget.MaxCalls > 0 {
				assert.Equal(t, spec.LoopStatusBudgetExhausted, receipt.LoopStatus)
				assert.Zero(t, backend.countRole(specReviewRoleJudge), "the refused judge never reaches the backend")
			}
		})
	}
}
