package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

func TestRunSpecReviewLoop_RecordsRoundTimingPerProvider(t *testing.T) {
	specDir := scaffoldReviewSpec(t, t.TempDir(), "SPEC-REVIEW-TIMING-001")
	doc, err := spec.Load(specDir)
	require.NoError(t, err)

	origRunner := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{
			{Provider: "claude", Output: `{"verdict":"PASS","summary":"ok","findings":[]}`, Duration: 90 * time.Second},
			{Provider: "codex", Output: `{"verdict":"PASS","summary":"ok","findings":[]}`, Duration: 4 * time.Minute, TimedOut: true},
		}}, nil
	}
	defer func() { specReviewRunOrchestra = origRunner }()

	evidence := &specReviewRuntimeEvidence{}
	params := reviewLoopParams("SPEC-REVIEW-TIMING-001", specDir)
	params.maxRevisions = 0
	params.runtimeEvidence = evidence
	_, err = runSpecReviewLoop(params, doc, nil)
	require.NoError(t, err)

	require.Len(t, evidence.Rounds, 1)
	round := evidence.Rounds[0]
	assert.Equal(t, 0, round.Revision)
	assert.NotEmpty(t, round.StartedAt)
	assert.Equal(t, []specReviewProviderTiming{
		{Provider: "claude", DurationMS: 90_000},
		{Provider: "codex", DurationMS: 240_000, TimedOut: true},
	}, round.Providers)
}

func TestPrintSpecReviewRoundTiming_NamesJudgeAndTimeouts(t *testing.T) {
	var out bytes.Buffer
	printSpecReviewRoundTiming(&out, specReviewRoundTiming{
		Revision:   1,
		DurationMS: 372_400,
		Providers: []specReviewProviderTiming{
			{Provider: "codex", DurationMS: 340_000, TimedOut: true},
			{Provider: "claude", Role: "judge", DurationMS: 31_600},
		},
	})

	assert.Equal(t, "SPEC 리뷰 라운드 1 소요: 6m12s (codex 5m40s timeout, judge:claude 32s)\n", out.String())
}

func TestSyncReviewedSpecStatusWithReceipt_CarriesTimingAndProfile(t *testing.T) {
	specDir := scaffoldReviewSpec(t, t.TempDir(), "SPEC-REVIEW-TIMING-002")
	started := time.Date(2026, time.October, 31, 1, 0, 0, 0, time.UTC)
	finished := started.Add(7*time.Minute + 30*time.Second)
	rounds := []specReviewRoundTiming{{Revision: 0, StartedAt: started.Format(time.RFC3339Nano), DurationMS: 420_000}}

	receipt, err := syncReviewedSpecStatusWithReceipt(specDir, newSyncPassResult(nil), false, specReviewRuntimeEvidence{
		RunID: "run", FinishedAt: finished, StartedAt: started, Rounds: rounds, Profile: spec.ProfileLean,
	})

	require.NoError(t, err)
	assert.Equal(t, started.Format(time.RFC3339Nano), receipt.StartedAt)
	assert.Equal(t, int64(450_000), receipt.DurationMS)
	assert.Equal(t, rounds, receipt.Rounds)
	assert.Equal(t, "lean", receipt.ReviewProfile)
}

func TestApplyLeanReviewProfile(t *testing.T) {
	full := config.ReviewGateConf{Providers: []string{"claude", "codex", "gemini"}, Judge: "codex", MinProviders: 2}
	tests := []struct {
		name          string
		doc           *spec.SpecDocument
		flagProviders []string
		wantProviders []string
		wantJudge     string
		wantMin       int
	}{
		{"full SPEC keeps the panel", &spec.SpecDocument{}, nil, nil, "codex", 2},
		{"lean SPEC uses the judge as its single reviewer", &spec.SpecDocument{Profile: "lean", ChangeClass: "test_only"}, nil, []string{"codex"}, "", 1},
		{"explicit providers win", &spec.SpecDocument{Profile: "lean", ChangeClass: "docs_only"}, []string{"gemini", "claude"}, []string{"gemini", "claude"}, "codex", 2},
		{"mislabelled high-risk lean keeps the panel", &spec.SpecDocument{Profile: "lean", ChangeClass: "feature"}, nil, nil, "codex", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			gate, opts := applyLeanReviewProfile(tt.doc, full, specReviewOptions{providers: tt.flagProviders}, &out)
			assert.Equal(t, tt.wantProviders, opts.providers)
			assert.Equal(t, tt.wantJudge, gate.Judge)
			assert.Equal(t, tt.wantMin, gate.MinProviders)
		})
	}
}

func TestLeanSpecReviewer_FallsBackToFirstProvider(t *testing.T) {
	assert.Equal(t, "claude", leanSpecReviewer(config.ReviewGateConf{Providers: []string{"claude", "codex"}, Judge: "opencode"}))
	assert.Equal(t, "gemini", leanSpecReviewer(config.ReviewGateConf{Judge: "gemini"}))
	assert.Equal(t, "", leanSpecReviewer(config.ReviewGateConf{}))
}

func TestPrintSpecTiming_ReadsReceiptEvidence(t *testing.T) {
	specDir := t.TempDir()
	_, err := persistSpecReviewPromotionReceipt(specDir, specReviewPromotionReceipt{
		SpecID: "SPEC-T-001", AnalysisVerdict: "PASS", ReviewProfile: "lean",
		StartedAt: "2026-10-31T01:00:00Z", DurationMS: 450_000,
		Rounds: []specReviewRoundTiming{{Revision: 0, DurationMS: 420_000, Providers: []specReviewProviderTiming{{Provider: "codex", DurationMS: 410_000}}}},
	})
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, printSpecTiming(&out, specDir))
	assert.Equal(t, "SPEC-T-001 (profile lean, verdict PASS)\n"+
		"리뷰 전체: 7m30s, 시작 2026-10-31T01:00:00Z\n"+
		"SPEC 리뷰 라운드 0 소요: 7m0s (codex 6m50s)\n", out.String())
}

func TestPrintSpecTiming_OldReceiptSaysSo(t *testing.T) {
	specDir := t.TempDir()
	_, err := persistSpecReviewPromotionReceipt(specDir, specReviewPromotionReceipt{SpecID: "SPEC-T-002"})
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, printSpecTiming(&out, specDir))
	assert.Contains(t, out.String(), "소요 시간 기록이 없습니다")
}
