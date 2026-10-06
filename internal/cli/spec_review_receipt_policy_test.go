package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// S9 (REQ-07): with ready claude and codex, the receipt holds one row per
// reviewer and the judge, and never the probe output behind the status.
func TestRunSpecReview_ReceiptRecordsProviderPolicyRows(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	fixture.useFakeBackend(t)
	useReadinessRunner(t, replyByExecutable(map[string]readinessReply{
		"claude": replyWith(0, claudeLoggedInR1, ""),
		"codex":  replyWith(0, "", "Logged in using ChatGPT\n"),
	}))

	stderr := captureSpecReviewStderr(t, func() {
		require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))
	})

	assert.Equal(t, []specReviewProviderPolicyRow{
		{Provider: "claude", Role: "reviewer", SandboxMode: "read-only", Readiness: "ready"},
		{Provider: "codex", Role: "reviewer", SandboxMode: "read-only", Readiness: "ready"},
		{Provider: "gemini", Role: "reviewer", SandboxMode: "read-only", Readiness: "unknown(no_status_command)"},
		{Provider: "claude", Role: "judge", SandboxMode: "read-only", Readiness: "ready"},
	}, readSpecReviewReceipt(t, fixture.specDir).ProviderPolicy)
	raw, err := os.ReadFile(filepath.Join(fixture.specDir, "review-receipt.json"))
	require.NoError(t, err)
	for _, secret := range []string{"dev@example.com", "org-123", "claude.ai"} {
		assert.NotContains(t, string(raw), secret)
		assert.NotContains(t, stderr, secret)
	}
	assert.Contains(t, stderr, "preflight: claude ready\n")
	assert.Contains(t, stderr, "preflight: gemini unknown(no_status_command)\n")
	assert.Contains(t, stderr, "preflight: claude ready (judge)\n")
}

// S16 (REQ-15): account identifiers and tokens in probe output reach neither
// the preflight stderr nor the receipt; the OMP account warning is printed
// from counts only.
func TestRunSpecReview_PreflightNeverPrintsAccountIdentifiers(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Providers = []string{"claude", "gemini"}
		cfg.Orchestra.Providers["gemini"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: ompAnthropicModel}
	})
	fixture.useFakeBackend(t)
	installFakeOMP(t)
	installReviewJSONRecorders(t, "claude")
	useReadinessRunner(t, replyByExecutable(map[string]readinessReply{
		"claude": replyWith(0, claudeLoggedInR1, ""),
		"omp": replyWith(0, ompUsageJSON(`[{"provider":"anthropic","account":"dev@example.com"}]`, "[]",
			`[{"provider":"anthropic","account":"sk-ant-oat01-abc123","reason":"Refresh token expired for dev@example.com"}]`), ""),
	}))

	stderr := captureSpecReviewStderr(t, func() {
		require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))
	})

	raw, err := os.ReadFile(filepath.Join(fixture.specDir, "review-receipt.json"))
	require.NoError(t, err)
	for _, secret := range []string{"dev@example.com", "org-123", "sk-ant-"} {
		assert.NotContains(t, stderr, secret)
		assert.NotContains(t, string(raw), secret)
	}
	assert.Contains(t, stderr, "preflight: gemini ready\n")
	assert.Contains(t, stderr,
		"preflight: omp anthropic: 1 of 2 accounts unusable (auth_expired); run \"omp usage --redact\" for details\n")
}

// S9 discriminator: with the projection skipped through the assembly seams,
// the rows report the sandbox the unprojected argv proves, not read-only.
func TestRunSpecReview_ReceiptSandboxModeFollowsArgvWhenProjectionIsSkipped(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	fixture.useFakeBackend(t)
	useHermeticReadiness(t)
	unprojected := fDefaultReadOnlyProviders()
	originalProviders, originalJudge := specReviewProviderAssembly, specReviewJudgeAssembly
	specReviewProviderAssembly = func(context.Context, specReviewProviderRequest) (specReviewProviderSet, error) {
		return specReviewProviderSet{Names: []string{"claude", "codex", "gemini"}, Providers: unprojected}, nil
	}
	specReviewJudgeAssembly = func(context.Context, *config.HarnessConfig, []orchestra.ProviderConfig, string, int) (*orchestra.ProviderConfig, error) {
		judge := unprojected[0]
		return &judge, nil
	}
	t.Cleanup(func() { specReviewProviderAssembly, specReviewJudgeAssembly = originalProviders, originalJudge })

	require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))

	modes := map[string]string{}
	for _, row := range readSpecReviewReceipt(t, fixture.specDir).ProviderPolicy {
		modes[row.Provider+"/"+row.Role] = row.SandboxMode
	}
	assert.Equal(t, map[string]string{
		"claude/reviewer": "unrestricted", "codex/reviewer": "workspace-write",
		"gemini/reviewer": "unrestricted", "claude/judge": "unrestricted",
	}, modes)
}

// S9 discriminator: a permission bypass in the executed argv beats the
// read-only policy stamp of a projected provider.
func TestRunSpecReview_ReceiptBypassInExecutedArgvBeatsReadOnlyStamp(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	backend := fixture.useFakeBackend(t)
	backend.execution = map[string]*orchestra.ProviderExecution{
		"claude": {Command: []string{"claude", "--print", "--dangerously-skip-permissions"}},
	}
	useHermeticReadiness(t)

	require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))

	modes := map[string]string{}
	for _, row := range readSpecReviewReceipt(t, fixture.specDir).ProviderPolicy {
		modes[row.Provider+"/"+row.Role] = row.SandboxMode
	}
	assert.Equal(t, map[string]string{
		"claude/reviewer": "unrestricted", "codex/reviewer": "read-only",
		"gemini/reviewer": "read-only", "claude/judge": "unrestricted",
	}, modes)
}

// S9: the provider_policy field is omitempty, so a receipt of a run without
// provider execution keeps its pre-change bytes.
func TestPersistSpecReviewPromotionReceipt_WithoutProviderExecutionKeepsBytes(t *testing.T) {
	t.Parallel()

	specDir := t.TempDir()
	path, err := persistSpecReviewPromotionReceipt(specDir, specReviewPromotionReceipt{
		Schema: specReviewPromotionReceiptSchema, RunID: "spec-review-run-1", FinishedAt: "2026-10-06T00:00:00Z",
		DegradedReasons: []string{}, GateStatus: "blocked",
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, `{
  "schema": "spec_review_promotion_receipt.v1",
  "run_id": "spec-review-run-1",
  "finished_at": "2026-10-06T00:00:00Z",
  "analysis_verdict": "",
  "gate_status": "blocked",
  "critical_veto": false,
  "status_changed": false,
  "degraded_reasons": [],
  "override_applied": false
}
`, string(data))
}

// A receipt built from runtime evidence copies the provider_policy rows; a
// nil result records none because no provider ran.
func TestSyncReviewedSpecStatusWithReceipt_CopiesProviderPolicyEvidence(t *testing.T) {
	specDir := scaffoldReviewSpec(t, t.TempDir(), "SPEC-REVIEWRO-RECEIPT-001")
	rows := []specReviewProviderPolicyRow{{Provider: "claude", Role: "reviewer", SandboxMode: "read-only", Readiness: "ready"}}
	evidence := specReviewRuntimeEvidence{RunID: "run-1", ProviderPolicy: rows}

	receipt, err := syncReviewedSpecStatusWithReceipt(specDir, newSyncPassResult(nil), false, evidence)
	require.NoError(t, err)
	assert.Equal(t, rows, receipt.ProviderPolicy)

	empty, err := syncReviewedSpecStatusWithReceipt(specDir, nil, false, evidence)
	require.NoError(t, err)
	assert.Nil(t, empty.ProviderPolicy)
}
