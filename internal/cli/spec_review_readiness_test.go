package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// readinessR1R8 answers claude with R1 (logged in) and codex with R8 (logged out).
func readinessR1R8() readinessReply {
	return replyByExecutable(map[string]readinessReply{
		"claude": replyWith(0, claudeLoggedInR1, ""),
		"codex":  replyWith(1, "", "Not logged in\n"),
	})
}

// S12 (REQ-11): a not-ready reviewer is excluded before execution, stays in
// the quorum denominator, and degrades promotion; --allow-degraded overrides.
func TestRunSpecReview_NotReadyReviewerIsExcludedAndDegradesPromotion(t *testing.T) {
	for _, allowDegraded := range []bool{false, true} {
		t.Run(fmt.Sprintf("allow-degraded=%t", allowDegraded), func(t *testing.T) {
			fixture := newReadOnlyReviewFixture(t, nil)
			backend := fixture.useFakeBackend(t)
			useReadinessRunner(t, readinessR1R8())

			stderr := captureSpecReviewStderr(t, func() {
				require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0,
					specReviewOptions{allowDegraded: allowDegraded}))
			})

			assert.Contains(t, strings.Split(stderr, "\n"),
				`preflight: codex not_ready(logged_out) - run "codex login" (excluded; degraded: provider_unready:codex:logged_out)`)
			assert.Zero(t, backend.calls("codex", "reviewer"), "an excluded reviewer never executes")
			assert.Equal(t, 1, backend.calls("claude", "reviewer"))
			assert.Equal(t, 1, backend.calls("gemini", "reviewer"))
			receipt := readSpecReviewReceipt(t, fixture.specDir)
			// The excluded reviewer keeps its Provider Health row, so the
			// quorum denominator stays 3.
			assert.Equal(t, []string{"claude success -", "codex error excluded: not_ready(logged_out)", "gemini success -"},
				providerHealthRows(receipt.Providers))
			assert.Equal(t, "PASS", receipt.Verdict)
			assert.Equal(t, []string{"provider_unready:codex:logged_out"}, receipt.DegradedReasons)
			verdict := specReviewVerdictLine(t, fixture.specDir)
			assert.True(t, strings.HasSuffix(verdict, "(degraded: provider_unready:codex:logged_out)"), verdict)
			assert.Equal(t, []specReviewProviderPolicyRow{
				{Provider: "claude", Role: "reviewer", SandboxMode: "read-only", Readiness: "ready"},
				{Provider: "codex", Role: "reviewer", Readiness: "not_ready(logged_out)", Excluded: true},
				{Provider: "gemini", Role: "reviewer", SandboxMode: "unverified", Readiness: "unknown(no_status_command)"},
				{Provider: "claude", Role: "judge", SandboxMode: "read-only", Readiness: "ready"},
			}, receipt.ProviderPolicy)
			doc, err := spec.Load(fixture.specDir)
			require.NoError(t, err)
			if allowDegraded {
				assert.Equal(t, "approved", doc.Status)
				assert.True(t, receipt.OverrideApplied)
			} else {
				assert.Equal(t, "draft", doc.Status)
				assert.False(t, receipt.OverrideApplied)
			}
		})
	}
}

// S12 (REQ-11): with every reviewer not ready the review fails, names every
// remedy, and executes nothing.
func TestRunSpecReview_NoReadyReviewerFailsWithEveryRemedy(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"}
		cfg.Spec.ReviewGate.Judge = ""
	})
	backend := fixture.useFakeBackend(t)
	useReadinessRunner(t, replyByExecutable(map[string]readinessReply{
		"claude": replyWith(1, `{"loggedIn":false,"authMethod":"none","apiProvider":"firstParty"}`, ""),
		"codex":  replyWith(1, "", "Not logged in\n"),
	}))

	err := runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{allowDegraded: true})

	require.Error(t, err)
	assert.Equal(t, `spec review: no ready reviewer remains; run "claude auth login"; run "codex login"`, err.Error())
	assert.Zero(t, backend.total())
	assert.NoFileExists(t, filepath.Join(fixture.specDir, "review-receipt.json"))
}

// S13 (REQ-12, REQ-13): the incident fixture. An expired OMP Anthropic account
// fails the claude judge before the first provider execution, after exactly
// one `omp usage` probe shared by every OMP-backed provider.
func TestRunSpecReview_NotReadyJudgeFailsBeforeFirstExecution(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
		cfg.Orchestra.Providers = map[string]config.ProviderEntry{
			"claude": {Backend: config.ProviderBackendOMP, Model: ompAnthropicModel},
			"codex":  {Backend: config.ProviderBackendOMP, Model: "openai-codex/gpt-5.6-sol:max"},
			"gemini": {Backend: config.ProviderBackendOMP, Model: "google-antigravity/gemini-3.5-pro:high"},
		}
	})
	backend := fixture.useFakeBackend(t)
	canonical := installFakeOMP(t)
	spy := useReadinessRunner(t, replyWith(0, ompUsageJSON(
		`[{"provider":"openai-codex","account":"e5f6"},{"provider":"google-antigravity","account":"g7h8"}]`, "[]",
		`[{"provider":"anthropic","account":"c3d4","reason":"Refresh token expired"}]`), ""))

	err := runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{allowDegraded: true})

	require.Error(t, err)
	assert.Equal(t, `spec review: judge "claude" is not ready: not_ready(auth_expired); run "omp login anthropic"`, err.Error())
	assert.Zero(t, backend.total())
	assert.NoFileExists(t, filepath.Join(fixture.specDir, "review-receipt.json"))
	assert.Equal(t, [][]string{{canonical, "usage", "--json", "--redact"}}, spy.argvs())
}

// S10 (REQ-09): --skip-provider-readiness runs no probe and records skipped.
func TestSpecReviewCmd_SkipProviderReadinessRunsNoProbe(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	fixture.useFakeBackend(t)
	spy := useReadinessRunner(t, replyWith(1, "", "Not logged in\n"))
	cmd := newSpecReviewCmd()
	cmd.SetArgs([]string{fixture.specID, "--skip-provider-readiness"})

	require.NoError(t, cmd.ExecuteContext(context.Background()))

	assert.Empty(t, spy.argvs())
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	require.Len(t, receipt.ProviderPolicy, 4)
	for _, row := range receipt.ProviderPolicy {
		assert.Equal(t, "skipped", row.Readiness, row.Provider+" "+row.Role)
	}
	assert.Empty(t, receipt.DegradedReasons)
}

func TestNewSpecReviewCmd_HelpNamesReadOnlySubprocessAndReadiness(t *testing.T) {
	t.Parallel()

	flags := newSpecReviewCmd().Flags()
	require.NotNil(t, flags.Lookup("skip-provider-readiness"))
	assert.Equal(t, "false", flags.Lookup("skip-provider-readiness").DefValue)
	assert.Contains(t, flags.Lookup("skip-provider-readiness").Usage, "readiness skipped")
	assert.Contains(t, flags.Lookup("allow-degraded").Usage, "provider_unready")
	assert.Contains(t, flags.Lookup("subprocess").Usage, "read-only")
}

// REQ-11 ordering: readiness reasons merge after the observation reasons in
// provider-name order, and Provider Health names the exclusion.
func TestApplySpecReviewReadiness_AppendsUnreadyReasonsByProviderName(t *testing.T) {
	t.Parallel()

	preflight := &specReviewPreflight{Reviewers: []specReviewReadinessEntry{
		{Provider: orchestra.ProviderConfig{Name: "codex"}, Readiness: notReadyReadiness("logged_out", "codex login"), Excluded: true},
		{Provider: orchestra.ProviderConfig{Name: "gemini"}, Readiness: unknownReadiness("no_status_command")},
		{Provider: orchestra.ProviderConfig{Name: "claude"}, Readiness: notReadyReadiness("auth_expired", "omp login anthropic"), Excluded: true},
	}}
	result := &spec.ReviewResult{
		DegradedReasons: []string{spec.DegradedReasonPartialDocContext, spec.DegradedReasonProviderQuorum},
		ProviderStatuses: []spec.ProviderStatus{
			{Provider: "codex", Status: "error", Note: "no response"},
			{Provider: "gemini", Status: "success", Note: "-"},
			{Provider: "claude", Status: "error", Note: "no response"},
		},
	}

	applySpecReviewReadiness(result, preflight)
	applySpecReviewReadiness(result, nil)

	assert.Equal(t, []string{
		spec.DegradedReasonPartialDocContext, spec.DegradedReasonProviderQuorum,
		"provider_unready:claude:auth_expired", "provider_unready:codex:logged_out",
	}, result.DegradedReasons)
	assert.Equal(t, []string{"codex error excluded: not_ready(logged_out)", "gemini success -", "claude error excluded: not_ready(auth_expired)"},
		providerHealthRows(result.ProviderStatuses))
}
