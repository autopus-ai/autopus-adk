package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSpecReviewLoop_JudgeOverridesSupermajorityAndRestoresSources(t *testing.T) {
	result, _ := runJudgeMergeLoop(t, validStructuredJudgeOutput("REVISE", "major"), nil)

	assert.Equal(t, spec.VerdictRevise, result.Verdict)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "Judge finding", result.Findings[0].Description)
	assert.Equal(t, "claude", result.Findings[0].Provider)
	assert.Equal(t, spec.FindingStatusOpen, result.Findings[0].Status)
	require.NotNil(t, result.Judge)
	assert.Equal(t, &spec.JudgeSummary{
		Provider: "claude", Family: "anthropic", Status: "ok", Verdict: "REVISE",
		Accepted: 1, Rejected: 0, Merged: 0,
		AcceptedIDs: []string{"F-001"}, Rationale: "reviewed",
		// SPEC-REVIEWREUSE-001 REQ-RR-JDG-07: the ruling links to its reviewer.
		Decisions: []spec.JudgeDecision{{Decision: "accept", Reviewers: []string{"claude"}, ScopeRef: "REQ-JUDGE-001"}},
	}, result.Judge)
}

func TestRunSpecReviewLoop_JudgePassWithAcceptedCriticalDowngradesToRevise(t *testing.T) {
	result, _ := runJudgeMergeLoop(t, validStructuredJudgeOutput("PASS", "critical"), nil)

	assert.Equal(t, spec.VerdictRevise, result.Verdict)
	require.NotNil(t, result.Judge)
	assert.Equal(t, "PASS", result.Judge.Verdict, "summary records the judge output before safety downgrade")
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "critical", result.Findings[0].Severity)
}

func TestRunSpecReviewLoop_ReviseWithoutAcceptedFindingsIsInvalidAndFallsBack(t *testing.T) {
	legacy, _ := runJudgeMergeLoopWithGate(t, "", nil, "")
	result, _ := runJudgeMergeLoop(
		t,
		`{"verdict":"REVISE","findings":[],"rationale":"unsupported revise"}`,
		nil,
	)

	assert.Equal(t, legacy.Verdict, result.Verdict)
	assert.Equal(t, findingKeys(legacy.Findings), findingKeys(result.Findings))
	require.NotNil(t, result.Judge)
	assert.Equal(t, "invalid", result.Judge.Status)
	assert.Contains(t, result.Judge.Reason, "REVISE without accepted findings")
}

func TestRunSpecReviewLoop_JudgeFailureFallsBackToSupermajority(t *testing.T) {
	tests := []struct {
		name       string
		failure    orchestra.FailedProvider
		wantStatus string
	}{
		{
			name: "timeout",
			failure: orchestra.FailedProvider{
				Name: "claude (judge)", Role: "judge", ModelFamily: "anthropic",
				FailureClass: "timeout", Error: "context deadline exceeded", TimedOut: true,
			},
			wantStatus: "failed",
		},
		{
			name: "invalid output",
			failure: orchestra.FailedProvider{
				Name: "claude (judge)", Role: "judge", ModelFamily: "anthropic",
				FailureClass: "invalid_output", Error: "invalid review judge JSON",
			},
			wantStatus: "invalid",
		},
	}
	// The fallback oracle is the legacy loop itself: the same reviewer
	// responses run with no judge configured.
	legacy, _ := runJudgeMergeLoopWithGate(t, "", nil, "")
	require.Nil(t, legacy.Judge)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, specDir := runJudgeMergeLoop(t, "", []orchestra.FailedProvider{tc.failure})

			assert.Equal(t, legacy.Verdict, result.Verdict)
			assert.Equal(t, findingKeys(legacy.Findings), findingKeys(result.Findings))
			require.NotNil(t, result.Judge)
			assert.Equal(t, tc.wantStatus, result.Judge.Status)
			assert.Equal(t, "claude", result.Judge.Provider)
			assert.Equal(t, "anthropic", result.Judge.Family)
			assert.Equal(t, tc.failure.Error, result.Judge.Reason)
			receipt, err := syncReviewedSpecStatusWithReceipt(specDir, result, false)
			require.NoError(t, err)
			require.NotNil(t, receipt.Judge)
			assert.Equal(t, tc.wantStatus, receipt.Judge.Status)
			assert.Equal(t, tc.failure.Error, receipt.Judge.Reason)
		})
	}
}

func findingKeys(findings []spec.ReviewFinding) []string {
	keys := make([]string, 0, len(findings))
	for _, finding := range findings {
		keys = append(keys, finding.Severity+"|"+finding.Description+"|"+string(finding.Status))
	}
	return keys
}

func TestRunSpecReviewLoop_NoJudgePreservesLegacyResultShape(t *testing.T) {
	specID := "SPEC-JUDGE-NONE-001"
	specDir := scaffoldReviewSpec(t, t.TempDir(), specID)
	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	var captured orchestra.OrchestraConfig
	original := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{Responses: passingReviewerResponses()}, nil
	}
	t.Cleanup(func() { specReviewRunOrchestra = original })
	params := reviewLoopParams(specID, specDir)
	params.maxRevisions = 0
	params.gate = config.ReviewGateConf{}

	result, err := runSpecReviewLoop(params, doc, nil)

	require.NoError(t, err)
	assert.True(t, captured.NoJudge)
	assert.Empty(t, captured.JudgeProvider)
	assert.Nil(t, result.Judge)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"judge"`)
}

// SPEC-REVIEWPOLICY-001 REQ-RP-POL-04 (codex review F-001): a judge PASS with
// no accepted findings no longer silences the reviewers' checklist FAILs; the
// judge clears each one only by rejecting that exact item for that reviewer.
func TestRunSpecReviewLoop_JudgePassKeepsReviewerChecklistFailsUnlessDismissed(t *testing.T) {
	reviewers := []orchestra.ProviderResponse{
		{
			Provider: "claude",
			Output:   `{"verdict":"REVISE","summary":"checklist failed","findings":[],"checklist":[{"id":"Q-CORR-01","status":"FAIL","reason":"missing evidence"}]}`,
		},
		{
			Provider: "codex",
			Output:   `{"verdict":"REVISE","summary":"checklist failed","findings":[],"checklist":[{"id":"Q-CORR-02","status":"FAIL","reason":"missing evidence"}]}`,
		},
		{Provider: "gemini", Output: `{"verdict":"PASS","summary":"ok","findings":[]}`},
	}
	legacy, _ := runJudgeMergeLoopWithResponses(t, "", nil, "", reviewers)
	require.Equal(t, spec.VerdictRevise, legacy.Verdict)

	judged, _ := runJudgeMergeLoopWithResponses(
		t, `{"verdict":"PASS","findings":[],"rationale":"no accepted findings"}`,
		nil, "claude", reviewers,
	)
	assert.Equal(t, spec.VerdictRevise, judged.Verdict)
	require.NotEmpty(t, judged.BlockingReasons)
	assert.Equal(t, spec.BlockingPolicyReviewerChecklist, judged.BlockingReasons[0].Policy)
	require.NotNil(t, judged.Judge)
	assert.Equal(t, "ok", judged.Judge.Status)

	dismissOne := `{"severity":"minor","category":"completeness","scope_ref":"%s","location":"checklist","description":"not supported","suggestion":"none","decision":"reject","reason":"the SPEC carries the evidence","sources":["%s"]}`
	partial, _ := runJudgeMergeLoopWithResponses(t,
		`{"verdict":"PASS","findings":[`+fmt.Sprintf(dismissOne, "Q-CORR-01", "Reviewer A")+`],"rationale":"one dismissed"}`,
		nil, "claude", reviewers)
	assert.Equal(t, spec.VerdictRevise, partial.Verdict, "codex's Q-CORR-02 FAIL was not addressed")

	cleared, _ := runJudgeMergeLoopWithResponses(t,
		`{"verdict":"PASS","findings":[`+fmt.Sprintf(dismissOne, "Q-CORR-01", "Reviewer A")+`,`+
			fmt.Sprintf(dismissOne, "Q-CORR-02", "Reviewer B")+`],"rationale":"both dismissed"}`,
		nil, "claude", reviewers)
	assert.Equal(t, spec.VerdictPass, cleared.Verdict)
	assert.Empty(t, cleared.BlockingReasons)
	require.Len(t, cleared.Judge.Dismissals, 2)
	assert.Equal(t, spec.JudgeDismissal{Reviewer: "claude", ScopeRef: "Q-CORR-01", Reason: "the SPEC carries the evidence"}, cleared.Judge.Dismissals[0])
}

func runJudgeMergeLoop(
	t *testing.T,
	judgeOutput string,
	failures []orchestra.FailedProvider,
) (*spec.ReviewResult, string) {
	t.Helper()
	return runJudgeMergeLoopWithGate(t, judgeOutput, failures, "claude")
}

func runJudgeMergeLoopWithGate(
	t *testing.T,
	judgeOutput string,
	failures []orchestra.FailedProvider,
	judge string,
) (*spec.ReviewResult, string) {
	t.Helper()
	return runJudgeMergeLoopWithResponses(t, judgeOutput, failures, judge, passingReviewerResponses())
}

func runJudgeMergeLoopWithResponses(
	t *testing.T,
	judgeOutput string,
	failures []orchestra.FailedProvider,
	judge string,
	reviewerResponses []orchestra.ProviderResponse,
) (*spec.ReviewResult, string) {
	t.Helper()
	specID := "SPEC-JUDGE-MERGE-001"
	specDir := scaffoldReviewSpec(t, t.TempDir(), specID)
	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	original := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		responses := append([]orchestra.ProviderResponse(nil), reviewerResponses...)
		if judgeOutput != "" {
			responses = append(responses, orchestra.ProviderResponse{
				Provider: "claude (judge)", Role: "judge", Output: judgeOutput, ModelFamily: "anthropic",
			})
		}
		return &orchestra.OrchestraResult{Responses: responses, FailedProviders: failures}, nil
	}
	t.Cleanup(func() { specReviewRunOrchestra = original })
	params := reviewLoopParams(specID, specDir)
	params.maxRevisions = 0
	params.gate.Judge = judge

	result, err := runSpecReviewLoop(params, doc, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result, specDir
}

func passingReviewerResponses() []orchestra.ProviderResponse {
	return []orchestra.ProviderResponse{
		{Provider: "claude", Output: `{"verdict":"PASS","summary":"ok","findings":[]}`},
		{Provider: "codex", Output: `{"verdict":"PASS","summary":"ok","findings":[]}`},
		{
			Provider: "gemini",
			Output:   `{"verdict":"REVISE","summary":"minority finding","findings":[{"severity":"major","category":"correctness","scope_ref":"REQ-MINORITY","location":"spec.md:1","description":"Minority finding","suggestion":"Fix it"}]}`,
		},
	}
}

// In verify mode a judge PASS that resolves every prior finding must converge
// even when a resolved prior was major: only active findings can veto PASS.
func TestRunSpecReviewLoop_VerifyJudgePassResolvingMajorPriorConverges(t *testing.T) {
	specID := "SPEC-JUDGE-VERIFY-001"
	specDir := scaffoldReviewSpec(t, t.TempDir(), specID)
	doc, err := spec.Load(specDir)
	require.NoError(t, err)
	prior := []spec.ReviewFinding{{
		ID: "F-001", Provider: "claude", Severity: "major", Category: spec.FindingCategoryCorrectness,
		ScopeRef: "REQ-001", Description: "Prior blocking issue", Status: spec.FindingStatusOpen,
		FirstSeenRev: 0, LastSeenRev: 0,
	}}
	original := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		responses := append(passingReviewerResponses(), orchestra.ProviderResponse{
			Provider: "claude (judge)", Role: "judge", ModelFamily: "anthropic",
			Output: `{"verdict":"PASS","findings":[],"rationale":"prior issue is fixed"}`,
		})
		return &orchestra.OrchestraResult{Responses: responses}, nil
	}
	t.Cleanup(func() { specReviewRunOrchestra = original })
	params := reviewLoopParams(specID, specDir)
	params.maxRevisions = 1
	params.gate.Judge = "claude"

	result, err := runSpecReviewLoop(params, doc, prior)

	require.NoError(t, err)
	require.NotNil(t, result.Judge)
	assert.Equal(t, "ok", result.Judge.Status)
	assert.Equal(t, spec.VerdictPass, result.Verdict, "a resolved major prior must not veto the judge PASS")
	for _, finding := range result.Findings {
		if finding.ID == "F-001" {
			assert.Equal(t, spec.FindingStatusResolved, finding.Status)
		}
	}
}

// A backend-routed reviewer that fails still records its executed backend in
// the receipt row.
func TestRunSpecReviewLoop_FailedOMPReviewerKeepsExecutedBackend(t *testing.T) {
	failure := orchestra.FailedProvider{
		Name: "gemini", Role: "reviewer", FailureClass: "timeout", Error: "context deadline exceeded",
		TimedOut: true, ExecutedBackend: "omp",
	}
	responses := append(passingReviewerResponses()[:2], orchestra.ProviderResponse{
		Provider: "gemini", TimedOut: true, ExecutedBackend: "omp",
	})
	result, specDir := runJudgeMergeLoopWithResponses(t, "", []orchestra.FailedProvider{failure}, "", responses)

	receipt, err := syncReviewedSpecStatusWithReceipt(specDir, result, false)
	require.NoError(t, err)
	var gemini *spec.ProviderStatus
	for i := range receipt.Providers {
		if receipt.Providers[i].Provider == "gemini" {
			gemini = &receipt.Providers[i]
		}
	}
	require.NotNil(t, gemini)
	assert.Equal(t, "timeout", gemini.Status)
	assert.Equal(t, "omp", gemini.Backend)
}
