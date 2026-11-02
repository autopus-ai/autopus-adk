package cli

// SPEC-JUDGEMODEL-001 S11 (REQ-JM-RUN-04, RUN-05) with SPEC-REVIEWREUSE-001:
// the judge model reaches the assembled judge the reuse fingerprint reads, so
// a changed judge model reviews again while an unchanged one is reused, and
// a judge a conditional judge mode skips names no judge model in the receipt.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// fingerprintIdentity is the receipt fingerprint's identity of name in role.
func fingerprintIdentity(t *testing.T, fixture *readOnlyReviewFixture, role, name string) specReviewProviderIdentity {
	t.Helper()
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	require.NotNil(t, receipt.Fingerprint)
	for _, identity := range receipt.Fingerprint.Providers {
		if identity.Role == role && identity.Name == name {
			return identity
		}
	}
	t.Fatalf("no %s identity for %s in %+v", role, name, receipt.Fingerprint.Providers)
	return specReviewProviderIdentity{}
}

func TestRunSpecReview_JudgeModelChangeInvalidatesReuse(t *testing.T) {
	fixture, backend := reuseFixture(t, config.ReviewBudgetConf{}, func(cfg *config.HarnessConfig) {
		snapshotModels(cfg)
		withJudgeModel("claude", "claude-opus-5-5", "high")(cfg)
		// The judge models are digit aliases; declaring them lets a PASS be
		// reused, so only the judge model change decides the second review.
		cfg.Spec.ReviewGate.PinnedModels = []string{"claude-opus-5-5", "claude-sonnet-5-5"}
	})
	runReviewOutput(t, fixture, specReviewOptions{})
	require.Equal(t, 3, backend.count(), "two reviewers and the judge")
	assert.Equal(t, "claude-opus-5-5", fingerprintIdentity(t, fixture, specReviewRoleJudge, "claude").Model)
	reviewer := fingerprintIdentity(t, fixture, specReviewRoleReviewer, "claude")
	assert.Equal(t, "claude-fable-5-1-20261015", reviewer.Model)

	stdout, _ := runReviewOutput(t, fixture, specReviewOptions{})
	assert.Equal(t, 3, backend.count(), "an unchanged judge model is reused")
	assert.Contains(t, stdout, "SPEC 리뷰 재사용: "+fixture.specID)

	editFixtureConfig(t, fixture.root, func(cfg *config.HarnessConfig) { cfg.Spec.ReviewGate.JudgeModel = "claude-sonnet-5-5" })
	_, stderr := runReviewOutput(t, fixture, specReviewOptions{})

	assert.Equal(t, 6, backend.count(), "a changed judge model is reviewed again")
	assert.Contains(t, stderr, "리뷰 재사용 안 함:")
	assert.Contains(t, stderr, "providers: claude")
	assert.Equal(t, "claude-sonnet-5-5", fingerprintIdentity(t, fixture, specReviewRoleJudge, "claude").Model)
	assert.Equal(t, reviewer, fingerprintIdentity(t, fixture, specReviewRoleReviewer, "claude"), "the claude reviewer is the same")
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	require.NotNil(t, receipt.Judge)
	assert.Equal(t, [2]string{"claude-sonnet-5-5", "high"}, [2]string{receipt.Judge.Model, receipt.Judge.Effort})
}

// A conditional judge mode that skips the judge records no judge model; when
// the round calls the judge, the call carries the judge model and the receipt
// names it.
func TestSpecReviewJudgeModel_SkippedJudgeRecordsNoModel(t *testing.T) {
	majorX := reviewerWith("REVISE", "major", "completeness", "REQ-001", "X")
	for _, row := range []struct {
		name          string
		claude, codex string
		judgeCalls    int
		status        string
		model         [2]string
	}{
		{"consensus skips the judge", passNoFinding, passNoFinding, 0, "skipped", [2]string{}},
		{"disagreement calls the judge", passNoFinding, majorX, 1, "ok", [2]string{"claude-opus-5-5", "high"}},
	} {
		t.Run(row.name, func(t *testing.T) {
			fixture, backend := budgetFixture(t, config.ReviewBudgetConf{}, func(cfg *config.HarnessConfig) {
				withJudgeModel("claude", "claude-opus-5-5", "high")(cfg)
				cfg.Spec.ReviewGate.JudgeMode = config.ReviewJudgeModeOnDisagreement
			})
			backend.replies["claude/reviewer"] = []scriptedReply{{output: row.claude}}
			backend.replies["codex/reviewer"] = []scriptedReply{{output: row.codex}}
			backend.replies["claude/judge"] = []scriptedReply{{output: judgeRejectsFirst}}

			stderr := captureSpecReviewStderr(t, func() {
				_ = captureStdout(t, func() {
					require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))
				})
			})

			assert.Equal(t, row.judgeCalls, backend.countRole(specReviewRoleJudge))
			assert.Contains(t, stderr, "  judge model: claude-opus-5-5 · high — spec.review_gate.judge_model\n")
			receipt := readSpecReviewReceipt(t, fixture.specDir)
			require.NotNil(t, receipt.Judge)
			assert.Equal(t, row.status, receipt.Judge.Status)
			assert.Equal(t, row.model, [2]string{receipt.Judge.Model, receipt.Judge.Effort})
			for _, request := range backend.requests {
				if request.Role == specReviewRoleJudge {
					assert.True(t, reviewArgsHasRun(request.Config.Args, "--model", "claude-opus-5-5", "--effort", "high"), "%v", request.Config.Args)
				}
			}
		})
	}
}
